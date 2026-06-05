// SPDX-License-Identifier: MIT

package asposepdf

import "testing"

// TestRenderSoftMaskLuminosity applies a luminosity soft mask whose group paints
// a white square over the left half (→ mask 1 left, 0 right), then fills the
// whole page red: only the unmasked left half should paint.
func TestRenderSoftMaskLuminosity(t *testing.T) {
	doc := NewDocument(100, 100)
	p, _ := doc.Page(1)

	group := &pdfStream{
		Dict: pdfDict{
			"/Type":      pdfName("/XObject"),
			"/Subtype":   pdfName("/Form"),
			"/BBox":      pdfArray{0, 0, 100, 100},
			"/Group":     pdfDict{"/S": pdfName("/Transparency")},
			"/Resources": pdfDict{},
		},
		Data:    []byte("1 1 1 rg 0 0 50 100 re f\n"), // white over the left half
		Decoded: true,
	}
	p.pageResources()["/ExtGState"] = pdfDict{"/GS1": pdfDict{
		"/SMask": pdfDict{"/S": pdfName("/Luminosity"), "/G": group},
	}}

	if err := p.appendToContentStream([]byte("/GS1 gs\n1 0 0 rg 0 0 100 100 re f\n")); err != nil {
		t.Fatal(err)
	}
	img, err := p.RenderImage(RenderOptions{DPI: 72})
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
	if !isRed(25, 50) {
		t.Error("unmasked (left) half not painted red")
	}
	if !isWhite(75, 50) {
		t.Error("masked (right) half was painted — soft mask not applied")
	}
}
