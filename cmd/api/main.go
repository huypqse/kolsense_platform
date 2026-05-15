package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.uber.org/zap"

	"kolsense/internal/api"
	"kolsense/internal/config"
	"kolsense/internal/embedding"
	"kolsense/internal/llm"
	"kolsense/internal/vectorstore"
)

func main() {
	// ── Logger ─────────────────────────────────────────────────────────────────
	log, _ := zap.NewProduction()
	defer log.Sync() //nolint:errcheck

	// ── Config ─────────────────────────────────────────────────────────────────
	cfg, err := config.Load()
	if err != nil {
		log.Fatal("load config", zap.Error(err))
	}
	log.Info("config loaded",
		zap.String("embedding_provider", string(cfg.Embedding.Provider)),
		zap.String("llm_provider", string(cfg.LLM.Provider)),
		zap.String("addr", cfg.Server.Addr()),
	)

	ctx := context.Background()

	// ── Vector store ───────────────────────────────────────────────────────────
	store, err := vectorstore.Connect(ctx, cfg.Database.URL, cfg.Database.MaxConns, cfg.Database.MinConns)
	if err != nil {
		log.Fatal("connect postgres", zap.Error(err))
	}
	defer store.Close()
	log.Info("postgres connected")

	// ── Embedder ───────────────────────────────────────────────────────────────
	var embedder embedding.Embedder
	switch cfg.Embedding.Provider {
	case config.ProviderDashScope:
		embedder = embedding.NewDashScopeEmbedder(
			cfg.Embedding.DashScopeAPIKey,
			cfg.Embedding.DashScopeModel,
			cfg.Embedding.Dimensions,
		)
		log.Info("embedder: dashscope", zap.String("model", cfg.Embedding.DashScopeModel))
	default:
		embedder = embedding.NewOllamaEmbedder(
			cfg.Embedding.OllamaBaseURL,
			cfg.Embedding.OllamaModel,
			cfg.Embedding.Dimensions,
		)
		log.Info("embedder: ollama", zap.String("model", cfg.Embedding.OllamaModel))
	}

	// ── LLM client ─────────────────────────────────────────────────────────────
	var llmClient llm.Client
	switch cfg.LLM.Provider {
	case config.ProviderDashScope:
		llmClient = llm.NewDashScopeClient(
			cfg.LLM.DashScopeAPIKey,
			cfg.LLM.DashScopeModel,
			cfg.LLM.MaxTokens,
			cfg.LLM.Temperature,
		)
		log.Info("llm: dashscope", zap.String("model", cfg.LLM.DashScopeModel))
	default:
		llmClient = llm.NewOllamaClient(cfg.LLM.OllamaBaseURL, cfg.LLM.OllamaModel)
		log.Info("llm: ollama", zap.String("model", cfg.LLM.OllamaModel))
	}

	// ── HTTP server ────────────────────────────────────────────────────────────
	srv := api.New(cfg, api.Dependencies{
		Store:    store,
		Embedder: embedder,
		LLM:      llmClient,
	}, log)

	httpSrv := srv.HTTPServer()

	go func() {
		log.Info("server starting", zap.String("addr", httpSrv.Addr))
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal("server error", zap.Error(err))
		}
	}()

	// ── Graceful shutdown ──────────────────────────────────────────────────────
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info("shutting down server...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := httpSrv.Shutdown(shutdownCtx); err != nil {
		log.Error("forced shutdown", zap.Error(err))
	}
	log.Info("server stopped")
}
