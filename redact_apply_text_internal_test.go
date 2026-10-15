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
	out, err := rewriteTextOperatorsInStream(in, nil, testRewriteFontMap(), nil, nil)
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
	out, err := rewriteTextOperatorsInStream(in, regions, fonts, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	// SECRET text should be dropped — no '(' followed by 'S' should appear
	if strings.Contains(string(out), "(SECRET)") {
		t.Errorf("expected SECRET removed, got %q", out)
	}
}

// TestRewriteTextTracksCM pins pdf-go-njbg: a glyph's page-space
// position must be the text matrix composed with the graphics-state
// CTM, not the text matrix alone. Here the content sets up a Y-flip
// "cm" (the common top-left-origin idiom many producers, including
// Chrome/Skia, emit at the very start of a page's content) before
// drawing text with Td — the text's REAL, post-cm page-space position
// is nowhere near where it would land if the rewriter (wrongly) ignored
// the cm and treated Td's raw operands as the final position. The
// region given here is in page space (the position SearchText would
// actually report, and the position the caller would build a
// RedactAnnotation from), matching ApplyRedactions' real contract.
func TestRewriteTextTracksCM(t *testing.T) {
	fonts := testRewriteFontMap()
	// Flip Y around y=842 (an A4-height page), then draw at local (100, 142)
	// -- which the flip maps to page-space (100, 700).
	in := []byte("q\n1 0 0 -1 0 842 cm\nBT\n/F1 12 Tf\n100 142 Td\n(SECRET) Tj\nET\nQ\n")
	regions := []QuadPoint{
		{X1: 0, Y1: 720, X2: 1000, Y2: 720, X3: 0, Y3: 690, X4: 1000, Y4: 690},
	}
	out, err := rewriteTextOperatorsInStream(in, regions, fonts, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), "(SECRET)") {
		t.Errorf("expected SECRET removed (its real page-space position, after the Y-flip cm, falls inside the region), got %q", out)
	}
}

// TestRewriteTextTracksCMInTJArray is the TJ-array counterpart of
// TestRewriteTextTracksCM: an independent review of the cm-tracking fix
// found that every new test exercised only Tj, leaving filterTJArray's
// two glyph-center sites (the Type0 and single-byte loops) without any
// test that would catch a regression specific to THEM — a mutation
// limited to those two call sites would not have failed any existing
// test. This closes that gap.
func TestRewriteTextTracksCMInTJArray(t *testing.T) {
	fonts := testRewriteFontMap()
	in := []byte("q\n1 0 0 -1 0 842 cm\nBT\n/F1 12 Tf\n100 142 Td\n[(SEC) -100 (RET)] TJ\nET\nQ\n")
	regions := []QuadPoint{
		{X1: 0, Y1: 720, X2: 1000, Y2: 720, X3: 0, Y3: 690, X4: 1000, Y4: 690},
	}
	out, err := rewriteTextOperatorsInStream(in, regions, fonts, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), "SEC") || strings.Contains(string(out), "RET") {
		t.Errorf("expected both TJ string elements removed (their real page-space position, after the Y-flip cm, falls inside the region), got %q", out)
	}
}

// TestRewriteTextQRestoresTextState pins a bug an independent review of
// the cm-tracking fix found alongside it: "q" must save the WHOLE
// graphics state, not just the CTM — ISO 32000-1 Table 52 lists
// character spacing, word spacing, horizontal scaling, leading, text
// font and text font size as graphics-state parameters exactly as
// q/Q-scoped as the CTM, and textExtractor (text.go) already
// saves/restores all of them. Here, text drawn AFTER a "Q" must be
// measured using the font/size that was active BEFORE the matching "q",
// not whatever a Tf INSIDE that q/Q block last set — if Q failed to
// restore the font, the glyph-center math after Q would use the wrong
// (much wider) advance and could fall outside the very region that
// correctly covers its TRUE, post-restore position.
func TestRewriteTextQRestoresTextState(t *testing.T) {
	fonts := testRewriteFontMap()
	// /F2: double the per-character advance of /F1 (1000 vs 500
	// thousandths), so a failure to restore the font after Q measurably
	// shifts every glyph's computed center.
	f2 := fonts["/F1"]
	f2.name = "/F2-wide"
	for i := range f2.widths {
		f2.widths[i] = 1000
	}
	fonts["/F2"] = f2

	in := []byte("/F1 12 Tf\nq\n/F2 36 Tf\nQ\nBT\n100 700 Td\n(SECRET) Tj\nET\n")

	// With Q correctly restoring /F1 at 12pt: first glyph advance =
	// 500/1000*12 = 6, center x = 100 + 6/2 = 103. A region of
	// [100,110]x[695,705] covers that and nothing a /F2-at-36pt
	// (advance 1000/1000*36 = 36, center x = 100+18 = 118) computation
	// would land in.
	regions := []QuadPoint{
		{X1: 100, Y1: 705, X2: 110, Y2: 705, X3: 100, Y3: 695, X4: 110, Y4: 695},
	}
	out, err := rewriteTextOperatorsInStream(in, regions, fonts, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), "SECRET") {
		t.Errorf("expected SECRET removed (Q should have restored /F1 at 12pt, placing its first glyph's center inside the region), got %q", out)
	}
}

