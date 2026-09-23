// SPDX-License-Identifier: MIT

package asposepdf_test

import (
	"bytes"
	"image"
	"testing"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

// buildTransparentPageDoc returns a 2-page document: page 1 has an
// alpha-filled rectangle (Color.A < 1, which goes through ExtGState /ca —
// see CLAUDE.md's vector.go section), page 2 is plain opaque text.
func buildTransparentPageDoc(t *testing.T) *pdf.Document {
	t.Helper()
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	p1, err := doc.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	fill := pdf.Color{R: 1, G: 0, B: 0, A: 0.5}
	if err := p1.DrawRectangle(pdf.Rectangle{LLX: 50, LLY: 700, URX: 300, URY: 780},
		pdf.ShapeStyle{FillColor: &fill}); err != nil {
		t.Fatal(err)
	}
	if err := doc.AddBlankPageFromFormat(pdf.PageFormatA4); err != nil {
		t.Fatal(err)
	}
	p2, err := doc.Page(2)
	if err != nil {
		t.Fatal(err)
	}
	if err := p2.AddText("plain opaque text", pdf.TextStyle{Size: 12},
		pdf.Rectangle{LLX: 50, LLY: 700, URX: 300, URY: 720}); err != nil {
		t.Fatal(err)
	}
	return doc
}

func TestFlattenTransparencyFlattensOnlyAffectedPages(t *testing.T) {
	doc := buildTransparentPageDoc(t)

	p2, err := doc.Page(2)
	if err != nil {
		t.Fatal(err)
	}
	before, err := p2.ExtractText()
	if err != nil {
		t.Fatal(err)
	}
	if before != "plain opaque text" {
		t.Fatalf("page 2 setup: got %q", before)
	}

	n, err := doc.FlattenTransparency()
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("FlattenTransparency returned %d, want 1", n)
	}

	p1, err := doc.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	p2, err = doc.Page(2)
	if err != nil {
		t.Fatal(err)
	}

	// Page 1 (flattened): content is now a raster, so nothing extracts.
	after1, err := p1.ExtractText()
	if err != nil {
		t.Fatal(err)
	}
	if after1 != "" {
		t.Errorf("flattened page still extracts text: %q", after1)
	}
	// But it still visually contains the rectangle.
	if !hasNonWhitePixel(t, p1) {
		t.Error("flattened page renders as blank")
	}

	// Page 2 (untouched): still fully vector, text still extracts.
	after2, err := p2.ExtractText()
	if err != nil {
		t.Fatal(err)
	}
	if after2 != "plain opaque text" {
		t.Errorf("untouched page 2 text changed: got %q", after2)
	}
}

func TestFlattenTransparencyNoOpWithoutTransparency(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	p, err := doc.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.AddText("hello", pdf.TextStyle{Size: 12}, pdf.Rectangle{LLX: 50, LLY: 700, URX: 300, URY: 720}); err != nil {
		t.Fatal(err)
	}

	n, err := doc.FlattenTransparency()
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("FlattenTransparency returned %d, want 0 (no transparency present)", n)
	}
	text, err := p.ExtractText()
	if err != nil {
		t.Fatal(err)
	}
	if text != "hello" {
		t.Errorf("text changed: got %q", text)
	}
}

func TestFlattenTransparencyPreservesAnnotations(t *testing.T) {
	doc := buildTransparentPageDoc(t)
	p1, err := doc.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	link := pdf.NewLinkAnnotation(p1, pdf.Rectangle{LLX: 60, LLY: 710, URX: 120, URY: 730})
	link.SetAction(pdf.NewGoToURIAction("https://example.com/"))
	if err := p1.Annotations().Add(link); err != nil {
		t.Fatal(err)
	}

	if _, err := doc.FlattenTransparency(); err != nil {
		t.Fatal(err)
	}

	p1, err = doc.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	annots := p1.Annotations()
	if annots.Count() != 1 {
		t.Fatalf("annotations after flatten = %d, want 1", annots.Count())
	}
	got, ok := annots.At(0).(*pdf.LinkAnnotation)
	if !ok {
		t.Fatalf("annotation is %T, want *LinkAnnotation", annots.At(0))
	}
	if got.Action() == nil {
		t.Error("link action lost after flatten")
	}
}

func TestFlattenTransparencyClearsPDFAIssue(t *testing.T) {
	doc := buildTransparentPageDoc(t)

	before := doc.ValidatePDFA(pdf.PDFA1B)
	foundBefore := false
	for _, iss := range before.Issues {
		if iss.Rule == "TRANSPARENCY" {
			foundBefore = true
		}
	}
	if !foundBefore {
		t.Fatal("setup: expected TRANSPARENCY issue before flattening")
	}

	if _, err := doc.FlattenTransparency(); err != nil {
		t.Fatal(err)
	}

	after := doc.ValidatePDFA(pdf.PDFA1B)
	for _, iss := range after.Issues {
		if iss.Rule == "TRANSPARENCY" {
			t.Errorf("TRANSPARENCY issue still present after FlattenTransparency: %s", iss.Message)
		}
	}
}

