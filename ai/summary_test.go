// SPDX-License-Identifier: MIT

package ai

import (
	"context"
	"strings"
	"testing"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

// fakeClient records requests and replies from a scripted queue (the last
// reply repeats when the queue runs dry).
type fakeClient struct {
	requests []CompletionRequest
	replies  []string
}

func (f *fakeClient) Complete(_ context.Context, req CompletionRequest) (*CompletionResponse, error) {
	f.requests = append(f.requests, req)
	reply := "summary"
	if len(f.replies) > 0 {
		reply = f.replies[0]
		if len(f.replies) > 1 {
			f.replies = f.replies[1:]
		}
	}
	return &CompletionResponse{Text: reply}, nil
}

// textDoc builds an in-memory PDF with the given page texts.
func textDoc(t *testing.T, pages ...string) *pdf.Document {
	t.Helper()
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	for i, text := range pages {
		if i > 0 {
			if err := doc.AddBlankPageFromFormat(pdf.PageFormatA4); err != nil {
				t.Fatal(err)
			}
		}
		p, err := doc.Page(i + 1)
		if err != nil {
			t.Fatal(err)
		}
		if err := p.AddText(text, pdf.TextStyle{Size: 11}, pdf.Rectangle{LLX: 50, LLY: 50, URX: 545, URY: 792}); err != nil {
			t.Fatal(err)
		}
	}
	return doc
}

func TestSummaryCopilotSingleChunk(t *testing.T) {
	doc := textDoc(t, "The quarterly revenue grew by twelve percent compared to last year.")
	client := &fakeClient{replies: []string{"Revenue grew 12%."}}
	cop := NewSummaryCopilot(client, SummaryOptions{Document: doc, Language: "English", MaxWords: 50, Prompt: "Focus on numbers."})

	got, err := cop.GetSummary(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got != "Revenue grew 12%." {
		t.Errorf("summary = %q", got)
	}
	if len(client.requests) != 1 {
		t.Fatalf("requests = %d; want 1", len(client.requests))
	}
	req := client.requests[0]
	if len(req.Messages) != 2 || req.Messages[0].Role != RoleSystem || req.Messages[1].Role != RoleUser {
		t.Fatalf("messages = %+v", req.Messages)
	}
	sys := req.Messages[0].Text
	for _, want := range []string{"in English", "under 50 words", "Focus on numbers."} {
		if !strings.Contains(sys, want) {
			t.Errorf("system prompt missing %q: %s", want, sys)
		}
	}
	if !strings.Contains(req.Messages[1].Text, "quarterly revenue") {
		t.Errorf("user message missing document text: %q", req.Messages[1].Text)
	}
}

func TestSummaryCopilotMapReduce(t *testing.T) {
	// Three page texts of ~9k runes each with a 24k budget → 2 chunks
	// (9+9 fits, the third starts chunk 2) → 2 map calls + 1 reduce call.
	page := strings.Repeat("word ", 1800) // 9000 runes
	client := &fakeClient{replies: []string{"part one", "part two", "final summary"}}
	cop := NewSummaryCopilot(client, SummaryOptions{Texts: []string{page, page, page}})

	got, err := cop.GetSummary(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got != "final summary" {
		t.Errorf("summary = %q", got)
	}
	if len(client.requests) != 3 {
		t.Fatalf("requests = %d; want 2 map + 1 reduce", len(client.requests))
	}
	if sys := client.requests[0].Messages[0].Text; !strings.Contains(sys, "one part of a longer document") {
		t.Errorf("map call not marked partial: %s", sys)
	}
	reduce := client.requests[2]
	if sys := reduce.Messages[0].Text; strings.Contains(sys, "one part of a longer document") {
		t.Errorf("reduce call marked partial: %s", sys)
	}
	if u := reduce.Messages[1].Text; !strings.Contains(u, "part one") || !strings.Contains(u, "part two") {
		t.Errorf("reduce input = %q", u)
	}
}

func TestSummaryCopilotNoContent(t *testing.T) {
	cop := NewSummaryCopilot(&fakeClient{}, SummaryOptions{})
	if _, err := cop.GetSummary(context.Background()); err == nil {
		t.Error("no content sources must error")
	}
	// A blank (scanned-like) document has no extractable text.
	cop = NewSummaryCopilot(&fakeClient{}, SummaryOptions{Document: pdf.NewDocumentFromFormat(pdf.PageFormatA4)})
	if _, err := cop.GetSummary(context.Background()); err == nil {
		t.Error("document without text must error with an OCR hint")
	}
}

func TestSummaryCopilotDocumentOutput(t *testing.T) {
	doc := textDoc(t, "Input text about apples and oranges.")
	client := &fakeClient{replies: []string{"Apples beat oranges.\n\nSecond paragraph."}}
	cop := NewSummaryCopilot(client, SummaryOptions{Document: doc})

	out, err := cop.GetSummaryDocument(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	pages, err := out.ExtractText()
	if err != nil {
		t.Fatal(err)
	}
	all := strings.Join(pages, "\n")
	for _, want := range []string{"Summary", "Apples beat oranges.", "Second paragraph."} {
		if !strings.Contains(all, want) {
			t.Errorf("summary document missing %q; got %q", want, all)
		}
	}
}
