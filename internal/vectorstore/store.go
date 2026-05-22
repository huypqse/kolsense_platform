// Package vectorstore defines the Store interface and its PostgreSQL implementation.
package vectorstore

import (
	"context"
	"time"
)

// DocType classifies what kind of document a chunk came from.
type DocType string

const (
	DocTypeCampaignReport  DocType = "campaign_report"
	DocTypeKOLPerformance  DocType = "kol_performance"
	DocTypeAudienceInsight DocType = "audience_insight"
	DocTypeBrandGuideline  DocType = "brand_guideline"
)

// Chunk is a single text segment from a PDF document with its embedding.
type Chunk struct {
	ID         string
	KOLName    string
	DocType    DocType
	SourceFile string
	PageNum    int
	Text       string
	TokenCount int
	Embedding  []float32
	Metadata   map[string]any
	CreatedAt  time.Time
}

// KOLProfile is a KOL's master record in the profiles table.
type KOLProfile struct {
	ID            string
	Name          string
	Platform      []string
	Category      []string
	AvgEngagement float64
	AvgReach      int64
	AvgROI        float64
	FollowerCount int64
	FeeMinVND     int64
	FeeMaxVND     int64
	Metadata      map[string]any
	UpdatedAt     time.Time
}

// SearchFilter restricts similarity search to KOLs matching given criteria.
type SearchFilter struct {
	Platforms  []string // OR match (array overlap)
	Categories []string // OR match (array overlap)
}

// ChunkResult is a retrieved chunk with its similarity distance.
type ChunkResult struct {
	Chunk    Chunk
	Distance float64 // cosine distance ∈ [0, 2]; lower = more similar
	SimScore float64 // 1 - distance/2, normalized to [0, 1]
}

// Store is the persistence interface for KOL vectors and profiles.
// All implementations must be safe for concurrent use.
type Store interface {
	// UpsertChunks stores or updates multiple chunks (including their embeddings).
	UpsertChunks(ctx context.Context, chunks []Chunk) error

	// SimilaritySearch performs ANN search with optional metadata pre-filtering.
	// Results are ordered by ascending cosine distance (most similar first).
	SimilaritySearch(ctx context.Context, queryVec []float32, filter SearchFilter, topK int, efSearch int) ([]ChunkResult, error)

	// UpsertKOLProfile stores or updates a KOL master profile.
	UpsertKOLProfile(ctx context.Context, profile KOLProfile) error

	// GetKOLProfile retrieves a KOL profile by name.
	GetKOLProfile(ctx context.Context, name string) (*KOLProfile, error)

	// ListKOLProfiles retrieves all KOL profiles (used for scoring).
	ListKOLProfilesByNames(ctx context.Context, names []string) ([]KOLProfile, error)

	// Close releases the underlying connection pool.
	Close()
}
