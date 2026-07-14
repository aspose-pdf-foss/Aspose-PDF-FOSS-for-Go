// SPDX-License-Identifier: MIT

package ai

import (
	"context"
	"errors"
	"fmt"
	"strings"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

// chunkRunes is the map-reduce chunk budget: pages are concatenated into
// chunks of at most this many runes (≈6–8k tokens — conservative enough for
// small-context local models). One chunk = one summarization call; multiple
// chunks are summarized individually and then reduced.
const chunkRunes = 24000

// SummaryOptions configures a SummaryCopilot. Exactly one content source is
// required: Document, Documents, or Texts (they may also be combined).
type SummaryOptions struct {
	Document  *pdf.Document   // single document to summarize
	Documents []*pdf.Document // multiple documents (mirrors Aspose's DocumentCollection)
	Texts     []string        // raw text inputs (mirrors Aspose's TextDocument)
	Language  string          // summary language; "" = same language as the document
	MaxWords  int             // approximate length cap; 0 = model's choice
	Prompt    string          // extra instruction appended to the system prompt
	// Markdown asks the model to format the summary as Markdown (headings,
	// lists, bold) and renders GetSummaryDocument/SaveSummary through the
	// Markdown→PDF pipeline, producing a formatted document instead of
	// plain paragraphs. GetSummary returns the raw Markdown text.
	Markdown bool
}

// SummaryCopilot produces document summaries with an AI model. Mirrors
// Aspose.PDF for .NET's OpenAISummaryCopilot (GetSummaryAsync /
// GetSummaryDocumentAsync / SaveSummaryAsync).
//
// Privacy: the documents' extracted text is sent to the configured AI
// endpoint.
type SummaryCopilot struct {
	client AIClient
	opts   SummaryOptions
}

// NewSummaryCopilot returns a copilot summarizing the content in opts via
// client.
func NewSummaryCopilot(client AIClient, opts SummaryOptions) *SummaryCopilot {
	return &SummaryCopilot{client: client, opts: opts}
}

// GetSummary extracts the documents' text and returns the model's summary.
func (c *SummaryCopilot) GetSummary(ctx context.Context) (string, error) {
	if c.client == nil {
		return "", errors.New("ai: SummaryCopilot has no client")
	}
	texts, err := c.collectTexts()
	if err != nil {
		return "", err
	}
	chunks := chunkTexts(texts, chunkRunes)
	if len(chunks) == 0 {
		return "", errors.New("ai: no text to summarize (scanned documents need OCR first — see OcrCopilot.MakeSearchable)")
	}

	if len(chunks) == 1 {
		return c.summarizeOnce(ctx, chunks[0], false)
	}
	// Map: summarize each chunk; reduce: summarize the summaries.
	partials := make([]string, 0, len(chunks))
	for i, chunk := range chunks {
		s, err := c.summarizeOnce(ctx, chunk, true)
		if err != nil {
			return "", fmt.Errorf("ai: summarize part %d/%d: %w", i+1, len(chunks), err)
		}
		partials = append(partials, s)
	}
	return c.summarizeOnce(ctx, strings.Join(partials, "\n\n"), false)
}

// GetSummaryDocument returns the summary rendered as a new PDF document
// (A4 by default, or the given page format).
func (c *SummaryCopilot) GetSummaryDocument(ctx context.Context, format ...pdf.PageFormat) (*pdf.Document, error) {
	summary, err := c.GetSummary(ctx)
	if err != nil {
		return nil, err
	}
	pf := pdf.PageFormatA4
	if len(format) > 0 {
		pf = format[0]
	}
	if c.opts.Markdown {
		md := "# " + c.summaryTitle() + "\n\n" + summary
		return pdf.MarkdownToDocumentFromStream(strings.NewReader(md), pdf.MarkdownOptions{Format: pf})
	}
	doc := pdf.NewDocumentFromFormat(pf)
	flow := doc.NewFlow(pdf.FlowOptions{Format: pf})
	flow.AddHeading(1, c.summaryTitle(), pdf.TextStyle{})
	for _, para := range strings.Split(summary, "\n\n") {
		para = strings.TrimSpace(para)
		if para == "" {
			continue
		}
		flow.AddParagraph(para, pdf.TextStyle{})
	}
	if _, err := flow.Render(); err != nil {
		return nil, fmt.Errorf("ai: render summary document: %w", err)
	}
	return doc, nil
}

// SaveSummary writes the summary as a PDF file at path.
func (c *SummaryCopilot) SaveSummary(ctx context.Context, path string) error {
	doc, err := c.GetSummaryDocument(ctx)
	if err != nil {
		return err
	}
	return doc.Save(path)
}

// summaryTitle derives the heading for GetSummaryDocument from the source
// document's Info title when available.
func (c *SummaryCopilot) summaryTitle() string {
	if c.opts.Document != nil {
		if info, err := c.opts.Document.Info(); err == nil && strings.TrimSpace(info.Title) != "" {
			return "Summary: " + strings.TrimSpace(info.Title)
		}
	}
	return "Summary"
}

// collectTexts gathers per-page text from every configured source. Pages with
// no extractable text are skipped (scanned pages — MakeSearchable first).
func (c *SummaryCopilot) collectTexts() ([]string, error) {
	docs := c.opts.Documents
	if c.opts.Document != nil {
		docs = append([]*pdf.Document{c.opts.Document}, docs...)
	}
	if len(docs) == 0 && len(c.opts.Texts) == 0 {
		return nil, errors.New("ai: SummaryOptions needs Document, Documents or Texts")
	}
	var texts []string
	for i, d := range docs {
		if d == nil {
			continue
		}
		pages, err := d.ExtractText()
		if err != nil {
			return nil, fmt.Errorf("ai: extract text from document %d: %w", i+1, err)
		}
		for _, pageText := range pages {
			if strings.TrimSpace(pageText) == "" {
				continue
			}
			texts = append(texts, pageText)
		}
	}
	for _, t := range c.opts.Texts {
		if strings.TrimSpace(t) == "" {
			continue
		}
		texts = append(texts, t)
	}
	return texts, nil
}

// chunkTexts concatenates page texts into chunks of at most maxRunes runes,
// never splitting a page across chunks (an oversized single page becomes its
// own chunk).
func chunkTexts(texts []string, maxRunes int) []string {
	var chunks []string
	var cur strings.Builder
	curRunes := 0
	for _, t := range texts {
		n := len([]rune(t))
		if curRunes > 0 && curRunes+n > maxRunes {
			chunks = append(chunks, cur.String())
			cur.Reset()
			curRunes = 0
		}
		if curRunes > 0 {
			cur.WriteString("\n\n")
		}
		cur.WriteString(t)
		curRunes += n
	}
	if curRunes > 0 {
		chunks = append(chunks, cur.String())
	}
	return chunks
}

// summarizeOnce performs one summarization call. partial=true marks a map
// step of the map-reduce pipeline (an intermediate, more detailed summary).
func (c *SummaryCopilot) summarizeOnce(ctx context.Context, text string, partial bool) (string, error) {
	var sys strings.Builder
	sys.WriteString("You are a document summarization assistant. Respond with only the summary — no preamble, no commentary.")
	if partial {
		sys.WriteString(" This is one part of a longer document; write a detailed partial summary that preserves key facts, names and figures, so the parts can be combined later.")
	}
	if c.opts.Language != "" {
		fmt.Fprintf(&sys, " Write the summary in %s.", c.opts.Language)
	} else {
		sys.WriteString(" Write the summary in the same language as the document.")
	}
	if !partial && c.opts.MaxWords > 0 {
		fmt.Fprintf(&sys, " Keep the summary under %d words.", c.opts.MaxWords)
	}
	if !partial && c.opts.Markdown {
		sys.WriteString(" Format the summary as Markdown: use headings, bullet lists, and bold for key facts and figures. Do not wrap the output in a code fence.")
	}
	if c.opts.Prompt != "" {
		sys.WriteString(" ")
		sys.WriteString(c.opts.Prompt)
	}

	resp, err := c.client.Complete(ctx, CompletionRequest{
		Messages: []Message{
			{Role: RoleSystem, Text: sys.String()},
			{Role: RoleUser, Text: text},
		},
	})
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(resp.Text), nil
}
