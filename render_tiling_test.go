// SPDX-License-Identifier: MIT

package asposepdf

import "testing"

// TestRenderTilingPattern fills a rectangle with a PatternType 1 pattern whose
// cell is a red 5×5 square in a 10×10 step, and checks the pattern tiles inside
// the rect (red on the squares, white in the gaps) and nothing paints outside.
func TestRenderTilingPattern(t *testing.T) {
	doc := NewDocument(100, 100)
	p, _ := doc.Page(1)

	p.pageResources()["/Pattern"] = pdfDict{"/P1": &pdfStream{
		Dict: pdfDict{
			"/PatternType": 1,
			"/PaintType":   1,
			"/TilingType":  1,
			"/BBox":        pdfArray{0, 0, 10, 10},
			"/XStep":       10,
			"/YStep":       10,
			"/Matrix":      pdfArray{1, 0, 0, 1, 0, 0},
			"/Resources":   pdfDict{},
		},
		Data:    []byte("1 0 0 rg 0 0 5 5 re f\n"), // red square at each cell origin
		Decoded: true,
	}}

	if err := p.appendToContentStream([]byte("/P1 scn\n20 20 60 60 re f\n")); err != nil {
		t.Fatal(err)
	}
	img, err := p.RenderImage(RenderOptions{DPI: 72}) // 1px/pt
	if err != nil {
		t.Fatal(err)
	}

	isRed := func(x, y int) bool {
		r, g, b, _ := img.At(x, y).RGBA()
		return r>>8 > 200 && g>>8 < 60 && b>>8 < 60
	}
	// user (22,22) → device (22,78): inside the red square of tile (20..25).
	if !isRed(22, 78) {
		t.Error("pattern tile not painted red inside the fill")
	}
	// user (27,27) → device (27,73): in the gap between squares (25..30).
	if isRed(27, 73) {
		t.Error("gap between tiles unexpectedly painted")
	}
	// outside the filled rectangle stays white.
	if isRed(5, 95) {
		t.Error("pattern painted outside the fill path")
	}
}
