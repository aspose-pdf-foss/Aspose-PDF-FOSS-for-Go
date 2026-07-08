// SPDX-License-Identifier: MIT

package asposepdf

import (
	"math"
	"strings"
	"testing"
)

// TestHTMLWidthFit: the scaleX/letter-spacing split matches the decided
// policy — small mismatch → letter-spacing only, large → scaleX, huge →
// clamped scaleX with the residual in letter-spacing.
func TestHTMLWidthFit(t *testing.T) {
	const size = 12.0
	text := "Hello width fitting"
	widthFn, _, err := fontWidthAndAscent(FontHelvetica, size)
	if err != nil {
		t.Fatal(err)
	}
	natural := measureString(text, widthFn)
	runes := float64(len([]rune(text)))

	cases := []struct {
		name        string
		target      float64
		wantScale   float64
		wantSpacing float64
	}{
		{"exact", natural, 1, 0},
		{"small gap → letter-spacing", natural * 1.03, 1, natural * 0.03 / runes},
		{"large gap → scaleX", natural * 1.5, 1.5, 0},
		{"clamped → scaleX + residual spacing", natural * 3, 2, natural / 2 / runes},
	}
	for _, c := range cases {
		frag := TextFragment{Text: text, Width: c.target, FontSize: size}
		scale, spacing := htmlWidthFit(text, "sans", frag, size)
		if math.Abs(scale-c.wantScale) > 1e-9 || math.Abs(spacing-c.wantSpacing) > 1e-9 {
			t.Errorf("%s: got scale=%v spacing=%v, want %v / %v",
				c.name, scale, spacing, c.wantScale, c.wantSpacing)
		}
	}

	// Unknown width → no fitting.
	if s, sp := htmlWidthFit(text, "sans", TextFragment{Text: text, FontSize: size}, size); s != 1 || sp != 0 {
		t.Errorf("zero Width: got scale=%v spacing=%v, want 1 / 0", s, sp)
	}
	// Single rune never gets letter-spacing.
	frag := TextFragment{Text: "W", Width: 20, FontSize: size}
	if _, sp := htmlWidthFit("W", "sans", frag, size); sp != 0 {
		t.Errorf("single rune: letter-spacing = %v, want 0", sp)
	}
}

// TestSubstituteFontFor: every family/style combination resolves to the
// metric-matching Standard-14 face.
func TestSubstituteFontFor(t *testing.T) {
	cases := []struct {
		family       string
		bold, italic bool
		want         string
	}{
		{"sans", false, false, "Helvetica"},
		{"sans", true, false, "Helvetica-Bold"},
		{"sans", false, true, "Helvetica-Oblique"},
		{"sans", true, true, "Helvetica-BoldOblique"},
		{"serif", false, false, "Times-Roman"},
		{"serif", true, true, "Times-BoldItalic"},
		{"mono", false, false, "Courier"},
		{"mono", true, false, "Courier-Bold"},
	}
	for _, c := range cases {
		got := substituteFontFor(c.family, c.bold, c.italic).BaseFont()
		if !strings.EqualFold(got, c.want) {
			t.Errorf("substituteFontFor(%q, %v, %v) = %q, want %q",
				c.family, c.bold, c.italic, got, c.want)
		}
	}
}

// TestHTMLColor: colour formatting clamps and rounds to hex.
func TestHTMLColor(t *testing.T) {
	cases := []struct {
		c    Color
		want string
	}{
		{Color{R: 0, G: 0, B: 0, A: 1}, "#000000"},
		{Color{R: 1, G: 1, B: 1, A: 1}, "#ffffff"},
		{Color{R: 0, G: 0, B: 0.8, A: 1}, "#0000cc"},
		{Color{R: -1, G: 2, B: 0.5}, "#00ff80"},
	}
	for _, c := range cases {
		if got := htmlColor(c.c); got != c.want {
			t.Errorf("htmlColor(%+v) = %q, want %q", c.c, got, c.want)
		}
	}
}
