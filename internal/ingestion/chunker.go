// Package ingestion handles PDF text extraction, chunking, embedding, and storage.
package ingestion

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

const (
	defaultWindowTokens  = 512
	defaultOverlapTokens = 64
)

// ChunkConfig configures the paragraph-aware chunker.
type ChunkConfig struct {
	WindowTokens  int // soft maximum chunk size in whitespace-delimited tokens
	OverlapTokens int // number of tokens to repeat at the start of the next chunk
}

// DefaultChunkConfig returns a sensible default configuration.
func DefaultChunkConfig() ChunkConfig {
	return ChunkConfig{
		WindowTokens:  defaultWindowTokens,
		OverlapTokens: defaultOverlapTokens,
	}
}

// TextChunk is a raw text window produced by the chunker.
type TextChunk struct {
	Text       string
	StartToken int
	EndToken   int
	TokenCount int
}

// Chunker splits text into overlapping, paragraph-aligned chunks.
//
// Strategy:
//  1. Split the input at blank-line boundaries (paragraph/section breaks).
//  2. Accumulate whole paragraphs into a chunk until WindowTokens is reached.
//  3. Begin the next chunk by stepping back OverlapTokens worth of paragraphs
//     so context is not lost at boundaries.
//
// This ensures no chunk breaks in the middle of a sentence or table row, and
// that section headers and their first paragraph always land in the same chunk.
type Chunker struct {
	cfg ChunkConfig
}

// NewChunker creates a Chunker with the given configuration.
func NewChunker(cfg ChunkConfig) *Chunker {
	return &Chunker{cfg: cfg}
}

// Chunk splits the input text into semantically coherent, overlapping TextChunks.
func (c *Chunker) Chunk(text string) []TextChunk {
	paragraphs := splitIntoParagraphs(text)
	if len(paragraphs) == 0 {
		return nil
	}

	window := c.cfg.WindowTokens
	overlap := c.cfg.OverlapTokens

	// Pre-compute token counts so we never re-scan a paragraph.
	tokenCounts := make([]int, len(paragraphs))
	for i, p := range paragraphs {
		tokenCounts[i] = EstimateTokenCount(p)
	}

	var chunks []TextChunk
	globalOffset := 0 // cumulative token offset for StartToken/EndToken tracking
	i := 0

	for i < len(paragraphs) {
		// --- Build one chunk: accumulate whole paragraphs up to WindowTokens ---
		var selected []string
		total := 0
		j := i

		for j < len(paragraphs) {
			tc := tokenCounts[j]
			// Always include at least one paragraph even if it exceeds the window
			// (avoids infinite loops on very long single paragraphs).
			if total > 0 && total+tc > window {
				break
			}
			selected = append(selected, paragraphs[j])
			total += tc
			j++
		}

		// Join selected paragraphs with a blank-line sentinel so the
		// embedded text retains section-boundary signals.
		chunkText := strings.Join(selected, "\n\n")

		chunks = append(chunks, TextChunk{
			Text:       chunkText,
			StartToken: globalOffset,
			EndToken:   globalOffset + total,
			TokenCount: total,
		})

		if j >= len(paragraphs) {
			break // reached the end of the document
		}

		// --- Compute overlap: step back from j until we have >= overlap tokens ---
		overlapAccum := 0
		backIdx := j
		for backIdx > i && overlapAccum < overlap {
			backIdx--
			overlapAccum += tokenCounts[backIdx]
		}

		// Advance by at least one paragraph to guarantee progress.
		nextI := max(i+1, backIdx)

		// Update the global offset by the number of paragraphs we are skipping.
		for k := i; k < nextI; k++ {
			globalOffset += tokenCounts[k]
		}
		i = nextI
	}

	return chunks
}

// splitIntoParagraphs splits text on blank lines (two or more consecutive newlines).
// Each paragraph's internal whitespace is normalised to single spaces so that
// wrapped lines (common in PDF extraction) don't produce spurious tokens.
var blankLineRe = regexp.MustCompile(`\r?\n[ \t]*(\r?\n[ \t]*)+`)
var internalWSRe = regexp.MustCompile(`[ \t\r\n]+`)

func splitIntoParagraphs(text string) []string {
	if !utf8.ValidString(text) {
		text = strings.ToValidUTF8(text, "")
	}

	parts := blankLineRe.Split(text, -1)

	var paragraphs []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		// Collapse internal whitespace (single newlines, tabs, multiple spaces)
		// to a single space.  This handles PDF line-wrapping artefacts without
		// destroying paragraph boundaries, which were already split above.
		p = internalWSRe.ReplaceAllString(p, " ")
		paragraphs = append(paragraphs, p)
	}
	return paragraphs
}

// normalizeWhitespace is kept for backward-compatibility with any external
// callers that may reference it directly.  New code should prefer Chunk().
func normalizeWhitespace(s string) string {
	if !utf8.ValidString(s) {
		s = strings.ToValidUTF8(s, "")
	}
	var b strings.Builder
	b.Grow(len(s))
	prevSpace := false
	for _, r := range s {
		if r == '\n' || r == '\r' || r == '\t' {
			r = ' '
		}
		if r == ' ' {
			if !prevSpace {
				b.WriteRune(r)
			}
			prevSpace = true
		} else {
			b.WriteRune(r)
			prevSpace = false
		}
	}
	return strings.TrimSpace(b.String())
}

// EstimateTokenCount returns a rough word-count approximation of token count.
func EstimateTokenCount(text string) int {
	return len(strings.Fields(text))
}

// ValidateChunkConfig returns an error if the configuration is invalid.
func ValidateChunkConfig(cfg ChunkConfig) error {
	if cfg.WindowTokens <= 0 {
		return fmt.Errorf("chunker: WindowTokens must be > 0")
	}
	if cfg.OverlapTokens < 0 {
		return fmt.Errorf("chunker: OverlapTokens must be >= 0")
	}
	if cfg.OverlapTokens >= cfg.WindowTokens {
		return fmt.Errorf("chunker: OverlapTokens (%d) must be < WindowTokens (%d)",
			cfg.OverlapTokens, cfg.WindowTokens)
	}
	return nil
}
