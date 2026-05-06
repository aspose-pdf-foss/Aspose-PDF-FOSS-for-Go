package asposepdf_test

import (
	"bytes"
	"testing"

	pdf "github.com/aspose/pdf-for-go"
)

func TestPointConstruction(t *testing.T) {
	p := pdf.Point{X: 10, Y: 20}
	if p.X != 10 || p.Y != 20 {
		t.Errorf("Point = %+v, want {10 20}", p)
	}
}

func TestBorderStyleConstants(t *testing.T) {
	if pdf.BorderSolid != 0 {
		t.Errorf("BorderSolid = %d, want 0", pdf.BorderSolid)
	}
	// Verify the 5 constants are distinct and ordered.
	all := []pdf.BorderStyle{
		pdf.BorderSolid,
		pdf.BorderDashed,
		pdf.BorderBeveled,
		pdf.BorderInset,
		pdf.BorderUnderline,
	}
	for i, v := range all {
		if int(v) != i {
			t.Errorf("BorderStyle[%d] = %d, want %d", i, int(v), i)
		}
	}
}

func TestLineEndingStyleConstants(t *testing.T) {
	if pdf.LineEndingNone != 0 {
		t.Errorf("LineEndingNone = %d, want 0", pdf.LineEndingNone)
	}
	all := []pdf.LineEndingStyle{
		pdf.LineEndingNone,
		pdf.LineEndingSquare,
		pdf.LineEndingCircle,
		pdf.LineEndingDiamond,
		pdf.LineEndingOpenArrow,
		pdf.LineEndingClosedArrow,
		pdf.LineEndingButt,
		pdf.LineEndingROpenArrow,
		pdf.LineEndingRClosedArrow,
		pdf.LineEndingSlash,
	}
	for i, v := range all {
		if int(v) != i {
			t.Errorf("LineEndingStyle[%d] = %d, want %d", i, int(v), i)
		}
	}
}

func TestSquareAnnotationSolidStroke(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	sq := pdf.NewSquareAnnotation(page, pdf.Rectangle{LLX: 50, LLY: 600, URX: 200, URY: 700})
	sq.SetColor(&pdf.Color{R: 1, G: 0, B: 0, A: 1})
	sq.SetBorderWidth(2)
	if err := page.Annotations().Add(sq); err != nil {
		t.Fatalf("Add: %v", err)
	}

	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatalf("WriteTo: %v", err)
	}
	doc2, err := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("OpenStream: %v", err)
	}
	page2, _ := doc2.Page(1)
	got := page2.Annotations().At(0)
	if got.AnnotationType() != pdf.AnnotationTypeSquare {
		t.Errorf("type = %v, want AnnotationTypeSquare", got.AnnotationType())
	}
	sq2, ok := got.(*pdf.SquareAnnotation)
	if !ok {
		t.Fatalf("concrete type = %T, want *pdf.SquareAnnotation", got)
	}
	if c := sq2.Color(); c == nil || c.R != 1 {
		t.Errorf("Color = %v, want red", c)
	}
	if w := sq2.BorderWidth(); w != 2 {
		t.Errorf("BorderWidth = %v, want 2", w)
	}
}

func TestSquareAnnotationSetterRegenerateOrder(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	sq := pdf.NewSquareAnnotation(page, pdf.Rectangle{LLX: 0, LLY: 0, URX: 100, URY: 100})
	if err := page.Annotations().Add(sq); err != nil {
		t.Fatalf("Add: %v", err)
	}
	// Set Color and Rect AFTER Add. Both must propagate to /AP/N.
	sq.SetColor(&pdf.Color{R: 0, G: 0, B: 1, A: 1})
	sq.SetRect(pdf.Rectangle{LLX: 50, LLY: 50, URX: 250, URY: 200})
	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatalf("WriteTo: %v", err)
	}
	doc2, err := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("OpenStream: %v", err)
	}
	sq2 := doc2.Pages()[0].Annotations().At(0).(*pdf.SquareAnnotation)
	if c := sq2.Color(); c == nil || c.B != 1 {
		t.Errorf("Color after roundtrip = %v, want blue", c)
	}
	r := sq2.Rect()
	if r.LLX != 50 || r.URY != 200 {
		t.Errorf("Rect after roundtrip = %+v, want LLX=50 URY=200", r)
	}
}

