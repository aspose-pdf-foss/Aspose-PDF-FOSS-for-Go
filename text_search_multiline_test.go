// SPDX-License-Identifier: MIT

package asposepdf_test

import (
	"strings"
	"testing"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

// twoLinePage draws "The quick brown" and "fox jumps over" as two lines.
func twoLinePage(t *testing.T, doc *pdf.Document, n int, first, second string) *pdf.Page {
	t.Helper()
	p, _ := doc.Page(n)
	style := pdf.TextStyle{Font: pdf.FontHelvetica, Size: 12}
	if err := p.AddTextAt(first, style, 72, 700); err != nil {
		t.Fatal(err)
	}
	if err := p.AddTextAt(second, style, 72, 684); err != nil {
		t.Fatal(err)
	}
	return p
}

// TestSearchTextAcrossLines: a regular expression matches across a line
// break, with a segment per line and the page rectangle covering both; a
// literal query stays within a line; a match within a line is unchanged
// (pdf-go-1kcx).
func TestSearchTextAcrossLines(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatLetter)
	p := twoLinePage(t, doc, 1, "The quick brown", "fox jumps over")

	ms, err := p.SearchText(`quick\s+brown\s+fox`, pdf.SearchOptions{Regex: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(ms) != 1 {
		t.Fatalf("%d matches, want 1", len(ms))
	}
	m := ms[0]
	if m.Text != "quick brown\nfox" {
		t.Errorf("text %q", m.Text)
	}
	if len(m.Segments) != 2 || m.Segments[0].Text != "quick brown" || m.Segments[1].Text != "fox" {
		t.Fatalf("segments %+v", m.Segments)
	}
	if m.Rect.URY < m.Segments[0].Rect.URY || m.Rect.LLY > m.Segments[1].Rect.LLY {
		t.Errorf("rect %+v does not cover both lines %+v", m.Rect, m.Segments)
	}

	if ms, _ := p.SearchText("brown fox"); len(ms) != 0 {
		t.Errorf("literal query matched across lines: %+v", ms)
	}

	lit, _ := p.SearchText("jumps")
	rx, _ := p.SearchText(`jum\w+`, pdf.SearchOptions{Regex: true})
	if len(lit) != 1 || len(rx) != 1 || lit[0].Rect != rx[0].Rect || len(rx[0].Segments) != 1 {
		t.Errorf("within a line: literal %+v, regex %+v", lit, rx)
	}

	// A rectangle around the second line keeps the match through its segment.
	r := m.Segments[1].Rect
	if ms, _ := p.SearchText(`quick\s+brown\s+fox`, pdf.SearchOptions{Regex: true, Rectangle: &r}); len(ms) != 1 {
		t.Errorf("rectangle on the second line: %d matches, want 1", len(ms))
	}
}

// TestSearchTextAcrossPages: on a document, a regular expression matches
// from the last line of one page into the first line of the next.
func TestSearchTextAcrossPages(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatLetter)
	if err := doc.AddBlankPageFromFormat(pdf.PageFormatLetter); err != nil {
		t.Fatal(err)
	}
	twoLinePage(t, doc, 1, "Intro line", "This sentence starts here")
	twoLinePage(t, doc, 2, "and ends here.", "Next paragraph")
	ms, err := doc.SearchText(`starts\s+here\s+and\s+ends`, pdf.SearchOptions{Regex: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(ms) != 1 || ms[0].PageNumber != 1 || len(ms[0].Segments) != 2 || ms[0].Segments[1].PageNumber != 2 {
		t.Fatalf("matches %+v", ms)
	}
	// A rectangle where only the page-2 segment lies keeps the match.
	r := ms[0].Segments[1].Rect
	if ms, _ := doc.SearchText(`starts\s+here\s+and\s+ends`, pdf.SearchOptions{Regex: true, Rectangle: &r}); len(ms) != 1 {
		t.Errorf("rectangle on the page-2 segment: %d matches, want 1", len(ms))
	}
}

// TestReplaceTextAcrossLines: a regular expression replaced across a line
// break removes the text on both lines and draws the replacement on the
// first.
func TestReplaceTextAcrossLines(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatLetter)
	p := twoLinePage(t, doc, 1, "The quick brown", "fox jumps over")
	n, err := p.ReplaceText(`quick\s+brown\s+fox`, "slow cat", pdf.ReplaceOptions{Regex: true})
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("%d replacements, want 1", n)
	}
	s, _ := p.ExtractText()
	if strings.Contains(s, "quick") || strings.Contains(s, "fox") {
		t.Errorf("old text left: %q", s)
	}
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) != 2 || !strings.Contains(lines[0], "slow cat") || strings.TrimSpace(lines[1]) != "jumps over" {
		t.Errorf("text after replacement %q", s)
	}
}
