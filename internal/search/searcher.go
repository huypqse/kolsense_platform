// Package search implements the pgvector similarity search pipeline.
package search

import (
	"context"
	"fmt"

	"kolsense/internal/brief"
	"kolsense/internal/embedding"
	"kolsense/internal/vectorstore"
)

// Result bundles retrieved chunk results grouped by KOL.
type Result struct {
	// ByKOL maps each KOL name to its top retrieved chunks.
	ByKOL map[string][]vectorstore.ChunkResult
	// SimScores holds the best (highest) similarity score per KOL.
	SimScores map[string]float64
	// TopChunks holds up to 2 representative text excerpts per KOL.
	TopChunks map[string][]string
}

// Searcher performs brief embedding + metadata-filtered similarity search.
type Searcher struct {
	store    vectorstore.Store
	embedder embedding.Embedder
	efSearch int
}

// NewSearcher creates a Searcher.
func NewSearcher(store vectorstore.Store, embedder embedding.Embedder, efSearch int) *Searcher {
	return &Searcher{store: store, embedder: embedder, efSearch: efSearch}
}

// Search embeds the campaign brief summary and retrieves the top-K most similar
// KOL chunks, pre-filtered by platform and category.
func (s *Searcher) Search(ctx context.Context, b *brief.CampaignBrief, topK int) (*Result, error) {
	// 1. Embed the brief summary
	summary := b.BriefSummary()
	vecs, err := s.embedder.Embed(ctx, []string{summary})
	if err != nil {
		return nil, fmt.Errorf("search: embed brief: %w", err)
	}
	queryVec := vecs[0]

	// 2. Build metadata filter
	filter := vectorstore.SearchFilter{
		Platforms:  platformStrings(b.Platforms),
		Categories: categoryStrings(b.Categories),
	}

	// 3. pgvector similarity search
	chunks, err := s.store.SimilaritySearch(ctx, queryVec, filter, topK, s.efSearch)
	if err != nil {
		return nil, fmt.Errorf("search: similarity search: %w", err)
	}

	// 4. Group by KOL, track best sim score and top excerpts
	byKOL     := make(map[string][]vectorstore.ChunkResult)
	simScores := make(map[string]float64)
	topChunks := make(map[string][]string)

	for _, cr := range chunks {
		name := cr.Chunk.KOLName
		byKOL[name] = append(byKOL[name], cr)

		if cr.SimScore > simScores[name] {
			simScores[name] = cr.SimScore
		}
		if len(topChunks[name]) < 2 {
			excerpt := truncate(cr.Chunk.Text, 200)
			topChunks[name] = append(topChunks[name], excerpt)
		}
	}

	return &Result{
		ByKOL:     byKOL,
		SimScores: simScores,
		TopChunks: topChunks,
	}, nil
}

func platformStrings(ps []brief.Platform) []string {
	if len(ps) == 0 { return nil }
	out := make([]string, len(ps))
	for i, p := range ps { out[i] = string(p) }
	return out
}

func categoryStrings(cs []brief.Category) []string {
	if len(cs) == 0 { return nil }
	out := make([]string, len(cs))
	for i, c := range cs { out[i] = string(c) }
	return out
}

func truncate(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max { return s }
	return string(runes[:max]) + "…"
}
