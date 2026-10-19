// SPDX-License-Identifier: MIT

package asposepdf

import "testing"

// TestRenderShowTextIdentityHNeverAppliesWordSpacingToMultiByteCode pins
// the render-side half of pdf-go-ifpy point 2: an Identity-H/V code is
// always 2 bytes (ISO 32000-1 §9.7.6.2), and ISO 32000-1 §9.3.3 restricts
// word spacing (Tw) to the single-byte code 32 only, never a multi-byte
// one — showText's Identity-H branch used to pass `code == 32` as the
// "is this eligible for Tw" flag, which for a 2-byte code checks the
// numeric CID value (0x0020) rather than any single-byte code at all, so
// a CJK font whose CID 32 happens to be a real, ordinary glyph (not a
// space) would have Tw wrongly added to its advance whenever Tw != 0.
// The cidCMap branch just above it was already correct (checks
// n == 1 && b[0] == 32, the raw single byte) — only the fixed-stride
// Identity-H path had this bug.
func TestRenderShowTextIdentityHNeverAppliesWordSpacingToMultiByteCode(t *testing.T) {
	rd := &renderer{}
	rd.ts.tm = identityMatrix()
	rd.ts.hScale = 1
	rd.ts.fontSize = 10
	rd.ts.wordSpace = 100 // deliberately large so a wrongly-applied Tw is unmissable
	rd.ts.font = &renderFont{
		isType0: true,
		fi:      fontInfo{defaultW: 500, cidWidths: map[uint16]float64{0x0020: 500}},
	}

	// One Identity-H glyph: the 2-byte code 0x00 0x20, i.e. CID 0x0020 —
	// numerically "32", but as a 2-byte code, not the single-byte space.
	rd.showText(string([]byte{0x00, 0x20}))

	gotTx := rd.ts.tm[4] // e (x translation) component
	wantTx := 500.0 / 1000.0 * 10
	if gotTx != wantTx {
		t.Errorf("tm x-translation after one Identity-H glyph (CID 0x0020) = %v, want %v (Tw must never apply to a 2-byte Identity-H code)", gotTx, wantTx)
	}
}
