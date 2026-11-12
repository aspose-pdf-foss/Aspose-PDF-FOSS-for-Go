// SPDX-License-Identifier: MIT

package asposepdf_test

import (
	"strings"
	"testing"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

// clipPage is a 300x200 page with text drawn across each edge: "Overhang"
// starting left of the page, "Rightmost" running past the right edge, one
// line above the top, one below the bottom, and an upright line inside.
func clipPage(t *testing.T) *pdf.Page {
	t.Helper()
	doc := pdf.NewDocument(300, 200)
	p, _ := doc.Page(1)
	style := pdf.TextStyle{Font: pdf.FontCourier, Size: 10} // 6pt per glyph
	for _, l := range []struct {
		s    string
		x, y float64
	}{
		{"Overhang", -18, 150}, // "Ove" lies left of the page
		{"Rightmost", 264, 120}, // ends at 318: "ost" lies right of it
		{"Above the page", 50, 205},
		{"Below the page", 50, -8},
		{"Inside the page", 50, 100},
	} {
		if err := p.AddTextAt(l.s, style, l.x, l.y); err != nil {
			t.Fatal(err)
		}
	}
	return p
}

// TestExtractTextLimitToPageBounds: only glyphs on the page come out, cut
// glyph by glyph at the edges, in every mode; without the option everything
// does (pdf-go-c0t0).
func TestExtractTextLimitToPageBounds(t *testing.T) {
	p := clipPage(t)
	for _, mode := range []pdf.TextExtractionMode{pdf.TextExtractReading, pdf.TextExtractRaw, pdf.TextExtractFlatten} {
		s, err := p.ExtractText(pdf.TextExtractOptions{Mode: mode, LimitToPageBounds: true})
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range []string{"rhang", "Rightm", "Inside the page"} {
			if !strings.Contains(s, want) {
				t.Errorf("mode %d: %q missing from %q", mode, want, s)
			}
		}
		for _, gone := range []string{"Overhang", "verhang", "Rightmo", "Above", "Below"} {
			if strings.Contains(s, gone) {
				t.Errorf("mode %d: %q extracted from %q", mode, gone, s)
			}
		}
	}
	all, err := p.ExtractText(pdf.TextExtractOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Overhang", "Rightmost", "Above the page", "Below the page"} {
		if !strings.Contains(all, want) {
			t.Errorf("without the limit %q missing from %q", want, all)
		}
	}
}

// TestExtractTextRectangle: a rectangle keeps the glyphs whose advance lies
// in it, also for text set at a quarter turn; with LimitToPageBounds the two
// regions intersect.
func TestExtractTextRectangle(t *testing.T) {
	p := clipPage(t)
	// "Inside the page" runs from x 50; [62, 92] holds "side " (glyphs 2..6).
	r := &pdf.Rectangle{LLX: 62, LLY: 95, URX: 92, URY: 110}
	s, err := p.ExtractText(pdf.TextExtractOptions{Rectangle: r})
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(s) != "side" {
		t.Errorf("rectangle = %q, want \"side\"", s)
	}
	// Reaching off the page, the rectangle is cut back to it.
	s, err = p.ExtractText(pdf.TextExtractOptions{LimitToPageBounds: true, Rectangle: &pdf.Rectangle{LLX: -30, LLY: 140, URX: 0, URY: 160}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(s) != "" {
		t.Errorf("rectangle off the page = %q, want nothing", s)
	}

	doc := pdf.NewDocument(300, 300)
	q, _ := doc.Page(1)
	if err := q.AddTextAt("Vertical", pdf.TextStyle{Font: pdf.FontCourier, Size: 10, Rotation: 90}, 100, 50); err != nil {
		t.Fatal(err)
	}
	// Up the page from y 50: [56, 80] holds "erti".
	s, err = q.ExtractText(pdf.TextExtractOptions{Rectangle: &pdf.Rectangle{LLX: 90, LLY: 56, URX: 110, URY: 80}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(s) != "erti" {
		t.Errorf("rotated = %q, want \"erti\"", s)
	}
}
