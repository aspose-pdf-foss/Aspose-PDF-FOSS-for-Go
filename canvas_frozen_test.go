// SPDX-License-Identifier: MIT

package asposepdf_test

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

// tinyTestPNG generates a minimal opaque PNG as bytes (createTestPNG in
// image_add_test.go lives in the internal asposepdf package and isn't
// visible from this external asposepdf_test package).
func tinyTestPNG(w, h int) []byte {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: 255, A: 255})
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

// pdf-go-nvpo / GitHub issue #17: detached drawing canvases (XForm.Canvas,
// TilingPattern.Canvas, the glyph canvas from Type3Font.AddGlyph) leaked
// their own content-stream objects into the saved file instead of being
// cleaned up once harvested into the Form XObject/pattern/CharProc stream
// they feed, and drawing on one after it was frozen (harvested) silently
// kept the extra content in the file even though it never reached any
// page. Separately, Size()/MediaBox()/AddTable() failed outright on a
// detached canvas with "object 0 not found", contradicting the documented
// "full *Page drawing API works on it" promise.

// TestXFormCanvasBuildLeavesNoDeadObjects reproduces the GitHub issue #17
// repro directly: three drawing calls on a form's canvas (which, like a
// page, trigger the internal q/Q content-stream wrap from the second call
// on) must leave zero unreachable objects after the form is placed and the
// document is saved and reopened.
func TestXFormCanvasBuildLeavesNoDeadObjects(t *testing.T) {
	doc := pdf.NewDocument(200, 200)
	p, err := doc.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	red := pdf.ShapeStyle{FillColor: &pdf.Color{R: 1, A: 1}}
	form := doc.CreateForm(10, 10)
	for i := 0; i < 3; i++ {
		if err := form.Canvas().DrawRectangle(
			pdf.Rectangle{LLX: float64(i), URX: float64(i) + 1, URY: 1}, red); err != nil {
			t.Fatalf("DrawRectangle %d: %v", i, err)
		}
	}
	if err := p.AddForm(form, pdf.Rectangle{URX: 100, URY: 100}); err != nil {
		t.Fatalf("AddForm: %v", err)
	}

	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatalf("WriteTo: %v", err)
	}
	saved, err := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("OpenStream: %v", err)
	}
	if n := saved.RemoveUnusedObjects(); n != 0 {
		t.Errorf("RemoveUnusedObjects() = %d, want 0 (the canvas's own content-stream objects must not survive the build)", n)
	}
}

// TestTilingPatternCanvasBuildLeavesNoDeadObjects is the same leak check for
// TilingPattern.Canvas().
func TestTilingPatternCanvasBuildLeavesNoDeadObjects(t *testing.T) {
	doc := pdf.NewDocument(200, 200)
	p, err := doc.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	red := pdf.ShapeStyle{FillColor: &pdf.Color{R: 1, A: 1}}
	pat := doc.CreateTilingPattern(10, 10)
	for i := 0; i < 3; i++ {
		if err := pat.Canvas().DrawRectangle(
			pdf.Rectangle{LLX: float64(i), URX: float64(i) + 1, URY: 1}, red); err != nil {
			t.Fatalf("DrawRectangle %d: %v", i, err)
		}
	}
	fillStyle := pdf.ShapeStyle{FillTiling: pat}
	if err := p.DrawRectangle(pdf.Rectangle{URX: 100, URY: 100}, fillStyle); err != nil {
		t.Fatalf("DrawRectangle (tiled fill): %v", err)
	}

	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatalf("WriteTo: %v", err)
	}
	saved, err := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("OpenStream: %v", err)
	}
	if n := saved.RemoveUnusedObjects(); n != 0 {
		t.Errorf("RemoveUnusedObjects() = %d, want 0 (the pattern cell's own content-stream objects must not survive the build)", n)
	}
}

