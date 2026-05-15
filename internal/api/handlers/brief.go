package handlers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"kolsense/internal/brief"
	"kolsense/internal/config"
	"kolsense/internal/llm"
	"kolsense/internal/prompt"
	"kolsense/internal/scoring"
	"kolsense/internal/search"
	"kolsense/internal/vectorstore"
)

// BriefHandler handles campaign brief analysis requests.
type BriefHandler struct {
	parser    *brief.Parser
	searcher  *search.Searcher
	scorer    *scoring.Scorer
	promptBld *prompt.Builder
	llm       llm.Client
	store     vectorstore.Store
	cfg       *config.Config
	log       *zap.Logger
}

// NewBriefHandler constructs a BriefHandler.
func NewBriefHandler(
	parser *brief.Parser,
	searcher *search.Searcher,
	scorer *scoring.Scorer,
	promptBld *prompt.Builder,
	llmClient llm.Client,
	store vectorstore.Store,
	cfg *config.Config,
	log *zap.Logger,
) *BriefHandler {
	return &BriefHandler{
		parser: parser, searcher: searcher, scorer: scorer,
		promptBld: promptBld, llm: llmClient, store: store,
		cfg: cfg, log: log,
	}
}

// analyzeRequest is the POST /brief/analyze request body.
type analyzeRequest struct {
	BriefText string `json:"brief_text" binding:"required"`
	TopK      int    `json:"top_k"`
}

// analyzeResponse is the POST /brief/analyze response body.
type analyzeResponse struct {
	SessionID      string           `json:"session_id"`
	Shortlist      []shortlistItem  `json:"shortlist"`
	ReportMarkdown string           `json:"report_markdown"`
	ParseStrategy  string           `json:"parse_strategy"`
	GeneratedAt    time.Time        `json:"generated_at"`
}

type shortlistItem struct {
	Rank          int                    `json:"rank"`
	KOLName       string                 `json:"kol_name"`
	FitScore      float64                `json:"fit_score"`
	SimScore      float64                `json:"sim_score"`
	Platform      []string               `json:"platform"`
	AvgEngagement float64                `json:"avg_engagement"`
	ScoreBreakdown scoring.ScoreBreakdown `json:"score_breakdown"`
	TopExcerpts   []string               `json:"top_excerpts"`
}

// Analyze is POST /api/v1/brief/analyze — the main query pipeline endpoint.
func (h *BriefHandler) Analyze(c *gin.Context) {
	var req analyzeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.TopK <= 0 || req.TopK > 10 {
		req.TopK = h.cfg.Search.DefaultTopK
	}

	ctx := c.Request.Context()

	// Step 1: Parse brief
	parsedBrief, err := h.parser.Parse(ctx, req.BriefText)
	if err != nil {
		h.log.Error("brief parse failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to parse brief"})
		return
	}
	h.log.Info("brief parsed",
		zap.String("strategy", parsedBrief.ParseStrategy),
		zap.Float64("confidence", parsedBrief.Confidence),
	)

	// Step 2: Similarity search
	searchResult, err := h.searcher.Search(ctx, parsedBrief, req.TopK*2)
	if err != nil {
		h.log.Error("search failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "search failed"})
		return
	}

	// Step 3: Fetch KOL profiles for candidates
	kolNames := make([]string, 0, len(searchResult.ByKOL))
	for name := range searchResult.ByKOL {
		kolNames = append(kolNames, name)
	}
	profiles, err := h.store.ListKOLProfilesByNames(ctx, kolNames)
	if err != nil {
		h.log.Error("fetch profiles failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch KOL profiles"})
		return
	}
	profileMap := make(map[string]vectorstore.KOLProfile, len(profiles))
	for _, p := range profiles {
		profileMap[p.Name] = p
	}

	// Step 4: Score and rerank
	scored := h.scorer.Score(scoring.ScoreInput{
		Brief:     parsedBrief,
		Profiles:  profileMap,
		SimScores: searchResult.SimScores,
		TopChunks: searchResult.TopChunks,
	})
	if len(scored) > req.TopK {
		scored = scored[:req.TopK]
	}

	// Step 5: Build prompt and call LLM
	sysPrompt, userPrompt, err := h.promptBld.Build(prompt.KOLReportData{
		Brief:     parsedBrief,
		Shortlist: scored,
		TopN:      req.TopK,
	})
	if err != nil {
		h.log.Error("prompt build failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to build prompt"})
		return
	}

	llmResp, err := h.llm.Complete(ctx, llm.CompletionRequest{
		SystemPrompt: sysPrompt,
		UserPrompt:   userPrompt,
		MaxTokens:    h.cfg.LLM.MaxTokens,
		Temperature:  h.cfg.LLM.Temperature,
	})
	if err != nil {
		h.log.Error("LLM failed", zap.Error(err))
		// Return shortlist without report rather than hard-failing
		llmResp.Text = "(Không thể tạo báo cáo LLM)"
	}

	// Build response
	items := make([]shortlistItem, len(scored))
	for i, sk := range scored {
		items[i] = shortlistItem{
			Rank:           sk.Rank,
			KOLName:        sk.Profile.Name,
			FitScore:       sk.FitScore,
			SimScore:       sk.SimScore,
			Platform:       sk.Profile.Platform,
			AvgEngagement:  sk.Profile.AvgEngagement,
			ScoreBreakdown: sk.Breakdown,
			TopExcerpts:    sk.TopChunks,
		}
	}

	c.JSON(http.StatusOK, analyzeResponse{
		SessionID:      generateID(),
		Shortlist:      items,
		ReportMarkdown: llmResp.Text,
		ParseStrategy:  parsedBrief.ParseStrategy,
		GeneratedAt:    time.Now().UTC(),
	})
}

// GetReport is GET /api/v1/brief/:id/report (stub — sessions not yet persisted).
func (h *BriefHandler) GetReport(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{"error": "session persistence not yet implemented"})
}

// generateID returns a simple unique session ID.
func generateID() string {
	return "sess_" + time.Now().Format("20060102150405")
}
