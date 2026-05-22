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
	dir     := flag.String("dir", "./data", "Directory containing PDF/Markdown files to ingest")
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

	// Collect PDF and Markdown files from the target directory.
	files := collectFiles(*dir, log)
	if len(files) == 0 {
		log.Fatal("no ingestible files found (*.pdf, *.md)",
			zap.String("dir", *dir))
	}
	log.Info("files to ingest", zap.Int("count", len(files)))

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

// collectFiles returns all *.pdf and *.md files inside dir, sorted by name.
func collectFiles(dir string, log *zap.Logger) []string {
	var files []string
	patterns := []string{"*.pdf", "*.md", "*.markdown"}
	for _, pat := range patterns {
		matches, err := filepath.Glob(filepath.Join(dir, pat))
		if err != nil {
			log.Warn("glob error", zap.String("pattern", pat), zap.Error(err))
			continue
		}
		files = append(files, matches...)
	}
	return files
}
