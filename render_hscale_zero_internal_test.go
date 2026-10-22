// SPDX-License-Identifier: MIT

package asposepdf

import (
	"fmt"
	"image"
	"testing"
)

// pdf-go-ifpy point 1: "0 Tz" squeezes glyphs to zero width (ISO 32000-1
// §9.3.4 — Th scales the glyph horizontally; pdf.js and MuPDF render
// nothing). The renderer used to read 0 as "unset" and draw the text at
// 100%, disagreeing with text extraction and the redaction rewriter,
// which both take 0 literally.

// renderWithHScale draws a line of heavy text under the given Tz and
// returns the rendered page.
func renderWithHScale(t *testing.T, tz string) image.Image {
	t.Helper()
	doc := NewDocument(300, 120)
	p, err := doc.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	res, _, _, _, _, err := p.resolveFontForPage(FontHelveticaBold, 40)
	if err != nil {
		t.Fatal(err)
	}
	content := fmt.Sprintf("BT %s 40 Tf %s Tz 20 40 Td (MMMMMM) Tj ET", res, tz)
	if err := replacePageContents(p, []byte(content)); err != nil {
		t.Fatal(err)
	}
	img, err := p.RenderImage(RenderOptions{DPI: 72})
	if err != nil {
		t.Fatal(err)
	}
	return img
}

// darkPixels counts pixels noticeably darker than the white background.
func darkPixels(img image.Image) int {
	n := 0
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bl, _ := img.At(x, y).RGBA()
			if (r+g+bl)/3 < 0x8000 {
				n++
			}
		}
	}
	return n
}

func TestRenderZeroHorizontalScalingDrawsNothing(t *testing.T) {
	if n := darkPixels(renderWithHScale(t, "100")); n == 0 {
		t.Fatal("control: text at 100 Tz rendered no ink — the test page itself is broken")
	}
	if n := darkPixels(renderWithHScale(t, "0")); n != 0 {
		t.Errorf("text at 0 Tz rendered %d dark pixels, want 0 (zero-width glyphs)", n)
	}
}
