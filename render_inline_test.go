// SPDX-License-Identifier: MIT

package asposepdf

import "testing"

// TestRenderInlineImage draws a 2×2 red inline image (BI/ID/EI) into a 40×40
// box and checks it paints there but not outside.
func TestRenderInlineImage(t *testing.T) {
	doc := NewDocument(100, 100)
	p, _ := doc.Page(1)

	var pix []byte
	for i := 0; i < 4; i++ { // 2×2 RGB, all red
		pix = append(pix, 255, 0, 0)
	}
	content := []byte("q 40 0 0 40 30 30 cm\nBI /W 2 /H 2 /CS /RGB /BPC 8 ID ")
	content = append(content, pix...)
	content = append(content, []byte("\nEI\nQ\n")...)
	if err := p.appendToContentStream(content); err != nil {
		t.Fatal(err)
	}

	img, err := p.RenderImage(RenderOptions{DPI: 72}) // 1px/pt
	if err != nil {
		t.Fatal(err)
	}
	// Box maps user (30,30)-(70,70) → device [30,70]×[30,70].
	if r, g, b, _ := img.At(50, 50).RGBA(); !(r>>8 > 200 && g>>8 < 60 && b>>8 < 60) {
		t.Errorf("inline image centre = (%d,%d,%d), want red", r>>8, g>>8, b>>8)
	}
	if r, _, _, _ := img.At(10, 10).RGBA(); r>>8 < 240 {
		t.Error("area outside the inline image was painted")
	}
}
