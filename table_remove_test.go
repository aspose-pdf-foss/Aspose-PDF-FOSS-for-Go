// SPDX-License-Identifier: MIT

package asposepdf_test

import (
	"strings"
	"testing"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

// removableTableDoc draws a ruled 3x3 table at (100, 600) with a caption just
// above it and a note beside it.
func removableTableDoc(t *testing.T) (*pdf.Document, *pdf.Page) {
	t.Helper()
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	p, _ := doc.Page(1)
	tb := pdf.NewTable().SetColumnWidths([]float64{80, 80, 80}).
		SetDefaultCellBorder(pdf.BorderInfo{Sides: pdf.BorderSideAll, Width: 1})
	tb.AddRows([][]string{{"Name", "Qty", "Price"}, {"Apple", "3", "1.20"}, {"Pear", "5", "0.90"}})
	if _, err := p.AddTable(tb, pdf.Rectangle{LLX: 100, LLY: 400, URX: 340, URY: 600}); err != nil {
		t.Fatal(err)
	}
	style := pdf.TextStyle{Font: pdf.FontHelvetica, Size: 10}
	if err := p.AddTextAt("Table 1: fruit", style, 100, 606); err != nil {
		t.Fatal(err)
	}
	if err := p.AddTextAt("Note beside", style, 360, 580); err != nil {
		t.Fatal(err)
	}
	return doc, p
}

func absorb(t *testing.T, p *pdf.Page) *pdf.TableAbsorber {
	t.Helper()
	a := pdf.NewTableAbsorber()
	if err := a.Visit(p); err != nil {
		t.Fatal(err)
	}
	return a
}

// TestTableAbsorberRemove: the table's text and rules go, the text around it
// stays, and the absorber forgets it (pdf-go-6jxp).
func TestTableAbsorberRemove(t *testing.T) {
	_, p := removableTableDoc(t)
	a := absorb(t, p)
	if n := len(a.TableList()); n != 1 {
		t.Fatalf("%d tables, want 1", n)
	}
	if err := a.Remove(a.TableList()[0]); err != nil {
		t.Fatal(err)
	}
	if n := len(a.TableList()); n != 0 {
		t.Errorf("absorber still lists %d tables", n)
	}
	if n := len(absorb(t, p).TableList()); n != 0 {
		t.Errorf("%d tables found after removal", n)
	}
	s, err := p.ExtractText()
	if err != nil {
		t.Fatal(err)
	}
	for _, gone := range []string{"Name", "Apple", "Pear", "1.20"} {
		if strings.Contains(s, gone) {
			t.Errorf("%q still on the page: %q", gone, s)
		}
	}
	for _, kept := range []string{"Table 1: fruit", "Note beside"} {
		if !strings.Contains(s, kept) {
			t.Errorf("%q lost: %q", kept, s)
		}
	}
}

// TestTableAbsorberReplace: the new table takes the old one's top-left
// corner and is found there with its own text.
func TestTableAbsorberReplace(t *testing.T) {
	_, p := removableTableDoc(t)
	a := absorb(t, p)
	old := a.TableList()[0].Rect
	nt := pdf.NewTable().SetColumnWidths([]float64{120, 120}).
		SetDefaultCellBorder(pdf.BorderInfo{Sides: pdf.BorderSideAll, Width: 1})
	nt.AddRows([][]string{{"Key", "Value"}, {"Colour", "Green"}})
	if err := a.Replace(p, a.TableList()[0], nt); err != nil {
		t.Fatal(err)
	}
	ts := absorb(t, p).TableList()
	if len(ts) != 1 {
		t.Fatalf("%d tables, want 1", len(ts))
	}
	r := ts[0].Rect
	if abs(r.LLX-old.LLX) > 0.5 || abs(r.URY-old.URY) > 0.5 {
		t.Errorf("new table at %+v, want its top-left at (%g, %g)", r, old.LLX, old.URY)
	}
	rows := ts[0].RowList()
	if len(rows) != 2 || rows[1].CellList()[1].Text() != "Green" {
		t.Errorf("new table content wrong: %d rows", len(rows))
	}
	s, _ := p.ExtractText()
	if strings.Contains(s, "Apple") || !strings.Contains(s, "Table 1: fruit") {
		t.Errorf("page text %q", s)
	}
}

// TestTableAbsorberRemoveRefusesForeign: a table Visit did not produce is
// refused, and a table cannot be replaced on another page.
func TestTableAbsorberRemoveRefusesForeign(t *testing.T) {
	doc, p := removableTableDoc(t)
	a := absorb(t, p)
	if err := a.Remove(&pdf.AbsorbedTable{}); err == nil {
		t.Error("a table not found by Visit was accepted")
	}
	if err := doc.AddBlankPage(595, 842); err != nil {
		t.Fatal(err)
	}
	p2, _ := doc.Page(2)
	nt := pdf.NewTable().SetColumnWidths([]float64{50})
	nt.AddRow().AddCell("x")
	if err := a.Replace(p2, a.TableList()[0], nt); err == nil {
		t.Error("a table was replaced on another page")
	}
}