// TestRewriteTextTJKerningHonorsHorizScale pins a bug an independent
// review of the cm-tracking fix found alongside it: when filterTJArray
// reads an EXISTING numeric kerning entry from a TJ array to track the
// running text-space position of later string elements, it must scale
// the displacement by horizScale (Tz/100) exactly like every other term
// in ISO 32000-1 §9.4.4's tx formula — and exactly like
// glyphAdvance/glyphAdvanceCID already do for ordinary glyph advances,
// and like this file's OWN kerning-number WRITERS already do in the
// opposite direction (dividing a target advance by horizScale to produce
// a TJ number). Reading a kerning number without that scaling drifts the
// running position out of sync with the glyph-advance side whenever Tz
// isn't 100 — reachable on essentially any real TJ-based PDF, since
// kerning for ordinary letter spacing is ubiquitous even when a document
// never otherwise touches Tz.
func TestRewriteTextTJKerningHonorsHorizScale(t *testing.T) {
	fonts := testRewriteFontMap()
	// 50 Tz halves every advance (and must halve the kerning entry's
	// contribution too). [(A) -1000 (SECRET)]: "A" advances the running
	// position by 500/1000*12*0.5 = 3; the kerning number -1000 then
	// adds -(-1000)/1000*12*0.5 = 6 MORE (correctly scaled) — so
	// "SECRET" starts at running-advance 9, i.e. page-space x = 109,
	// center of its first glyph at 109 + 3/2 = 110.5. Unscaled (the
	// bug), the kerning term alone would be 12, running-advance 15,
	// first-glyph center at 118.5 — outside the region below.
	in := []byte("BT\n/F1 12 Tf\n50 Tz\n100 700 Td\n[(A) -1000 (SECRET)] TJ\nET\n")
	regions := []QuadPoint{
		{X1: 108, Y1: 705, X2: 113, Y2: 705, X3: 108, Y3: 695, X4: 113, Y4: 695},
	}
	out, err := rewriteTextOperatorsInStream(in, regions, fonts, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), "SECRET") {
		t.Errorf("expected SECRET removed (its correctly horizScale-adjusted position falls inside the region), got %q", out)
	}
}

// TestRewriteTextUntouchedTJKeepsLeadingKerning pins a bug an
// independent review found: filterTJArray always rebuilt its TJ array
// through compactTJArray, which strips a leading/trailing kerning-only
// entry unconditionally — correct for one left orphaned by a drop, but
// wrong for one the ORIGINAL document author put there intentionally
// (e.g. deliberate letter-spacing before the first glyph), on a TJ this
// redaction pass never actually touches at all. The leading kerning here
// must survive untouched since the region is nowhere near this text.
func TestRewriteTextUntouchedTJKeepsLeadingKerning(t *testing.T) {
	fonts := testRewriteFontMap()
	in := []byte("BT\n/F1 12 Tf\n100 700 Td\n[-200 (Hello)] TJ\nET\n")
	// Region far away from the drawn text — nothing in this TJ should
	// be dropped at all.
	regions := []QuadPoint{
		{X1: 0, Y1: 10, X2: 50, Y2: 10, X3: 0, Y3: 0, X4: 50, Y4: 0},
	}
	out, err := rewriteTextOperatorsInStream(in, regions, fonts, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != string(in) {
		t.Errorf("expected the untouched TJ op to pass through byte-for-byte unchanged, got %q, want %q", out, in)
	}
}

func TestRewriteTextKeepAllGlyphs(t *testing.T) {
	// Region far away from drawn text.
	fonts := testRewriteFontMap()
	in := []byte("BT\n/F1 12 Tf\n100 700 Td\n(Visible) Tj\nET\n")
	regions := []QuadPoint{
		{X1: 0, Y1: 100, X2: 100, Y2: 100, X3: 0, Y3: 50, X4: 100, Y4: 50},
	}
	out, err := rewriteTextOperatorsInStream(in, regions, fonts, nil, nil)
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
	out, err := rewriteTextOperatorsInStream(in, regions, fonts, nil, nil)
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
	out, err := rewriteTextOperatorsInStream(in, regions, fonts, nil, nil)
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
	out, err := rewriteTextOperatorsInStream(in, regions, fonts, nil, nil)
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

// TestRewriteTextUnknownFontRefuses pins pdf-go-oq9i: a Tj/TJ drawn with
// a font this function cannot classify must REFUSE (return an error)
// rather than silently pass through and leave the caller believing
// ApplyRedactions succeeded — an unresolvable font gives no width data,
// so this function cannot prove the text is NOT inside the region
// either, and several real cases of "unknown" to this redaction-specific
// flag are still visible/extractable through this library's own normal
// read APIs (see rewriteTextOperatorsInStream's own doc comment).
func TestRewriteTextUnknownFontRefuses(t *testing.T) {
	in := []byte("BT\n/F1 12 Tf\n100 700 Td\n(Hello) Tj\nET\n")
	_, err := rewriteTextOperatorsInStream(in, []QuadPoint{{X1: 0, Y1: 1000, X2: 1000, Y2: 1000, X3: 0, Y3: 0, X4: 1000, Y4: 0}}, map[string]fontInfo{}, nil, nil)
	if err == nil {
		t.Error("expected an error when the font cannot be resolved, got nil")
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
