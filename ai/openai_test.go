// SPDX-License-Identifier: MIT

package ai

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// completionJSON is a minimal valid chat-completions response.
func completionJSON(text string) string {
	b, _ := json.Marshal(map[string]any{
		"choices": []map[string]any{{"message": map[string]any{"content": text}}},
		"usage":   map[string]int{"prompt_tokens": 10, "completion_tokens": 5, "total_tokens": 15},
	})
	return string(b)
}

func TestOpenAIClientRequestShape(t *testing.T) {
	var got struct {
		Model    string `json:"model"`
		Messages []struct {
			Role    string          `json:"role"`
			Content json.RawMessage `json:"content"`
		} `json:"messages"`
		Temperature *float64 `json:"temperature"`
		MaxTokens   int      `json:"max_tokens"`
	}
	var auth, path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		path = r.URL.Path
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Errorf("bad request body: %v", err)
		}
		w.Write([]byte(completionJSON("hello")))
	}))
	defer srv.Close()

	client := NewOpenAIClient(OpenAIClientOptions{BaseURL: srv.URL + "/v1/", APIKey: "sk-test", Model: "test-model"})
	temp := 0.2
	resp, err := client.Complete(context.Background(), CompletionRequest{
		Messages: []Message{
			{Role: RoleSystem, Text: "You summarize."},
			{Role: RoleUser, Text: "look", Images: []MessageImage{{MIME: "image/png", Data: []byte{1, 2, 3}}}},
		},
		Temperature: &temp,
		MaxTokens:   64,
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Text != "hello" {
		t.Errorf("Text = %q; want hello", resp.Text)
	}
	if resp.Usage.TotalTokens != 15 {
		t.Errorf("TotalTokens = %d; want 15", resp.Usage.TotalTokens)
	}
	if auth != "Bearer sk-test" {
		t.Errorf("Authorization = %q", auth)
	}
	if path != "/v1/chat/completions" {
		t.Errorf("path = %q; want /v1/chat/completions (trailing slash trimmed)", path)
	}
	if got.Model != "test-model" || got.MaxTokens != 64 || got.Temperature == nil || *got.Temperature != 0.2 {
		t.Errorf("request fields = %+v", got)
	}
	if len(got.Messages) != 2 {
		t.Fatalf("messages = %d; want 2", len(got.Messages))
	}
	// Text-only message → plain string content.
	var plain string
	if err := json.Unmarshal(got.Messages[0].Content, &plain); err != nil || plain != "You summarize." {
		t.Errorf("system content = %s", got.Messages[0].Content)
	}
	// Vision message → content parts with a data: URL.
	var parts []struct {
		Type     string `json:"type"`
		Text     string `json:"text"`
		ImageURL *struct {
			URL string `json:"url"`
		} `json:"image_url"`
	}
	if err := json.Unmarshal(got.Messages[1].Content, &parts); err != nil {
		t.Fatalf("vision content not an array: %s", got.Messages[1].Content)
	}
	if len(parts) != 2 || parts[0].Type != "text" || parts[1].Type != "image_url" {
		t.Fatalf("vision parts = %+v", parts)
	}
	if !strings.HasPrefix(parts[1].ImageURL.URL, "data:image/png;base64,AQID") {
		t.Errorf("image URL = %q", parts[1].ImageURL.URL)
	}
}

func TestOpenAIClientRetriesOn429(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls < 3 {
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte(`{"error":{"message":"slow down"}}`))
			return
		}
		w.Write([]byte(completionJSON("ok")))
	}))
	defer srv.Close()

	client := NewOpenAIClient(OpenAIClientOptions{BaseURL: srv.URL, Model: "m"})
	resp, err := client.Complete(context.Background(), CompletionRequest{Messages: []Message{{Role: RoleUser, Text: "hi"}}})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Text != "ok" || calls != 3 {
		t.Errorf("Text=%q calls=%d; want ok after 3 calls", resp.Text, calls)
	}
}

func TestOpenAIClientAPIErrorNoRetryOn400(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":{"message":"bad model","code":"model_not_found"}}`))
	}))
	defer srv.Close()

	client := NewOpenAIClient(OpenAIClientOptions{BaseURL: srv.URL, Model: "m"})
	_, err := client.Complete(context.Background(), CompletionRequest{Messages: []Message{{Role: RoleUser, Text: "hi"}}})
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("err = %v; want *APIError", err)
	}
	if apiErr.Status != 400 || apiErr.Message != "bad model" || apiErr.Code != "model_not_found" {
		t.Errorf("APIError = %+v", apiErr)
	}
	if calls != 1 {
		t.Errorf("calls = %d; 400 must not be retried", calls)
	}
}

func TestOpenAIClientValidation(t *testing.T) {
	client := NewOpenAIClient(OpenAIClientOptions{Model: ""})
	if _, err := client.Complete(context.Background(), CompletionRequest{Messages: []Message{{Role: RoleUser, Text: "x"}}}); err == nil {
		t.Error("missing model must error")
	}
	client = NewOpenAIClient(OpenAIClientOptions{Model: "m"})
	if _, err := client.Complete(context.Background(), CompletionRequest{}); err == nil {
		t.Error("empty messages must error")
	}
}
