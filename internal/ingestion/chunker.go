// Package ingestion handles PDF text extraction, chunking, embedding, and storage.
package ingestion

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

const (
	defaultWindowTokens = 512
	defaultOverlapTokens = 64
)

// ChunkConfig configures the sliding-window chunker.
type ChunkConfig struct {
	WindowTokens int // target chunk size in whitespace-delimited tokens
	OverlapTokens int // number of tokens to repeat at the start of next chunk
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

// Chunker splits text into overlapping windows of approximately WindowTokens tokens.
// Token approximation: whitespace-split words (fast, language-agnostic).
type Chunker struct {
	cfg ChunkConfig
}

// NewChunker creates a Chunker with the given configuration.
func NewChunker(cfg ChunkConfig) *Chunker {
	return &Chunker{cfg: cfg}
}

// Chunk splits the input text into overlapping TextChunks.
func (c *Chunker) Chunk(text string) []TextChunk {
	text = normalizeWhitespace(text)
	words := strings.Fields(text)
	if len(words) == 0 {
		return nil
	}

	var chunks []TextChunk
	window := c.cfg.WindowTokens
	overlap := c.cfg.OverlapTokens
	step := window - overlap
	if step <= 0 {
		step = window
	}

	for start := 0; start < len(words); start += step {
		end := start + window
		if end > len(words) {
			end = len(words)
		}
		segment := words[start:end]
		chunks = append(chunks, TextChunk{
			Text:       strings.Join(segment, " "),
			StartToken: start,
			EndToken:   end,
			TokenCount: end - start,
		})
		if end == len(words) {
			break
		}
	}
	return chunks
}

// normalizeWhitespace collapses multiple whitespace characters into a single space
// and removes leading/trailing whitespace.
func normalizeWhitespace(s string) string {
	if !utf8.ValidString(s) {
		// Replace invalid UTF-8 sequences
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