func TestFlattenTransparencyPagesOption(t *testing.T) {
	doc := buildTransparentPageDoc(t)
	// Restrict to page 2 (no transparency there) — page 1 must stay untouched.
	n, err := doc.FlattenTransparency(pdf.FlattenTransparencyOptions{Pages: []int{2}})
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("got %d, want 0 (page 2 has no transparency)", n)
	}
	p1, err := doc.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	// Page 1's transparency is still there — round trip should still flag it.
	r := doc.ValidatePDFA(pdf.PDFA1B)
	found := false
	for _, iss := range r.Issues {
		if iss.Rule == "TRANSPARENCY" {
			found = true
		}
	}
	if !found {
		t.Error("page 1 was flattened despite being excluded by the Pages option")
	}
	_ = p1
}

func TestFlattenTransparencyPagesOptionIncludes(t *testing.T) {
	doc := buildTransparentPageDoc(t)
	// Restrict to page 1 (the one with transparency) explicitly.
	n, err := doc.FlattenTransparency(pdf.FlattenTransparencyOptions{Pages: []int{1}})
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("got %d, want 1 (page 1 has transparency)", n)
	}
	r := doc.ValidatePDFA(pdf.PDFA1B)
	for _, iss := range r.Issues {
		if iss.Rule == "TRANSPARENCY" {
			t.Errorf("TRANSPARENCY issue still present: %s", iss.Message)
		}
	}
}

func TestFlattenTransparencyOutOfRangePage(t *testing.T) {
	doc := buildTransparentPageDoc(t)
	if _, err := doc.FlattenTransparency(pdf.FlattenTransparencyOptions{Pages: []int{99}}); err == nil {
		t.Error("expected an error for an out-of-range page number")
	}
}

func TestFlattenTransparencyRoundTrip(t *testing.T) {
	doc := buildTransparentPageDoc(t)
	if _, err := doc.FlattenTransparency(); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	out, err := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if out.PageCount() != 2 {
		t.Fatalf("page count after round trip = %d, want 2", out.PageCount())
	}
	p1, err := out.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	if !hasNonWhitePixel(t, p1) {
		t.Error("flattened page lost its content across a Save+Open round trip")
	}
}

// TestFlattenTransparencyPreservesRotation guards renderContentForFlatten's
// deliberate choice to rasterize in the page's own unrotated coordinate
// space rather than the rotated space RenderImage normally produces (see
// flatten_transparency.go): a rotated page's replacement image must render
// the same way as the same page rotated but never flattened, not
// double-rotated or un-rotated. Previously verified only by an ad hoc
// visual check outside the test suite; this pins it down as an automated
// regression test.
//
// The comparison is a small-tolerance byte diff, not exact equality:
// deviceMatrix rounds the embed-time pixel dimensions (roundPx) and the
// image is then rescaled to the box's exact point dimensions on redisplay,
// so a few tenths of a percent of bytes legitimately differ at content
// edges from ordinary sub-pixel resampling — the same thing any
// rasterize-then-redisplay pipeline does, not a defect. A real
// double-rotation/flip/translation bug differs everywhere (empirically
// >50% of bytes for this fixture), nowhere close to the threshold below.
func TestFlattenTransparencyPreservesRotation(t *testing.T) {
	const dpi = 150
	build := func(t *testing.T) *pdf.Document {
		t.Helper()
		doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
		p, err := doc.Page(1)
		if err != nil {
			t.Fatal(err)
		}
		fill := pdf.Color{R: 0, G: 0.4, B: 1, A: 0.5}
		if err := p.DrawCircle(pdf.Point{X: 200, Y: 700}, 80, pdf.ShapeStyle{FillColor: &fill}); err != nil {
			t.Fatal(err)
		}
		if err := p.AddText("ROTATION TEST", pdf.TextStyle{Size: 24},
			pdf.Rectangle{LLX: 50, LLY: 400, URX: 500, URY: 440}); err != nil {
			t.Fatal(err)
		}
		if err := doc.Rotate(pdf.Rotate90, 1); err != nil {
			t.Fatal(err)
		}
		return doc
	}

	reference := build(t)
	pRef, err := reference.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	wantImg, err := pRef.RenderImage(pdf.RenderOptions{DPI: dpi})
	if err != nil {
		t.Fatal(err)
	}

	flattenedDoc := build(t)
	if _, err := flattenedDoc.FlattenTransparency(pdf.FlattenTransparencyOptions{DPI: dpi}); err != nil {
		t.Fatal(err)
	}
	pFlat, err := flattenedDoc.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	gotImg, err := pFlat.RenderImage(pdf.RenderOptions{DPI: dpi})
	if err != nil {
		t.Fatal(err)
	}

	wantRGBA, ok1 := wantImg.(*image.RGBA)
	gotRGBA, ok2 := gotImg.(*image.RGBA)
	if !ok1 || !ok2 {
		t.Fatalf("expected *image.RGBA from RenderImage, got %T / %T", wantImg, gotImg)
	}
	if wantRGBA.Bounds() != gotRGBA.Bounds() {
		t.Fatalf("bounds differ: %v vs %v", wantRGBA.Bounds(), gotRGBA.Bounds())
	}

	diffBytes := 0
	for i := range wantRGBA.Pix {
		d := int(wantRGBA.Pix[i]) - int(gotRGBA.Pix[i])
		if d != 0 {
			diffBytes++
		}
	}
	fraction := float64(diffBytes) / float64(len(wantRGBA.Pix))
	const maxDiffFraction = 0.02 // generous; observed baseline is ~0.001
	if fraction > maxDiffFraction {
		t.Errorf("flattened rotated page differs from the unflattened original in %.3f%% of bytes (want <%.0f%%) — rotation may have been baked in twice, lost, or otherwise mis-transformed",
			fraction*100, maxDiffFraction*100)
	}
}
