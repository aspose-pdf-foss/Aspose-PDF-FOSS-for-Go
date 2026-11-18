// SPDX-License-Identifier: MIT

package asposepdf

import (
	"strings"
	"testing"
)

// colourRunsContent changes the fill colour inside a word, across a TJ with
// kerning, and after a gap.
const colourRunsContent = "BT /F0 12 Tf 1 0 0 1 100 700 Tm 0 0 0 rg (Hel) Tj 1 0 0 rg (lo) Tj 0 0 0 rg ( world) Tj ET\n" +
	"BT /F0 12 Tf 1 0 0 1 100 650 Tm 0 0 0 rg [(Ke) 80 (rn)] TJ 0 0 1 rg [(ed) -250 (text)] TJ ET\n" +
	"BT /F0 12 Tf 1 0 0 1 100 600 Tm 0 0 0 rg [(left) -400] TJ 0 1 0 rg (right) Tj 0 0 0 rg [(x) -120 (y)] TJ ET\n"

func colourRunsPage(t *testing.T) *Page {
	t.Helper()
	d := NewDocumentFromFormat(PageFormatA4)
	p := mustPage(t, d)
	if _, err := p.ensureStandardFontResource("/Helvetica"); err != nil {
		t.Fatal(err)
	}
	if err := p.appendToContentStream([]byte(colourRunsContent)); err != nil {
		t.Fatal(err)
	}
	return mustPage(t, reopenDoc(t, d))
}

// A colour change does not change extraction: the values below are what the
// extractor produced before colour was tracked per rune (main 4cf62b6).
func TestColourChangeKeepsExtraction(t *testing.T) {
	p := colourRunsPage(t)
	lines, err := p.ExtractTextWithLayout()
	if err != nil {
		t.Fatal(err)
	}
	wantLines := []struct {
		text  string
		frags int
	}{{"Hello world", 1}, {"Kerned text", 2}, {"left rightxy", 3}}
	if len(lines) != len(wantLines) {
		t.Fatalf("%d lines", len(lines))
	}
	for i, w := range wantLines {
		if lines[i].Text != w.text || len(lines[i].Fragments) != w.frags {
			t.Fatalf("line %d: %q with %d fragments, want %q with %d", i, lines[i].Text, len(lines[i].Fragments), w.text, w.frags)
		}
	}
	if txt, _ := p.ExtractText(); txt != "Hello world\n\nKerned text\n\nleft rightxy" {
		t.Fatalf("text %q", txt)
	}
	for _, w := range []struct {
		q    string
		rect Rectangle
	}{
		{"Hello", Rectangle{100, 697.516, 127.336, 708.616}},
		{"llo w", Rectangle{115.336, 697.516, 139.33599999999998, 708.616}},
		{"Kerned", Rectangle{100, 647.516, 137.728, 658.616}},
		{"ed text", Rectangle{124.384, 647.516, 160.07200000000003, 658.616}},
		{"left right", Rectangle{100, 597.516, 144.148, 608.616}},
		{"rightx", Rectangle{120.80799999999999, 597.516, 150.148, 608.616}},
		{"rightxy", Rectangle{120.80799999999999, 597.516, 157.588, 608.616}},
	} {
		ms, err := p.SearchText(w.q)
		if err != nil || len(ms) != 1 || ms[0].Rect != w.rect {
			t.Fatalf("%q: %+v %v", w.q, ms, err)
		}
	}
	for _, q := range []string{"leftright", "x y"} {
		if ms, _ := p.SearchText(q); len(ms) != 0 {
			t.Fatalf("%q found", q)
		}
	}
}

// The absorber still splits segments where the colour changes.
func TestAbsorberSplitsSegmentsByRuneColour(t *testing.T) {
	p := colourRunsPage(t)
	for _, w := range []struct {
		q    string
		segs []string
		red  []bool // R channel set per segment
		blue []bool
	}{
		{"Hello world", []string{"Hel", "lo", " world"}, []bool{false, true, false}, []bool{false, false, false}},
		{"Kerned text", []string{"Kern", "ed text"}, []bool{false, false}, []bool{false, true}},
	} {
		a := NewTextFragmentAbsorber(w.q)
		if err := a.Visit(p); err != nil {
			t.Fatal(err)
		}
		fs := a.TextFragments()
		if len(fs) != 1 || len(fs[0].Segments) != len(w.segs) {
			t.Fatalf("%q: %d fragments", w.q, len(fs))
		}
		var got []string
		for i, s := range fs[0].Segments {
			got = append(got, s.Text)
			c := s.TextState.ForegroundColor
			if c == nil || (c.R == 1) != w.red[i] || (c.B == 1) != w.blue[i] {
				t.Fatalf("%q segment %d colour %+v", w.q, i, c)
			}
		}
		if strings.Join(got, "|") != strings.Join(w.segs, "|") {
			t.Fatalf("%q segments %q", w.q, got)
		}
	}
	// Each segment's box is a part of the match's.
	a := NewTextFragmentAbsorber("Hello")
	if err := a.Visit(p); err != nil {
		t.Fatal(err)
	}
	s := a.TextFragments()[0].Segments
	if len(s) != 2 || s[0].Rectangle.LLX != 100 || s[1].Rectangle.URX != 127.336 || s[0].Rectangle.URX != s[1].Rectangle.LLX {
		t.Fatalf("segments %+v", s)
	}
}

// Clipping keeps the per-rune colours of the runes it keeps.
func TestClipKeepsRuneColours(t *testing.T) {
	var f textFragment
	f.text.WriteString("abcd")
	f.runeX, f.runeY = []float64{0, 1, 2, 3}, []float64{0, 0, 0, 0}
	f.endX = 4
	f.runeColor = []Color{{A: 1}, {R: 1, A: 1}, {R: 1, A: 1}, {B: 1, A: 1}}
	out := clipFragments([]textFragment{f}, Rectangle{LLX: 0.5, LLY: -1, URX: 3.5, URY: 1})
	if len(out) != 1 || out[0].text.String() != "bc" || len(out[0].runeColor) != 2 || out[0].runeColor[1].R != 1 {
		t.Fatalf("%+v", out)
	}
}
