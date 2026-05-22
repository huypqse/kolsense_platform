package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const geminiBaseURL = "https://generativelanguage.googleapis.com/v1beta/models/"

// GeminiClient calls the Google Gemini generateContent API.
type GeminiClient struct {
	apiKey      string
	model       string
	maxTokens   int
	temperature float64
	client      *http.Client
}

// NewGeminiClient creates a new Gemini-backed LLM client.
func NewGeminiClient(apiKey, model string, maxTokens int, temperature float64) *GeminiClient {
	return &GeminiClient{
		apiKey:      apiKey,
		model:       model,
		maxTokens:   maxTokens,
		temperature: temperature,
		client:      &http.Client{Timeout: 180 * time.Second},
	}
}

type geminiRequest struct {
	Contents          []geminiContent          `json:"contents"`
	SystemInstruction *geminiSystemInstruction `json:"systemInstruction,omitempty"`
	GenerationConfig  geminiGenerationConfig   `json:"generationConfig,omitempty"`
}

type geminiContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []geminiPart `json:"parts"`
}

type geminiPart struct {
	Text string `json:"text"`
}

type geminiSystemInstruction struct {
	Parts []geminiPart `json:"parts"`
}

type geminiGenerationConfig struct {
	Temperature     float64 `json:"temperature,omitempty"`
	MaxOutputTokens int     `json:"maxOutputTokens,omitempty"`
}

type geminiResponse struct {
	Candidates []struct {
		Content struct {
			Parts []geminiPart `json:"parts"`
		} `json:"content"`
	} `json:"candidates"`
	UsageMetadata struct {
		PromptTokenCount     int `json:"promptTokenCount"`
		CandidatesTokenCount int `json:"candidatesTokenCount"`
	} `json:"usageMetadata"`
	Error *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Status  string `json:"status"`
	} `json:"error"`
}

// Complete sends a prompt and returns the full response (non-streaming).
func (c *GeminiClient) Complete(ctx context.Context, req CompletionRequest) (CompletionResponse, error) {
	body, err := json.Marshal(c.buildRequest(req))
	if err != nil {
		return CompletionResponse{}, fmt.Errorf("gemini complete: marshal: %w", err)
	}

	url := geminiBaseURL + c.model + ":generateContent?key=" + c.apiKey
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return CompletionResponse{}, fmt.Errorf("gemini complete: create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return CompletionResponse{}, fmt.Errorf("gemini complete: do request: %w", err)
	}
	defer resp.Body.Close()

	var result geminiResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return CompletionResponse{}, fmt.Errorf("gemini complete: decode: %w", err)
	}
	if result.Error != nil {
		return CompletionResponse{}, fmt.Errorf("gemini complete: API error [%s]: %s", result.Error.Status, result.Error.Message)
	}
	if len(result.Candidates) == 0 || len(result.Candidates[0].Content.Parts) == 0 {
		return CompletionResponse{}, fmt.Errorf("gemini complete: empty candidates")
	}

	return CompletionResponse{
		Text:         result.Candidates[0].Content.Parts[0].Text,
		InputTokens:  result.UsageMetadata.PromptTokenCount,
		OutputTokens: result.UsageMetadata.CandidatesTokenCount,
	}, nil
}

// Stream sends a prompt and returns a channel of text chunks using SSE streaming.
func (c *GeminiClient) Stream(ctx context.Context, req CompletionRequest) (<-chan string, error) {
	body, err := json.Marshal(c.buildRequest(req))
	if err != nil {
		return nil, fmt.Errorf("gemini stream: marshal: %w", err)
	}

	url := geminiBaseURL + c.model + ":streamGenerateContent?key=" + c.apiKey
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("gemini stream: create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("gemini stream: do request: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("gemini stream: status %d", resp.StatusCode)
	}

	ch := make(chan string, 64)
	go func() {
		defer close(ch)
		defer resp.Body.Close()
		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" {
				continue
			}
			if strings.HasPrefix(line, "data: ") {
				line = strings.TrimPrefix(line, "data: ")
			}
			if line == "[DONE]" {
				return
			}

			var chunk geminiResponse
			if err := json.Unmarshal([]byte(line), &chunk); err != nil {
				continue
			}
			if chunk.Error != nil {
				return
			}
			if len(chunk.Candidates) == 0 || len(chunk.Candidates[0].Content.Parts) == 0 {
				continue
			}
			text := chunk.Candidates[0].Content.Parts[0].Text
			if text == "" {
				continue
			}
			select {
			case ch <- text:
			case <-ctx.Done():
				return
			}
		}
	}()

	return ch, nil
}

func (c *GeminiClient) buildRequest(req CompletionRequest) geminiRequest {
	maxTok := req.MaxTokens
	if maxTok == 0 {
		maxTok = c.maxTokens
	}
	temp := req.Temperature
	if temp == 0 {
		temp = c.temperature
	}

	gr := geminiRequest{
		Contents: []geminiContent{{
			Role:  "user",
			Parts: []geminiPart{{Text: req.UserPrompt}},
		}},
		GenerationConfig: geminiGenerationConfig{
			MaxOutputTokens: maxTok,
			Temperature:     temp,
		},
	}
	if req.SystemPrompt != "" {
		gr.SystemInstruction = &geminiSystemInstruction{
			Parts: []geminiPart{{Text: req.SystemPrompt}},
		}
	}
	return gr
}
