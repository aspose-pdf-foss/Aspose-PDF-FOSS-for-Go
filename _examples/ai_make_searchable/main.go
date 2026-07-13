// SPDX-License-Identifier: MIT

// ai_make_searchable OCRs a scanned PDF with a vision-capable model and
// writes an invisible text layer over each scanned page, so the output is
// selectable and Ctrl+F-searchable while looking pixel-identical.
//
// Requires a live vision endpoint — configure via environment:
//
//	AI_BASE_URL  e.g. https://api.openai.com/v1 (default),
//	             http://localhost:11434/v1 (Ollama)
//	AI_API_KEY   the bearer token (may be empty for local endpoints)
//	AI_MODEL     a vision model, e.g. gpt-4o-mini, llama3.2-vision
//
// Usage: go run ./_examples/ai_make_searchable <scanned.pdf>
//
// Note: rendered page images are sent to the configured endpoint.
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
		fmt.Fprintln(os.Stderr, "usage: ai_make_searchable <scanned.pdf>")
		os.Exit(2)
	}
	model := os.Getenv("AI_MODEL")
	if model == "" {
		fmt.Fprintln(os.Stderr, "AI_MODEL is required (a vision model, e.g. gpt-4o-mini)")
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
	engine := ai.NewLLMOCREngine(client, ai.LLMOCROptions{})
	copilot := ai.NewOcrCopilot(engine, ai.OcrOptions{Document: doc})

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	n, err := copilot.MakeSearchable(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, "ocr:", err)
		os.Exit(1)
	}
	fmt.Printf("OCRed %d scanned page(s)\n", n)

	out := filepath.Join("result_files", strings.TrimSuffix(filepath.Base(os.Args[1]), filepath.Ext(os.Args[1]))+"_searchable.pdf")
	_ = os.MkdirAll("result_files", 0o755)
	if err := doc.Save(out); err != nil {
		fmt.Fprintln(os.Stderr, "save:", err)
		os.Exit(1)
	}
	fmt.Println("Saved:", out)
}
