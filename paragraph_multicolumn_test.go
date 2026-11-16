// SPDX-License-Identifier: MIT

package asposepdf_test

import (
	"strings"
	"testing"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

// twoColumnDoc lays out two pages of two columns of 10pt Courier, 12pt apart.
// A paragraph runs from the foot of page 1's left column into its right
// column; another runs from the foot of the right column onto page 2. Page 2's
// left column ends a paragraph with a full stop on a short line, and its right
// column opens a new one. Each page carries a number at its foot, page 2 a
// running head, which the joins step over.
func twoColumnDoc(t *testing.T) *pdf.Document {
	t.Helper()
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	if err := doc.AddBlankPageFromFormat(pdf.PageFormatA4); err != nil {
		t.Fatal(err)
	}
	style := pdf.TextStyle{Font: pdf.FontCourier, Size: 10}
	put := func(page int, x, y float64, lines ...string) {
		p, _ := doc.Page(page)
		for i, l := range lines {
			if err := p.AddTextAt(l, style, x, y-12*float64(i)); err != nil {
				t.Fatal(err)
			}
		}
	}
	full := "words fill this column line to its end"
	put(1, 72, 700, "First paragraph opens the page and", full, "ends here.")
	put(1, 72, 652, "The second paragraph starts below that", full, full, "and is cut off at the foot of a column")
	put(1, 330, 700, "where it goes on in the next one", full, "to its close.")
	put(1, 330, 652, "A third paragraph fills all the rest", full, full, "of the column and then it runs onto the")
	put(2, 72, 700, "next page, where it is finished.", full, "Done.")
	put(2, 330, 700, "Here a new paragraph opens fresh", full, "and closes.")
	// Page numbers at the foot of each page, a running head atop page 2.
	put(1, 290, 40, "1")
	put(2, 290, 40, "2")
	put(2, 72, 800, "Report")
	return doc
}

func markupTexts(ms []pdf.PageMarkup) []string {
	var out []string
	for _, m := range ms {
		for _, s := range m.Sections {
			for _, p := range s.Paragraphs {
				out = append(out, strings.Join(strings.Fields(p.Text), " "))
			}
		}
	}
	return out
}

// TestMulticolumnParagraphs: with MulticolumnParagraphs a paragraph cut off by
// a column foot is joined with its continuation in the next column and on the
// next page, its parts recorded; a paragraph that ends, or one that opens in
// capitals after a finished one, is not joined; toggling re-groups a page's
// markup (pdf-go-6u85).
func TestMulticolumnParagraphs(t *testing.T) {
	doc := twoColumnDoc(t)
	plain, err := doc.Paragraphs()
	if err != nil {
		t.Fatal(err)
	}
	if got := markupTexts(plain); len(got) != 9 {
		t.Fatalf("plain paragraphs %d: %q", len(got), got)
	}
	joined, err := doc.Paragraphs(pdf.ParagraphOptions{MulticolumnParagraphs: true})
	if err != nil {
		t.Fatal(err)
	}
	got := markupTexts(joined)
	if len(got) != 7 {
		t.Fatalf("joined paragraphs %d: %q", len(got), got)
	}
	find := func(prefix string) string {
		for _, g := range got {
			if strings.HasPrefix(g, prefix) {
				return g
			}
		}
		return ""
	}
	if !strings.HasSuffix(find("The second"), "cut off at the foot of a column where it goes on in the next one words fill this column line to its end to its close.") {
		t.Errorf("column continuation not joined: %q", got)
	}
	if !strings.HasSuffix(find("A third"), "it runs onto the next page, where it is finished. words fill this column line to its end Done.") {
		t.Errorf("page continuation not joined: %q", got)
	}
	joinedNew := false
	for _, g := range got {
		joinedNew = joinedNew || (strings.Contains(g, "Here a new paragraph") && !strings.HasPrefix(g, "Here"))
	}
	if joinedNew {
		t.Errorf("a new paragraph was joined: %q", got)
	}
	second := joined[0].Sections[0].Paragraphs[1]
	if len(second.Continuations) != 1 || second.Continuations[0].PageNumber != 1 || second.Continuations[0].Rectangle.LLX < 320 ||
		len(second.Lines) != 7 {
		t.Errorf("continuation %+v, %d lines", second.Continuations, len(second.Lines))
	}
	third := joined[0].Sections[len(joined[0].Sections)-1].Paragraphs[0]
	if len(third.Continuations) != 1 || third.Continuations[0].PageNumber != 2 {
		t.Errorf("page continuation %+v", third.Continuations)
	}
	if err := third.SetText("x"); err == nil {
		t.Error("SetText accepted a paragraph with a continuation")
	}

	// One page's markup, toggled.
	p, _ := doc.Page(1)
	m, err := p.Paragraphs()
	if err != nil {
		t.Fatal(err)
	}
	before := strings.Join(markupTexts([]pdf.PageMarkup{m}), "|")
	m.SetMulticolumnParagraphsAllowed(true)
	if !m.IsMulticolumnParagraphsAllowed() || len(markupTexts([]pdf.PageMarkup{m})) != 4 {
		t.Errorf("page 1 joined: %q", markupTexts([]pdf.PageMarkup{m}))
	}
	direct, err := p.Paragraphs(pdf.ParagraphOptions{MulticolumnParagraphs: true})
	if err != nil || !direct.IsMulticolumnParagraphsAllowed() ||
		strings.Join(markupTexts([]pdf.PageMarkup{direct}), "|") != strings.Join(markupTexts([]pdf.PageMarkup{m}), "|") {
		t.Errorf("Page.Paragraphs with the option: %q", markupTexts([]pdf.PageMarkup{direct}))
	}
	m.SetMulticolumnParagraphsAllowed(false)
	if after := strings.Join(markupTexts([]pdf.PageMarkup{m}), "|"); after != before || m.IsMulticolumnParagraphsAllowed() {
		t.Errorf("toggled back:\n%s\nwant\n%s", after, before)
	}
}
