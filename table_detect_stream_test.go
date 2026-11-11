// SPDX-License-Identifier: MIT

package asposepdf_test

import (
	"fmt"
	"strings"
	"testing"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

// addTextAt draws a single-line string at (x, y).
func addTextAt(t *testing.T, p *pdf.Page, text string, x, y, size float64) {
	t.Helper()
	style := pdf.TextStyle{Font: pdf.FontHelvetica, Size: size}
	rect := pdf.Rectangle{LLX: x, LLY: y, URX: x + 300, URY: y + size*1.4}
	if err := p.AddText(text, style, rect); err != nil {
		t.Fatal(err)
	}
}

// A borderless 5x3 layout with aligned columns must be recognized by the
// stream pass (no rules anywhere on the page).
func TestStreamTableDetected(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	p, _ := doc.Page(1)
	names := []string{"Alpha", "Beta", "Gamma", "Delta", "Epsilon"}
	for i, name := range names {
		y := 700 - float64(i)*16
		addTextAt(t, p, name, 60, y, 10)
		addTextAt(t, p, fmt.Sprintf("%d", 10+i), 200, y, 10)
		addTextAt(t, p, fmt.Sprintf("%d.50", 20+i), 300, y, 10)
	}

	ta := pdf.NewTableAbsorber()
	if err := ta.Visit(p); err != nil {
		t.Fatal(err)
	}
	tables := ta.TableList()
	if len(tables) != 1 {
		t.Fatalf("tables = %d; want 1", len(tables))
	}
	rows := tables[0].RowList()
	if len(rows) != 5 {
		t.Fatalf("rows = %d; want 5", len(rows))
	}
	if cols := len(rows[0].CellList()); cols != 3 {
		t.Fatalf("cols = %d; want 3", cols)
	}
	if got := rows[0].CellList()[0].Text(); got != "Alpha" {
		t.Errorf("cell(0,0) = %q; want Alpha", got)
	}
	if got := rows[4].CellList()[2].Text(); got != "24.50" {
		t.Errorf("cell(4,2) = %q; want 24.50", got)
	}
}

// Flowing prose paragraphs must NOT be detected as a table even when word
// gaps occasionally align.
func TestStreamTableRejectsProse(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	p, _ := doc.Page(1)
	prose := strings.Repeat("The quick brown fox jumps over the lazy dog and keeps running through the calm forest. ", 8)
	style := pdf.TextStyle{Font: pdf.FontHelvetica, Size: 11, LineSpacing: 1.4}
	if err := p.AddText(prose, style, pdf.Rectangle{LLX: 60, LLY: 400, URX: 530, URY: 760}); err != nil {
		t.Fatal(err)
	}

	ta := pdf.NewTableAbsorber()
	if err := ta.Visit(p); err != nil {
		t.Fatal(err)
	}
	if n := len(ta.TableList()); n != 0 {
		t.Fatalf("tables = %d on a prose page; want 0", n)
	}
}

// A table of contents (title .... page) must be rejected by the dot-leader
// guard even though its lines are two aligned "columns".
func TestStreamTableRejectsTOC(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	p, _ := doc.Page(1)
	for i := 0; i < 8; i++ {
		y := 700 - float64(i)*18
		addTextAt(t, p, fmt.Sprintf("Chapter %d %s", i+1, strings.Repeat(".", 40)), 60, y, 11)
		addTextAt(t, p, fmt.Sprintf("%d", 3+i*7), 500, y, 11)
	}

	ta := pdf.NewTableAbsorber()
	if err := ta.Visit(p); err != nil {
		t.Fatal(err)
	}
	if n := len(ta.TableList()); n != 0 {
		t.Fatalf("tables = %d on a TOC page; want 0", n)
	}
}

// wrappedHeaderDoc lays out a borderless table whose first header cell wraps
// ("Hea" / "der") while the others sit on the second line; empty cells hold a
// space, as Word writes them. first and firstFont replace "Hea" (a group label
// above the rows); second, the second line's cells ("" for a blank one).
func wrappedHeaderDoc(t *testing.T, first string, firstFont pdf.Font, second ...string) *pdf.Page {
	t.Helper()
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	p, _ := doc.Page(1)
	cols := []float64{60, 180, 300, 420}
	at := func(s string, f pdf.Font, x, y float64) {
		if err := p.AddTextAt(s, pdf.TextStyle{Font: f, Size: 10}, x, y); err != nil {
			t.Fatal(err)
		}
	}
	y := 700.0
	at(first, firstFont, cols[0], y)
	for _, x := range cols[1:] {
		at(" ", pdf.FontHelvetica, x, y)
	}
	y -= 12
	if len(second) == 0 {
		second = []string{"der", "Header 1", "Header 2", "Header 3"}
	}
	for i, s := range second {
		if s == "" {
			s = " "
		}
		at(s, pdf.FontHelvetica, cols[i], y)
	}
	for r := range 5 {
		y -= 14
		for i, s := range []string{"A", "B", "C", "D"} {
			at(fmt.Sprintf("%s%d", s, r), pdf.FontHelvetica, cols[i], y)
		}
	}
	return p
}

func streamRows(t *testing.T, p *pdf.Page) []*pdf.AbsorbedRow {
	t.Helper()
	ta := pdf.NewTableAbsorber()
	if err := ta.Visit(p); err != nil {
		t.Fatal(err)
	}
	if len(ta.TableList()) != 1 {
		t.Fatalf("tables = %d; want 1", len(ta.TableList()))
	}
	return ta.TableList()[0].RowList()
}

// TestStreamTableWrappedHeaderIsOneRow: a header cell wrapped onto two lines
// makes one header row, not two (pdf-go-2yez).
func TestStreamTableWrappedHeaderIsOneRow(t *testing.T) {
	rows := streamRows(t, wrappedHeaderDoc(t, "Hea", pdf.FontHelvetica))
	if len(rows) != 6 {
		t.Fatalf("rows = %d; want 6 (one header + 5)", len(rows))
	}
	if got := strings.Fields(rows[0].CellList()[0].Text()); strings.Join(got, " ") != "Hea der" {
		t.Errorf("header cell = %q; want the two lines", rows[0].CellList()[0].Text())
	}
	if got := rows[0].CellList()[1].Text(); got != "Header 1" {
		t.Errorf("cell(0,1) = %q", got)
	}
}

// TestStreamTableGroupLabelKeepsItsRow: a label above the rows set in another
// style stays a row of its own.
func TestStreamTableGroupLabelKeepsItsRow(t *testing.T) {
	rows := streamRows(t, wrappedHeaderDoc(t, "Fruits", pdf.FontHelveticaBold))
	if len(rows) != 7 {
		t.Fatalf("rows = %d; want 7", len(rows))
	}
}

// TestStreamTableTwoPartialLinesStaySeparate: the first line joins the second
// only when the second fills every column.
func TestStreamTableTwoPartialLinesStaySeparate(t *testing.T) {
	rows := streamRows(t, wrappedHeaderDoc(t, "Hea", pdf.FontHelvetica, "der", "Header 1", "", "Header 3"))
	if len(rows) != 7 {
		t.Fatalf("rows = %d; want 7", len(rows))
	}
}
