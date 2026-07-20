// SPDX-License-Identifier: MIT

package ai

import (
	"context"
	"errors"
	"strings"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

// defaultChatContextRunes is the default budget for the document text stuffed
// into the chat context (≈12k tokens — conservative for small-context local
// models, leaving room for the conversation and the answer).
const defaultChatContextRunes = 48000

// ChatOptions configures a ChatCopilot. At least one content source
// (Document, Documents or Texts) is required.
type ChatOptions struct {
	Document  *pdf.Document   // single document to chat about
	Documents []*pdf.Document // multiple documents
	Texts     []string        // raw text inputs

	// SystemPrompt overrides the default instruction that frames the
	// assistant as answering strictly from the document context.
	SystemPrompt string
	// MaxContextRunes caps the document text placed in the context; 0 uses
	// the default. Text beyond the cap is dropped and the model is told the
	// context was truncated.
	MaxContextRunes int
	// Temperature for the completion; nil = provider default.
	Temperature *float64
}

// ChatCopilot answers questions about a document, keeping conversation
// history. It stuffs the document's extracted text into the system context
// (no embeddings/vector store in v1). Mirrors Aspose.PDF for .NET's
// OpenAIChatCopilot.
//
// Privacy: the documents' extracted text and the conversation are sent to the
// configured AI endpoint.
type ChatCopilot struct {
	client    AIClient
	opts      ChatOptions
	context   string    // cached, truncated document text
	built     bool      // context has been gathered
	truncated bool      // document text exceeded the budget
	history   []Message // alternating user/assistant turns
}

// NewChatCopilot returns a copilot that answers questions about the content in
// opts via client.
func NewChatCopilot(client AIClient, opts ChatOptions) *ChatCopilot {
	return &ChatCopilot{client: client, opts: opts}
}

// Ask sends a question and returns the model's answer, appending both to the
// conversation history so follow-up questions have context.
func (c *ChatCopilot) Ask(ctx context.Context, question string) (string, error) {
	if c.client == nil {
		return "", errors.New("ai: ChatCopilot has no client")
	}
	if strings.TrimSpace(question) == "" {
		return "", errors.New("ai: empty question")
	}
	if err := c.buildContext(); err != nil {
		return "", err
	}

	messages := make([]Message, 0, len(c.history)+2)
	messages = append(messages, Message{Role: RoleSystem, Text: c.systemPrompt()})
	messages = append(messages, c.history...)
	messages = append(messages, Message{Role: RoleUser, Text: question})

	resp, err := c.client.Complete(ctx, CompletionRequest{
		Messages:    messages,
		Temperature: c.opts.Temperature,
	})
	if err != nil {
		return "", err
	}
	answer := strings.TrimSpace(resp.Text)
	c.history = append(c.history,
		Message{Role: RoleUser, Text: question},
		Message{Role: RoleAssistant, Text: answer},
	)
	return answer, nil
}

// History returns a copy of the conversation so far (alternating user and
// assistant turns).
func (c *ChatCopilot) History() []Message {
	out := make([]Message, len(c.history))
	copy(out, c.history)
	return out
}

// Reset clears the conversation history. The document context is retained.
func (c *ChatCopilot) Reset() {
	c.history = nil
}

// systemPrompt frames the assistant and embeds the document context.
func (c *ChatCopilot) systemPrompt() string {
	var b strings.Builder
	if c.opts.SystemPrompt != "" {
		b.WriteString(c.opts.SystemPrompt)
	} else {
		b.WriteString("You are a helpful assistant answering questions about the document below. " +
			"Base your answers only on the document's content; if the answer is not in the document, say so plainly. " +
			"Be concise.")
	}
	if c.truncated {
		b.WriteString(" Note: the document was too long to include in full, so only its beginning is provided.")
	}
	b.WriteString("\n\n--- DOCUMENT ---\n")
	b.WriteString(c.context)
	b.WriteString("\n--- END DOCUMENT ---")
	return b.String()
}

// buildContext gathers and caches the document text, truncated to the budget.
func (c *ChatCopilot) buildContext() error {
	if c.built {
		return nil
	}
	texts, err := gatherSourceTexts(c.opts.Document, c.opts.Documents, c.opts.Texts)
	if err != nil {
		return err
	}
	if len(texts) == 0 {
		return errors.New("ai: no document text (scanned documents need OCR first — see OcrCopilot.MakeSearchable)")
	}
	budget := c.opts.MaxContextRunes
	if budget <= 0 {
		budget = defaultChatContextRunes
	}
	joined := strings.Join(texts, "\n\n")
	runes := []rune(joined)
	if len(runes) > budget {
		joined = string(runes[:budget])
		c.truncated = true
	}
	c.context = joined
	c.built = true
	return nil
}
