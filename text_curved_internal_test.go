// SPDX-License-Identifier: MIT

package asposepdf

import (
	"fmt"
	"math"
	"strings"
	"testing"
)

// curvedText draws s one glyph at a time along an arc: each glyph is set with
// its own text matrix, turned by step degrees from the previous one, starting
// where the previous glyph's advance ended. A '~' is a word gap of half an
// em with no glyph.
func curvedText(t *testing.T, s string, x, y, angle, step, size float64) string {
	t.Helper()
	var b strings.Builder
	for _, r := range s {
		w, err := MeasureText(string(r), TextStyle{Font: FontHelvetica, Size: size})
		if err != nil {
			t.Fatal(err)
		}
		sin, cos := math.Sincos(angle * math.Pi / 180)
		if r == '~' {
			w = size / 2 // a word gap left as a move, no space glyph
		} else {
			fmt.Fprintf(&b, "BT /F1 %g Tf %g %g %g %g %g %g Tm (%c) Tj ET ", size, cos, sin, -sin, cos, x, y, r)
		}
		x, y = x+w*cos, y+w*sin
		angle += step
	}
	return b.String()
}

func curvedPage(t *testing.T, content string) *Page {
	t.Helper()
	d := NewDocument(600, 600)
	font := d.addObject(pdfDict{"/Type": pdfName("/Font"), "/Subtype": pdfName("/Type1"), "/BaseFont": pdfName("/Helvetica")})
	p, _ := d.Page(1)
	p.pageDict()["/Resources"] = pdfDict{"/Font": pdfDict{"/F1": pdfRef{Num: font}}}
	if err := p.appendToContentStream([]byte(content)); err != nil {
		t.Fatal(err)
	}
	return p
}

// TestCurvedTextIsOneLine: text set along a curve, every glyph at its own
// angle, reads as one line with its word gap, and is found by search; a
// second curved word elsewhere on the page stays its own line (pdf-go-3iwy).
func TestCurvedTextIsOneLine(t *testing.T) {
	p := curvedPage(t, curvedText(t, "Curved text", 100, 300, 50, -10, 24)+
		curvedText(t, "Other", 450, 80, 120, 8, 24))
	lines, err := p.ExtractTextWithLayout()
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, l := range lines {
		got = append(got, l.Text)
	}
	if len(got) != 2 || got[0] != "Curved text" || got[1] != "Other" {
		t.Fatalf("lines = %q, want [\"Curved text\" \"Other\"]", got)
	}
	ms, err := p.SearchText("Curved text")
	if err != nil {
		t.Fatal(err)
	}
	if len(ms) != 1 {
		t.Errorf("SearchText found %d matches, want 1", len(ms))
	}
}

// TestRotatedFragmentsApartStaySeparate: rotated words that do not run on
// from one another — too far along the baseline, off it, or turned too
// sharply — are not joined.
func TestRotatedFragmentsApartStaySeparate(t *testing.T) {
	for _, c := range []struct {
		name    string
		content string
	}{
		{"far along", "BT /F1 24 Tf 0.7071 0.7071 -0.7071 0.7071 100 100 Tm (Ab) Tj ET " +
			"BT /F1 24 Tf 0.7071 0.7071 -0.7071 0.7071 160 160 Tm (Cd) Tj ET "},
		{"off the baseline", "BT /F1 24 Tf 0.7071 0.7071 -0.7071 0.7071 100 100 Tm (Ab) Tj 0 -30 Td (Cd) Tj ET "},
		{"sharp turn", "BT /F1 24 Tf 0.7071 0.7071 -0.7071 0.7071 100 100 Tm (Ab) Tj ET " +
			"BT /F1 24 Tf 0.7071 -0.7071 0.7071 0.7071 120.75 120.75 Tm (Cd) Tj ET "},
	} {
		t.Run(c.name, func(t *testing.T) {
			lines, err := curvedPage(t, c.content).ExtractTextWithLayout()
			if err != nil {
				t.Fatal(err)
			}
			if len(lines) != 2 {
				var got []string
				for _, l := range lines {
					got = append(got, l.Text)
				}
				t.Errorf("lines = %q, want two", got)
			}
		})
	}
}

// TestSteepTextWordGap: on a nearly vertical baseline a word gap left as a
// move is measured along the baseline, where its horizontal part would be
// too small to count.
func TestSteepTextWordGap(t *testing.T) {
	p := curvedPage(t, curvedText(t, "Up~word", 300, 100, 80, 0, 24))
	if ms, err := p.SearchText("Up word"); err != nil || len(ms) != 1 {
		t.Errorf("SearchText: %d matches, %v; want 1", len(ms), err)
	}
	lines, err := p.ExtractTextWithLayout()
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 1 || lines[0].Text != "Up word" {
		var got []string
		for _, l := range lines {
			got = append(got, l.Text)
		}
		t.Errorf("lines = %q, want [\"Up word\"]", got)
	}
}