// TestType3GlyphCanvasBuildLeavesNoDeadObjects is the same leak check for a
// Type3Font glyph canvas.
func TestType3GlyphCanvasBuildLeavesNoDeadObjects(t *testing.T) {
	doc := pdf.NewDocument(200, 200)
	p, err := doc.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	t3 := doc.CreateType3Font()
	canvas, err := t3.AddGlyph('★', 900)
	if err != nil {
		t.Fatal(err)
	}
	red := pdf.ShapeStyle{FillColor: &pdf.Color{R: 1, A: 1}}
	for i := 0; i < 3; i++ {
		if err := canvas.DrawRectangle(
			pdf.Rectangle{LLX: float64(i * 100), URX: float64(i*100 + 50), URY: 500}, red); err != nil {
			t.Fatalf("DrawRectangle %d: %v", i, err)
		}
	}
	if err := p.AddText("★", pdf.TextStyle{Font: t3, Size: 24},
		pdf.Rectangle{URX: 100, URY: 100}); err != nil {
		t.Fatalf("AddText: %v", err)
	}

	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatalf("WriteTo: %v", err)
	}
	saved, err := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("OpenStream: %v", err)
	}
	if n := saved.RemoveUnusedObjects(); n != 0 {
		t.Errorf("RemoveUnusedObjects() = %d, want 0 (the glyph canvas's own content-stream objects must not survive the build)", n)
	}
}

// TestXFormCanvasDrawAfterFreezeErrors is the data-hiding half of point 1:
// once a form has been placed (frozen), drawing on its canvas again must
// not be silently accepted and kept in the saved file — it must error, so
// nothing new is ever written for content that can never reach any page.
func TestXFormCanvasDrawAfterFreezeErrors(t *testing.T) {
	doc := pdf.NewDocument(200, 200)
	p, err := doc.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	form := doc.CreateForm(100, 50)
	if err := form.Canvas().AddText("Early", pdf.TextStyle{Size: 10},
		pdf.Rectangle{URX: 100, URY: 50}); err != nil {
		t.Fatal(err)
	}
	if err := p.AddForm(form, pdf.Rectangle{URX: 100, URY: 50}); err != nil {
		t.Fatalf("AddForm: %v", err)
	}

	if err := form.Canvas().AddText("Late", pdf.TextStyle{Size: 10},
		pdf.Rectangle{URX: 100, URY: 50}); err == nil {
		t.Error("AddText on a frozen canvas: want error, got nil")
	}
}

// TestType3GlyphCanvasDrawAfterFreezeErrors is the same check for a Type3
// glyph canvas, frozen by the font's first use in AddText.
func TestType3GlyphCanvasDrawAfterFreezeErrors(t *testing.T) {
	doc := pdf.NewDocument(200, 200)
	p, err := doc.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	t3 := doc.CreateType3Font()
	canvas, err := t3.AddGlyph('★', 900)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.AddText("★", pdf.TextStyle{Font: t3, Size: 24},
		pdf.Rectangle{URX: 100, URY: 100}); err != nil {
		t.Fatalf("AddText (freezes the font): %v", err)
	}

	if err := canvas.DrawRectangle(pdf.Rectangle{URX: 10, URY: 10},
		pdf.ShapeStyle{FillColor: &pdf.Color{A: 1}}); err == nil {
		t.Error("DrawRectangle on a frozen glyph canvas: want error, got nil")
	}
}

// TestXFormCanvasAddImageAfterFreezeErrors covers the one draw primitive
// that creates a separate, payload-bearing object (the image XObject)
// BEFORE appending the "cm ... Do" operators that place it: if only the
// content-stream append were guarded, AddImage after freeze would still
// leave a real image object registered into the (already-built, shared)
// Form XObject's own /Resources — reachable, and so written to the saved
// file, even though no Do operator referencing it was ever emitted.
func TestXFormCanvasAddImageAfterFreezeErrors(t *testing.T) {
	doc := pdf.NewDocument(200, 200)
	p, err := doc.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	form := doc.CreateForm(100, 50)
	if err := form.Canvas().AddText("Early", pdf.TextStyle{Size: 10},
		pdf.Rectangle{URX: 100, URY: 50}); err != nil {
		t.Fatal(err)
	}
	if err := p.AddForm(form, pdf.Rectangle{URX: 100, URY: 50}); err != nil {
		t.Fatalf("AddForm: %v", err)
	}

	imgData := tinyTestPNG(4, 4)
	if err := form.Canvas().AddImageFromStream(bytes.NewReader(imgData),
		pdf.Rectangle{URX: 10, URY: 10}); err == nil {
		t.Error("AddImageFromStream on a frozen canvas: want error, got nil")
	}

	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatalf("WriteTo: %v", err)
	}
	saved, err := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("OpenStream: %v", err)
	}
	if n := saved.RemoveUnusedObjects(); n != 0 {
		t.Errorf("RemoveUnusedObjects() = %d, want 0 (the rejected image must never have been registered into the built form's Resources)", n)
	}
}

