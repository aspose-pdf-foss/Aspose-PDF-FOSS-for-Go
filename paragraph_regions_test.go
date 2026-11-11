// SPDX-License-Identifier: MIT

package asposepdf_test

import (
	"strings"
	"testing"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

// textLine is one piece of a synthetic page's text and where it starts.
type textLine struct {
	text string
	x, y float64
	bold bool
}

func linesPage(t *testing.T, lines []textLine) *pdf.Page {
	t.Helper()
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatLetter)
	p, _ := doc.Page(1)
	for _, l := range lines {
		style := pdf.TextStyle{Font: pdf.FontHelvetica, Size: 10}
		if l.bold {
			style.Font = pdf.FontHelveticaBold
		}
		mustNoErr(t, p.AddTextAt(l.text, style, l.x, l.y))
	}
	return p
}

func paragraphTexts(t *testing.T, p *pdf.Page) []string {
	t.Helper()
	pm, err := p.Paragraphs()
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, s := range pm.Sections {
		for _, par := range s.Paragraphs {
			out = append(out, strings.Join(strings.Fields(par.Text), " "))
		}
	}
	return out
}

func holding(paras []string, want string) (string, bool) {
	for _, p := range paras {
		if strings.Contains(p, want) {
			return p, true
		}
	}
	return "", false
}

// bodyLines are full-width body lines below y.
func bodyLines(y float64, n int) []textLine {
	var out []textLine
	for i := range n {
		out = append(out, textLine{text: "Body text that runs right across the page from one margin to the other margin.", x: 36, y: y - float64(i)*12})
	}
	return out
}

// TestParagraphsOpeningLineAtMargin: a paragraph whose first line opens at the
// left margin, followed by two short lines there and an indented block below,
// keeps its first line whole — the empty strip left of the indented block is
// no column gutter, as nothing beside the short lines stands next to them
// (pdf-go-dfax).
func TestParagraphsOpeningLineAtMargin(t *testing.T) {
	// The opening line in three runs, a bold one in the middle, so that it is
	// several fragments rather than one.
	var lines []textLine
	x := 36.0
	for _, run := range []textLine{{text: "This "}, {text: "MUTUAL NON-DISCLOSURE AGREEMENT", bold: true}, {text: " is made as of this day by and between the parties"}} {
		font := pdf.FontHelvetica
		if run.bold {
			font = pdf.FontHelveticaBold
		}
		w, err := pdf.MeasureText(run.text, pdf.TextStyle{Font: font, Size: 10})
		mustNoErr(t, err)
		lines = append(lines, textLine{run.text, x, 700, run.bold})
		x += w
	}
	lines = append(lines, textLine{text: "Technologies Ltd.", x: 36, y: 688}, textLine{text: "BETWEEN:", x: 36, y: 664})
	for i := range 7 {
		lines = append(lines, textLine{text: "Indented detail line", x: 200, y: 640 - float64(i)*24})
	}
	paras := paragraphTexts(t, linesPage(t, lines))
	if _, ok := holding(paras, "This MUTUAL NON-DISCLOSURE AGREEMENT is made as of this day by and between the parties"); !ok {
		t.Errorf("first line cut; paragraphs: %q", paras)
	}
}

// TestParagraphsLetterheadBlocks: a letterhead with an address block on the
// left and the sender's block on the right, above full-width text, reads block
// by block (pdf-go-dfax).
func TestParagraphsLetterheadBlocks(t *testing.T) {
	lines := []textLine{
		{text: "Homeowners Insurance Program", x: 340, y: 740},
		{text: "Underwritten By:", x: 340, y: 728},
		{text: "MIDWEST INSURANCE COMPANY", x: 340, y: 716},
		{text: "P.O. Box 5300", x: 36, y: 704}, {text: "Tel: (866) 111-1111", x: 340, y: 704},
		{text: "Binghamton, NY 13902", x: 36, y: 692},
	}
	lines = append(lines, bodyLines(640, 8)...)
	paras := paragraphTexts(t, linesPage(t, lines))
	got, ok := holding(paras, "Underwritten By: MIDWEST INSURANCE COMPANY Tel: (866) 111-1111")
	if !ok {
		t.Fatalf("sender block not one paragraph; paragraphs: %q", paras)
	}
	if strings.Contains(got, "P.O. Box") || strings.Contains(got, "Body text") {
		t.Errorf("sender block mixed with other text: %q", got)
	}
	if _, ok := holding(paras, "P.O. Box 5300 Binghamton, NY 13902"); !ok {
		t.Errorf("address block not one paragraph; paragraphs: %q", paras)
	}
}

