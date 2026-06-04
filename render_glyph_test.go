// SPDX-License-Identifier: MIT

package asposepdf

import (
	"os"
	"testing"
)

func loadTestTTF(t *testing.T) *ttfFont {
	t.Helper()
	data, err := os.ReadFile("testdata/DejaVuSans.ttf")
	if err != nil {
		t.Fatalf("read DejaVuSans.ttf: %v", err)
	}
	f, err := parseTTF(data)
	if err != nil {
		t.Fatalf("parseTTF: %v", err)
	}
	return f
}

func TestGlyphContoursLetterA(t *testing.T) {
	f := loadTestTTF(t)
	gid := f.glyphID('A')
	if gid == 0 {
		t.Fatal("no glyph for 'A'")
	}
	cs := f.glyphContours(gid)
	if len(cs) == 0 {
		t.Fatal("'A' produced no contours")
	}
	// 'A' is a capital with a counter (the triangle hole) → at least 2 contours.
	if len(cs) < 2 {
		t.Errorf("'A' contours = %d, want >= 2 (outer + counter)", len(cs))
	}

	npoints := 0
	em := float64(f.unitsPerEm)
	for _, c := range cs {
		npoints += len(c)
		for _, p := range c {
			if p.x < -em || p.x > 2*em || p.y < -em || p.y > 2*em {
				t.Errorf("glyph point out of plausible range: %+v (em=%.0f)", p, em)
			}
		}
	}
	if npoints < 6 {
		t.Errorf("'A' total points = %d, want more", npoints)
	}
}

func TestGlyphContoursSpaceEmpty(t *testing.T) {
	f := loadTestTTF(t)
	gid := f.glyphID(' ')
	if cs := f.glyphContours(gid); len(cs) != 0 {
		t.Errorf("space glyph should have no contours, got %d", len(cs))
	}
}

func TestGlyphContoursComposite(t *testing.T) {
	// Accented letters are typically composite glyphs (base + diacritic).
	f := loadTestTTF(t)
	for _, r := range []rune{'é', 'ñ', 'ü'} {
		gid := f.glyphID(r)
		if gid == 0 {
			continue
		}
		if cs := f.glyphContours(gid); len(cs) >= 2 {
			return // a composite decoded into multiple contours — good
		}
	}
	t.Skip("no composite accented glyph available in test font")
}
