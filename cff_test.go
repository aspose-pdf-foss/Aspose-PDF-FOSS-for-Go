// SPDX-License-Identifier: MIT

package asposepdf

import (
	"math"
	"testing"
)

// cffIndex builds a CFF INDEX (offSize 1; entries must keep offsets < 256).
func cffIndex(entries [][]byte) []byte {
	if len(entries) == 0 {
		return []byte{0, 0}
	}
	var data []byte
	offsets := []int{1}
	for _, e := range entries {
		data = append(data, e...)
		offsets = append(offsets, len(data)+1)
	}
	out := []byte{byte(len(entries) >> 8), byte(len(entries)), 1}
	for _, o := range offsets {
		out = append(out, byte(o))
	}
	return append(out, data...)
}

func dictInt16(v int) []byte { return []byte{28, byte(v >> 8), byte(v)} }

// buildMinimalCFF assembles a tiny name-keyed CFF with two glyphs: .notdef
// (empty) and a 80×80 square at (10,10) drawn with rmoveto + rlineto.
func buildMinimalCFF() []byte {
	square := []byte{
		149, 149, 21, // rmoveto 10 10
		219, 139, // 80 0
		139, 219, // 0 80
		59, 139, // -80 0
		139, 59, // 0 -80
		5,  // rlineto
		14, // endchar
	}
	curve := []byte{
		139, 139, 21, // rmoveto 0 0
		139, 239, // 0 100  (control 1)
		239, 139, // 100 0  (control 2)
		139, 39, // 0 -100 (end)
		8,  // rrcurveto
		14, // endchar
	}
	header := []byte{1, 0, 4, 1}
	nameIndex := cffIndex([][]byte{[]byte("Test")})
	csIndex := cffIndex([][]byte{{0x0e}, square, curve}) // .notdef = endchar
	stringIndex := []byte{0, 0}
	gsubrIndex := []byte{0, 0}

	const topDictIndexLen = 16 // cffIndex of an 11-byte Top DICT
	csOff := len(header) + len(nameIndex) + topDictIndexLen + len(stringIndex) + len(gsubrIndex)
	privOff := csOff + len(csIndex)

	var topDict []byte
	topDict = append(topDict, dictInt16(csOff)...)
	topDict = append(topDict, 17) // CharStrings
	topDict = append(topDict, dictInt16(0)...)
	topDict = append(topDict, dictInt16(privOff)...)
	topDict = append(topDict, 18) // Private (size 0, offset privOff)
	topDictIndex := cffIndex([][]byte{topDict})
	if len(topDictIndex) != topDictIndexLen {
		panic("topDict size assumption broken")
	}

	var out []byte
	for _, s := range [][]byte{header, nameIndex, topDictIndex, stringIndex, gsubrIndex, csIndex} {
		out = append(out, s...)
	}
	return out
}

func TestParseCFFSquareGlyph(t *testing.T) {
	f, err := parseCFF(buildMinimalCFF())
	if err != nil {
		t.Fatalf("parseCFF: %v", err)
	}
	if f.numGlyphs != 3 {
		t.Fatalf("numGlyphs = %d, want 3", f.numGlyphs)
	}
	if got := f.glyphContours(0); len(got) != 0 {
		t.Errorf(".notdef produced %d contours, want 0", len(got))
	}

	contours := f.glyphContours(1)
	if len(contours) != 1 {
		t.Fatalf("square produced %d contours, want 1", len(contours))
	}
	minx, miny := math.Inf(1), math.Inf(1)
	maxx, maxy := math.Inf(-1), math.Inf(-1)
	for _, p := range contours[0] {
		if !p.on {
			t.Error("CFF contour point not on-curve (should be pre-flattened)")
		}
		minx, miny = math.Min(minx, p.x), math.Min(miny, p.y)
		maxx, maxy = math.Max(maxx, p.x), math.Max(maxy, p.y)
	}
	if math.Abs(minx-10) > 0.5 || math.Abs(miny-10) > 0.5 ||
		math.Abs(maxx-90) > 0.5 || math.Abs(maxy-90) > 0.5 {
		t.Errorf("square bbox = [%.1f %.1f %.1f %.1f], want ~[10 10 90 90]", minx, miny, maxx, maxy)
	}
}

// TestParseCFFCurveGlyph checks the cubic curve operator (rrcurveto) is
// interpreted and flattened into a multi-point on-curve contour that bulges out
// to the control hull.
func TestParseCFFCurveGlyph(t *testing.T) {
	f, err := parseCFF(buildMinimalCFF())
	if err != nil {
		t.Fatalf("parseCFF: %v", err)
	}
	contours := f.glyphContours(2)
	if len(contours) != 1 {
		t.Fatalf("curve produced %d contours, want 1", len(contours))
	}
	if len(contours[0]) < 5 {
		t.Errorf("curve flattened to %d points, want a smooth polyline (>=5)", len(contours[0]))
	}
	var maxx, maxy float64
	for _, p := range contours[0] {
		maxx, maxy = math.Max(maxx, p.x), math.Max(maxy, p.y)
	}
	// Control hull reaches (100,100); the curve should bulge well past the chord.
	if maxx < 50 || maxy < 50 {
		t.Errorf("curve extent = (%.1f,%.1f), want it to bulge past (50,50)", maxx, maxy)
	}
}

// TestCFFStdStrings sanity-checks the standard strings table against known SIDs
// (CFF spec Appendix A): the glyph-name lookups behind the PDF-encoding →
// charset path depend on these exact positions.
func TestCFFStdStrings(t *testing.T) {
	checks := map[int]string{
		0: ".notdef", 1: "space", 34: "A", 66: "a", 45: "L",
		228: "zcaron", 390: "Semibold",
	}
	for sid, want := range checks {
		if got := cffStdStrings[sid]; got != want {
			t.Errorf("cffStdStrings[%d] = %q, want %q", sid, got, want)
		}
	}
}

// TestCFFGlyphNameRune covers the AGL lookup plus the uniXXXX / uXXXX forms
// used by subset fonts for non-standard glyph names.
func TestCFFGlyphNameRune(t *testing.T) {
	cases := map[string]rune{
		"A":         'A',
		"adieresis": 'ä',
		"uni00E4":   'ä',
		"u00E4":     'ä',
		"u1F600":    '\U0001F600',
		"":          0,
		"nosuch":    0,
	}
	for name, want := range cases {
		if got := cffGlyphNameRune(name); got != want {
			t.Errorf("cffGlyphNameRune(%q) = %U, want %U", name, got, want)
		}
	}
}
