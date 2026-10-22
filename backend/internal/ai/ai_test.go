package ai

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

// testClient points a Client at a local httptest server instead of
// api.openai.com, so these tests make no network calls.
func testClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return &Client{apiKey: "key", model: "gpt-4o-mini", baseURL: server.URL, httpClient: server.Client()}
}

func TestCompleteReturnsFirstChoice(t *testing.T) {
	var gotReq chatRequest
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if auth := r.Header.Get("Authorization"); auth != "Bearer key" {
			t.Errorf("Authorization header = %q, want %q", auth, "Bearer key")
		}
		if err := json.NewDecoder(r.Body).Decode(&gotReq); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		_ = json.NewEncoder(w).Encode(chatResponse{
			Choices: []struct {
				Message chatMessage `json:"message"`
			}{{Message: chatMessage{Role: "assistant", Content: "because card X earns 3% on groceries"}}},
		})
	})

	got, err := client.Complete(context.Background(), "system prompt", "user question")
	if err != nil {
		t.Fatalf("Complete() returned error: %v", err)
	}
	if want := "because card X earns 3% on groceries"; got != want {
		t.Errorf("Complete() = %q, want %q", got, want)
	}
	if gotReq.Model != "gpt-4o-mini" {
		t.Errorf("request model = %q, want %q", gotReq.Model, "gpt-4o-mini")
	}
	if len(gotReq.Messages) != 2 || gotReq.Messages[0].Content != "system prompt" || gotReq.Messages[1].Content != "user question" {
		t.Errorf("request messages = %+v, want system+user prompt", gotReq.Messages)
	}
}

func TestCompleteSurfacesAPIError(t *testing.T) {
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]string{"message": "rate limit exceeded", "type": "rate_limit_error"},
		})
	})

	_, err := client.Complete(context.Background(), "system", "user")
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("Complete() error = %v, want *APIError", err)
	}
	if apiErr.StatusCode != http.StatusTooManyRequests {
		t.Errorf("StatusCode = %d, want %d", apiErr.StatusCode, http.StatusTooManyRequests)
	}
	if apiErr.Message != "rate limit exceeded" {
		t.Errorf("Message = %q, want %q", apiErr.Message, "rate limit exceeded")
	}
}
