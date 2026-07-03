// SPDX-License-Identifier: MIT

package asposepdf

import "testing"

// TestPredefinedCMapDecode verifies the predefined Adobe CJK CMaps decode the
// variable-width codespace to the right CIDs, and that the per-ordering
// CID→Unicode tables map those CIDs to the expected characters.
func TestPredefinedCMapDecode(t *testing.T) {
	cases := []struct {
		cmap, ordering string
		bytes          []byte
		wantN          int
		wantRune       rune
	}{
		// GBK-EUC-H (Simplified Chinese, Adobe-GB1): 这 = GBK D5E2.
		{"GBK-EUC-H", "GB1", []byte{0xD5, 0xE2}, 2, '这'},
		// 90ms-RKSJ-H (Japanese, Adobe-Japan1): 日 = Shift-JIS 93FA.
		{"90ms-RKSJ-H", "Japan1", []byte{0x93, 0xFA}, 2, '日'},
		// 90ms-RKSJ-H half-width katakana ｱ = single byte 0xB1.
		{"90ms-RKSJ-H", "Japan1", []byte{0xB1}, 1, 'ｱ'},
	}
	for _, c := range cases {
		cm := predefinedCMap(c.cmap)
		if cm == nil {
			t.Errorf("%s: not bundled", c.cmap)
			continue
		}
		uni := cidToUnicodeForOrdering(c.ordering)
		if uni == nil {
			t.Errorf("%s: no CID→Unicode table", c.ordering)
			continue
		}
		_, cid, n := cm.next(c.bytes)
		if n != c.wantN {
			t.Errorf("%s % x: byte length = %d, want %d", c.cmap, c.bytes, n, c.wantN)
		}
		if got := uni[cid]; got != c.wantRune {
			t.Errorf("%s % x: CID %d → %U (%q), want %U (%q)", c.cmap, c.bytes, cid, got, got, c.wantRune, c.wantRune)
		}
	}
}

// TestCJKAsciiFallback verifies Latin codes that a CJK CMap maps to
// proportional-Latin CIDs (with no Unicode in Adobe's table) fall back to the
// ASCII code itself, so "ABC" inside a GBK font still resolves.
func TestCJKAsciiFallback(t *testing.T) {
	cm := predefinedCMap("GBK-EUC-H")
	uni := cidToUnicodeForOrdering("GB1")
	fb := cjkASCIIFallback(cm, uni)
	_, cid, _ := cm.next([]byte{0x41}) // 'A'
	if got := fb[cid]; got != 'A' {
		t.Errorf("ASCII fallback for 0x41 → %U, want 'A' (CID %d)", got, cid)
	}
}

// TestPredefinedCMapUsecmap verifies a CMap that chains another via usecmap
// inherits the parent's ranges (ETenms-B5-H usecmap ETen-B5-H).
func TestPredefinedCMapUsecmap(t *testing.T) {
	child := predefinedCMap("ETenms-B5-H")
	if child == nil {
		t.Skip("ETenms-B5-H not bundled")
	}
	parent := predefinedCMap("ETen-B5-H")
	if parent == nil {
		t.Fatal("ETen-B5-H not bundled")
	}
	// A Big5 ideograph (一 = Big5 A440) must decode identically through both.
	b := []byte{0xA4, 0x40}
	_, cidChild, _ := child.next(b)
	_, cidParent, _ := parent.next(b)
	if cidChild == 0 || cidChild != cidParent {
		t.Errorf("usecmap inheritance: child CID %d, parent CID %d", cidChild, cidParent)
	}
}
