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

const dashScopeChatURL = "https://dashscope.aliyuncs.com/compatible-mode/v1/chat/completions"

// DashScopeClient calls the Alibaba DashScope chat completion API (OpenAI-compatible).
type DashScopeClient struct {
	apiKey      string
	model       string
	maxTokens   int
	temperature float64
	client      *http.Client
}

// NewDashScopeClient creates a new DashScope-backed LLM client.
func NewDashScopeClient(apiKey, model string, maxTokens int, temperature float64) *DashScopeClient {
	return &DashScopeClient{
		apiKey:      apiKey,
		model:       model,
		maxTokens:   maxTokens,
		temperature: temperature,
		client:      &http.Client{Timeout: 180 * time.Second},
	}
}

type dashScopeChatRequest struct {
	Model       string              `json:"model"`
	Messages    []dashScopeMessage  `json:"messages"`
	MaxTokens   int                 `json:"max_tokens"`
	Temperature float64             `json:"temperature"`
	Stream      bool                `json:"stream"`
}

type dashScopeMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type dashScopeChatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
		Code    string `json:"code"`
	} `json:"error"`
}

// Complete sends a prompt and returns the full response (non-streaming).
func (c *DashScopeClient) Complete(ctx context.Context, req CompletionRequest) (CompletionResponse, error) {
	maxTok := req.MaxTokens
	if maxTok == 0 {
		maxTok = c.maxTokens
	}
	temp := req.Temperature
	if temp == 0 {
		temp = c.temperature
	}

	body, err := json.Marshal(dashScopeChatRequest{
		Model:       c.model,
		Messages:    buildDashScopeMessages(req),
		MaxTokens:   maxTok,
		Temperature: temp,
		Stream:      false,
	})
	if err != nil {
		return CompletionResponse{}, fmt.Errorf("dashscope complete: marshal: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, dashScopeChatURL, bytes.NewReader(body))
	if err != nil {
		return CompletionResponse{}, fmt.Errorf("dashscope complete: create request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return CompletionResponse{}, fmt.Errorf("dashscope complete: do request: %w", err)
	}
	defer resp.Body.Close()

	var result dashScopeChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return CompletionResponse{}, fmt.Errorf("dashscope complete: decode: %w", err)
	}
	if result.Error != nil {
		return CompletionResponse{}, fmt.Errorf("dashscope complete: API error [%s]: %s", result.Error.Code, result.Error.Message)
	}
	if len(result.Choices) == 0 {
		return CompletionResponse{}, fmt.Errorf("dashscope complete: empty choices")
	}

	return CompletionResponse{
		Text:         result.Choices[0].Message.Content,
		InputTokens:  result.Usage.PromptTokens,
		OutputTokens: result.Usage.CompletionTokens,
	}, nil
}

// Stream sends a prompt and returns a channel of text chunks using SSE streaming.
func (c *DashScopeClient) Stream(ctx context.Context, req CompletionRequest) (<-chan string, error) {
	maxTok := req.MaxTokens
	if maxTok == 0 {
		maxTok = c.maxTokens
	}
	temp := req.Temperature
	if temp == 0 {
		temp = c.temperature
	}

	body, err := json.Marshal(dashScopeChatRequest{
		Model:       c.model,
		Messages:    buildDashScopeMessages(req),
		MaxTokens:   maxTok,
		Temperature: temp,
		Stream:      true,
	})
	if err != nil {
		return nil, fmt.Errorf("dashscope stream: marshal: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, dashScopeChatURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("dashscope stream: create request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("dashscope stream: do request: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("dashscope stream: status %d", resp.StatusCode)
	}

	ch := make(chan string, 64)
	go func() {
		defer close(ch)
		defer resp.Body.Close()
		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			data := strings.TrimPrefix(line, "data: ")
			if data == "[DONE]" {
				return
			}
			var chunk dashScopeChatResponse
			if err := json.Unmarshal([]byte(data), &chunk); err != nil {
				continue
			}
			if len(chunk.Choices) > 0 && chunk.Choices[0].Delta.Content != "" {
				select {
				case ch <- chunk.Choices[0].Delta.Content:
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	return ch, nil
}

func buildDashScopeMessages(req CompletionRequest) []dashScopeMessage {
	msgs := make([]dashScopeMessage, 0, 2)
	if req.SystemPrompt != "" {
		msgs = append(msgs, dashScopeMessage{Role: "system", Content: req.SystemPrompt})
	}
	msgs = append(msgs, dashScopeMessage{Role: "user", Content: req.UserPrompt})
	return msgs
}
