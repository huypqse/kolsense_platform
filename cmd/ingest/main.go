package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"go.uber.org/zap"

	"kolsense/internal/config"
	"kolsense/internal/embedding"
	"kolsense/internal/ingestion"
	"kolsense/internal/vectorstore"
)

func main() {
	dir     := flag.String("dir", "./data", "Directory containing PDF files to ingest")
	kolName := flag.String("kol", "", "KOL name override (derived from filename if empty)")
	docType := flag.String("doc-type", "", "Document type override (auto-detected if empty)")
	flag.Parse()

	log, _ := zap.NewProduction()
	defer log.Sync() //nolint:errcheck

	cfg, err := config.Load()
	if err != nil {
		log.Fatal("load config", zap.Error(err))
	}

	ctx := context.Background()

	store, err := vectorstore.Connect(ctx, cfg.Database.URL, cfg.Database.MaxConns, cfg.Database.MinConns)
	if err != nil {
		log.Fatal("connect postgres", zap.Error(err))
	}
	defer store.Close()

	var embedder embedding.Embedder
	switch cfg.Embedding.Provider {
	case config.ProviderDashScope:
		embedder = embedding.NewDashScopeEmbedder(
			cfg.Embedding.DashScopeAPIKey,
			cfg.Embedding.DashScopeModel,
			cfg.Embedding.Dimensions,
		)
	default:
		embedder = embedding.NewOllamaEmbedder(
			cfg.Embedding.OllamaBaseURL,
			cfg.Embedding.OllamaModel,
			cfg.Embedding.Dimensions,
		)
	}

	svc := ingestion.NewService(embedder, store, log)

	// Collect PDF files
	pattern := filepath.Join(*dir, "*.pdf")
	files, err := filepath.Glob(pattern)
	if err != nil || len(files) == 0 {
		log.Fatal("no PDF files found", zap.String("dir", *dir))
	}

	ingestCfg := ingestion.DefaultIngestConfig()
	ingestCfg.KOLName = *kolName
	if *docType != "" {
		ingestCfg.DocType = vectorstore.DocType(*docType)
	}

	total := 0
	for _, f := range files {
		n, err := svc.IngestFile(ctx, f, ingestCfg)
		if err != nil {
			log.Error("ingest failed", zap.String("file", f), zap.Error(err))
			continue
		}
		total += n
		fmt.Printf("✓ %s → %d chunks\n", filepath.Base(f), n)
	}
	fmt.Printf("\nTotal chunks ingested: %d\n", total)
	os.Exit(0)
}
