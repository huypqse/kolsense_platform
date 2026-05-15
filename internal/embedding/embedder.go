// Package embedding defines the Embedder interface and its implementations.
// Supported backends: Ollama (local dev) and DashScope (production).
package embedding

import "context"

// Embedder converts text into fixed-dimension float32 vectors.
// All implementations must be safe for concurrent use.
type Embedder interface {
	// Embed returns one embedding vector per input text.
	// Texts are processed in a single batch call where the provider allows it.
	Embed(ctx context.Context, texts []string) ([][]float32, error)

	// Dimensions returns the fixed output vector length for this embedder.
	Dimensions() int
}
