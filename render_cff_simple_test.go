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
// correspondence over the predefined ISOAdobe charset (Top DICT charset offset
// 0 → GID i = SID i): code 'H' (72) → Standard SID 41 → GID 41. Before the fix
// the predefined charset was left empty so nothing mapped (e.g. NewsGothicBT,
// whose body text rendered blank).
func TestBuildSimpleEncodingStandardRange(t *testing.T) {
	f := &cffFont{numGlyphs: 100} // no custom charset/encoding → ISOAdobe + Standard
	f.buildSimpleEncoding(nil, map[int][]float64{})
	if g := f.simpleGID['H']; g != 41 {
		t.Errorf("simpleGID['H'] = %d, want 41 (ISOAdobe identity + Standard encoding)", g)
	}
	if len(f.simpleGID) == 0 {
		t.Error("simpleGID empty — predefined ISOAdobe charset not applied")
	}
}
