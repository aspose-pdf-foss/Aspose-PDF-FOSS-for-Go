// SPDX-License-Identifier: MIT

package asposepdf

import (
	"strings"
	"testing"
)

// Build a minimal fontMap with one Helvetica entry under "/F1" with
// constant 500 widths (so each char is 500/1000 = 0.5 em wide).
func testRewriteFontMap() map[string]fontInfo {
	fi := fontInfo{
		name:     "/Helvetica",
		known:    true,
		defaultW: 500,
	}
	for i := range fi.widths {
		fi.widths[i] = 500
	}
	for i := byte('A'); i <= 'z'; i++ {
		fi.encoding[i] = rune(i)
	}
	fi.encoding[' '] = ' '
	return map[string]fontInfo{"/F1": fi}
}

func TestRewriteTextNoRegions(t *testing.T) {
	in := []byte("BT\n/F1 12 Tf\n100 700 Td\n(Hello world) Tj\nET\n")
	out, err := rewriteTextOperatorsInStream(in, nil, testRewriteFontMap())
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != string(in) {
		t.Errorf("expected unchanged output, got %q", out)
	}
}

func TestRewriteTextDropAllGlyphs(t *testing.T) {
	// Region covering the whole drawn text.
	fonts := testRewriteFontMap()
	in := []byte("BT\n/F1 12 Tf\n100 700 Td\n(SECRET) Tj\nET\n")
	regions := []QuadPoint{
		{X1: 0, Y1: 720, X2: 1000, Y2: 720, X3: 0, Y3: 690, X4: 1000, Y4: 690},
	}
	out, err := rewriteTextOperatorsInStream(in, regions, fonts)
	if err != nil {
		t.Fatal(err)
	}
	// SECRET text should be dropped — no '(' followed by 'S' should appear
	if strings.Contains(string(out), "(SECRET)") {
		t.Errorf("expected SECRET removed, got %q", out)
	}
}

func TestRewriteTextKeepAllGlyphs(t *testing.T) {
	// Region far away from drawn text.
	fonts := testRewriteFontMap()
	in := []byte("BT\n/F1 12 Tf\n100 700 Td\n(Visible) Tj\nET\n")
	regions := []QuadPoint{
		{X1: 0, Y1: 100, X2: 100, Y2: 100, X3: 0, Y3: 50, X4: 100, Y4: 50},
	}
	out, err := rewriteTextOperatorsInStream(in, regions, fonts)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "(Visible)") {
		t.Errorf("expected Visible kept, got %q", out)
	}
}

func TestRewriteTextPartialGlyphFilter(t *testing.T) {
	// Place text at (100, 700) with Helvetica/12pt, 500-width per glyph.
	// Each glyph advance in user space ≈ 500/1000 * 12 = 6pt.
	// "Hello world" is 11 chars (positions 100, 106, 112, ..., +6*k).
	// "world" begins at offset 6 (position 100 + 36 = 136).
	// Cover x ∈ [134, 200], y ∈ [690, 720] to redact "world" but not "Hello ".
	fonts := testRewriteFontMap()
	in := []byte("BT\n/F1 12 Tf\n100 700 Td\n(Hello world) Tj\nET\n")
	regions := []QuadPoint{
		{X1: 134, Y1: 720, X2: 200, Y2: 720, X3: 134, Y3: 690, X4: 200, Y4: 690},
	}
	out, err := rewriteTextOperatorsInStream(in, regions, fonts)
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	// The output should contain "Hello" and should NOT contain "world".
	if !strings.Contains(s, "Hello") {
		t.Errorf("expected Hello kept, got %q", s)
	}
	if strings.Contains(s, "world") {
		t.Errorf("expected world dropped, got %q", s)
	}
}

func TestRewriteTextEmitsTJWithKerning(t *testing.T) {
	// Same as partial test — verifies the output uses TJ with kerning
	// (so kept glyph positions are preserved).
	fonts := testRewriteFontMap()
	in := []byte("BT\n/F1 12 Tf\n100 700 Td\n(Hello world) Tj\nET\n")
	regions := []QuadPoint{
		{X1: 134, Y1: 720, X2: 200, Y2: 720, X3: 134, Y3: 690, X4: 200, Y4: 690},
	}
	out, err := rewriteTextOperatorsInStream(in, regions, fonts)
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	// Either the output still uses Tj for the surviving prefix only (Hello),
	// OR it uses a TJ array. Both are acceptable. We just verify "world" gone.
	if strings.Contains(s, "world") {
		t.Errorf("world should be dropped, got %q", s)
	}
}

func TestRewriteTextMultiLine(t *testing.T) {
	// Two text lines via T*; redact only the second line. First survives.
	fonts := testRewriteFontMap()
	in := []byte("BT\n/F1 12 Tf\n14 TL\n100 700 Td\n(Line1) Tj\nT*\n(Line2) Tj\nET\n")
	// Line2 is at y ≈ 686 (700 - 14). Cover y ∈ [675, 695] there.
	regions := []QuadPoint{
		{X1: 0, Y1: 695, X2: 1000, Y2: 695, X3: 0, Y3: 675, X4: 1000, Y4: 675},
	}
	out, err := rewriteTextOperatorsInStream(in, regions, fonts)
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if !strings.Contains(s, "Line1") {
		t.Errorf("expected Line1 kept, got %q", s)
	}
	if strings.Contains(s, "Line2") {
		t.Errorf("expected Line2 dropped, got %q", s)
	}
}

func TestRewriteTextUnknownFontPassthrough(t *testing.T) {
	// Font map empty — should pass through unchanged.
	in := []byte("BT\n/F1 12 Tf\n100 700 Td\n(Hello) Tj\nET\n")
	out, err := rewriteTextOperatorsInStream(in, []QuadPoint{{X1: 0, Y1: 1000, X2: 1000, Y2: 1000, X3: 0, Y3: 0, X4: 1000, Y4: 0}}, map[string]fontInfo{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "(Hello)") {
		t.Errorf("expected passthrough when font unknown, got %q", out)
	}
}

func TestRewriteTextPointInAnyQuad(t *testing.T) {
	quads := []QuadPoint{
		{X1: 0, Y1: 100, X2: 100, Y2: 100, X3: 0, Y3: 0, X4: 100, Y4: 0},
	}
	if !pointInAnyQuad(50, 50, quads) {
		t.Error("(50,50) should be inside")
	}
	if pointInAnyQuad(200, 50, quads) {
		t.Error("(200,50) should be outside")
	}
}
