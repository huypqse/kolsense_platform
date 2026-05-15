package api

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"kolsense/internal/api/handlers"
	"kolsense/internal/api/middleware"
	"kolsense/internal/brief"
	"kolsense/internal/config"
	"kolsense/internal/embedding"
	"kolsense/internal/ingestion"
	"kolsense/internal/llm"
	"kolsense/internal/prompt"
	"kolsense/internal/scoring"
	"kolsense/internal/search"
	"kolsense/internal/vectorstore"
)

// Server wraps the Gin engine with all application dependencies.
type Server struct {
	engine *gin.Engine
	cfg    *config.Config
	log    *zap.Logger
}

// Dependencies bundles all constructed service dependencies.
type Dependencies struct {
	Store    vectorstore.Store
	Embedder embedding.Embedder
	LLM      llm.Client
}

// New constructs the Gin server, wires all routes and middleware.
func New(cfg *config.Config, deps Dependencies, log *zap.Logger) *Server {
	if cfg.Log.Level != "debug" {
		gin.SetMode(gin.ReleaseMode)
	}

	engine := gin.New()
	engine.Use(middleware.Logger(log))
	engine.Use(middleware.Recovery(log))
	engine.Use(corsMiddleware())

	// Build service layer
	briefParser  := brief.NewParser(deps.LLM, cfg.Parser.ConfidenceThreshold)
	searcher     := search.NewSearcher(deps.Store, deps.Embedder, cfg.Search.HNSWEfSearch)
	scorer       := scoring.NewScorer(scoring.DefaultWeights())
	promptBld    := prompt.NewBuilder()
	ingestSvc    := ingestion.NewService(deps.Embedder, deps.Store, log)

	// Handlers
	briefH  := handlers.NewBriefHandler(briefParser, searcher, scorer, promptBld, deps.LLM, deps.Store, cfg, log)
	kolH    := handlers.NewKOLHandler(deps.Store, log)
	ingestH := handlers.NewIngestHandler(ingestSvc, log)

	v1 := engine.Group("/api/v1")
	{
		v1.GET("/health", healthHandler)

		v1.POST("/brief/analyze", briefH.Analyze)
		v1.GET("/brief/:id/report", briefH.GetReport)

		v1.GET("/kol/:name", kolH.GetProfile)

		v1.POST("/ingest/pdf", ingestH.IngestPDF)
	}

	return &Server{engine: engine, cfg: cfg, log: log}
}

// HTTPServer builds a configured *http.Server.
func (s *Server) HTTPServer() *http.Server {
	return &http.Server{
		Addr:         s.cfg.Server.Addr(),
		Handler:      s.engine,
		ReadTimeout:  s.cfg.Server.ReadTimeout,
		WriteTimeout: s.cfg.Server.WriteTimeout,
		IdleTimeout:  120 * time.Second,
	}
}

func healthHandler(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok", "ts": time.Now().UTC()})
}

func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}
