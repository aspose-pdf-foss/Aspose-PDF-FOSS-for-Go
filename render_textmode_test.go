// SPDX-License-Identifier: MIT

package asposepdf

import (
	"image"
	"testing"
)

// renderText draws "Hello" in Helvetica with the given Tr mode prefix and
// returns the rendered page.
func renderText(t *testing.T, trAndColour string) image.Image {
	t.Helper()
	doc := NewDocument(140, 50)
	p, _ := doc.Page(1)
	p.pageResources()["/Font"] = pdfDict{"/F1": pdfDict{
		"/Type":     pdfName("/Font"),
		"/Subtype":  pdfName("/Type1"),
		"/BaseFont": pdfName("/Helvetica"),
	}}
	content := "BT /F1 32 Tf 8 14 Td " + trAndColour + " (Hello) Tj ET\n"
	if err := p.appendToContentStream([]byte(content)); err != nil {
		t.Fatal(err)
	}
	img, err := p.RenderImage(RenderOptions{DPI: 72})
	if err != nil {
		t.Fatal(err)
	}
	return img
}

// TestTextRenderModeInvisible is the key fix: Tr 3 (invisible) must paint
// nothing, while Tr 0 (fill) paints the glyphs.
func TestTextRenderModeInvisible(t *testing.T) {
	vis := renderText(t, "0 0 0 rg 0 Tr")
	inv := renderText(t, "0 0 0 rg 3 Tr")

	if blackCount(vis, 0, 0, vis.Bounds().Dx(), vis.Bounds().Dy()) == 0 {
		t.Error("fill-mode (Tr 0) text was not painted")
	}
	if n := blackCount(inv, 0, 0, inv.Bounds().Dx(), inv.Bounds().Dy()); n != 0 {
		t.Errorf("invisible-mode (Tr 3) text painted %d black pixels", n)
	}
}

// TestTextRenderModeStroke checks Tr 1 (stroke) paints with the stroke colour,
// not the fill colour: fill is set to a colour we forbid, stroke to black.
func TestTextRenderModeStroke(t *testing.T) {
	// Fill white, stroke black, Tr 1 → only the black outline should appear.
	img := renderText(t, "1 1 1 rg 0 0 0 RG 1 w 1 Tr")
	if blackCount(img, 0, 0, img.Bounds().Dx(), img.Bounds().Dy()) == 0 {
		t.Error("stroke-mode (Tr 1) text was not outlined")
	}
}
