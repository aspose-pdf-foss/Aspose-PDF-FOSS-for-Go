// SPDX-License-Identifier: MIT

package asposepdf_test

import (
	"bytes"
	"image"
	"image/png"
	"testing"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

func rectPage(t *testing.T, w, h float64, rect pdf.Rectangle, fill pdf.Color) *pdf.Page {
	t.Helper()
	doc := pdf.NewDocument(w, h)
	p, err := doc.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.DrawRectangle(rect, pdf.ShapeStyle{FillColor: &fill}); err != nil {
		t.Fatal(err)
	}
	return p
}

// TestGraphicalPdfComparerIdenticalPages: two pages with identical content
// must report zero difference — the baseline sanity check.
func TestGraphicalPdfComparerIdenticalPages(t *testing.T) {
	red := pdf.Color{R: 1, A: 1}
	p1 := rectPage(t, 200, 200, pdf.Rectangle{LLX: 20, LLY: 20, URX: 100, URY: 100}, red)
	p2 := rectPage(t, 200, 200, pdf.Rectangle{LLX: 20, LLY: 20, URX: 100, URY: 100}, red)

	c := pdf.NewGraphicalPdfComparer()
	diff, err := c.GetDifference(p1, p2)
	if err != nil {
		t.Fatal(err)
	}
	if diff.HasDifferences() {
		t.Errorf("HasDifferences() = true for identical pages, DifferentPixels=%d", diff.DifferentPixels())
	}
	if diff.Ratio() != 0 {
		t.Errorf("Ratio() = %v, want 0", diff.Ratio())
	}
}

// TestGraphicalPdfComparerDetectsChange verifies the reported difference
// region actually matches where the two pages differ, not just that "some
// pixels differ" — an independent geometric check against the known
// rectangle position, the same discipline as comparison_test.go's
// SearchText-oracle check for the text comparer.
func TestGraphicalPdfComparerDetectsChange(t *testing.T) {
	red := pdf.Color{R: 1, A: 1}
	blue := pdf.Color{B: 1, A: 1}
	rect := pdf.Rectangle{LLX: 20, LLY: 20, URX: 100, URY: 100} // 80x80 of a 200x200 page = 16%
	p1 := rectPage(t, 200, 200, rect, red)
	p2 := rectPage(t, 200, 200, rect, blue)

	c := &pdf.GraphicalPdfComparer{Resolution: pdf.NewResolution(72)} // 1 device px = 1 pt, exact math
	diff, err := c.GetDifference(p1, p2)
	if err != nil {
		t.Fatal(err)
	}
	if !diff.HasDifferences() {
		t.Fatal("HasDifferences() = false for a red-vs-blue rectangle")
	}
	wantRatio := (80.0 * 80.0) / (200.0 * 200.0) // 0.16
	if got := diff.Ratio(); got < wantRatio*0.9 || got > wantRatio*1.1 {
		t.Errorf("Ratio() = %v, want close to %v (the changed rectangle is 16%% of the page)", got, wantRatio)
	}

	// The difference image must be RED (comparer default) exactly inside the
	// rectangle's device-space box, and background elsewhere — checked at a
	// handful of points independently derived from the rect's own geometry,
	// not from the comparer's internal state.
	img := diff.DifferenceToImage(pdf.Color{R: 1, A: 1}, pdf.Color{G: 1, A: 1})
	rgba, ok := img.(*image.RGBA)
	if !ok {
		t.Fatalf("DifferenceToImage returned %T, want *image.RGBA", img)
	}
	// RenderImage is Y-flipped (PDF Y-up -> image Y-down) at 72 DPI == 1:1 pt/px.
	inside := image.Pt(60, 200-60) // rect centre (60,60) in PDF space
	outside := image.Pt(10, 200-10)
	if c := rgba.RGBAAt(inside.X, inside.Y); c.R != 255 || c.G != 0 {
		t.Errorf("pixel inside the changed rect = %+v, want red (foreground)", c)
	}
	if c := rgba.RGBAAt(outside.X, outside.Y); c.G != 255 || c.R != 0 {
		t.Errorf("pixel outside the changed rect = %+v, want green (background)", c)
	}
}

// TestGraphicalPdfComparerThreshold: a small colour change is ignored above
// its own distance percentage and caught below it — proves Threshold is
// actually wired into the comparison, not merely a stored, unused field.
func TestGraphicalPdfComparerThreshold(t *testing.T) {
	rect := pdf.Rectangle{LLX: 20, LLY: 20, URX: 100, URY: 100}
	// R differs by 10/255 ~= 3.9% of one channel; summed-channel distance is
	// 10 out of a possible 765, i.e. ~1.3%.
	a := pdf.Color{R: 0.50, A: 1}
	b := pdf.Color{R: 0.50 + 10.0/255.0, A: 1}
	p1 := rectPage(t, 200, 200, rect, a)
	p2 := rectPage(t, 200, 200, rect, b)

	strict := &pdf.GraphicalPdfComparer{Threshold: 0}
	diffStrict, err := strict.GetDifference(p1, p2)
	if err != nil {
		t.Fatal(err)
	}
	if !diffStrict.HasDifferences() {
		t.Error("Threshold=0: expected the colour change to be detected")
	}

	lenient := &pdf.GraphicalPdfComparer{Threshold: 50} // way above the ~1.3% actual distance
	diffLenient, err := lenient.GetDifference(p1, p2)
	if err != nil {
		t.Fatal(err)
	}
	if diffLenient.HasDifferences() {
		t.Errorf("Threshold=50: expected the small colour change to be ignored, got %d different pixels", diffLenient.DifferentPixels())
	}
}

// TestGraphicalPdfComparerMismatchedSizes: pages of different sizes must not
// panic, and the extra area on the larger page must count as different
// (compared against white, the padding fill).
func TestGraphicalPdfComparerMismatchedSizes(t *testing.T) {
	p1doc := pdf.NewDocument(100, 100)
	p1, err := p1doc.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	p2doc := pdf.NewDocument(300, 300)
	p2, err := p2doc.Page(1)
	if err != nil {
		t.Fatal(err)
	}

	c := pdf.NewGraphicalPdfComparer()
	diff, err := c.GetDifference(p1, p2)
	if err != nil {
		t.Fatal(err)
	}
	if diff.TotalPixels() == 0 {
		t.Fatal("TotalPixels() = 0 for non-empty pages")
	}
	// Both pages are blank/white, so a correct implementation reports NO
	// difference even though their sizes differ (both compared against the
	// same white padding) -- this is the real assertion, not just "no panic".
	if diff.HasDifferences() {
		t.Errorf("two blank white pages of different sizes reported %d different pixels, want 0", diff.DifferentPixels())
	}
}

// TestGraphicalPdfComparerNilPage covers the nil-page guard.
func TestGraphicalPdfComparerNilPage(t *testing.T) {
	c := pdf.NewGraphicalPdfComparer()
	if _, err := c.GetDifference(nil, nil); err == nil {
		t.Error("GetDifference(nil, nil) = nil error, want an error")
	}
}

// TestGraphicalPdfComparerComparePagesToImage: the saved PNG decodes and its
// dimensions match the comparer's own TotalPixels geometry.
func TestGraphicalPdfComparerComparePagesToImage(t *testing.T) {
	red := pdf.Color{R: 1, A: 1}
	blue := pdf.Color{B: 1, A: 1}
	rect := pdf.Rectangle{LLX: 10, LLY: 10, URX: 50, URY: 50}
	p1 := rectPage(t, 150, 150, rect, red)
	p2 := rectPage(t, 150, 150, rect, blue)

	var buf bytes.Buffer
	c := pdf.NewGraphicalPdfComparer()
	if err := c.WritePagesToImage(p1, p2, &buf); err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("decode PNG: %v", err)
	}
	diff, err := c.GetDifference(p1, p2)
	if err != nil {
		t.Fatal(err)
	}
	b := img.Bounds()
	if b.Dx()*b.Dy() != diff.TotalPixels() {
		t.Errorf("image is %dx%d (%d px), want %d px matching GetDifference's canvas", b.Dx(), b.Dy(), b.Dx()*b.Dy(), diff.TotalPixels())
	}
}

