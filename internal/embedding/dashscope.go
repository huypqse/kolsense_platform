package embedding

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

const dashScopeEmbedURL = "https://dashscope.aliyuncs.com/api/v1/services/embeddings/text-embedding/text-embedding"

// DashScopeEmbedder calls the Alibaba DashScope text-embedding API.
// Used as the production fallback when Ollama is unavailable.
type DashScopeEmbedder struct {
	apiKey     string
	model      string
	dimensions int
	client     *http.Client
}

// NewDashScopeEmbedder creates a new DashScope-backed embedder.
func NewDashScopeEmbedder(apiKey, model string, dimensions int) *DashScopeEmbedder {
	return &DashScopeEmbedder{
		apiKey:     apiKey,
		model:      model,
		dimensions: dimensions,
		client:     &http.Client{Timeout: 60 * time.Second},
	}
}

func (e *DashScopeEmbedder) Dimensions() int { return e.dimensions }

type dashScopeEmbedRequest struct {
	Model string                    `json:"model"`
	Input dashScopeEmbedInput       `json:"input"`
}

type dashScopeEmbedInput struct {
	Texts []string `json:"texts"`
}

type dashScopeEmbedResponse struct {
	Output struct {
		Embeddings []struct {
			Embedding []float32 `json:"embedding"`
			TextIndex int       `json:"text_index"`
		} `json:"embeddings"`
	} `json:"output"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Embed sends texts to DashScope and returns embeddings.
// DashScope supports batches up to 25 texts; this implementation sends in one call.
func (e *DashScopeEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}

	reqBody := dashScopeEmbedRequest{
		Model: e.model,
		Input: dashScopeEmbedInput{Texts: texts},
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("dashscope embed: marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, dashScopeEmbedURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("dashscope embed: create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+e.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("dashscope embed: do request: %w", err)
	}
	defer resp.Body.Close()

	var result dashScopeEmbedResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("dashscope embed: decode response: %w", err)
	}
	if result.Code != "" {
		return nil, fmt.Errorf("dashscope embed: API error %s: %s", result.Code, result.Message)
	}

	// DashScope returns results sorted by text_index
	embeddings := make([][]float32, len(texts))
	for _, e := range result.Output.Embeddings {
		if e.TextIndex < len(embeddings) {
			embeddings[e.TextIndex] = e.Embedding
		}
	}
	return embeddings, nil
}
