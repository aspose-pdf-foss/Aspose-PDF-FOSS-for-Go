// SPDX-License-Identifier: MIT

package asposepdf_test

import (
	"fmt"
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

// drawGrid rules a rows×cols grid of 100×20pt cells with its top-left at
// (x, top) and writes a label in each cell.
func drawGrid(t *testing.T, p *pdf.Page, x, top float64, rows, cols int, label string) {
	t.Helper()
	line := pdf.LineStyle{Color: &pdf.Color{A: 1}, Width: 1}
	for r := 0; r <= rows; r++ {
		y := top - float64(r)*20
		if err := p.DrawLine(pdf.Point{X: x, Y: y}, pdf.Point{X: x + float64(cols)*100, Y: y}, line); err != nil {
			t.Fatal(err)
		}
	}
	for c := 0; c <= cols; c++ {
		xx := x + float64(c)*100
		if err := p.DrawLine(pdf.Point{X: xx, Y: top}, pdf.Point{X: xx, Y: top - float64(rows)*20}, line); err != nil {
			t.Fatal(err)
		}
	}
	for r := 0; r < rows; r++ {
		for c := 0; c < cols; c++ {
			y := top - float64(r+1)*20
			if err := p.AddText(fmt.Sprintf("%s%d%d", label, r, c), pdf.TextStyle{Size: 9},
				pdf.Rectangle{LLX: x + float64(c)*100 + 3, LLY: y + 3, URX: x + float64(c+1)*100 - 3, URY: y + 17}); err != nil {
				t.Fatal(err)
			}
		}
	}
}

// TestTableAbsorberSplitsTablesAFewPointsApart: two ruled tables drawn 4pt
// apart are two tables, not one with an empty row between them; the same for
// tables side by side (pdf-go-t81f).
func TestTableAbsorberSplitsTablesAFewPointsApart(t *testing.T) {
	for _, tc := range []struct {
		name         string
		x2, top2     float64
		rows1, rows2 int
	}{
		{"stacked", 60, 700 - 3*20 - 4, 3, 4},
		{"side by side", 60 + 2*100 + 4, 700, 3, 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
			p, _ := doc.Page(1)
			drawGrid(t, p, 60, 700, tc.rows1, 2, "a")
			drawGrid(t, p, tc.x2, tc.top2, tc.rows2, 2, "b")
			tables := detectOn(t, doc)
			if len(tables) != 2 {
				t.Fatalf("detected %d tables; want 2", len(tables))
			}
			for i, want := range []struct {
				rows  int
				first string
			}{{tc.rows1, "a00"}, {tc.rows2, "b00"}} {
				rows := tables[i].RowList() // top to bottom, then left to right
				if len(rows) != want.rows || len(rows[0].CellList()) != 2 {
					t.Errorf("table %d: %d rows × %d; want %d × 2", i, len(rows), len(rows[0].CellList()), want.rows)
					continue
				}
				if got := rows[0].CellList()[0].Text(); got != want.first {
					t.Errorf("table %d first cell %q; want %q", i, got, want.first)
				}
			}
		})
	}
}