func TestSquareAnnotationDashedBorder(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	sq := pdf.NewSquareAnnotation(page, pdf.Rectangle{LLX: 50, LLY: 600, URX: 200, URY: 700})
	sq.SetBorderStyle(pdf.BorderDashed)
	sq.SetDashPattern([]float64{5, 2})
	if err := page.Annotations().Add(sq); err != nil {
		t.Fatalf("Add: %v", err)
	}

	var buf bytes.Buffer
	doc.WriteTo(&buf)
	doc2, _ := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	sq2 := doc2.Pages()[0].Annotations().At(0).(*pdf.SquareAnnotation)
	if got := sq2.BorderStyle(); got != pdf.BorderDashed {
		t.Errorf("BorderStyle = %v, want BorderDashed", got)
	}
	dp := sq2.DashPattern()
	if len(dp) != 2 || dp[0] != 5 || dp[1] != 2 {
		t.Errorf("DashPattern = %v, want [5 2]", dp)
	}
}

func TestSquareAnnotationDashPatternDefensiveCopy(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	sq := pdf.NewSquareAnnotation(page, pdf.Rectangle{LLX: 0, LLY: 0, URX: 10, URY: 10})
	in := []float64{3, 3}
	sq.SetDashPattern(in)
	in[0] = 99 // mutate caller's slice
	if got := sq.DashPattern(); got[0] != 3 {
		t.Errorf("DashPattern[0] = %v after caller mutation, want 3 (defensive copy)", got[0])
	}
}

func TestSquareAnnotationBeveledRendersTwoColors(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	sq := pdf.NewSquareAnnotation(page, pdf.Rectangle{LLX: 50, LLY: 600, URX: 200, URY: 700})
	sq.SetBorderStyle(pdf.BorderBeveled)
	sq.SetColor(&pdf.Color{R: 0.5, G: 0.5, B: 0.5, A: 1})
	if err := page.Annotations().Add(sq); err != nil {
		t.Fatalf("Add: %v", err)
	}
	var buf bytes.Buffer
	doc.WriteTo(&buf)
	doc2, _ := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	sq2 := doc2.Pages()[0].Annotations().At(0).(*pdf.SquareAnnotation)
	if got := sq2.BorderStyle(); got != pdf.BorderBeveled {
		t.Errorf("BorderStyle = %v, want BorderBeveled", got)
	}
}

func TestSquareAnnotationInsetRoundTrip(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	sq := pdf.NewSquareAnnotation(page, pdf.Rectangle{LLX: 50, LLY: 600, URX: 200, URY: 700})
	sq.SetBorderStyle(pdf.BorderInset)
	if err := page.Annotations().Add(sq); err != nil {
		t.Fatalf("Add: %v", err)
	}
	var buf bytes.Buffer
	doc.WriteTo(&buf)
	doc2, _ := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	sq2 := doc2.Pages()[0].Annotations().At(0).(*pdf.SquareAnnotation)
	if got := sq2.BorderStyle(); got != pdf.BorderInset {
		t.Errorf("BorderStyle = %v, want BorderInset", got)
	}
}

func TestSquareAnnotationUnderlineRoundTrip(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	sq := pdf.NewSquareAnnotation(page, pdf.Rectangle{LLX: 50, LLY: 600, URX: 200, URY: 700})
	sq.SetBorderStyle(pdf.BorderUnderline)
	if err := page.Annotations().Add(sq); err != nil {
		t.Fatalf("Add: %v", err)
	}
	var buf bytes.Buffer
	doc.WriteTo(&buf)
	doc2, _ := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	sq2 := doc2.Pages()[0].Annotations().At(0).(*pdf.SquareAnnotation)
	if got := sq2.BorderStyle(); got != pdf.BorderUnderline {
		t.Errorf("BorderStyle = %v, want BorderUnderline", got)
	}
}

func TestSquareAnnotationInteriorColorFill(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	sq := pdf.NewSquareAnnotation(page, pdf.Rectangle{LLX: 50, LLY: 600, URX: 200, URY: 700})
	sq.SetColor(&pdf.Color{R: 0, G: 0, B: 1, A: 1})
	sq.SetInteriorColor(&pdf.Color{R: 1, G: 1, B: 0, A: 1})
	if err := page.Annotations().Add(sq); err != nil {
		t.Fatalf("Add: %v", err)
	}
	var buf bytes.Buffer
	doc.WriteTo(&buf)
	doc2, _ := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	sq2 := doc2.Pages()[0].Annotations().At(0).(*pdf.SquareAnnotation)
	ic := sq2.InteriorColor()
	if ic == nil || ic.R != 1 || ic.G != 1 || ic.B != 0 {
		t.Errorf("InteriorColor = %v, want yellow", ic)
	}
}

