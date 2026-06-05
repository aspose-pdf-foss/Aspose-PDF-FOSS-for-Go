// SPDX-License-Identifier: MIT

package asposepdf

import "testing"

// TestRenderImageMask draws a 2×2 inline image mask (a diagonal: samples 0 at
// the top-left and bottom-right paint) with a red fill colour, and checks the
// mask paints the fill colour on the diagonal and leaves the off-diagonal white.
func TestRenderImageMask(t *testing.T) {
	doc := NewDocument(100, 100)
	p, _ := doc.Page(1)

	// row0 bits [0,1] = 0x40, row1 bits [1,0] = 0x80. /Decode default → sample 0
	// paints, so top-left (0,0) and bottom-right (1,1) paint.
	mask := []byte{0x40, 0x80}
	content := []byte("1 0 0 rg\nq 40 0 0 40 30 30 cm\nBI /W 2 /H 2 /IM true ID ")
	content = append(content, mask...)
	content = append(content, []byte("\nEI\nQ\n")...)
	if err := p.appendToContentStream(content); err != nil {
		t.Fatal(err)
	}

	img, err := p.RenderImage(RenderOptions{DPI: 72}) // unit square → device (30..70)
	if err != nil {
		t.Fatal(err)
	}
	isRed := func(x, y int) bool {
		r, g, b, _ := img.At(x, y).RGBA()
		return r>>8 > 200 && g>>8 < 60 && b>>8 < 60
	}
	isWhite := func(x, y int) bool {
		r, g, b, _ := img.At(x, y).RGBA()
		return r>>8 > 240 && g>>8 > 240 && b>>8 > 240
	}
	if !isRed(40, 40) { // top-left cell → painted
		t.Error("mask did not paint the fill colour on the diagonal (top-left)")
	}
	if !isRed(60, 60) { // bottom-right cell → painted
		t.Error("mask did not paint the fill colour on the diagonal (bottom-right)")
	}
	if !isWhite(60, 40) { // top-right cell → masked out
		t.Error("masked-out sample was painted (top-right)")
	}
}
