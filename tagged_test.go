// SPDX-License-Identifier: MIT

package asposepdf_test

import (
	"bytes"
	"image"
	"testing"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

func uaHas(rep *pdf.PDFUAValidationReport, rule string) bool {
	for _, is := range rep.Issues {
		if is.Rule == rule {
			return true
		}
	}
	return false
}

// authorTagged builds a tagged document; figureAlt controls whether the figure
// gets alternate text.
func authorTagged(t *testing.T, figureAlt bool) *pdf.Document {
	t.Helper()
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	tc := doc.TaggedContent()
	tc.SetTitle("Quarterly Report")
	tc.SetLanguage("en-US")
	p, _ := doc.Page(1)

	if _, err := p.TagContent(tc.Root(), pdf.StructH1, func() error {
		return p.AddText("Quarterly Report", pdf.TextStyle{Font: pdf.FontHelveticaBold, Size: 24},
			pdf.Rectangle{LLX: 50, LLY: 760, URX: 545, URY: 800})
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := p.TagContent(tc.Root(), pdf.StructP, func() error {
		return p.AddText("This report covers Q3 results.", pdf.TextStyle{Font: pdf.FontHelvetica, Size: 12},
			pdf.Rectangle{LLX: 50, LLY: 700, URX: 545, URY: 750})
	}); err != nil {
		t.Fatal(err)
	}
	fig, err := p.TagContent(tc.Root(), pdf.StructFigure, func() error {
		return p.DrawRectangle(pdf.Rectangle{LLX: 50, LLY: 550, URX: 250, URY: 680},
			pdf.ShapeStyle{FillColor: &pdf.Color{R: 0.2, G: 0.5, B: 0.8, A: 1}})
	})
	if err != nil {
		t.Fatal(err)
	}
	if figureAlt {
		fig.SetAlt("Bar chart of Q3 sales by region")
	}
	return doc
}

// TestTaggedAuthoringConformant: an authored tagged document passes ValidatePDFUA
// and stays conformant after a round-trip, with text and structure intact.
func TestTaggedAuthoringConformant(t *testing.T) {
	doc := authorTagged(t, true)
	if rep := doc.ValidatePDFUA(); !rep.Conformant {
		t.Fatalf("authored document not PDF/UA-conformant: %+v", rep.Issues)
	}

	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	// The output carries the structure tree (the page content stream, where the
	// BDC/EMC marked content lives, is Flate-compressed — see the internal test
	// for the operator-level check).
	for _, marker := range []string{"/StructTreeRoot", "/MarkInfo", "/StructElem", "/ParentTree", "/Figure", "/Alt"} {
		if !bytes.Contains(buf.Bytes(), []byte(marker)) {
			t.Errorf("output missing %s", marker)
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
	txt, _ := page.ExtractText()
	if !bytes.Contains([]byte(txt), []byte("Quarterly Report")) {
		t.Errorf("tagged text lost after round-trip: %q", txt)
	}
}

// TestTaggedFigureNeedsAlt: a figure without alt text fails PDF/UA.
func TestTaggedFigureNeedsAlt(t *testing.T) {
	doc := authorTagged(t, false)
	rep := doc.ValidatePDFUA()
	if !uaHas(rep, "UA_FIGURE_NO_ALT") {
		t.Errorf("expected UA_FIGURE_NO_ALT for an untagged figure; got %+v", rep.Issues)
	}
}

// TestTaggedNesting: grouping elements (Table → TR → TD) nest correctly and the
// document validates.
func TestTaggedNesting(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	tc := doc.TaggedContent()
	tc.SetTitle("Table")
	tc.SetLanguage("en")
	p, _ := doc.Page(1)

	tbl := tc.Root().AddChild(pdf.StructTable)
	row := tbl.AddChild(pdf.StructTR)
	cell := row.AddChild(pdf.StructTD)
	if _, err := p.TagContent(cell, pdf.StructP, func() error {
		return p.AddText("Region A: $1.2M", pdf.TextStyle{Font: pdf.FontHelvetica, Size: 11},
			pdf.Rectangle{LLX: 50, LLY: 700, URX: 300, URY: 730})
	}); err != nil {
		t.Fatal(err)
	}
	if rep := doc.ValidatePDFUA(); !rep.Conformant {
		t.Errorf("nested-table document not conformant: %+v", rep.Issues)
	}
}

// TestTagContentRequiresSetup: TagContent without TaggedContent() errors.
func TestTagContentRequiresSetup(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	p, _ := doc.Page(1)
	if _, err := p.TagContent(nil, pdf.StructP, func() error { return nil }); err == nil {
		t.Error("expected an error tagging content before TaggedContent()")
	}
}

// TestTaggedRenders: marked content does not break rendering.
func TestTaggedRenders(t *testing.T) {
	doc := authorTagged(t, true)
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
		t.Error("tagged page rendered blank")
	}
}
