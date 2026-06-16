// SPDX-License-Identifier: MIT

package asposepdf

import "testing"

// TestRenderTextClip checks text rendering mode 7 (add glyphs to clip, paint
// nothing): glyphs drawn with "7 Tr" become a clip path at ET, so a full-page
// fill that follows shows only through the glyph shapes — the "glyphs as a
// stencil for an image/fill" idiom (ISO 32000-1 §9.4.3). Without the clip the
// fill would cover the whole page, including the corners.
func TestRenderTextClip(t *testing.T) {
	doc := NewDocument(200, 100)
	p, _ := doc.Page(1)
	p.pageResources()["/Font"] = pdfDict{"/F1": pdfDict{
		"/Type":     pdfName("/Font"),
		"/Subtype":  pdfName("/Type1"),
		"/BaseFont": pdfName("/Helvetica-Bold"),
	}}

	content := "q\n" +
		"BT /F1 80 Tf 7 Tr 10 25 Td (HI) Tj ET\n" + // glyphs → clip
		"0 0 1 rg 0 0 200 100 re f\n" + // blue fill, clipped to the glyphs
		"Q\n"
	if err := p.appendToContentStream([]byte(content)); err != nil {
		t.Fatal(err)
	}

	img, err := p.RenderImage(RenderOptions{DPI: 72}) // 1px/pt
	if err != nil {
		t.Fatal(err)
	}

	isBlue := func(x, y int) bool {
		r, g, b, _ := img.At(x, y).RGBA()
		return r>>8 < 80 && g>>8 < 80 && b>>8 > 150
	}

	// Corners sit outside any glyph: the clip must keep them white.
	for _, c := range [][2]int{{2, 2}, {197, 2}, {2, 97}, {197, 97}} {
		if isBlue(c[0], c[1]) {
			t.Errorf("corner (%d,%d) is blue — text clip not applied (fill leaked)", c[0], c[1])
		}
	}

	// Some blue must appear (the glyphs), but far less than the whole page —
	// otherwise the fill was unclipped.
	blue := 0
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if isBlue(x, y) {
				blue++
			}
		}
	}
	total := b.Dx() * b.Dy()
	if blue == 0 {
		t.Fatal("no blue pixels — glyph clip produced an empty (or missing) region")
	}
	if blue > total/2 {
		t.Errorf("blue pixels = %d/%d (>50%%) — fill not constrained to glyphs", blue, total)
	}
}
