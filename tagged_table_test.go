// SPDX-License-Identifier: MIT

package asposepdf_test

import (
	"bytes"
	"image"
	"testing"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

func salesTable() *pdf.Table {
	t := pdf.NewTable().SetColumnWidths([]float64{150, 150, 150})
	t.SetRepeatingRowsCount(1) // first row = header
	t.AddRow().AddCells("Region", "Q3", "Q4")
	t.AddRow().AddCells("North", "$1.2M", "$1.5M")
	t.AddRow().AddCells("South", "$0.9M", "$1.1M")
	return t
}

// TestAddTaggedTable: a tagged table builds the structure tree, validates as
// PDF/UA, round-trips and keeps its text.
func TestAddTaggedTable(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	tc := doc.TaggedContent()
	tc.SetTitle("Sales Report")
	tc.SetLanguage("en-US")
	p, _ := doc.Page(1)

	elem, pages, err := p.AddTaggedTable(tc, tc.Root(), salesTable(),
		pdf.Rectangle{LLX: 50, LLY: 500, URX: 550, URY: 760})
	if err != nil {
		t.Fatal(err)
	}
	if elem == nil {
		t.Fatal("nil table element")
	}
	if pages != 0 {
		t.Errorf("unexpected continuation pages: %d", pages)
	}
	if rep := doc.ValidatePDFUA(); !rep.Conformant {
		t.Fatalf("tagged table not PDF/UA-conformant: %+v", rep.Issues)
	}

	var buf bytes.Buffer
	doc.WriteTo(&buf)
	for _, marker := range []string{"/Table", "/TR", "/TH", "/TD"} {
		if !bytes.Contains(buf.Bytes(), []byte(marker)) {
			t.Errorf("output missing structure type %s", marker)
		}
	}
	out, err := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if rep := out.ValidatePDFUA(); !rep.Conformant {
		t.Errorf("not conformant after round-trip: %+v", rep.Issues)
	}
	page, _ := out.Page(1)
	if txt, _ := page.ExtractText(); !bytes.Contains([]byte(txt), []byte("North")) {
		t.Errorf("table text lost: %q", txt)
	}
}

// TestAddTaggedTablePaginates: a tall tagged table that overflows onto
// continuation pages stays conformant.
func TestAddTaggedTablePaginates(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	tc := doc.TaggedContent()
	tc.SetTitle("Big Table")
	tc.SetLanguage("en")
	p, _ := doc.Page(1)

	tbl := pdf.NewTable().SetColumnWidths([]float64{200, 200})
	tbl.SetRepeatingRowsCount(1)
	tbl.AddRow().AddCells("Name", "Value")
	for i := 0; i < 80; i++ {
		tbl.AddRow().AddCells("Row", "data")
	}
	_, pages, err := p.AddTaggedTable(tc, tc.Root(), tbl,
		pdf.Rectangle{LLX: 50, LLY: 60, URX: 450, URY: 760})
	if err != nil {
		t.Fatal(err)
	}
	if pages == 0 {
		t.Fatal("expected the tall table to paginate")
	}
	if rep := doc.ValidatePDFUA(); !rep.Conformant {
		t.Errorf("paginated tagged table not conformant: %+v", rep.Issues)
	}
}

// TestAddTaggedTableRequiresSetup: calling without TaggedContent errors.
func TestAddTaggedTableRequiresSetup(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	p, _ := doc.Page(1)
	if _, _, err := p.AddTaggedTable(nil, nil, salesTable(),
		pdf.Rectangle{LLX: 50, LLY: 500, URX: 550, URY: 760}); err == nil {
		t.Error("expected an error tagging a table before TaggedContent()")
	}
}

// TestAddTaggedTableRenders: the tagged table still renders (marked content does
// not break drawing).
func TestAddTaggedTableRenders(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	tc := doc.TaggedContent()
	tc.SetTitle("R")
	tc.SetLanguage("en")
	p, _ := doc.Page(1)
	tbl := salesTable()
	tbl.SetBorder(pdf.BorderInfo{Sides: pdf.BorderSideAll, Width: 1})
	if _, _, err := p.AddTaggedTable(tc, tc.Root(), tbl,
		pdf.Rectangle{LLX: 50, LLY: 500, URX: 550, URY: 760}); err != nil {
		t.Fatal(err)
	}
	img, err := doc.RenderImage(1, pdf.RenderOptions{DPI: 96})
	if err != nil {
		t.Fatal(err)
	}
	rgba := img.(*image.RGBA)
	b := rgba.Bounds()
	nonwhite := 0
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if r, g, bl, _ := rgba.At(x, y).RGBA(); r < 60000 || g < 60000 || bl < 60000 {
				nonwhite++
			}
		}
	}
	if nonwhite == 0 {
		t.Error("tagged table rendered blank")
	}
}
