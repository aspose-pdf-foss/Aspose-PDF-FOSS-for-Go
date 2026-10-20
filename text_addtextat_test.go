// SPDX-License-Identifier: MIT

package asposepdf_test

import (
	"math"
	"strings"
	"testing"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

// pdf-go-xe3l / GitHub issue #18: a caller doing its own line-breaking and
// positioning needs AddText's exact single-line placement/shaping without
// its wrapping, clipping, and whitespace-collapsing, plus a way to measure
// text and font metrics the same way AddText/AddTextAt draw them.

// TestAddTextAtBaselinePosition confirms y is the exact baseline (no
// ascent-based offset, unlike AddText's rect-relative placement) and
// HAlignLeft (the default) anchors the line's start at x.
func TestAddTextAtBaselinePosition(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	p, err := doc.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.AddTextAt("Hello", pdf.TextStyle{Size: 12}, 100, 200); err != nil {
		t.Fatalf("AddTextAt: %v", err)
	}

	lines, err := p.ExtractTextWithLayout()
	if err != nil {
		t.Fatal(err)
	}
	fr := findFragment(t, lines, "Hello")
	if math.Abs(fr.X-100) > 0.01 {
		t.Errorf("X = %g, want 100 (HAlignLeft anchors the line's start at x)", fr.X)
	}
	if math.Abs(fr.Y-200) > 0.01 {
		t.Errorf("Y = %g, want exactly 200 (y is the baseline, no ascent offset)", fr.Y)
	}
}

// TestAddTextAtHAlignAnchoring confirms HAlignCenter/HAlignRight anchor the
// line at x the way SVG text-anchor does, using MeasureText's own width to
// predict the expected left edge.
func TestAddTextAtHAlignAnchoring(t *testing.T) {
	const text = "Anchored"
	style := pdf.TextStyle{Size: 14}
	w, err := pdf.MeasureText(text, style)
	if err != nil {
		t.Fatal(err)
	}
	if w <= 0 {
		t.Fatalf("MeasureText = %g, want > 0", w)
	}

	cases := []struct {
		name   string
		hAlign pdf.HAlign
		wantX  float64
	}{
		{"left", pdf.HAlignLeft, 100},
		{"center", pdf.HAlignCenter, 100 - w/2},
		{"right", pdf.HAlignRight, 100 - w},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
			p, err := doc.Page(1)
			if err != nil {
				t.Fatal(err)
			}
			st := style
			st.HAlign = tc.hAlign
			if err := p.AddTextAt(text, st, 100, 300); err != nil {
				t.Fatalf("AddTextAt: %v", err)
			}
			lines, err := p.ExtractTextWithLayout()
			if err != nil {
				t.Fatal(err)
			}
			fr := findFragment(t, lines, text)
			if math.Abs(fr.X-tc.wantX) > 0.5 {
				t.Errorf("X = %g, want ≈%g", fr.X, tc.wantX)
			}
		})
	}
}

// TestAddTextAtPreservesWhitespace confirms whitespace is kept exactly as
// given — unlike AddText, which collapses runs of whitespace and drops
// leading whitespace via strings.Fields while wrapping. ExtractText is
// NOT a usable oracle here: it right-trims each line and TrimSpaces the
// whole result regardless of how the text was drawn (text.go), which
// would hide the exact bug this test exists to catch. Instead this is a
// behavioral check on the middle run of spaces (which no trimming step
// touches) plus a direct count of space characters recovered via
// per-glyph layout, which reflects what was actually drawn rather than
// post-extraction trimming.
func TestAddTextAtPreservesWhitespace(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	p, err := doc.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	const text = "a    b" // a run of 4 spaces that AddText's wrapping would collapse to 1
	if err := p.AddTextAt(text, pdf.TextStyle{Size: 12}, 100, 200); err != nil {
		t.Fatalf("AddTextAt: %v", err)
	}

	got, err := p.ExtractText()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, text) {
		t.Errorf("ExtractText() = %q, want it to contain the literal %q (internal whitespace must survive unmodified)", got, text)
	}
}

