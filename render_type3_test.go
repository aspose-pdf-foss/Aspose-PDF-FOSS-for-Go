// SPDX-License-Identifier: MIT

package asposepdf

import "testing"

// TestRenderType3Glyph renders a Type3 font whose single glyph (code 65) is a
// char-proc drawing a red square, and checks the square is painted at the
// expected place.
func TestRenderType3Glyph(t *testing.T) {
	doc := NewDocument(100, 100)
	p, _ := doc.Page(1)

	charProc := &pdfStream{
		Dict:    pdfDict{},
		Data:    []byte("700 0 d0\n1 0 0 rg 0 0 700 700 re f\n"),
		Decoded: true,
	}
	p.pageResources()["/Font"] = pdfDict{"/F1": pdfDict{
		"/Type":       pdfName("/Font"),
		"/Subtype":    pdfName("/Type3"),
		"/FontBBox":   pdfArray{0, 0, 750, 750},
		"/FontMatrix": pdfArray{0.001, 0, 0, 0.001, 0, 0},
		"/CharProcs":  pdfDict{"/sq": charProc},
		"/Encoding":   pdfDict{"/Differences": pdfArray{65, pdfName("/sq")}},
		"/FirstChar":  65,
		"/LastChar":   65,
		"/Widths":     pdfArray{700},
		"/Resources":  pdfDict{},
	}}

	// 'A' == code 65. fontSize 100, FontMatrix 0.001 → glyph 0..700 maps to
	// 0..70 text units; with Td 10,10 the square spans user (10,10)-(80,80).
	if err := p.appendToContentStream([]byte("BT /F1 100 Tf 10 10 Td (A) Tj ET\n")); err != nil {
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
	if !isRed(45, 55) { // inside the square (device x[10,80] y[20,90])
		t.Error("Type3 glyph (red square) not rendered")
	}
	if isRed(5, 5) { // outside the glyph
		t.Error("Type3 glyph painted outside its box")
	}
}