func TestSquareAnnotationInteriorColorClear(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	sq := pdf.NewSquareAnnotation(page, pdf.Rectangle{LLX: 0, LLY: 0, URX: 10, URY: 10})
	sq.SetInteriorColor(&pdf.Color{R: 1, G: 0, B: 0, A: 1})
	sq.SetInteriorColor(nil)
	if got := sq.InteriorColor(); got != nil {
		t.Errorf("InteriorColor after clear = %v, want nil", got)
	}
}

func TestSquareAnnotationNoXObjectLeak(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	sq := pdf.NewSquareAnnotation(page, pdf.Rectangle{LLX: 0, LLY: 0, URX: 10, URY: 10})
	if err := page.Annotations().Add(sq); err != nil {
		t.Fatalf("Add: %v", err)
	}
	// Multiple property mutations — must reuse the same XObject objID.
	sq.SetBorderWidth(2)
	sq.SetBorderWidth(3)
	sq.SetBorderStyle(pdf.BorderDashed)
	sq.SetColor(&pdf.Color{R: 1, G: 0, B: 0, A: 1})
	sq.SetInteriorColor(&pdf.Color{R: 0, G: 1, B: 0, A: 1})
	removed := doc.RemoveUnusedObjects()
	if removed != 0 {
		t.Errorf("RemoveUnusedObjects removed %d objects after multiple setters; want 0 (mutate-in-place expected)", removed)
	}
}

func TestLineAnnotationRoundTrip(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	ln := pdf.NewLineAnnotation(page, pdf.Point{X: 100, Y: 700}, pdf.Point{X: 300, Y: 600})
	ln.SetColor(&pdf.Color{R: 0, G: 0, B: 1, A: 1})
	ln.SetBorderWidth(2)
	if err := page.Annotations().Add(ln); err != nil {
		t.Fatalf("Add: %v", err)
	}
	var buf bytes.Buffer
	doc.WriteTo(&buf)
	doc2, _ := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	got := doc2.Pages()[0].Annotations().At(0)
	if got.AnnotationType() != pdf.AnnotationTypeLine {
		t.Errorf("type = %v, want AnnotationTypeLine", got.AnnotationType())
	}
	ln2 := got.(*pdf.LineAnnotation)
	if s := ln2.Start(); s.X != 100 || s.Y != 700 {
		t.Errorf("Start = %+v, want {100 700}", s)
	}
	if e := ln2.End(); e.X != 300 || e.Y != 600 {
		t.Errorf("End = %+v, want {300 600}", e)
	}
	if w := ln2.BorderWidth(); w != 2 {
		t.Errorf("BorderWidth = %v, want 2", w)
	}
}

func TestCircleAnnotationRoundTrip(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	c := pdf.NewCircleAnnotation(page, pdf.Rectangle{LLX: 50, LLY: 600, URX: 250, URY: 700})
	c.SetColor(&pdf.Color{R: 1, G: 0, B: 0, A: 1})
	c.SetInteriorColor(&pdf.Color{R: 1, G: 1, B: 0, A: 1})
	c.SetBorderWidth(3)
	c.SetBorderStyle(pdf.BorderDashed)
	c.SetDashPattern([]float64{4, 2})
	if err := page.Annotations().Add(c); err != nil {
		t.Fatalf("Add: %v", err)
	}
	var buf bytes.Buffer
	doc.WriteTo(&buf)
	doc2, _ := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	got := doc2.Pages()[0].Annotations().At(0)
	if got.AnnotationType() != pdf.AnnotationTypeCircle {
		t.Errorf("type = %v, want AnnotationTypeCircle", got.AnnotationType())
	}
	c2, ok := got.(*pdf.CircleAnnotation)
	if !ok {
		t.Fatalf("concrete type = %T", got)
	}
	if c2.BorderStyle() != pdf.BorderDashed {
		t.Errorf("BorderStyle = %v", c2.BorderStyle())
	}
	if w := c2.BorderWidth(); w != 3 {
		t.Errorf("BorderWidth = %v, want 3", w)
	}
	if ic := c2.InteriorColor(); ic == nil || ic.R != 1 {
		t.Errorf("InteriorColor = %v", ic)
	}
}