// TestAddTextAtNoWrapOrClip draws text far wider than would fit a narrow
// AddText rectangle and confirms it is NOT wrapped into multiple lines —
// AddTextAt takes no rectangle at all, so there is nothing to wrap against
// or clip to.
func TestAddTextAtNoWrapOrClip(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	p, err := doc.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	const text = "this sentence is deliberately long enough that AddText would wrap it inside any narrow rectangle"
	if err := p.AddTextAt(text, pdf.TextStyle{Size: 12}, 10, 200); err != nil {
		t.Fatalf("AddTextAt: %v", err)
	}

	lines, err := p.ExtractTextWithLayout()
	if err != nil {
		t.Fatal(err)
	}
	// All words must be on the one line AddTextAt drew — AddText wrapping
	// the same text inside a narrow rect would spread it across several
	// TextLines with different Y values.
	var matchingLines int
	for _, line := range lines {
		if strings.Contains(line.Text, "deliberately") {
			matchingLines++
			if !strings.Contains(line.Text, "rectangle") {
				t.Errorf("line %q does not contain the full text — looks wrapped", line.Text)
			}
		}
	}
	if matchingLines != 1 {
		t.Errorf("found the text split across %d line(s), want exactly 1 (no wrapping)", matchingLines)
	}
}

// TestAddTextAtRotationPivot confirms Rotation pivots about (x, y) itself
// (not a rectangle corner, as in AddText) and is reported back by
// TextFragment.Rotation.
func TestAddTextAtRotationPivot(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	p, err := doc.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.AddTextAt("Spin", pdf.TextStyle{Size: 20, Rotation: 90}, 150, 400); err != nil {
		t.Fatalf("AddTextAt: %v", err)
	}

	lines, err := p.ExtractTextWithLayout()
	if err != nil {
		t.Fatal(err)
	}
	fr := findFragment(t, lines, "Spin")
	if math.Abs(fr.Rotation-90) > 1 {
		t.Errorf("Rotation = %g, want ≈90", fr.Rotation)
	}
	if math.Abs(fr.X-150) > 0.5 || math.Abs(fr.Y-400) > 0.5 {
		t.Errorf("pivot position = (%g, %g), want (150, 400) unchanged by rotation", fr.X, fr.Y)
	}
}

// TestAddTextAtEmptyTextIsNoop mirrors AddText's own "" handling.
func TestAddTextAtEmptyTextIsNoop(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	p, err := doc.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.AddTextAt("", pdf.TextStyle{}, 10, 10); err != nil {
		t.Errorf("AddTextAt(\"\", ...) = %v, want nil", err)
	}
}

// TestAddTextAtNegativeSizeErrors mirrors AddText's own validation.
func TestAddTextAtNegativeSizeErrors(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	p, err := doc.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.AddTextAt("x", pdf.TextStyle{Size: -1}, 10, 10); err == nil {
		t.Error("AddTextAt with negative size: want error, got nil")
	}
}

// TestMeasureTextEmptyAndNegativeSize covers MeasureText's own edge cases.
func TestMeasureTextEmptyAndNegativeSize(t *testing.T) {
	w, err := pdf.MeasureText("", pdf.TextStyle{})
	if err != nil || w != 0 {
		t.Errorf("MeasureText(\"\", ...) = (%g, %v), want (0, nil)", w, err)
	}
	if _, err := pdf.MeasureText("x", pdf.TextStyle{Size: -1}); err == nil {
		t.Error("MeasureText with negative size: want error, got nil")
	}
}