// rotatedGrid rules a grid of 3 page columns × 2 page rows (60×100pt cells)
// and writes each logical cell's name rotated by rot degrees, as a table set
// sideways on the page. name gives the logical cell a page cell holds.
func rotatedGrid(t *testing.T, rot float64, name func(pc, pr int) string, lines int) *pdf.Document {
	t.Helper()
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	p, _ := doc.Page(1)
	line := pdf.LineStyle{Color: &pdf.Color{A: 1}, Width: 1}
	x0, y0 := 100.0, 300.0 // bottom-left
	for c := 0; c <= 3; c++ {
		x := x0 + float64(c)*60
		if err := p.DrawLine(pdf.Point{X: x, Y: y0}, pdf.Point{X: x, Y: y0 + 300}, line); err != nil {
			t.Fatal(err)
		}
	}
	for r := 0; r <= 2; r++ {
		y := y0 + float64(r)*150
		if err := p.DrawLine(pdf.Point{X: x0, Y: y}, pdf.Point{X: x0 + 180, Y: y}, line); err != nil {
			t.Fatal(err)
		}
	}
	for pc := range 3 {
		for pr := range 2 { // pr 0 = bottom
			for l := range lines {
				// Rotated 90, the baseline runs up and lines step right; at
				// 270 it runs down and lines step left.
				x, y := x0+float64(pc)*60+20+float64(l)*14, y0+float64(pr)*150+10
				if rot == 270 {
					x, y = x0+float64(pc+1)*60-20-float64(l)*14, y0+float64(pr+1)*150-10
				}
				s := name(pc, pr) + " a longer cell text" // far longer than the cell is wide
				if lines > 1 {
					s = fmt.Sprintf("%s.%d", s, l)
				}
				if err := p.AddTextAt(s, pdf.TextStyle{Size: 10, Rotation: rot}, x, y); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
	return doc
}

// TestTableAbsorberRotatedText: a table whose text runs at 90 or 270 degrees
// is reported in the orientation the text reads in (pdf-go-c19b).
func TestTableAbsorberRotatedText(t *testing.T) {
	for _, tc := range []struct {
		rot  float64
		name func(pc, pr int) string
	}{
		// At 90 the leftmost page column is the first row and columns run up.
		{90, func(pc, pr int) string { return fmt.Sprintf("r%dc%d", pc, pr) }},
		// At 270 the rightmost page column is the first row and columns run down.
		{270, func(pc, pr int) string { return fmt.Sprintf("r%dc%d", 2-pc, 1-pr) }},
	} {
		t.Run(fmt.Sprint(tc.rot), func(t *testing.T) {
			tables := detectOn(t, rotatedGrid(t, tc.rot, tc.name, 1))
			if len(tables) != 1 {
				t.Fatalf("detected %d tables; want 1", len(tables))
			}
			rows := tables[0].RowList()
			if len(rows) != 3 || len(rows[0].CellList()) != 2 {
				t.Fatalf("%d rows × %d; want 3 × 2", len(rows), len(rows[0].CellList()))
			}
			for i, row := range rows {
				for j, c := range row.CellList() {
					if want := fmt.Sprintf("r%dc%d a longer cell text", i, j); c.Text() != want {
						t.Errorf("cell[%d][%d] = %q; want %q", i, j, c.Text(), want)
					}
				}
			}
		})
	}
}

// TestTableAbsorberRotatedCellLines: the lines of a rotated cell read in order,
// one per line.
func TestTableAbsorberRotatedCellLines(t *testing.T) {
	tables := detectOn(t, rotatedGrid(t, 90, func(pc, pr int) string { return fmt.Sprintf("r%dc%d", pc, pr) }, 2))
	if len(tables) != 1 {
		t.Fatalf("detected %d tables; want 1", len(tables))
	}
	if got := tables[0].RowList()[1].CellList()[0].Text(); got != "r1c0 a longer cell text.0\nr1c0 a longer cell text.1" {
		t.Errorf("cell[1][0] = %q; want two lines", got)
	}
}

// TestTableAbsorberOpenEdge: a grid whose last row has no closing rule — the
// column rules run on to a common end — keeps that row (pdf-go-c19b).
func TestTableAbsorberOpenEdge(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	p, _ := doc.Page(1)
	line := pdf.LineStyle{Color: &pdf.Color{A: 1}, Width: 1}
	for c := 0; c <= 3; c++ {
		x := 100 + float64(c)*100
		if err := p.DrawLine(pdf.Point{X: x, Y: 500}, pdf.Point{X: x, Y: 560}, line); err != nil {
			t.Fatal(err)
		}
	}
	for _, y := range []float64{500, 530} { // no rule at 560
		if err := p.DrawLine(pdf.Point{X: 100, Y: y}, pdf.Point{X: 400, Y: y}, line); err != nil {
			t.Fatal(err)
		}
	}
	for r, y := range []float64{540, 510} {
		for c := range 3 {
			if err := p.AddTextAt(fmt.Sprintf("t%d%d", r, c), pdf.TextStyle{Size: 9}, 105+float64(c)*100, y); err != nil {
				t.Fatal(err)
			}
		}
	}
	tables := detectOn(t, doc)
	if len(tables) != 1 {
		t.Fatalf("detected %d tables; want 1", len(tables))
	}
	rows := tables[0].RowList()
	if len(rows) != 2 || rows[0].CellList()[2].Text() != "t02" {
		t.Errorf("%d rows; first row %v", len(rows), rows[0].CellList())
	}
}

// TestTableAbsorberTicksAreNoTable: short ticks crossing two long lines and
// ending together above them are no table edge — the lines are not the ticks'
// border.
func TestTableAbsorberTicksAreNoTable(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	p, _ := doc.Page(1)
	line := pdf.LineStyle{Color: &pdf.Color{A: 1}, Width: 1}
	for _, y := range []float64{500, 510} {
		if err := p.DrawLine(pdf.Point{X: 100, Y: y}, pdf.Point{X: 500, Y: y}, line); err != nil {
			t.Fatal(err)
		}
	}
	for k := range 5 {
		x := 200 + float64(k)*40
		if err := p.DrawLine(pdf.Point{X: x, Y: 500}, pdf.Point{X: x, Y: 530}, line); err != nil {
			t.Fatal(err)
		}
	}
	if tables := detectOn(t, doc); len(tables) != 0 {
		t.Errorf("detected %d tables; want none", len(tables))
	}
}