// TestXFormCanvasSizeAndMediaBox confirms Size()/MediaBox() work on a
// detached canvas instead of failing with "object 0 not found" — the box
// is the form's own, never looked up by object number.
func TestXFormCanvasSizeAndMediaBox(t *testing.T) {
	doc := pdf.NewDocument(200, 200)
	c := doc.CreateForm(100, 50).Canvas()

	sz, err := c.Size()
	if err != nil {
		t.Fatalf("Size: %v", err)
	}
	if sz.Width != 100 || sz.Height != 50 {
		t.Errorf("Size = %+v, want {100 50}", sz)
	}

	mb, err := c.MediaBox()
	if err != nil {
		t.Fatalf("MediaBox: %v", err)
	}
	want := pdf.Rectangle{LLX: 0, LLY: 0, URX: 100, URY: 50}
	if mb != want {
		t.Errorf("MediaBox = %+v, want %+v", mb, want)
	}

	cb, err := c.CropBox()
	if err != nil {
		t.Fatalf("CropBox: %v", err)
	}
	if cb != want {
		t.Errorf("CropBox = %+v, want %+v (falls back to MediaBox)", cb, want)
	}
}

// TestTilingPatternCanvasSizeAndMediaBox is the same check for
// TilingPattern.Canvas().
func TestTilingPatternCanvasSizeAndMediaBox(t *testing.T) {
	doc := pdf.NewDocument(200, 200)
	c := doc.CreateTilingPattern(30, 40).Canvas()

	sz, err := c.Size()
	if err != nil {
		t.Fatalf("Size: %v", err)
	}
	if sz.Width != 30 || sz.Height != 40 {
		t.Errorf("Size = %+v, want {30 40}", sz)
	}
}

// TestXFormCanvasAddTableFits confirms AddTable draws into a detached
// canvas when the table fits, instead of failing on the Size() lookup it
// needs internally to compute the continuation rectangle.
func TestXFormCanvasAddTableFits(t *testing.T) {
	doc := pdf.NewDocument(200, 200)
	c := doc.CreateForm(100, 50).Canvas()

	tbl := pdf.NewTable()
	tbl.SetColumnWidths([]float64{40, 40})
	tbl.AddRow().AddCells("a", "b")
	// Default overflow margins (50pt top + 50pt bottom) deliberately left
	// as-is: a detached canvas never continues onto a next page at all,
	// so the continuation rect those margins would otherwise produce is
	// skipped entirely for one (GitHub issue #20 point 3,
	// shadowhunter1470-svg) — this table used to fail here purely
	// because 50+50 >= this 50pt-tall canvas's own height, regardless of
	// whether the one-row table itself fit (it does, easily).

	pages, err := c.AddTable(tbl, pdf.Rectangle{URX: 100, URY: 50})
	if err != nil {
		t.Fatalf("AddTable: %v", err)
	}
	if pages != 0 {
		t.Errorf("AddTable pagesAdded = %d, want 0", pages)
	}
}

// TestXFormCanvasAddTableOverflowErrors confirms that a table too tall for
// a detached canvas returns an error instead of calling AddBlankPage on
// the DOCUMENT (which would silently append an unrelated page having
// nothing to do with the form being built).
func TestXFormCanvasAddTableOverflowErrors(t *testing.T) {
	doc := pdf.NewDocument(200, 200)
	before := doc.PageCount()
	c := doc.CreateForm(100, 20).Canvas() // too short for the table below

	tbl := pdf.NewTable()
	tbl.SetColumnWidths([]float64{40})
	tbl.SetOverflowMargins(0, 0) // the 50pt default margins would themselves make the
	// continuation rect invalid for this 20pt-tall canvas, masking the
	// overflow-guard check this test targets behind an unrelated error.
	for i := 0; i < 5; i++ {
		r := tbl.AddRow()
		r.SetHeight(20)
		r.AddCells("row")
	}

	if _, err := c.AddTable(tbl, pdf.Rectangle{URX: 100, URY: 20}); err == nil {
		t.Error("AddTable overflowing a detached canvas: want error, got nil")
	}
	if got := doc.PageCount(); got != before {
		t.Errorf("doc.PageCount() = %d, want unchanged %d (must not append a page to the document)", got, before)
	}
}
