// SPDX-License-Identifier: MIT

package asposepdf

import "testing"

// painted reports whether the rendered page has any non-white pixel.
func painted(t *testing.T, content string) bool {
	t.Helper()
	doc := NewDocument(120, 60)
	p, _ := doc.Page(1)
	p.pageResources()["/Font"] = pdfDict{"/ZaDb": pdfDict{
		"/Type":     pdfName("/Font"),
		"/Subtype":  pdfName("/Type1"),
		"/BaseFont": pdfName("/ZapfDingbats"),
	}}
	if err := p.appendToContentStream([]byte(content)); err != nil {
		t.Fatal(err)
	}
	img, err := p.RenderImage(RenderOptions{DPI: 72})
	if err != nil {
		t.Fatal(err)
	}
	for y := 0; y < img.Bounds().Dy(); y++ {
		for x := 0; x < img.Bounds().Dx(); x++ {
			if r, g, b, _ := img.At(x, y).RGBA(); r>>8 < 250 || g>>8 < 250 || b>>8 < 250 {
				return true
			}
		}
	}
	return false
}

// TestZapfDingbatsCheckRendered checks that a non-embedded ZapfDingbats check
// mark (code '4') now renders via the synthesized outline, so checkbox widget
// appearances are no longer empty.
func TestZapfDingbatsCheckRendered(t *testing.T) {
	if !painted(t, "BT /ZaDb 40 Tf 8 14 Td (4) Tj ET\n") {
		t.Error("ZapfDingbats check mark '4' rendered nothing — synthesized glyph missing")
	}
}

// TestZapfDingbatsMarksRendered covers the other canonical checkbox/radio marks.
func TestZapfDingbatsMarksRendered(t *testing.T) {
	for _, code := range []string{"8", "l", "n", "u", "H"} { // cross, circle, square, diamond, star
		if !painted(t, "BT /ZaDb 40 Tf 8 14 Td ("+code+") Tj ET\n") {
			t.Errorf("ZapfDingbats mark %q rendered nothing", code)
		}
	}
}

// TestZapfDingbatsUnmappedSilent checks that a code without a synthesized shape
// draws nothing rather than tofu.
func TestZapfDingbatsUnmappedSilent(t *testing.T) {
	if painted(t, "BT /ZaDb 40 Tf 8 14 Td (a) Tj ET\n") { // 'a' (0x61) is not in our synth set
		t.Error("unmapped ZapfDingbats code painted something — expected nothing")
	}
}

// TestZapfDingbatsContoursTable spot-checks the synth table directly.
func TestZapfDingbatsContoursTable(t *testing.T) {
	if zapfDingbatsContours('4') == nil {
		t.Error("code '4' should synthesize a check mark")
	}
	if zapfDingbatsContours('a') != nil {
		t.Error("code 'a' should not be synthesized")
	}
}
