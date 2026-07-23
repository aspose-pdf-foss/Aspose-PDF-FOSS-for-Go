// SPDX-License-Identifier: MIT

package ai_test

import (
	"context"
	"fmt"
	"log"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
	"github.com/aspose-pdf-foss/aspose-pdf-foss-for-go/ai"
)

// Wire the copilots to any OpenAI-compatible endpoint — OpenAI, LiteLLM,
// Ollama, OpenRouter — and summarize a document, then make a scanned PDF
// searchable. Requires a live endpoint, so this example is not executed by
// go test; it demonstrates the intended wiring.
func Example() {
	client := ai.NewOpenAIClient(ai.OpenAIClientOptions{
		BaseURL: "http://localhost:11434/v1", // e.g. a local Ollama
		Model:   "llama3.2-vision",
	})
	ctx := context.Background()

	// Summarize a document (optionally as formatted Markdown).
	doc, err := pdf.Open("report.pdf")
	if err != nil {
		log.Fatal(err)
	}
	summary := ai.NewSummaryCopilot(client, ai.SummaryOptions{Document: doc, Markdown: true})
	text, err := summary.GetSummary(ctx)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(text)

	// Turn a scanned PDF into a selectable, searchable one.
	scan, err := pdf.Open("scanned.pdf")
	if err != nil {
		log.Fatal(err)
	}
	engine := ai.NewLLMOCREngine(client, ai.LLMOCROptions{})
	ocr := ai.NewOcrCopilot(engine, ai.OcrOptions{Document: scan})
	pages, err := ocr.MakeSearchable(ctx)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("OCRed pages:", pages)
	_ = scan.Save("searchable.pdf")
}
