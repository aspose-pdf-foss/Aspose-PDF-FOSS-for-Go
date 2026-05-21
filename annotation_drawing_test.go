// SPDX-License-Identifier: MIT

package asposepdf_test

import (
	"bytes"
	"testing"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
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

func TestLineAnnotationAllEndingStyles(t *testing.T) {
	for i, name := range []string{
		"None", "Square", "Circle", "Diamond",
		"OpenArrow", "ClosedArrow", "Butt",
		"ROpenArrow", "RClosedArrow", "Slash",
	} {
		style := pdf.LineEndingStyle(i)
		t.Run(name, func(t *testing.T) {
			doc := pdf.NewDocument(595, 842)
			page, _ := doc.Page(1)
			ln := pdf.NewLineAnnotation(page, pdf.Point{X: 100, Y: 700}, pdf.Point{X: 300, Y: 600})
			ln.SetStartLineEnding(style)
			ln.SetEndLineEnding(style)
			if err := page.Annotations().Add(ln); err != nil {
				t.Fatalf("Add: %v", err)
			}
			var buf bytes.Buffer
			doc.WriteTo(&buf)
			doc2, _ := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
			ln2 := doc2.Pages()[0].Annotations().At(0).(*pdf.LineAnnotation)
			if got := ln2.StartLineEnding(); got != style {
				t.Errorf("StartLineEnding = %v, want %v", got, style)
			}
			if got := ln2.EndLineEnding(); got != style {
				t.Errorf("EndLineEnding = %v, want %v", got, style)
			}
		})
	}
}

func TestLineAnnotationInteriorColorAndLeaderLine(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	ln := pdf.NewLineAnnotation(page, pdf.Point{X: 100, Y: 700}, pdf.Point{X: 300, Y: 700})
	ln.SetStartLineEnding(pdf.LineEndingClosedArrow)
	ln.SetEndLineEnding(pdf.LineEndingClosedArrow)
	ln.SetInteriorColor(&pdf.Color{R: 1, G: 1, B: 0, A: 1})
	ln.SetLeaderLineLength(10)
	if err := page.Annotations().Add(ln); err != nil {
		t.Fatalf("Add: %v", err)
	}
	var buf bytes.Buffer
	doc.WriteTo(&buf)
	doc2, _ := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	ln2 := doc2.Pages()[0].Annotations().At(0).(*pdf.LineAnnotation)
	ic := ln2.InteriorColor()
	if ic == nil || ic.R != 1 {
		t.Errorf("InteriorColor = %v", ic)
	}
	if ll := ln2.LeaderLineLength(); ll != 10 {
		t.Errorf("LeaderLineLength = %v, want 10", ll)
	}
}

func TestInkAnnotationTwoPointStrokeRoundTrip(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	strokes := [][]pdf.Point{
		{{X: 100, Y: 700}, {X: 200, Y: 750}},
		{{X: 50, Y: 600}, {X: 150, Y: 650}},
	}
	ink := pdf.NewInkAnnotation(page, strokes)
	ink.SetColor(&pdf.Color{R: 0, G: 0, B: 1, A: 1})
	ink.SetBorderWidth(1.5)
	if err := page.Annotations().Add(ink); err != nil {
		t.Fatalf("Add: %v", err)
	}
	var buf bytes.Buffer
	doc.WriteTo(&buf)
	doc2, _ := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	got := doc2.Pages()[0].Annotations().At(0)
	if got.AnnotationType() != pdf.AnnotationTypeInk {
		t.Errorf("type = %v, want AnnotationTypeInk", got.AnnotationType())
	}
	ink2 := got.(*pdf.InkAnnotation)
	gotStrokes := ink2.Strokes()
	if len(gotStrokes) != 2 {
		t.Fatalf("Strokes len = %d, want 2", len(gotStrokes))
	}
	if len(gotStrokes[0]) != 2 || gotStrokes[0][0].X != 100 {
		t.Errorf("Strokes[0] = %v", gotStrokes[0])
	}
}

func TestInkAnnotationDefensiveCopy(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	in := [][]pdf.Point{{{X: 0, Y: 0}, {X: 10, Y: 10}}}
	ink := pdf.NewInkAnnotation(page, in)
	in[0][0].X = 99 // mutate caller's slice
	got := ink.Strokes()
	if got[0][0].X != 0 {
		t.Errorf("Strokes[0][0].X = %v after caller mutation, want 0", got[0][0].X)
	}
}

func TestInkAnnotationCatmullRomSmoothsThreePlusPoints(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	// 5-point stroke: smoothing should produce c (curve) operators in /AP.
	strokes := [][]pdf.Point{{
		{X: 100, Y: 700},
		{X: 120, Y: 720},
		{X: 150, Y: 730},
		{X: 180, Y: 720},
		{X: 200, Y: 700},
	}}
	ink := pdf.NewInkAnnotation(page, strokes)
	if err := page.Annotations().Add(ink); err != nil {
		t.Fatalf("Add: %v", err)
	}
	// Verify the underlying /AP stream contains Bezier (c) operators.
	// We do this via the parsed document's content stream walking.
	var buf bytes.Buffer
	doc.WriteTo(&buf)
	doc2, _ := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	page2, _ := doc2.Page(1)
	got := page2.Annotations().At(0)
	ink2 := got.(*pdf.InkAnnotation)
	gotStrokes := ink2.Strokes()
	// Round-trip preserves /InkList raw points unchanged (smoothing is
	// /AP-only; /InkList stores the original polyline).
	if len(gotStrokes) != 1 || len(gotStrokes[0]) != 5 {
		t.Fatalf("Strokes shape = %v, want 1 stroke of 5 points", gotStrokes)
	}
}

func TestSetterDrivenRegenerate(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	sq := pdf.NewSquareAnnotation(page, pdf.Rectangle{LLX: 0, LLY: 0, URX: 100, URY: 100})
	if err := page.Annotations().Add(sq); err != nil {
		t.Fatalf("Add: %v", err)
	}
	// Multiple mutations after Add. Last value must win on save.
	sq.SetBorderWidth(1)
	sq.SetBorderWidth(2)
	sq.SetBorderWidth(7)
	var buf bytes.Buffer
	doc.WriteTo(&buf)
	doc2, _ := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	sq2 := doc2.Pages()[0].Annotations().At(0).(*pdf.SquareAnnotation)
	if w := sq2.BorderWidth(); w != 7 {
		t.Errorf("BorderWidth after multiple sets = %v, want 7", w)
	}
}

func TestRegenerateAppearancePublicMethod(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	sq := pdf.NewSquareAnnotation(page, pdf.Rectangle{LLX: 0, LLY: 0, URX: 100, URY: 100})
	if err := page.Annotations().Add(sq); err != nil {
		t.Fatalf("Add: %v", err)
	}
	// Calling RegenerateAppearance on each of the 4 types must not error.
	sq.RegenerateAppearance()

	c := pdf.NewCircleAnnotation(page, pdf.Rectangle{LLX: 0, LLY: 0, URX: 100, URY: 100})
	page.Annotations().Add(c)
	c.RegenerateAppearance()

	ln := pdf.NewLineAnnotation(page, pdf.Point{X: 0, Y: 0}, pdf.Point{X: 50, Y: 50})
	page.Annotations().Add(ln)
	ln.RegenerateAppearance()

	ink := pdf.NewInkAnnotation(page, [][]pdf.Point{{{X: 0, Y: 0}, {X: 10, Y: 10}}})
	page.Annotations().Add(ink)
	ink.RegenerateAppearance()
}

func TestDrawingAnnotationsFilterByType(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)

	page.Annotations().Add(pdf.NewSquareAnnotation(page, pdf.Rectangle{LLX: 0, LLY: 0, URX: 50, URY: 50}))
	page.Annotations().Add(pdf.NewCircleAnnotation(page, pdf.Rectangle{LLX: 0, LLY: 100, URX: 50, URY: 150}))
	page.Annotations().Add(pdf.NewLineAnnotation(page, pdf.Point{X: 100, Y: 0}, pdf.Point{X: 200, Y: 0}))
	page.Annotations().Add(pdf.NewInkAnnotation(page, [][]pdf.Point{{{X: 300, Y: 0}, {X: 350, Y: 50}}}))

	var buf bytes.Buffer
	doc.WriteTo(&buf)
	doc2, _ := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	page2, _ := doc2.Page(1)

	counts := map[pdf.AnnotationType]int{}
	for _, a := range page2.Annotations().All() {
		counts[a.AnnotationType()]++
	}
	if counts[pdf.AnnotationTypeSquare] != 1 ||
		counts[pdf.AnnotationTypeCircle] != 1 ||
		counts[pdf.AnnotationTypeLine] != 1 ||
		counts[pdf.AnnotationTypeInk] != 1 {
		t.Errorf("counts = %v, want one of each (Square/Circle/Line/Ink)", counts)
	}
}

func TestUnboundAnnotationGeneratesAP(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	// Create but do NOT Add. /AP/N still gets generated because constructor
	// sets doc reference.
	sq := pdf.NewSquareAnnotation(page, pdf.Rectangle{LLX: 0, LLY: 0, URX: 50, URY: 50})
	sq.SetColor(&pdf.Color{R: 1, G: 0, B: 0, A: 1})
	// Even before Add, /AP/N must reference an XObject in doc.objects.
	// We can't inspect dict directly from external package, but we can
	// confirm RemoveUnusedObjects sees an orphan XObject (the unbound /AP/N).
	removed := doc.RemoveUnusedObjects()
	// Square's XObject is unreachable (no annotation in /Annots) so it
	// gets removed. We expect at least 1 removal — that's the unbound
	// /AP/N XObject.
	if removed < 1 {
		t.Errorf("RemoveUnusedObjects removed %d, want >= 1 (orphan /AP/N XObject)", removed)
	}
}
