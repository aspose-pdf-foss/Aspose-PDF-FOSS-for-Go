// SPDX-License-Identifier: MIT

// ai_summary summarizes a PDF with an OpenAI-compatible model and saves the
// summary as both text output and a PDF next to the input.
//
// Requires a live endpoint — configure via environment:
//
//	AI_BASE_URL  e.g. https://api.openai.com/v1 (default),
//	             http://localhost:11434/v1 (Ollama), http://localhost:4000 (LiteLLM)
//	AI_API_KEY   the bearer token (may be empty for local endpoints)
//	AI_MODEL     e.g. gpt-4o-mini, llama3.2-vision
//
// Usage: go run ./_examples/ai_summary <input.pdf>
//
// Note: the document's extracted text is sent to the configured endpoint.
package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
	"github.com/aspose-pdf-foss/aspose-pdf-foss-for-go/ai"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: ai_summary <input.pdf>")
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
	copilot := ai.NewSummaryCopilot(client, ai.SummaryOptions{
		Document: doc,
		MaxWords: 200,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	summary, err := copilot.GetSummary(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, "summarize:", err)
		os.Exit(1)
	}
	fmt.Println(summary)

	out := filepath.Join("result_files", strings.TrimSuffix(filepath.Base(os.Args[1]), filepath.Ext(os.Args[1]))+"_summary.pdf")
	_ = os.MkdirAll("result_files", 0o755)
	if err := copilot.SaveSummary(ctx, out); err != nil {
		fmt.Fprintln(os.Stderr, "save summary pdf:", err)
		os.Exit(1)
	}
	fmt.Println("\nSaved:", out)
}
