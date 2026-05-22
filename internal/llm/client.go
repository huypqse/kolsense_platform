// Package llm defines the LLM client interface and its implementations.
// Supported backends: Ollama (local dev), DashScope (production), and Gemini.
package llm

import "context"

// CompletionRequest is the input to the LLM.
type CompletionRequest struct {
	SystemPrompt string
	UserPrompt   string
	MaxTokens    int
	Temperature  float64
}

// CompletionResponse is the output from the LLM.
type CompletionResponse struct {
	Text         string
	InputTokens  int
	OutputTokens int
}

// Client is the interface for interacting with a large language model.
// All implementations must be safe for concurrent use.
type Client interface {
	// Complete sends a prompt and returns the full response.
	Complete(ctx context.Context, req CompletionRequest) (CompletionResponse, error)

	// Stream sends a prompt and returns a channel of text chunks as they arrive.
	// The caller must drain the channel until it is closed.
	Stream(ctx context.Context, req CompletionRequest) (<-chan string, error)
}
