// SPDX-License-Identifier: MIT

package asposepdf_test

import (
	"math"
	"strings"
	"testing"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

// TestTextFragmentRotation: rotated text (AddText with Rotation) comes back
// with the angle on the fragment, isolated from horizontal lines, and the
// HTML text mode emits a CSS rotation for it.
func TestTextFragmentRotation(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	p, err := doc.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.AddText("Horizontal body text on the page.", pdf.TextStyle{Size: 12},
		pdf.Rectangle{LLX: 50, LLY: 500, URX: 545, URY: 540}); err != nil {
		t.Fatal(err)
	}
	if err := p.AddText("WATERMARK", pdf.TextStyle{Size: 48, Rotation: 45},
		pdf.Rectangle{LLX: 100, LLY: 100, URX: 500, URY: 700}); err != nil {
		t.Fatal(err)
	}

	lines, err := p.ExtractTextWithLayout()
	if err != nil {
		t.Fatal(err)
	}
	var rotated *pdf.TextFragment
	for li := range lines {
		for fi := range lines[li].Fragments {
			fr := &lines[li].Fragments[fi]
			if strings.Contains(fr.Text, "WATERMARK") {
				rotated = fr
			} else if fr.Rotation != 0 {
				t.Errorf("horizontal fragment %q has rotation %g", fr.Text, fr.Rotation)
			}
			if strings.Contains(lines[li].Text, "WATERMARK") && strings.Contains(lines[li].Text, "Horizontal") {
				t.Errorf("rotated text merged into a horizontal line: %q", lines[li].Text)
			}
		}
	}
	if rotated == nil {
		t.Fatal("rotated fragment not found")
	}
	if math.Abs(rotated.Rotation-45) > 1 {
		t.Errorf("Rotation = %g; want ≈45", rotated.Rotation)
	}

	// HTML text mode carries the rotation as a CSS transform.
	var html strings.Builder
	if err := doc.WriteHTML(&html, pdf.HTMLSaveOptions{Mode: pdf.HTMLModeText}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(html.String(), "rotate(-45") {
		t.Error("HTML output missing rotate(-45…) for the watermark span")
	}

	// The markdown export drops the rotated overlay as decoration.
	var md strings.Builder
	if err := doc.WriteMarkdown(&md); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(md.String(), "WATERMARK") {
		t.Errorf("rotated watermark leaked into markdown:\n%s", md.String())
	}
	if !strings.Contains(md.String(), "Horizontal body text") {
		t.Error("body text lost")
	}
}

// TestTextStyleSkew: synthetic oblique slants glyphs without an italic face —
// the render differs from upright, extraction still reads the text upright.
func TestTextStyleSkew(t *testing.T) {
	upright := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	slanted := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	rect := pdf.Rectangle{LLX: 50, LLY: 650, URX: 545, URY: 760}
	for _, c := range []struct {
		doc  *pdf.Document
		skew float64
	}{{upright, 0}, {slanted, 12}} {
		p, err := c.doc.Page(1)
		if err != nil {
			t.Fatal(err)
		}
		if err := p.AddText("Faux italic sample text\nsecond line", pdf.TextStyle{Size: 24, Skew: c.skew}, rect); err != nil {
			t.Fatal(err)
		}
	}
	iu, err := upright.RenderImage(1, pdf.RenderOptions{DPI: 96})
	if err != nil {
		t.Fatal(err)
	}
	is, err := slanted.RenderImage(1, pdf.RenderOptions{DPI: 96})
	if err != nil {
		t.Fatal(err)
	}
	same := true
	bo := iu.Bounds()
	for y := bo.Min.Y; y < bo.Max.Y && same; y++ {
		for x := bo.Min.X; x < bo.Max.X; x++ {
			if iu.At(x, y) != is.At(x, y) {
				same = false
				break
			}
		}
	}
	if same {
		t.Error("Skew did not change the rendering")
	}
	p, _ := slanted.Page(1)
	text, err := p.ExtractText()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "Faux italic sample text") || !strings.Contains(text, "second line") {
		t.Errorf("skewed text extraction broken: %q", text)
	}
	// Both lines keep distinct baselines (per-line Tm did not stack).
	lines, _ := p.ExtractTextWithLayout()
	if len(lines) != 2 {
		t.Errorf("lines = %d; want 2", len(lines))
	}
}