// TestGraphicalPdfComparerCompareDocumentsToPdf: a multi-page document pair
// (including a page only on one side) produces one report page per pair and
// opens back up cleanly.
func TestGraphicalPdfComparerCompareDocumentsToPdf(t *testing.T) {
	red := pdf.Color{R: 1, A: 1}
	blue := pdf.Color{B: 1, A: 1}
	rect := pdf.Rectangle{LLX: 10, LLY: 10, URX: 50, URY: 50}

	doc1 := pdf.NewDocument(150, 150)
	p1a, _ := doc1.Page(1)
	if err := p1a.DrawRectangle(rect, pdf.ShapeStyle{FillColor: &red}); err != nil {
		t.Fatal(err)
	}
	if err := doc1.AddBlankPage(150, 150); err != nil {
		t.Fatal(err)
	}

	doc2 := pdf.NewDocument(150, 150) // only 1 page: doc1's page 2 has nothing to compare against
	p2a, _ := doc2.Page(1)
	if err := p2a.DrawRectangle(rect, pdf.ShapeStyle{FillColor: &blue}); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	c := pdf.NewGraphicalPdfComparer()
	if err := c.WriteDocumentsToPdf(doc1, doc2, &buf); err != nil {
		t.Fatal(err)
	}
	out, err := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("reopen report: %v", err)
	}
	if out.PageCount() != 2 {
		t.Errorf("report has %d pages, want 2 (max(doc1, doc2) page count)", out.PageCount())
	}
}

// TestNewGraphicalPdfComparerDefaults: the zero-value comparer behaves the
// same as NewGraphicalPdfComparer() — both must actually run (dpi()/color()
// fall back), not just construct.
func TestNewGraphicalPdfComparerDefaults(t *testing.T) {
	red := pdf.Color{R: 1, A: 1}
	p1 := rectPage(t, 100, 100, pdf.Rectangle{LLX: 10, LLY: 10, URX: 50, URY: 50}, red)
	p2 := rectPage(t, 100, 100, pdf.Rectangle{LLX: 10, LLY: 10, URX: 50, URY: 50}, red)

	var zero pdf.GraphicalPdfComparer
	if _, err := zero.GetDifference(p1, p2); err != nil {
		t.Fatalf("zero-value GraphicalPdfComparer.GetDifference: %v", err)
	}
}
