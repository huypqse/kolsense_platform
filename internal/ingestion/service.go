package ingestion

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"kolsense/internal/embedding"
	"kolsense/internal/vectorstore"

	"go.uber.org/zap"
)

// IngestConfig holds parameters for the ingestion pipeline.
type IngestConfig struct {
	ChunkCfg   ChunkConfig
	KOLName    string          // override KOL name; if empty, derived from filename
	DocType    vectorstore.DocType
	EmbedBatch int             // number of chunks per embedding batch call
}

// DefaultIngestConfig returns a sensible default IngestConfig.
func DefaultIngestConfig() IngestConfig {
	return IngestConfig{
		ChunkCfg:   DefaultChunkConfig(),
		EmbedBatch: 32,
	}
}

// Service orchestrates the full ingestion pipeline:
// File (PDF or Markdown) → pages → chunks → embeddings → vectorstore.
type Service struct {
	pdfParser *PDFParser
	mdParser  *MDParser
	chunker   *Chunker
	embedder  embedding.Embedder
	store     vectorstore.Store
	log       *zap.Logger
}

// NewService constructs a new ingestion Service.
func NewService(
	embedder embedding.Embedder,
	store vectorstore.Store,
	log *zap.Logger,
) *Service {
	return &Service{
		pdfParser: NewPDFParser(),
		mdParser:  NewMDParser(),
		chunker:   NewChunker(DefaultChunkConfig()),
		embedder:  embedder,
		store:     store,
		log:       log,
	}
}

// IngestFile processes a single PDF or Markdown file through the full pipeline.
func (s *Service) IngestFile(ctx context.Context, filePath string, cfg IngestConfig) (int, error) {
	kolName := cfg.KOLName
	if kolName == "" {
		kolName = kolNameFromFile(filePath)
	}
	docType := cfg.DocType
	if docType == "" {
		docType = docTypeFromFile(filePath)
	}

	ext := strings.ToLower(filepath.Ext(filePath))
	isMD := ext == ".md" || ext == ".markdown"

	if isMD {
		s.log.Info("ingesting Markdown",
			zap.String("file", filePath),
			zap.String("kol", kolName),
			zap.String("doc_type", string(docType)),
		)
	} else {
		s.log.Info("ingesting PDF",
			zap.String("file", filePath),
			zap.String("kol", kolName),
			zap.String("doc_type", string(docType)),
		)
	}

	// ── Step 1: Parse file into page-text sections ──────────────────────────
	var pages []PageText
	var err error

	if isMD {
		pages, err = s.mdParser.ParseFile(filePath)
	} else {
		pages, err = s.pdfParser.ParseFile(filePath)
	}
	if err != nil {
		return 0, fmt.Errorf("ingest %s: parse: %w", filePath, err)
	}
	s.log.Debug("extracted sections/pages", zap.Int("count", len(pages)))

	// ── Step 2: Chunk all pages ──────────────────────────────────────────────
	var rawChunks []rawChunk
	for _, page := range pages {
		for _, tc := range s.chunker.Chunk(page.Text) {
			rawChunks = append(rawChunks, rawChunk{
				text:    tc.Text,
				pageNum: page.PageNum,
				tokens:  tc.TokenCount,
			})
		}
	}
	if len(rawChunks) == 0 {
		s.log.Warn("no chunks produced", zap.String("file", filePath))
		return 0, nil
	}
	s.log.Debug("produced chunks", zap.Int("count", len(rawChunks)))

	// ── Step 3: Embed in batches ─────────────────────────────────────────────
	batchSize := cfg.EmbedBatch
	if batchSize <= 0 {
		batchSize = 32
	}
	var chunks []vectorstore.Chunk
	for i := 0; i < len(rawChunks); i += batchSize {
		end := i + batchSize
		if end > len(rawChunks) {
			end = len(rawChunks)
		}
		batch := rawChunks[i:end]

		texts := make([]string, len(batch))
		for j, rc := range batch {
			texts[j] = rc.text
		}

		vecs, err := s.embedder.Embed(ctx, texts)
		if err != nil {
			return 0, fmt.Errorf("ingest %s: embed batch [%d:%d]: %w", filePath, i, end, err)
		}

		for j, rc := range batch {
			chunks = append(chunks, vectorstore.Chunk{
				KOLName:    kolName,
				DocType:    docType,
				SourceFile: filepath.Base(filePath),
				PageNum:    rc.pageNum,
				Text:       rc.text,
				TokenCount: rc.tokens,
				Embedding:  vecs[j],
			})
		}
	}

	// ── Step 4: Upsert KOL profile ───────────────────────────────────────────
	// For Markdown KOL profile files, attempt to parse the Summary Profile Card
	// so we populate platform, category, engagement, ROI and fee data that the
	// scoring engine depends on. For brand-wide documents and PDF files we fall
	// back to an empty skeleton — the name is still required for the FK.
	profile := buildProfile(kolName, filePath, isMD)
	if err := s.store.UpsertKOLProfile(ctx, profile); err != nil {
		return 0, fmt.Errorf("ingest %s: upsert kol profile: %w", filePath, err)
	}

	// ── Step 5: Store chunks ─────────────────────────────────────────────────
	if err := s.store.UpsertChunks(ctx, chunks); err != nil {
		return 0, fmt.Errorf("ingest %s: upsert chunks: %w", filePath, err)
	}

	s.log.Info("ingestion complete",
		zap.String("file", filePath),
		zap.Int("chunks_stored", len(chunks)),
	)
	return len(chunks), nil
}

