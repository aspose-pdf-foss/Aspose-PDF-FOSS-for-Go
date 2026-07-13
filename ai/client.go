// SPDX-License-Identifier: MIT

// Package ai provides AI-powered document operations — summarization, OCR of
// scanned pages, and a hidden-text-layer "make searchable" pipeline — built on
// the asposepdf core. It mirrors the copilot surface of Aspose.PDF for .NET's
// Aspose.Pdf.AI namespace, adapted to Go idioms (contexts and error returns
// instead of async methods, options structs instead of builders).
//
// The package is pure Go, standard library only: the bundled client talks to
// any OpenAI-compatible chat-completions endpoint (OpenAI, LiteLLM, Ollama,
// OpenRouter, …) over net/http. The asposepdf root package remains free of AI
// and network code — nothing here runs unless a copilot is explicitly invoked.
//
// Privacy: these features send document content (extracted text and/or
// rendered page images) to the configured AI endpoint. For sensitive
// documents, point the client at a local endpoint (e.g. Ollama or a LiteLLM
// gateway on your own infrastructure) — the same client works unchanged.
package ai

import (
	"context"
	"fmt"
)

// AIClient is the contract every copilot consumes: one call, one chat
// completion. The bundled implementation is [OpenAIClient]; supply your own to
// integrate a different provider or transport. Mirrors the role of
// Aspose.Pdf.AI's IAIClient.
type AIClient interface {
	Complete(ctx context.Context, req CompletionRequest) (*CompletionResponse, error)
}

// Message roles, per the OpenAI chat-completions convention.
const (
	RoleSystem    = "system"
	RoleUser      = "user"
	RoleAssistant = "assistant"
)

// Message is a single chat message. Images (for vision models) accompany the
// text as data-URL content parts.
type Message struct {
	Role   string // RoleSystem, RoleUser or RoleAssistant
	Text   string
	Images []MessageImage
}

// MessageImage is an inline image attached to a message, sent as a
// base64 data: URL. MIME is e.g. "image/png" or "image/jpeg".
type MessageImage struct {
	MIME string
	Data []byte
}

// CompletionRequest describes one chat-completion call.
type CompletionRequest struct {
	Messages    []Message
	Temperature *float64 // nil = provider default
	MaxTokens   int      // 0 = provider default
}

// Usage reports token counts when the provider returns them.
type Usage struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
}

// CompletionResponse is the model's reply.
type CompletionResponse struct {
	Text  string
	Usage Usage
}

// APIError is returned when the AI endpoint answers with a non-2xx status.
type APIError struct {
	Status  int    // HTTP status code
	Code    string // provider error code, when present
	Message string // provider error message, when present
}

func (e *APIError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("ai: API error %d: %s", e.Status, e.Message)
	}
	return fmt.Sprintf("ai: API error %d", e.Status)
}
