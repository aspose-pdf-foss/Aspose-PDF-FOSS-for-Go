// SPDX-License-Identifier: MIT

package asposepdf_test

import (
	"strings"
	"testing"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

func paraCount(pm pdf.PageMarkup) int {
	n := 0
	for _, s := range pm.Sections {
		n += len(s.Paragraphs)
	}
	return n
}

// TestParagraphsSingleColumn: two blocks separated by a vertical gap become one
// section with two paragraphs.
func TestParagraphsSingleColumn(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	p, _ := doc.Page(1)
	body := pdf.TextStyle{Font: pdf.FontHelvetica, Size: 11, LineSpacing: 1.3}
	a := strings.Repeat("Alpha paragraph text that wraps across several lines in the box. ", 4)
	b := strings.Repeat("Beta paragraph text that also wraps across several lines here. ", 4)
	mustNoErr(t, p.AddText(a, body, pdf.Rectangle{LLX: 60, LLY: 660, URX: 540, URY: 770}))
	mustNoErr(t, p.AddText(b, body, pdf.Rectangle{LLX: 60, LLY: 480, URX: 540, URY: 590}))

	pm, err := p.Paragraphs()
	if err != nil {
		t.Fatal(err)
	}
	if len(pm.Sections) != 1 {
		t.Fatalf("sections = %d, want 1", len(pm.Sections))
	}
	if got := paraCount(pm); got != 2 {
		t.Errorf("paragraphs = %d, want 2", got)
	}
	paras := pm.Sections[0].Paragraphs
	if !strings.Contains(paras[0].Text, "Alpha") {
		t.Errorf("first paragraph = %q, want it to start with Alpha", paras[0].Text)
	}
	if !strings.Contains(paras[1].Text, "Beta") {
		t.Errorf("second paragraph = %q, want it to contain Beta", paras[1].Text)
	}
}

// TestParagraphsTwoColumns: side-by-side blocks become two sections ordered
// left-to-right, each carrying its own column's text.
func TestParagraphsTwoColumns(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	p, _ := doc.Page(1)
	body := pdf.TextStyle{Font: pdf.FontHelvetica, Size: 10, LineSpacing: 1.3}
	left := strings.Repeat("LEFTCOL word here and there filling the left column nicely. ", 5)
	right := strings.Repeat("RIGHTCOL word here and there filling the right column nicely. ", 5)
	mustNoErr(t, p.AddText(left, body, pdf.Rectangle{LLX: 50, LLY: 400, URX: 280, URY: 760}))
	mustNoErr(t, p.AddText(right, body, pdf.Rectangle{LLX: 320, LLY: 400, URX: 550, URY: 760}))

	pm, err := p.Paragraphs()
	if err != nil {
		t.Fatal(err)
	}
	if len(pm.Sections) != 2 {
		t.Fatalf("sections = %d, want 2 columns", len(pm.Sections))
	}
	if !strings.Contains(sectionText(pm.Sections[0]), "LEFTCOL") {
		t.Errorf("left section should contain LEFTCOL: %q", sectionText(pm.Sections[0]))
	}
	if !strings.Contains(sectionText(pm.Sections[1]), "RIGHTCOL") {
		t.Errorf("right section should contain RIGHTCOL: %q", sectionText(pm.Sections[1]))
	}
	// Sections are ordered left-to-right.
	if pm.Sections[0].Rectangle.LLX >= pm.Sections[1].Rectangle.LLX {
		t.Error("sections not ordered left-to-right")
	}
}

func sectionText(s pdf.MarkupSection) string {
	var b strings.Builder
	for _, p := range s.Paragraphs {
		b.WriteString(p.Text)
		b.WriteByte(' ')
	}
	return b.String()
}

// TestParagraphsEmptyPage: a blank page has no sections.
func TestParagraphsEmptyPage(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	p, _ := doc.Page(1)
	pm, err := p.Paragraphs()
	if err != nil {
		t.Fatal(err)
	}
	if len(pm.Sections) != 0 {
		t.Errorf("blank page has %d sections, want 0", len(pm.Sections))
	}
	if pm.PageNumber != 1 {
		t.Errorf("page number = %d, want 1", pm.PageNumber)
	}
}

// TestParagraphsRealFile: a real PDF yields non-empty paragraphs with text and
// bounding boxes.
func TestParagraphsRealFile(t *testing.T) {
	doc, err := pdf.Open(testFile(t))
	if err != nil {
		t.Fatal(err)
	}
	pages, err := doc.Paragraphs()
	if err != nil {
		t.Fatal(err)
	}
	total := 0
	for _, pm := range pages {
		for _, s := range pm.Sections {
			for _, par := range s.Paragraphs {
				if strings.TrimSpace(par.Text) != "" {
					total++
					if par.Rectangle.URX <= par.Rectangle.LLX {
						t.Errorf("paragraph has a degenerate rectangle: %+v", par.Rectangle)
					}
				}
			}
		}
	}
	if total == 0 {
		t.Error("expected at least one non-empty paragraph in the document")
	}
}