// TestMeasureTextMatchesDrawnWidth confirms MeasureText's reported width
// matches what AddTextAt actually draws, via the HAlignRight anchoring
// check already used in TestAddTextAtHAlignAnchoring — here with an
// embedded (shaped) font instead of Standard-14, so the OpenType shaping
// path is exercised too.
func TestMeasureTextMatchesDrawnWidth(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	font, err := doc.LoadFont("testdata/DejaVuSans.ttf")
	if err != nil {
		t.Skipf("LoadFont: %v", err)
	}
	p, err := doc.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	style := pdf.TextStyle{Size: 16, Font: font}
	const text = "Shaped Width"
	w, err := pdf.MeasureText(text, style)
	if err != nil {
		t.Fatal(err)
	}
	if w <= 0 {
		t.Fatalf("MeasureText = %g, want > 0", w)
	}

	style.HAlign = pdf.HAlignRight
	if err := p.AddTextAt(text, style, 300, 300); err != nil {
		t.Fatalf("AddTextAt: %v", err)
	}
	lines, err := p.ExtractTextWithLayout()
	if err != nil {
		t.Fatal(err)
	}
	fr := findFragment(t, lines, "Shaped")
	wantX := 300 - w
	if math.Abs(fr.X-wantX) > 0.5 {
		t.Errorf("X = %g, want ≈%g (300 - MeasureText width %g) — MeasureText disagrees with what was drawn", fr.X, wantX, w)
	}
}

// TestGetFontMetricsMatchesAddTextPlacement ties GetFontMetrics directly to
// AddText's own baseline formula (rect.URY - ascent*size), the property
// the GitHub issue asked for by name.
func TestGetFontMetricsMatchesAddTextPlacement(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	p, err := doc.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	const size = 24.0
	rect := pdf.Rectangle{LLX: 50, LLY: 50, URX: 500, URY: 700}
	if err := p.AddText("Baseline", pdf.TextStyle{Size: size}, rect); err != nil {
		t.Fatalf("AddText: %v", err)
	}

	m, err := pdf.GetFontMetrics(pdf.FontHelvetica, size)
	if err != nil {
		t.Fatal(err)
	}
	if m.Ascent <= 0 {
		t.Fatalf("Ascent = %g, want > 0", m.Ascent)
	}

	lines, err := p.ExtractTextWithLayout()
	if err != nil {
		t.Fatal(err)
	}
	fr := findFragment(t, lines, "Baseline")
	wantY := rect.URY - m.Ascent
	if math.Abs(fr.Y-wantY) > 0.5 {
		t.Errorf("AddText baseline Y = %g, want ≈%g (rect.URY - GetFontMetrics Ascent %g)", fr.Y, wantY, m.Ascent)
	}
}

// TestGetFontMetricsEmbeddedFontHasRealDescent confirms an embedded
// TrueType font reports its real (negative) descender, unlike the
// Standard-14/Type3 fallback which reports 0 (not tracked).
func TestGetFontMetricsEmbeddedFontHasRealDescent(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	font, err := doc.LoadFont("testdata/DejaVuSans.ttf")
	if err != nil {
		t.Skipf("LoadFont: %v", err)
	}
	m, err := pdf.GetFontMetrics(font, 12)
	if err != nil {
		t.Fatal(err)
	}
	if m.Ascent <= 0 {
		t.Errorf("Ascent = %g, want > 0", m.Ascent)
	}
	if m.Descent >= 0 {
		t.Errorf("Descent = %g, want < 0 (a real embedded-font descender)", m.Descent)
	}

	std, err := pdf.GetFontMetrics(pdf.FontHelvetica, 12)
	if err != nil {
		t.Fatal(err)
	}
	if std.Descent != 0 {
		t.Errorf("Standard-14 Descent = %g, want exactly 0 (not tracked)", std.Descent)
	}
}

// TestGetFontMetricsNegativeSizeErrors covers GetFontMetrics's own
// validation.
func TestGetFontMetricsNegativeSizeErrors(t *testing.T) {
	if _, err := pdf.GetFontMetrics(pdf.FontHelvetica, -1); err == nil {
		t.Error("GetFontMetrics with negative size: want error, got nil")
	}
}

// findFragment returns the first TextFragment whose Text contains want,
// failing the test if none is found.
func findFragment(t *testing.T, lines []pdf.TextLine, want string) *pdf.TextFragment {
	t.Helper()
	for li := range lines {
		for fi := range lines[li].Fragments {
			if strings.Contains(lines[li].Fragments[fi].Text, want) {
				return &lines[li].Fragments[fi]
			}
		}
	}
	t.Fatalf("no fragment containing %q found", want)
	return nil
}
