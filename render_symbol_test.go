// SPDX-License-Identifier: MIT

package asposepdf

import "testing"

// TestSymbolEncodingApplied checks the Symbol font's built-in encoding maps code
// 'a' (0x61) to a Greek letter (α) rather than Latin 'a', so embedded or
// registered Symbol fonts select the right glyphs.
func TestSymbolEncodingApplied(t *testing.T) {
	fi := resolveFont(map[int]*pdfObject{}, pdfDict{
		"/Subtype":  pdfName("/Type1"),
		"/BaseFont": pdfName("/Symbol"),
	})
	r := fi.encoding['a']
	if r < 0x0370 || r > 0x03FF { // Greek and Coptic block
		t.Errorf("Symbol encoding for 'a' = U+%04X, want a Greek letter", r)
	}

	zd := resolveFont(map[int]*pdfObject{}, pdfDict{
		"/Subtype":  pdfName("/Type1"),
		"/BaseFont": pdfName("/ZapfDingbats"),
	})
	if zd.encoding[0x61] == 'a' {
		t.Error("ZapfDingbats encoding not applied (code 0x61 still Latin 'a')")
	}
}

// TestRenderSymbolNoGarbage checks that a non-embedded Symbol font (no covering
// font registered) renders nothing rather than substituting a Latin font and
// drawing .notdef boxes.
func TestRenderSymbolNoGarbage(t *testing.T) {
	doc := NewDocument(120, 60)
	p, _ := doc.Page(1)
	p.pageResources()["/Font"] = pdfDict{"/F1": pdfDict{
		"/Type":     pdfName("/Font"),
		"/Subtype":  pdfName("/Type1"),
		"/BaseFont": pdfName("/Symbol"),
	}}
	if err := p.appendToContentStream([]byte("BT /F1 40 Tf 8 20 Td (abcde) Tj ET\n")); err != nil {
		t.Fatal(err)
	}
	img, err := p.RenderImage(RenderOptions{DPI: 72})
	if err != nil {
		t.Fatal(err)
	}
	for y := 0; y < img.Bounds().Dy(); y++ {
		for x := 0; x < img.Bounds().Dx(); x++ {
			if r, g, b, _ := img.At(x, y).RGBA(); r>>8 < 250 || g>>8 < 250 || b>>8 < 250 {
				t.Fatalf("non-embedded Symbol painted at (%d,%d) — expected nothing", x, y)
			}
		}
	}
}
