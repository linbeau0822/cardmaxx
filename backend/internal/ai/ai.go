// Package ai is a minimal HTTP client for OpenAI's chat completions API.
//
// It knows nothing about CardMaxx's domain: callers build the prompt and get
// back the model's reply. That keeps the one-way rule from the README's
// design principles enforceable in one place (the system prompt a caller
// supplies), not duplicated here.
package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	defaultBaseURL = "https://api.openai.com/v1/chat/completions"
	requestTimeout = 30 * time.Second
)

// APIError is returned when OpenAI rejects a request.
type APIError struct {
	StatusCode int
	Message    string
	Type       string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("openai: %s (status %d)", e.Message, e.StatusCode)
}

// Client talks to OpenAI's chat completions endpoint.
type Client struct {
	apiKey     string
	model      string
	baseURL    string
	httpClient *http.Client
}

// NewClient builds a Client that asks model for completions.
func NewClient(apiKey, model string) *Client {
	return &Client{
		apiKey:     apiKey,
		model:      model,
		baseURL:    defaultBaseURL,
		httpClient: &http.Client{Timeout: requestTimeout},
	}
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Temperature float64       `json:"temperature"`
}

type chatResponse struct {
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
}

type errorEnvelope struct {
	Error struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error"`
}

// Complete sends a system prompt and a user prompt and returns the model's
// reply text.
func (c *Client) Complete(ctx context.Context, system, user string) (string, error) {
	body, err := json.Marshal(chatRequest{
		Model: c.model,
		Messages: []chatMessage{
			{Role: "system", Content: system},
			{Role: "user", Content: user},
		},
		// Low temperature: this assistant explains numbers already computed
		// elsewhere, it isn't being creative.
		Temperature: 0.2,
	})
	if err != nil {
		return "", fmt.Errorf("openai: encode request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("openai: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)

	httpResp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("openai: call chat completions: %w", err)
	}
	defer httpResp.Body.Close()

	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return "", fmt.Errorf("openai: read response: %w", err)
	}

	if httpResp.StatusCode != http.StatusOK {
		apiErr := &APIError{StatusCode: httpResp.StatusCode, Message: string(respBody)}
		var envelope errorEnvelope
		if err := json.Unmarshal(respBody, &envelope); err == nil && envelope.Error.Message != "" {
			apiErr.Message = envelope.Error.Message
			apiErr.Type = envelope.Error.Type
		}
		return "", apiErr
	}

	var resp chatResponse
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return "", fmt.Errorf("openai: decode response: %w", err)
	}
	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("openai: response had no choices")
	}
	return resp.Choices[0].Message.Content, nil
}