// TestParagraphsKeyValueRowsStayRows: a two-column table of labels and values
// above full-width text is not taken for two blocks — it reads row by row.
func TestParagraphsKeyValueRowsStayRows(t *testing.T) {
	var lines []textLine
	for i, kv := range [][2]string{{"Period", "January"}, {"Employee", "000159"}, {"Function", "Machinist"}, {"Salary", "2177,00"}, {"Hours", "165,30"}} {
		y := 740 - float64(i)*12
		lines = append(lines, textLine{text: kv[0], x: 36, y: y}, textLine{text: kv[1], x: 340, y: y})
	}
	lines = append(lines, bodyLines(660, 8)...)
	paras := paragraphTexts(t, linesPage(t, lines))
	if _, ok := holding(paras, "Period January Employee 000159"); !ok {
		t.Errorf("rows not read row by row; paragraphs: %q", paras)
	}
}

// TestParagraphsListLabelsStayWithItems: a list item whose label stands apart
// from its wrapped text is one paragraph, not a column of labels.
func TestParagraphsListLabelsStayWithItems(t *testing.T) {
	var lines []textLine
	for i, label := range []string{"G", "H", "K"} {
		y := 740 - float64(i)*72
		lines = append(lines, textLine{text: label, x: 36, y: y})
		for k, s := range []string{"All costs of repairing, renewing, maintaining and painting", "the land and the common areas of the land and its buildings,", "with the costs of the contractors engaged to perform that", "work and of the plant and equipment hired for it."} {
			lines = append(lines, textLine{text: s, x: 90, y: y - float64(k)*12})
		}
	}
	lines = append(lines, bodyLines(540, 8)...)
	paras := paragraphTexts(t, linesPage(t, lines))
	if _, ok := holding(paras, "H All costs of repairing"); !ok {
		t.Errorf("label parted from its item; paragraphs: %q", paras)
	}
}

// TestParagraphsHeadAndPageNumberApart: a running head at the left and a page
// number at the right, at different heights with nothing across the gap
// between them, stay apart.
func TestParagraphsHeadAndPageNumberApart(t *testing.T) {
	paras := paragraphTexts(t, linesPage(t, []textLine{{text: "Annual Report 2024", x: 36, y: 750}, {text: "Page 8", x: 520, y: 735}}))
	if len(paras) != 2 {
		t.Errorf("paragraphs = %q, want the head and the page number apart", paras)
	}
}

// TestParagraphsAlignedColumnsUnderTitle: two columns whose lines share their
// baselines, under a title running across both, are still two columns — a
// line with text either side of the gutter is what columns look like.
func TestParagraphsAlignedColumnsUnderTitle(t *testing.T) {
	lines := []textLine{{text: "A title that runs right across both of the columns set out below it on this page", x: 60, y: 750}}
	for i := range 8 {
		y := 720 - float64(i)*12
		lines = append(lines, textLine{text: "left column words", x: 36, y: y}, textLine{text: "right column words", x: 330, y: y})
	}
	paras := paragraphTexts(t, linesPage(t, lines))
	if _, ok := holding(paras, strings.Repeat("left column words ", 7)+"left column words"); !ok {
		t.Errorf("columns interleaved; paragraphs: %q", paras)
	}
}
