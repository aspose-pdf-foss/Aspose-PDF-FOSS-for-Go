// SPDX-License-Identifier: MIT

package asposepdf

import (
	"os"
	"testing"
)

// TestStandaloneSFNT deterministically exercises the .ttc re-wrap machinery: it
// re-wraps a TrueType face through standaloneSFNT/assembleSFNT (the same path a
// .ttc sub-font takes) and verifies the result is a valid, equivalent sfnt. A
// real .ttc collection is not committed, so a single-font .ttf stands in — the
// re-wrap copies the same table directory either way.
func TestStandaloneSFNT(t *testing.T) {
	data, err := os.ReadFile("testdata/DejaVuSans.ttf")
	if err != nil {
		t.Skipf("test font unavailable: %v", err)
	}
	f, err := parseTTF(data)
	if err != nil {
		t.Fatalf("parseTTF: %v", err)
	}
	sfnt, err := standaloneSFNT(f)
	if err != nil {
		t.Fatalf("standaloneSFNT: %v", err)
	}
	f2, err := parseTTF(sfnt)
	if err != nil {
		t.Fatalf("re-wrapped sfnt does not parse: %v", err)
	}
	if f2.numGlyphs != f.numGlyphs {
		t.Errorf("glyph count = %d, want %d", f2.numGlyphs, f.numGlyphs)
	}
	if f2.postScriptName != f.postScriptName {
		t.Errorf("PostScript name = %q, want %q", f2.postScriptName, f.postScriptName)
	}
	if f2.unitsPerEm != f.unitsPerEm {
		t.Errorf("unitsPerEm = %d, want %d", f2.unitsPerEm, f.unitsPerEm)
	}
}
