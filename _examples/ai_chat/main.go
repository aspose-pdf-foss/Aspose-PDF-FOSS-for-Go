// SPDX-License-Identifier: MIT

// ai_chat opens an interactive Q&A session about a PDF using an
// OpenAI-compatible model: type a question, get an answer grounded in the
// document; the conversation keeps its history.
//
// Requires a live endpoint — configure via environment:
//
//	AI_BASE_URL  e.g. https://api.openai.com/v1 (default),
//	             http://localhost:11434/v1 (Ollama), http://localhost:4000 (LiteLLM)
//	AI_API_KEY   the bearer token (may be empty for local endpoints)
//	AI_MODEL     e.g. gpt-4o-mini, llama3.1
//
// Usage: go run ./_examples/ai_chat <input.pdf>
// Type a blank line (or Ctrl-D) to quit.
//
// Note: the document's extracted text and your questions are sent to the
// configured endpoint.
package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
	"github.com/aspose-pdf-foss/aspose-pdf-foss-for-go/ai"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: ai_chat <input.pdf>")
		os.Exit(2)
	}
	model := os.Getenv("AI_MODEL")
	if model == "" {
		fmt.Fprintln(os.Stderr, "AI_MODEL is required (e.g. gpt-4o-mini)")
		os.Exit(2)
	}

	doc, err := pdf.Open(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, "open:", err)
		os.Exit(1)
	}

	client := ai.NewOpenAIClient(ai.OpenAIClientOptions{
		BaseURL: os.Getenv("AI_BASE_URL"),
		APIKey:  os.Getenv("AI_API_KEY"),
		Model:   model,
	})
	chat := ai.NewChatCopilot(client, ai.ChatOptions{Document: doc})

	fmt.Println("Ask about the document (blank line to quit):")
	in := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print("\n> ")
		if !in.Scan() {
			break
		}
		q := strings.TrimSpace(in.Text())
		if q == "" {
			break
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		answer, err := chat.Ask(ctx, q)
		cancel()
		if err != nil {
			fmt.Fprintln(os.Stderr, "ask:", err)
			continue
		}
		fmt.Println(answer)
	}
}
