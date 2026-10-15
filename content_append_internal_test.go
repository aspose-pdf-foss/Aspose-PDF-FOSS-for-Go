// SPDX-License-Identifier: MIT

package asposepdf

import (
	"fmt"
	"image"
	"image/color"
	"testing"
)

// TestAppendToContentStreamLeavesNoDeadCopies pins pdf-go-jbiu (GitHub
// issue #16 point 1, shadowhunter1470-svg): repeated AddText calls on one
// page used to leave every previous combined /Contents stream dead in
// d.objects, growing the saved file roughly quadratically with the
// number of calls. After the fix, nothing is ever abandoned -- each call
// adds one array entry, so RemoveUnusedObjects should find NOTHING to
// remove no matter how many calls were made.
func TestAppendToContentStreamLeavesNoDeadCopies(t *testing.T) {
	d := NewDocumentFromFormat(PageFormatA4)
	p, err := d.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	const n = 5
	for i := 0; i < n; i++ {
		if err := p.AddText(fmt.Sprintf("Line%d", i), TextStyle{Font: FontHelvetica, Size: 12},
			Rectangle{LLX: 10, LLY: float64(150 - 20*i), URX: 190, URY: float64(165 - 20*i)}); err != nil {
			t.Fatal(err)
		}
	}

	if removed := d.RemoveUnusedObjects(); removed != 0 {
		t.Errorf("RemoveUnusedObjects removed %d object(s) after %d AddText calls, want 0 (nothing should ever have been abandoned)", removed, n)
	}

	txt, err := p.ExtractText()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < n; i++ {
		want := fmt.Sprintf("Line%d", i)
		count := 0
		for idx := 0; idx+len(want) <= len(txt); idx++ {
			if txt[idx:idx+len(want)] == want {
				count++
			}
		}
		if count != 1 {
			t.Errorf("%q appears %d times in extracted text, want exactly 1", want, count)
		}
	}
}

// TestPrependToContentStreamLeavesNoDeadCopies is
// TestAppendToContentStreamLeavesNoDeadCopies's counterpart for the
// "draw behind everything" direction (pdf-go-jbiu applies symmetrically
// to prependToContentStream, used by Stamp.Background and
// TextStyle.Behind): repeated background stamps on one page must not
// leave any previous combined stream dead either.
func TestPrependToContentStreamLeavesNoDeadCopies(t *testing.T) {
	d := NewDocumentFromFormat(PageFormatA4)
	p, err := d.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.AddText("Foreground", TextStyle{Font: FontHelvetica, Size: 12}, Rectangle{LLX: 10, LLY: 400, URX: 190, URY: 420}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		stamp := NewTextStamp(fmt.Sprintf("BG%d", i), TextStyle{Font: FontHelvetica, Size: 40})
		stamp.Background = true
		if err := p.AddStamp(stamp); err != nil {
			t.Fatal(err)
		}
	}

	if removed := d.RemoveUnusedObjects(); removed != 0 {
		t.Errorf("RemoveUnusedObjects removed %d object(s) after 5 background stamps, want 0", removed)
	}
}

