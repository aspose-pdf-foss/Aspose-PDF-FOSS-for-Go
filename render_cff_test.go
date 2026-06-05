// SPDX-License-Identifier: MIT

package asposepdf

import (
	"image"
	"testing"
)

// TestRenderCFFGlyph drives the renderer's glyph path with a CFF-backed font:
// it paints the hand-built square glyph (glyph 1) and checks the rasterizer
// filled the expected region, proving CFF outlines flow through paintGlyph.
func TestRenderCFFGlyph(t *testing.T) {
	cff, err := parseCFF(buildMinimalCFF())
	if err != nil {
		t.Fatal(err)
	}
	rfont := &renderFont{cff: cff, em: cff.unitsPerEm, isType0: true}

	img := image.NewRGBA(image.Rect(0, 0, 100, 100))
	fillBackground(img, nil)
	rd := &renderer{
		img:  img,
		w:    100,
		h:    100,
		base: identityMatrix(),
		ras:  newRasterizer(100, 100),
		gs:   gstate{ctm: identityMatrix(), fillA: 1},
		ts:   textState{hScale: 1, fontSize: cff.unitsPerEm, tm: identityMatrix()},
	}
	// glyphOutline must produce the square's contour first.
	if got := rfont.glyphOutline(1); len(got) != 1 {
		t.Fatalf("glyphOutline(1) = %d contours, want 1", len(got))
	}

	rd.paintGlyph(rfont, 1)

	// The square spans em (10..90); at fontSize == em and identity matrices it
	// maps to device pixels (10..90). Its interior must be filled, the corner
	// outside it must stay white.
	if !isBlack(img, 50, 50) {
		t.Error("CFF square interior not filled")
	}
	if isBlack(img, 2, 2) {
		t.Error("area outside the CFF glyph was painted")
	}
}
