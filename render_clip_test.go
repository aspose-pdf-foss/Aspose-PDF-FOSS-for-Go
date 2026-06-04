// SPDX-License-Identifier: MIT

package asposepdf

import "testing"

// TestRenderClipPath checks that W / W* restricts subsequent painting: a
// full-page red fill clipped to the left half must leave the right half white.
func TestRenderClipPath(t *testing.T) {
	doc := NewDocument(100, 100)
	p, _ := doc.Page(1)
	content := []byte("1 0 0 rg\n" + // red fill
		"0 0 50 100 re\n" + // clip rect = left half
		"W n\n" +
		"0 0 100 100 re\n" + // fill the whole page…
		"f\n") // …but only the clipped left half should paint
	if err := p.appendToContentStream(content); err != nil {
		t.Fatalf("appendToContentStream: %v", err)
	}
	img, err := p.RenderImage(RenderOptions{DPI: 72}) // 1px per point → 100×100
	if err != nil {
		t.Fatalf("RenderImage: %v", err)
	}

	check := func(x, y int, wantRed bool) {
		t.Helper()
		r, g, b, _ := img.At(x, y).RGBA()
		isRed := r>>8 > 200 && g>>8 < 60 && b>>8 < 60
		isWhite := r>>8 > 240 && g>>8 > 240 && b>>8 > 240
		if wantRed && !isRed {
			t.Errorf("pixel (%d,%d) = (%d,%d,%d), want red", x, y, r>>8, g>>8, b>>8)
		}
		if !wantRed && !isWhite {
			t.Errorf("pixel (%d,%d) = (%d,%d,%d), want white (clipped out)", x, y, r>>8, g>>8, b>>8)
		}
	}
	check(25, 50, true)  // inside clip → red
	check(75, 50, false) // outside clip → white
	check(10, 10, true)  // inside clip → red
	check(90, 90, false) // outside clip → white
}

// TestRenderClipRestoredByQ checks that a clip set inside q…Q does not leak past
// the Q that restores the graphics state.
func TestRenderClipRestoredByQ(t *testing.T) {
	doc := NewDocument(100, 100)
	p, _ := doc.Page(1)
	content := []byte("q\n" +
		"0 0 50 100 re\nW n\n" + // clip to left half, inside q…Q
		"Q\n" +
		"1 0 0 rg\n0 0 100 100 re\nf\n") // after Q: clip gone, whole page red
	if err := p.appendToContentStream(content); err != nil {
		t.Fatalf("appendToContentStream: %v", err)
	}
	img, err := p.RenderImage(RenderOptions{DPI: 72})
	if err != nil {
		t.Fatalf("RenderImage: %v", err)
	}
	r, g, b, _ := img.At(75, 50).RGBA() // right half must be painted now
	if !(r>>8 > 200 && g>>8 < 60 && b>>8 < 60) {
		t.Errorf("pixel (75,50) = (%d,%d,%d), want red — clip leaked past Q", r>>8, g>>8, b>>8)
	}
}
