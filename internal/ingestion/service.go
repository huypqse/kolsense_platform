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
	ChunkCfg    ChunkConfig
	KOLName     string  // override KOL name; if empty, derived from filename
	DocType     vectorstore.DocType
	EmbedBatch  int     // number of chunks per embedding batch call
}

// DefaultIngestConfig returns a sensible default IngestConfig.
func DefaultIngestConfig() IngestConfig {
	return IngestConfig{
		ChunkCfg:   DefaultChunkConfig(),
		EmbedBatch: 2,
	}
}

// Service orchestrates the full ingestion pipeline:
// PDF → pages → chunks → embeddings → vectorstore.
type Service struct {
	parser   *PDFParser
	chunker  *Chunker
	embedder embedding.Embedder
	store    vectorstore.Store
	log      *zap.Logger
}

// NewService constructs a new ingestion Service.
func NewService(
	embedder embedding.Embedder,
	store vectorstore.Store,
	log *zap.Logger,
) *Service {
	return &Service{
		parser:   NewPDFParser(),
		chunker:  NewChunker(DefaultChunkConfig()),
		embedder: embedder,
		store:    store,
		log:      log,
	}
}

// IngestFile processes a single PDF file through the full pipeline.
func (s *Service) IngestFile(ctx context.Context, filePath string, cfg IngestConfig) (int, error) {
	kolName := cfg.KOLName
	if kolName == "" {
		kolName = kolNameFromFile(filePath)
	}
	docType := cfg.DocType
	if docType == "" {
		docType = docTypeFromFile(filePath)
	}

	s.log.Info("ingesting PDF",
		zap.String("file", filePath),
		zap.String("kol", kolName),
		zap.String("doc_type", string(docType)),
	)

	// Step 1: Extract pages
	pages, err := s.parser.ParseFile(filePath)
	if err != nil {
		return 0, fmt.Errorf("ingest %s: parse PDF: %w", filePath, err)
	}
	s.log.Debug("extracted pages", zap.Int("count", len(pages)))

	// Step 2: Chunk all pages
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

	// Step 3: Embed in batches
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

	// Step 4: Ensure KOL Profile exists before storing chunks
	profile := vectorstore.KOLProfile{
		Name:     kolName,
		Platform: []string{},
		Category: []string{},
		Metadata: map[string]any{},
	}
	if err := s.store.UpsertKOLProfile(ctx, profile); err != nil {
		return 0, fmt.Errorf("ingest %s: upsert kol profile: %w", filePath, err)
	}

	if err := s.store.UpsertChunks(ctx, chunks); err != nil {
		return 0, fmt.Errorf("ingest %s: upsert chunks: %w", filePath, err)
	}

	s.log.Info("ingestion complete",
		zap.String("file", filePath),
		zap.Int("chunks_stored", len(chunks)),
	)
	return len(chunks), nil
}

// rawChunk is an intermediate representation before embedding.
type rawChunk struct {
	text    string
	pageNum int
	tokens  int
}

// kolNameFromFile derives a KOL name from the filename.
// e.g. "Lumire_Collective__KOL_Performance_Analytics.pdf" → "Lumire Collective"
func kolNameFromFile(filePath string) string {
	base := strings.TrimSuffix(filepath.Base(filePath), filepath.Ext(filePath))
	// Replace underscores with spaces, collapse double spaces
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