// TestAppendToContentStreamIsolatesFromUnbalancedExistingState pins
// pdf-go-p1yc (GitHub issue #16 point 2, shadowhunter1470-svg): the
// reporter's own repro -- a page whose existing content leaves an active
// half-scale `cm` and an active invisible (`3 Tr`) text render mode open,
// neither ever popped (plausible real-world producer output, not
// something this library's own balanced drawing calls would leave).
// Before the fix, content added afterward inherited that state: new text
// rendered invisible, a new rectangle rendered at half size and the
// wrong position. The fix wraps the pre-existing content in a one-time
// q...Q pair before appending anything new, so new content always starts
// from a clean graphics state regardless of what came before it.
func TestAppendToContentStreamIsolatesFromUnbalancedExistingState(t *testing.T) {
	d := NewDocumentFromFormat(PageFormatA4)
	p, err := d.Page(1)
	if err != nil {
		t.Fatal(err)
	}

	// Simulate a page already carrying unbalanced, third-party-shaped
	// content: a half-scale cm and an invisible text render mode, neither
	// ever reset.
	contentRef, ok := p.pageDict()["/Contents"].(pdfRef)
	if !ok {
		t.Fatal("blank page's /Contents is not a direct pdfRef")
	}
	existing, ok := d.objects[contentRef.Num].Value.(*pdfStream)
	if !ok {
		t.Fatal("blank page's /Contents does not resolve to a stream")
	}
	existing.Data = []byte("0.5 0 0 0.5 0 0 cm 3 Tr 0 0 1 rg 10 10 50 50 re f")
	existing.Decoded = true

	black := Color{R: 0, G: 0, B: 0, A: 1}
	if err := p.AddText("New text", TextStyle{Font: FontHelvetica, Size: 12, Color: &black},
		Rectangle{LLX: 100, LLY: 150, URX: 190, URY: 170}); err != nil {
		t.Fatal(err)
	}
	red := Color{R: 1, G: 0, B: 0, A: 1}
	rectRect := Rectangle{LLX: 120, LLY: 20, URX: 180, URY: 60}
	if err := p.DrawRectangle(rectRect, ShapeStyle{FillColor: &red}); err != nil {
		t.Fatal(err)
	}

	// DPI 72 == 1 point per pixel, so PDF-space rects map directly onto
	// pixel-space rects without any further scale math.
	img, err := p.RenderImage(RenderOptions{DPI: 72})
	if err != nil {
		t.Fatal(err)
	}
	pageH := img.Bounds().Dy()
	toPixel := func(x, y float64) (int, int) {
		return int(x), pageH - int(y)
	}

	// The rectangle, drawn AFTER the inherited half-scale cm, must appear
	// at its real (unscaled) position and size: its center must be red.
	cx, cy := toPixel((rectRect.LLX+rectRect.URX)/2, (rectRect.LLY+rectRect.URY)/2)
	if !isApproxColor(img.At(cx, cy), color.RGBA{R: 255, A: 255}, 40) {
		t.Errorf("rectangle center (%d,%d) = %v, want red (not half-scaled/mispositioned by the inherited cm)", cx, cy, img.At(cx, cy))
	}
	// Where the rectangle would have landed if the half-scale cm HAD
	// leaked through (half size, same origin) must NOT be red -- it
	// should still be background (white), confirming the rect wasn't
	// drawn there instead.
	bx, by := toPixel(rectRect.LLX/2+5, rectRect.LLY/2+5)
	if isApproxColor(img.At(bx, by), color.RGBA{R: 255, A: 255}, 40) {
		t.Errorf("found red at the BUGGY half-scaled position (%d,%d); the inherited cm leaked through", bx, by)
	}

	// The text, drawn AFTER the inherited "3 Tr" (invisible) render mode,
	// must actually be visible: somewhere in its rectangle must be
	// non-background ink.
	if !regionHasNonWhitePixel(img, textPixelRect(toPixel, Rectangle{LLX: 100, LLY: 150, URX: 190, URY: 170})) {
		t.Error("no visible ink found in the new text's rectangle; the inherited invisible (3 Tr) render mode leaked through")
	}
}

// textPixelRect converts a PDF-space rectangle to a pixel-space one using toPixel.
func textPixelRect(toPixel func(x, y float64) (int, int), r Rectangle) image.Rectangle {
	x0, y0 := toPixel(r.LLX, r.URY)
	x1, y1 := toPixel(r.URX, r.LLY)
	return image.Rect(x0, y0, x1, y1)
}

func regionHasNonWhitePixel(img image.Image, r image.Rectangle) bool {
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			if !isApproxColor(img.At(x, y), color.RGBA{R: 255, G: 255, B: 255, A: 255}, 10) {
				return true
			}
		}
	}
	return false
}

func isApproxColor(c color.Color, want color.RGBA, tol int) bool {
	r, g, b, a := c.RGBA()
	wr, wg, wb, wa := want.RGBA()
	diff := func(a, b uint32) int {
		d := int(a>>8) - int(b>>8)
		if d < 0 {
			d = -d
		}
		return d
	}
	return diff(r, wr) <= tol && diff(g, wg) <= tol && diff(b, wb) <= tol && diff(a, wa) <= tol
}
