// SPDX-License-Identifier: MIT

package ai

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// DefaultBaseURL is the OpenAI API endpoint used when
// OpenAIClientOptions.BaseURL is empty.
const DefaultBaseURL = "https://api.openai.com/v1"

// OpenAIClientOptions configures NewOpenAIClient.
type OpenAIClientOptions struct {
	// BaseURL of the OpenAI-compatible API, without the trailing
	// "/chat/completions" (e.g. "http://localhost:11434/v1" for Ollama,
	// "http://localhost:4000" for LiteLLM). Empty = DefaultBaseURL.
	BaseURL string
	// APIKey sent as "Authorization: Bearer …". May be empty for local
	// endpoints that don't check it.
	APIKey string
	// Model name, e.g. "gpt-4o-mini". Required.
	Model string
	// HTTPClient overrides the default client (120 s timeout).
	HTTPClient *http.Client
	// MaxRetries for HTTP 429 and 5xx responses, with exponential backoff.
	// Negative = no retries. Zero = default (2).
	MaxRetries int
}

// OpenAIClient talks to an OpenAI-compatible chat-completions endpoint using
// only the standard library. It implements [AIClient]. Mirrors the role of
// Aspose.Pdf.AI's OpenAIClient.
type OpenAIClient struct {
	opts OpenAIClientOptions
}

// NewOpenAIClient returns a client for an OpenAI-compatible API.
func NewOpenAIClient(opts OpenAIClientOptions) *OpenAIClient {
	if opts.BaseURL == "" {
		opts.BaseURL = DefaultBaseURL
	}
	if opts.HTTPClient == nil {
		opts.HTTPClient = &http.Client{Timeout: 120 * time.Second}
	}
	switch {
	case opts.MaxRetries == 0:
		opts.MaxRetries = 2
	case opts.MaxRetries < 0:
		opts.MaxRetries = 0
	}
	return &OpenAIClient{opts: opts}
}

// Wire format (subset of the OpenAI chat-completions schema).

type oaContentPart struct {
	Type     string      `json:"type"`
	Text     string      `json:"text,omitempty"`
	ImageURL *oaImageURL `json:"image_url,omitempty"`
}

type oaImageURL struct {
	URL string `json:"url"`
}

// oaMessage carries either a plain string content (text-only message) or an
// array of content parts (vision).
type oaMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

type oaRequest struct {
	Model       string      `json:"model"`
	Messages    []oaMessage `json:"messages"`
	Temperature *float64    `json:"temperature,omitempty"`
	MaxTokens   int         `json:"max_tokens,omitempty"`
}

type oaResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
		Code    any    `json:"code"`
	} `json:"error"`
}

// Complete sends one chat completion and returns the model's reply.
func (c *OpenAIClient) Complete(ctx context.Context, req CompletionRequest) (*CompletionResponse, error) {
	if c.opts.Model == "" {
		return nil, errors.New("ai: OpenAIClientOptions.Model is required")
	}
	if len(req.Messages) == 0 {
		return nil, errors.New("ai: completion request has no messages")
	}

	body, err := json.Marshal(c.buildRequest(req))
	if err != nil {
		return nil, fmt.Errorf("ai: marshal request: %w", err)
	}
	url := strings.TrimRight(c.opts.BaseURL, "/") + "/chat/completions"

	var lastErr error
	for attempt := 0; attempt <= c.opts.MaxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff: 0.5s, 1s, 2s, …
			delay := time.Duration(500*(1<<(attempt-1))) * time.Millisecond
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}
		}
		resp, retryable, err := c.doOnce(ctx, url, body)
		if err == nil {
			return resp, nil
		}
		lastErr = err
		if !retryable {
			return nil, err
		}
	}
	return nil, lastErr
}

func (c *OpenAIClient) buildRequest(req CompletionRequest) oaRequest {
	out := oaRequest{
		Model:       c.opts.Model,
		Temperature: req.Temperature,
		MaxTokens:   req.MaxTokens,
	}
	for _, m := range req.Messages {
		if len(m.Images) == 0 {
			out.Messages = append(out.Messages, oaMessage{Role: m.Role, Content: m.Text})
			continue
		}
		parts := make([]oaContentPart, 0, len(m.Images)+1)
		if m.Text != "" {
			parts = append(parts, oaContentPart{Type: "text", Text: m.Text})
		}
		for _, img := range m.Images {
			dataURL := "data:" + img.MIME + ";base64," + base64.StdEncoding.EncodeToString(img.Data)
			parts = append(parts, oaContentPart{Type: "image_url", ImageURL: &oaImageURL{URL: dataURL}})
		}
		out.Messages = append(out.Messages, oaMessage{Role: m.Role, Content: parts})
	}
	return out
}

// doOnce performs a single HTTP round-trip. retryable reports whether the
// failure is worth retrying (429, 5xx, transport error).
func (c *OpenAIClient) doOnce(ctx context.Context, url string, body []byte) (_ *CompletionResponse, retryable bool, _ error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, false, fmt.Errorf("ai: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if c.opts.APIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.opts.APIKey)
	}

	httpResp, err := c.opts.HTTPClient.Do(httpReq)
	if err != nil {
		// Do not retry context cancellation.
		if ctx.Err() != nil {
			return nil, false, ctx.Err()
		}
		return nil, true, fmt.Errorf("ai: request failed: %w", err)
	}
	defer func() { _ = httpResp.Body.Close() }()
	respBody, err := io.ReadAll(io.LimitReader(httpResp.Body, 16<<20))
	if err != nil {
		return nil, true, fmt.Errorf("ai: read response: %w", err)
	}

	var parsed oaResponse
	// A non-JSON error body (e.g. an HTML gateway page) still yields a
	// useful APIError below; ignore the unmarshal failure for those.
	_ = json.Unmarshal(respBody, &parsed)

	if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
		apiErr := &APIError{Status: httpResp.StatusCode}
		if parsed.Error != nil {
			apiErr.Message = parsed.Error.Message
			apiErr.Code = fmt.Sprint(parsed.Error.Code)
		}
		retry := httpResp.StatusCode == http.StatusTooManyRequests || httpResp.StatusCode >= 500
		return nil, retry, apiErr
	}
	if len(parsed.Choices) == 0 {
		return nil, false, errors.New("ai: response contains no choices")
	}
	return &CompletionResponse{
		Text: parsed.Choices[0].Message.Content,
		Usage: Usage{
			PromptTokens:     parsed.Usage.PromptTokens,
			CompletionTokens: parsed.Usage.CompletionTokens,
			TotalTokens:      parsed.Usage.TotalTokens,
		},
	}, false, nil
}
