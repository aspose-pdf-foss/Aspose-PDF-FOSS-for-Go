// SPDX-License-Identifier: MIT

package asposepdf_test

import (
	"fmt"
	"math"
	"testing"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

// rotatedGridDoc draws a borderless 5x3 table whose text runs at rot degrees:
// row r, column c starts at the point the frame step functions give.
func rotatedGridDoc(t *testing.T, rot float64, at func(r, c int) (x, y float64)) *pdf.Page {
	t.Helper()
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	p, _ := doc.Page(1)
	style := pdf.TextStyle{Font: pdf.FontHelvetica, Size: 10, Rotation: rot}
	names := []string{"Alpha", "Beta", "Gamma", "Delta", "Epsilon"}
	for r, name := range names {
		for c, s := range []string{name, fmt.Sprintf("%d", 10+r), fmt.Sprintf("%d.50", 20+r)} {
			x, y := at(r, c)
			if err := p.AddTextAt(s, style, x, y); err != nil {
				t.Fatal(err)
			}
		}
	}
	return p
}

// TestStreamTableRotatedText: a borderless table set at a quarter turn is
// found in the text's own frame — rows and columns as the text reads — with
// its rectangles on the page (pdf-go-sirj).
func TestStreamTableRotatedText(t *testing.T) {
	for _, c := range []struct {
		rot float64
		at  func(r, c int) (x, y float64)
	}{
		// At 90 degrees the text runs up the page and rows follow rightwards.
		{90, func(r, c int) (float64, float64) { return 100 + float64(r)*16, 300 + float64(c)*100 }},
		// At 270 it runs down and rows follow leftwards.
		{270, func(r, c int) (float64, float64) { return 400 - float64(r)*16, 600 - float64(c)*100 }},
	} {
		t.Run(fmt.Sprint(c.rot), func(t *testing.T) {
			p := rotatedGridDoc(t, c.rot, c.at)
			ta := pdf.NewTableAbsorber()
			if err := ta.Visit(p); err != nil {
				t.Fatal(err)
			}
			tables := ta.TableList()
			if len(tables) != 1 {
				t.Fatalf("tables = %d; want 1", len(tables))
			}
			rows := tables[0].RowList()
			if len(rows) != 5 || len(rows[0].CellList()) != 3 {
				t.Fatalf("grid = %dx%d; want 5x3", len(rows), len(rows[0].CellList()))
			}
			if got := rows[0].CellList()[0].Text(); got != "Alpha" {
				t.Errorf("cell(0,0) = %q; want Alpha", got)
			}
			if got := rows[4].CellList()[2].Text(); got != "24.50" {
				t.Errorf("cell(4,2) = %q; want 24.50", got)
			}
			// Each cell's rectangle holds the point its text starts at.
			for r, row := range rows {
				for k, cell := range row.CellList() {
					x, y := c.at(r, k)
					if rc := cell.Rect; x < rc.LLX || x > rc.URX || y < rc.LLY || y > rc.URY {
						t.Errorf("cell(%d,%d) rect %+v does not hold its text's start (%g, %g)", r, k, rc, x, y)
					}
					// and its fragments are the page's, where the text is drawn.
					if fs := cell.TextFragments(); len(fs) != 1 || fs[0].X < x-0.5 || fs[0].X > x+0.5 || fs[0].Y < y-0.5 || fs[0].Y > y+0.5 || math.Abs(math.Mod(fs[0].Rotation-c.rot+540, 360)-180) > 0.5 {
						t.Errorf("cell(%d,%d) fragments %+v; want one at (%g, %g)", r, k, fs, x, y)
					}
				}
			}
		})
	}
}

// TestStreamTableSideBySideOffsetBlocks: two key-value blocks side by side,
// their rows at heights half a row apart, are two tables rather than none.
func TestStreamTableSideBySideOffsetBlocks(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	p, _ := doc.Page(1)
	left := [][2]string{{"Season:", "Winter 2015"}, {"Working:", "F1506"}, {"Factory:", "Ningbo"}, {"Status:", "Ready"}}
	right := [][2]string{{"Date:", "27-Jan-15"}, {"Workmanship:", "Approved"}, {"Artwork:", "Approved"}, {"Fit:", "Approved"}}
	for i := range left {
		y := 700 - float64(i)*12
		addTextAt(t, p, left[i][0], 60, y, 8)
		addTextAt(t, p, left[i][1], 150, y, 8)
		addTextAt(t, p, right[i][0], 330, y-6, 8)
		addTextAt(t, p, right[i][1], 420, y-6, 8)
	}
	ta := pdf.NewTableAbsorber()
	if err := ta.Visit(p); err != nil {
		t.Fatal(err)
	}
	tables := ta.TableList()
	if len(tables) != 2 {
		t.Fatalf("tables = %d; want 2", len(tables))
	}
	for i, want := range []string{"Winter 2015", "27-Jan-15"} {
		rows := tables[i].RowList()
		if len(rows) != 4 || len(rows[0].CellList()) != 2 {
			t.Fatalf("table %d grid = %dx%d; want 4x2", i, len(rows), len(rows[0].CellList()))
		}
		if got := rows[0].CellList()[1].Text(); got != want {
			t.Errorf("table %d cell(0,1) = %q; want %q", i, got, want)
		}
	}
}
