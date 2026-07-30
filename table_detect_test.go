// SPDX-License-Identifier: MIT

package asposepdf_test

import (
	"strings"
	"testing"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

// The library's own AddTable renderer is a free oracle: generate a ruled
// table with known structure, detect it, assert equality.

func detectOn(t *testing.T, doc *pdf.Document) []*pdf.AbsorbedTable {
	t.Helper()
	p, err := doc.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	ta := pdf.NewTableAbsorber()
	if err := ta.Visit(p); err != nil {
		t.Fatal(err)
	}
	return ta.TableList()
}

func TestTableAbsorberRoundTrip(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	p, _ := doc.Page(1)
	tbl := pdf.NewTable().
		SetColumnWidths([]float64{120, 120, 120}).
		SetBorder(pdf.BorderInfo{Sides: pdf.BorderSideAll, Width: 1}).
		SetDefaultCellBorder(pdf.BorderInfo{Sides: pdf.BorderSideAll, Width: 1})
	tbl.AddRow().AddCells("Item", "Qty", "Price")
	tbl.AddRow().AddCells("Apples", "3", "4.50")
	tbl.AddRow().AddCells("Pears", "7", "9.10")
	if _, err := p.AddTable(tbl, pdf.Rectangle{LLX: 60, LLY: 500, URX: 420, URY: 700}); err != nil {
		t.Fatal(err)
	}

	tables := detectOn(t, doc)
	if len(tables) != 1 {
		t.Fatalf("detected %d tables; want 1", len(tables))
	}
	tab := tables[0]
	rows := tab.RowList()
	if len(rows) != 3 {
		t.Fatalf("rows = %d; want 3", len(rows))
	}
	want := [][]string{
		{"Item", "Qty", "Price"},
		{"Apples", "3", "4.50"},
		{"Pears", "7", "9.10"},
	}
	for r, row := range rows {
		cells := row.CellList()
		if len(cells) != 3 {
			t.Fatalf("row %d cells = %d; want 3", r, len(cells))
		}
		for c, cell := range cells {
			if got := cell.Text(); got != want[r][c] {
				t.Errorf("cell[%d][%d] = %q; want %q", r, c, got, want[r][c])
			}
			if cell.Covered || cell.RowSpan != 1 || cell.ColSpan != 1 {
				t.Errorf("cell[%d][%d] unexpected span/covered: %+v", r, c, cell)
			}
		}
	}
	if tab.PageNumber != 1 {
		t.Errorf("page = %d", tab.PageNumber)
	}
}

func TestTableAbsorberSpans(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	p, _ := doc.Page(1)
	tbl := pdf.NewTable().
		SetColumnWidths([]float64{100, 100, 100}).
		SetBorder(pdf.BorderInfo{Sides: pdf.BorderSideAll, Width: 1}).
		SetDefaultCellBorder(pdf.BorderInfo{Sides: pdf.BorderSideAll, Width: 1})
	r1 := tbl.AddRow()
	r1.AddCell("Merged head").SetColSpan(2)
	r1.AddCell("C")
	r2 := tbl.AddRow()
	r2.AddCell("tall").SetRowSpan(2)
	r2.AddCells("b2", "c2")
	tbl.AddRow().AddCells("b3", "c3")
	if _, err := p.AddTable(tbl, pdf.Rectangle{LLX: 60, LLY: 500, URX: 360, URY: 700}); err != nil {
		t.Fatal(err)
	}

	tables := detectOn(t, doc)
	if len(tables) != 1 {
		t.Fatalf("detected %d tables; want 1", len(tables))
	}
	rows := tables[0].RowList()
	if len(rows) != 3 {
		t.Fatalf("rows = %d; want 3", len(rows))
	}
	head := rows[0].CellList()
	if head[0].ColSpan != 2 || head[0].Text() != "Merged head" {
		t.Errorf("head[0] = span %d text %q", head[0].ColSpan, head[0].Text())
	}
	if !head[1].Covered {
		t.Error("head[1] must be covered by the colspan")
	}
	if head[2].Text() != "C" {
		t.Errorf("head[2] = %q", head[2].Text())
	}
	mid := rows[1].CellList()
	if mid[0].RowSpan != 2 || mid[0].Text() != "tall" {
		t.Errorf("mid[0] = rowspan %d text %q", mid[0].RowSpan, mid[0].Text())
	}
	bot := rows[2].CellList()
	if !bot[0].Covered {
		t.Error("bottom[0] must be covered by the rowspan")
	}
	if bot[1].Text() != "b3" || bot[2].Text() != "c3" {
		t.Errorf("bottom = %q %q", bot[1].Text(), bot[2].Text())
	}
}

func TestTableAbsorberPlainPageNoTables(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	p, _ := doc.Page(1)
	if err := p.AddText("Just a paragraph of prose with no table anywhere near it.",
		pdf.TextStyle{Size: 12}, pdf.Rectangle{LLX: 60, LLY: 600, URX: 535, URY: 760}); err != nil {
		t.Fatal(err)
	}
	// A single underline rule must not become a table.
	if err := p.DrawLine(pdf.Point{X: 60, Y: 590}, pdf.Point{X: 300, Y: 590},
		pdf.LineStyle{Color: &pdf.Color{A: 1}, Width: 1}); err != nil {
		t.Fatal(err)
	}
	if tables := detectOn(t, doc); len(tables) != 0 {
		t.Fatalf("detected %d tables on a prose page", len(tables))
	}
}

func TestTableAbsorberRealFile(t *testing.T) {
	doc, err := pdf.Open(testFile(t))
	if err != nil {
		t.Fatal(err)
	}
	p, err := doc.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	ta := pdf.NewTableAbsorber()
	if err := ta.Visit(p); err != nil {
		t.Fatal(err)
	}
	tables := ta.TableList()
	if len(tables) == 0 {
		t.Fatal("no table detected in PdfWithTable.pdf")
	}
	tab := tables[0]
	if len(tab.RowList()) < 3 {
		t.Errorf("rows = %d; want several", len(tab.RowList()))
	}
	var all strings.Builder
	for _, row := range tab.RowList() {
		for _, cell := range row.CellList() {
			all.WriteString(cell.Text())
			all.WriteString(" | ")
		}
	}
	if strings.TrimSpace(strings.ReplaceAll(all.String(), "|", "")) == "" {
		t.Error("detected table carries no text")
	}
	t.Logf("PdfWithTable: %d tables, first %dx%d", len(tables),
		len(tab.RowList()), len(tab.RowList()[0].CellList()))
}
