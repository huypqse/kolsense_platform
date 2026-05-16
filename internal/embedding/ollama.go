package embedding

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// OllamaEmbedder calls the local Ollama /api/embed endpoint.
type OllamaEmbedder struct {
	baseURL    string
	model      string
	dimensions int
	client     *http.Client
}

// NewOllamaEmbedder creates a new Ollama-backed embedder.
func NewOllamaEmbedder(baseURL, model string, dimensions int) *OllamaEmbedder {
	return &OllamaEmbedder{
		baseURL:    baseURL,
		model:      model,
		dimensions: dimensions,
		client: &http.Client{
			Timeout: 300 * time.Second,
		},
	}
}

func (e *OllamaEmbedder) Dimensions() int { return e.dimensions }

// ollamaEmbedRequest is the Ollama /api/embed request body.
type ollamaEmbedRequest struct {
	Model  string   `json:"model"`
	Input  []string `json:"input"`
}

// ollamaEmbedResponse is the Ollama /api/embed response body.
type ollamaEmbedResponse struct {
	Embeddings [][]float32 `json:"embeddings"`
}

// Embed calls Ollama to embed a batch of texts.
func (e *OllamaEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}

	reqBody := ollamaEmbedRequest{
		Model: e.model,
		Input: texts,
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("ollama embed: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.baseURL+"/api/embed", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("ollama embed: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ollama embed: do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ollama embed: unexpected status %d", resp.StatusCode)
	}

	var result ollamaEmbedResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("ollama embed: decode response: %w", err)
	}
	if len(result.Embeddings) != len(texts) {
		return nil, fmt.Errorf("ollama embed: expected %d embeddings, got %d", len(texts), len(result.Embeddings))
	}

	return result.Embeddings, nil
}
