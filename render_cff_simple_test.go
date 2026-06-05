// SPDX-License-Identifier: MIT

package asposepdf

import "testing"

// TestSimpleCFFGidUsesEncoding checks that a simple (non-Type0) CFF font selects
// glyphs through the CFF code→GID map (simpleGID), not the TrueType cmap path —
// the fix for embedded Type1C fonts (e.g. MinionPro subsets) that previously
// mapped every code to GID 0 and so rendered no text.
func TestSimpleCFFGidUsesEncoding(t *testing.T) {
	f := &renderFont{
		cff: &cffFont{simpleGID: map[uint16]uint16{'A': 7, 'b': 12}},
		em:  1000,
	}
	if g := f.gid('A'); g != 7 {
		t.Errorf("gid('A') = %d, want 7", g)
	}
	if g := f.gid('b'); g != 12 {
		t.Errorf("gid('b') = %d, want 12", g)
	}
	if g := f.gid('Z'); g != 0 { // not in the map → no glyph
		t.Errorf("gid('Z') = %d, want 0", g)
	}
}

// TestBuildSimpleEncodingStandardRange checks the Standard-encoding SID = code−31
// correspondence: a charset that gives glyph 'A' (SID 34) at GID 5 should map
// code 'A' (65) to GID 5.
func TestBuildSimpleEncodingStandardRange(t *testing.T) {
	f := &cffFont{
		numGlyphs: 6,
		charset:   []uint16{0, 0, 0, 0, 0, 34}, // GID 5 → SID 34 ('A')
	}
	// Top DICT with no Encoding (→ Standard) and no charset offset (we set
	// f.charset directly), so buildSimpleEncoding reuses it.
	f.buildSimpleEncoding(nil, map[int][]float64{})
	if g := f.simpleGID['A']; g != 5 {
		t.Errorf("simpleGID['A'] = %d, want 5", g)
	}
}
