// SPDX-License-Identifier: MIT

package asposepdf_test

import (
	"bytes"
	"fmt"
	"image"
	"strings"
	"testing"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

// buildPDFWithNarrowSubstitutedFont returns a one-page PDF using a
// non-embedded TrueType font that no system has installed, whose declared
// /Widths (300/1000 em) are far narrower than any substitute face. Painting
// the substitute glyphs at natural width would overlap and overshoot the
// declared advances (46921.pdf, "Sweet Hipster").
func buildPDFWithNarrowSubstitutedFont() []byte {
	var buf bytes.Buffer
	buf.WriteString("%PDF-1.4\n")

	offsets := map[int]int{}
	writeObj := func(id int, body string) {
		offsets[id] = buf.Len()
		fmt.Fprintf(&buf, "%d 0 obj\n%s\nendobj\n", id, body)
	}

	content := "BT /F1 72 Tf 1 0 0 1 20 700 Tm (AAAAA) Tj ET"
	writeObj(1, "<< /Type /Catalog /Pages 2 0 R >>")
	writeObj(2, "<< /Type /Pages /Count 1 /Kids [3 0 R] /MediaBox [0 0 612 792] >>")
	writeObj(3, "<< /Type /Page /Parent 2 0 R /Contents 4 0 R"+
		" /Resources << /Font << /F1 5 0 R >> >> >>")
	writeObj(4, fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(content), content))
	writeObj(5, "<< /Type /Font /Subtype /TrueType /BaseFont /NoSuchScriptFace"+
		" /FirstChar 65 /LastChar 65 /Widths [300] /Encoding /WinAnsiEncoding >>")

	xrefOff := buf.Len()
	fmt.Fprintf(&buf, "xref\n0 6\n0000000000 65535 f \n")
	for i := 1; i <= 5; i++ {
		fmt.Fprintf(&buf, "%010d 00000 n \n", offsets[i])
	}
	fmt.Fprintf(&buf, "trailer\n<< /Size 6 /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", xrefOff)
	return buf.Bytes()
}

// TestSubstitutedGlyphCondensedToDeclaredWidth verifies that glyphs of a
// substituted (non-embedded, not installed) font are condensed horizontally
// to the document's declared /Widths the way Acrobat and MuPDF do, instead of
// painting at the substitute's natural width and overlapping. Five "A"s at 72pt
// with width 300/1000 must end near x = 20 + 5*0.3*72 = 128pt; the Arimo
// substitute's natural 'A' (~667/1000) would push ink past 150pt.
func TestSubstitutedGlyphCondensedToDeclaredWidth(t *testing.T) {
	doc, err := pdf.OpenStream(bytes.NewReader(buildPDFWithNarrowSubstitutedFont()))
	if err != nil {
		t.Fatalf("OpenStream: %v", err)
	}
	page, err := doc.Page(1)
	if err != nil {
		t.Fatalf("Page(1): %v", err)
	}
	img, err := page.RenderImage(pdf.RenderOptions{DPI: 72})
	if err != nil {
		t.Fatalf("RenderImage: %v", err)
	}
	rgba, ok := img.(*image.RGBA)
	if !ok {
		t.Fatalf("expected *image.RGBA, got %T", img)
	}
	// Scan the text band (PDF y 700..772 → device rows 20..92) for the
	// rightmost dark pixel.
	maxX := -1
	b := rgba.Bounds()
	for y := 20; y < 92 && y < b.Max.Y; y++ {
		for x := b.Max.X - 1; x >= 0; x-- {
			r, g, bl, _ := rgba.At(x, y).RGBA()
			if r < 0x8000 && g < 0x8000 && bl < 0x8000 {
				if x > maxX {
					maxX = x
				}
				break
			}
		}
	}
	if maxX < 0 {
		t.Fatal("no text ink found in expected band")
	}
	// Declared advances end at 128pt = 128px at 72 DPI; allow a little slack.
	if maxX > 140 {
		t.Errorf("rightmost text ink at x=%dpx, want <= 140 (glyphs not condensed to declared /Widths)", maxX)
	}
}

// buildPDFWithMissingFontResource returns a one-page PDF whose content stream
// selects /F1 while the page Resources declare an EMPTY /Font dict (producer
// bug, 44963.pdf). Viewers substitute a default text face instead of dropping
// the text.
func buildPDFWithMissingFontResource() []byte {
	var buf bytes.Buffer
	buf.WriteString("%PDF-1.4\n")

	offsets := map[int]int{}
	writeObj := func(id int, body string) {
		offsets[id] = buf.Len()
		fmt.Fprintf(&buf, "%d 0 obj\n%s\nendobj\n", id, body)
	}

	content := "BT /F1 24 Tf 1 0 0 1 50 700 Tm (Hello) Tj ET"
	writeObj(1, "<< /Type /Catalog /Pages 2 0 R >>")
	writeObj(2, "<< /Type /Pages /Count 1 /Kids [3 0 R] /MediaBox [0 0 612 792] >>")
	writeObj(3, "<< /Type /Page /Parent 2 0 R /Contents 4 0 R"+
		" /Resources << /Font << >> >> >>")
	writeObj(4, fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(content), content))

	xrefOff := buf.Len()
	fmt.Fprintf(&buf, "xref\n0 5\n0000000000 65535 f \n")
	for i := 1; i <= 4; i++ {
		fmt.Fprintf(&buf, "%010d 00000 n \n", offsets[i])
	}
	fmt.Fprintf(&buf, "trailer\n<< /Size 5 /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", xrefOff)
	return buf.Bytes()
}

// TestMissingFontResourceSubstituted verifies that text selecting a font
// absent from /Resources/Font still extracts and renders through the default
// Helvetica substitute (regression: 44963.pdf rendered its chart but no text).
func TestMissingFontResourceSubstituted(t *testing.T) {
	doc, err := pdf.OpenStream(bytes.NewReader(buildPDFWithMissingFontResource()))
	if err != nil {
		t.Fatalf("OpenStream: %v", err)
	}
	page, err := doc.Page(1)
	if err != nil {
		t.Fatalf("Page(1): %v", err)
	}
	txt, err := page.ExtractText()
	if err != nil || !strings.Contains(txt, "Hello") {
		t.Errorf("ExtractText = %q, %v; want text containing Hello", txt, err)
	}
	img, err := page.RenderImage(pdf.RenderOptions{DPI: 72})
	if err != nil {
		t.Fatalf("RenderImage: %v", err)
	}
	rgba := img.(*image.RGBA)
	ink := 0
	b := rgba.Bounds()
	for y := 60; y < 110 && y < b.Max.Y; y++ {
		for x := 0; x < b.Max.X; x++ {
			r, g, bl, _ := rgba.At(x, y).RGBA()
			if r < 0x8000 && g < 0x8000 && bl < 0x8000 {
				ink++
			}
		}
	}
	if ink < 20 {
		t.Errorf("text band ink pixels = %d, want >= 20 (text not painted)", ink)
	}
}
