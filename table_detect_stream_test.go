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