// buildProfile returns a KOLProfile for the ingested file.
// For Markdown files it tries to parse the "## Summary Profile Card" table;
// for all other files it returns a minimal skeleton.
func buildProfile(kolName, filePath string, isMD bool) vectorstore.KOLProfile {
	if isMD {
		if p := ExtractProfileFromMD(filePath, kolName); p != nil {
			return *p
		}
	}
	return vectorstore.KOLProfile{
		Name:     kolName,
		Platform: []string{},
		Category: []string{},
		Metadata: map[string]any{},
	}
}

// rawChunk is an intermediate representation before embedding.
type rawChunk struct {
	text    string
	pageNum int
	tokens  int
}

// kolNameFromFile derives a KOL name from the filename.
//
// Brand-wide documents (campaign reports, brand guidelines, audience insights)
// are attributed to the organisation itself rather than a KOL, so they return
// "Lumiere Collective".  Individual KOL files that use the underscore-separator
// convention (e.g. "Priya_Subramaniam.md") have underscores replaced with spaces.
//
// Examples:
//
//	"lumiere_campaign_report_glow_forward_2025.md" → "Lumiere Collective"
//	"Priya_Subramaniam.md"                         → "Priya Subramaniam"
func kolNameFromFile(filePath string) string {
	lower := strings.ToLower(filepath.Base(filePath))

	// Brand-wide document keywords — return the organisation name.
	brandKeywords := []string{"campaign", "brand", "audience", "guideline", "insight", "report"}
	for _, kw := range brandKeywords {
		if strings.Contains(lower, kw) {
			return "Lumiere Collective"
		}
	}

	// Individual KOL file: derive name from the base filename.
	base := strings.TrimSuffix(filepath.Base(filePath), filepath.Ext(filePath))
	name := strings.ReplaceAll(base, "__", " — ")
	name = strings.ReplaceAll(name, "_", " ")
	return strings.TrimSpace(name)
}

// docTypeFromFile infers the DocType from known filename keywords.
func docTypeFromFile(filePath string) vectorstore.DocType {
	lower := strings.ToLower(filepath.Base(filePath))
	switch {
	case strings.Contains(lower, "campaign") || strings.Contains(lower, "report"):
		return vectorstore.DocTypeCampaignReport
	case strings.Contains(lower, "performance") || strings.Contains(lower, "analytics"):
		return vectorstore.DocTypeKOLPerformance
	case strings.Contains(lower, "audience") || strings.Contains(lower, "insight"):
		return vectorstore.DocTypeAudienceInsight
	case strings.Contains(lower, "brand") || strings.Contains(lower, "guideline"):
		return vectorstore.DocTypeBrandGuideline
	default:
		return vectorstore.DocTypeCampaignReport
	}
}
