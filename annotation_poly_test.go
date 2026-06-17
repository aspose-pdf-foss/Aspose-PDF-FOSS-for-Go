// SPDX-License-Identifier: MIT

package asposepdf_test

import (
	"testing"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

// hasNonWhitePixel reports whether the rendered page has any pixel that is
// not (near-)white — a smoke test that an /AP/N actually painted something.
func hasNonWhitePixel(t *testing.T, page *pdf.Page) bool {
	t.Helper()
	img, err := page.RenderImage(pdf.RenderOptions{DPI: 72})
	if err != nil {
		t.Fatalf("RenderImage: %v", err)
	}
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bl, _ := img.At(x, y).RGBA()
			if r < 0xf000 || g < 0xf000 || bl < 0xf000 {
				return true
			}
		}
	}
	return false
}

func TestPolygonAnnotationRoundTrip(t *testing.T) {
	doc := pdf.NewDocument(400, 400)
	page, _ := doc.Page(1)
	verts := []pdf.Point{{X: 100, Y: 100}, {X: 300, Y: 120}, {X: 250, Y: 300}, {X: 120, Y: 280}}
	poly := pdf.NewPolygonAnnotation(page, verts)
	poly.SetColor(&pdf.Color{R: 1, A: 1})
	poly.SetInteriorColor(&pdf.Color{B: 1, A: 1})
	poly.SetBorderWidth(2)
	if err := page.Annotations().Add(poly); err != nil {
		t.Fatalf("Add: %v", err)
	}

	// /Rect must enclose all vertices.
	r := poly.Rect()
	if r.LLX > 100 || r.LLY > 100 || r.URX < 300 || r.URY < 300 {
		t.Errorf("Rect %+v does not enclose vertices", r)
	}

	doc2 := reopen(t, doc)
	got := doc2.Pages()[0].Annotations().At(0)
	if got.AnnotationType() != pdf.AnnotationTypePolygon {
		t.Fatalf("type = %v, want Polygon", got.AnnotationType())
	}
	p2, ok := got.(*pdf.PolygonAnnotation)
	if !ok {
		t.Fatalf("concrete type = %T, want *pdf.PolygonAnnotation", got)
	}
	gv := p2.Vertices()
	if len(gv) != len(verts) {
		t.Fatalf("Vertices len = %d, want %d", len(gv), len(verts))
	}
	for i := range gv {
		if gv[i] != verts[i] {
			t.Errorf("Vertices[%d] = %+v, want %+v", i, gv[i], verts[i])
		}
	}
	if ic := p2.InteriorColor(); ic == nil || ic.B != 1 {
		t.Errorf("InteriorColor = %v, want blue", ic)
	}

	if !hasNonWhitePixel(t, doc2.Pages()[0]) {
		t.Error("polygon /AP/N painted nothing")
	}
}

func TestPolylineAnnotationRoundTrip(t *testing.T) {
	doc := pdf.NewDocument(400, 400)
	page, _ := doc.Page(1)
	verts := []pdf.Point{{X: 50, Y: 50}, {X: 150, Y: 200}, {X: 300, Y: 100}}
	pl := pdf.NewPolylineAnnotation(page, verts)
	pl.SetColor(&pdf.Color{G: 1, A: 1})
	pl.SetBorderWidth(3)
	pl.SetStartLineEnding(pdf.LineEndingOpenArrow)
	pl.SetEndLineEnding(pdf.LineEndingClosedArrow)
	pl.SetInteriorColor(&pdf.Color{R: 1, A: 1})
	if err := page.Annotations().Add(pl); err != nil {
		t.Fatalf("Add: %v", err)
	}

	doc2 := reopen(t, doc)
	got := doc2.Pages()[0].Annotations().At(0)
	if got.AnnotationType() != pdf.AnnotationTypePolyLine {
		t.Fatalf("type = %v, want PolyLine", got.AnnotationType())
	}
	p2 := got.(*pdf.PolylineAnnotation)
	if len(p2.Vertices()) != 3 {
		t.Errorf("Vertices len = %d, want 3", len(p2.Vertices()))
	}
	if p2.StartLineEnding() != pdf.LineEndingOpenArrow {
		t.Errorf("StartLineEnding = %v, want OpenArrow", p2.StartLineEnding())
	}
	if p2.EndLineEnding() != pdf.LineEndingClosedArrow {
		t.Errorf("EndLineEnding = %v, want ClosedArrow", p2.EndLineEnding())
	}

	if !hasNonWhitePixel(t, doc2.Pages()[0]) {
		t.Error("polyline /AP/N painted nothing")
	}
}

func TestCaretAnnotationRoundTrip(t *testing.T) {
	doc := pdf.NewDocument(400, 400)
	page, _ := doc.Page(1)
	car := pdf.NewCaretAnnotation(page, pdf.Rectangle{LLX: 100, LLY: 100, URX: 140, URY: 160})
	car.SetColor(&pdf.Color{R: 0.8, A: 1})
	car.SetSymbol(pdf.CaretSymbolParagraph)
	if err := page.Annotations().Add(car); err != nil {
		t.Fatalf("Add: %v", err)
	}

	doc2 := reopen(t, doc)
	got := doc2.Pages()[0].Annotations().At(0)
	if got.AnnotationType() != pdf.AnnotationTypeCaret {
		t.Fatalf("type = %v, want Caret", got.AnnotationType())
	}
	c2 := got.(*pdf.CaretAnnotation)
	if c2.Symbol() != pdf.CaretSymbolParagraph {
		t.Errorf("Symbol = %v, want Paragraph", c2.Symbol())
	}
	if r := c2.Rect(); r.LLX != 100 || r.URY != 160 {
		t.Errorf("Rect = %+v, want preserved", r)
	}

	if !hasNonWhitePixel(t, doc2.Pages()[0]) {
		t.Error("caret /AP/N painted nothing")
	}
}

func TestPolygonAnnotationNilPagePanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("NewPolygonAnnotation(nil, …) did not panic")
		}
	}()
	pdf.NewPolygonAnnotation(nil, []pdf.Point{{X: 0, Y: 0}})
}

func TestPolylineSetVerticesUpdatesRect(t *testing.T) {
	doc := pdf.NewDocument(400, 400)
	page, _ := doc.Page(1)
	pl := pdf.NewPolylineAnnotation(page, []pdf.Point{{X: 10, Y: 10}, {X: 20, Y: 20}})
	pl.SetColor(&pdf.Color{A: 1})
	if err := page.Annotations().Add(pl); err != nil {
		t.Fatalf("Add: %v", err)
	}
	pl.SetVertices([]pdf.Point{{X: 100, Y: 100}, {X: 350, Y: 350}})
	r := pl.Rect()
	if r.URX < 350 || r.URY < 350 {
		t.Errorf("Rect %+v not updated for new vertices", r)
	}
}
