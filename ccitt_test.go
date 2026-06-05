// SPDX-License-Identifier: MIT

package asposepdf

import "testing"

// TestCCITTTablesBuilt spot-checks a few well-known T.4 run-length codes.
func TestCCITTTablesBuilt(t *testing.T) {
	// White run 0 = 00110101 (8 bits, 0x35).
	if v, ok := whiteCodes[uint32(8)<<16|0x35]; !ok || v != 0 {
		t.Errorf("white 0x35/8 = %d,%v want 0,true", v, ok)
	}
	// Black run 2 = 11 (2 bits, 0x3).
	if v, ok := blackCodes[uint32(2)<<16|0x03]; !ok || v != 2 {
		t.Errorf("black 0x03/2 = %d,%v want 2,true", v, ok)
	}
	// Shared makeup 1792 = 00000001000 (11 bits, 0x08) in both tables.
	if v, ok := whiteCodes[uint32(11)<<16|0x08]; !ok || v != 1792 {
		t.Errorf("white makeup 0x08/11 = %d,%v want 1792,true", v, ok)
	}
}

// TestCCITTDecodeAllWhite decodes a Group 4 stream of all-white rows: each row
// is a single V0 mode bit ('1'). With /BlackIs1 false, white packs as 1 bits, so
// every output byte is 0xFF.
func TestCCITTDecodeAllWhite(t *testing.T) {
	// 8 columns, 8 rows → 8 V0 bits = one 0xFF byte.
	out, err := ccittDecode([]byte{0xFF}, ccittParams{k: -1, columns: 8, rows: 8})
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 8 {
		t.Fatalf("got %d bytes, want 8", len(out))
	}
	for i, b := range out {
		if b != 0xFF {
			t.Errorf("row %d = %#02x, want 0xff (all white)", i, b)
		}
	}
}

// TestCCITTRejectsGroup3TwoD checks that mixed Group 3 2-D (/K>0) is reported
// as unsupported rather than mis-decoded.
func TestCCITTRejectsGroup3TwoD(t *testing.T) {
	if _, err := ccittDecode([]byte{0x00}, ccittParams{k: 4, columns: 8, rows: 1}); err == nil {
		t.Error("expected error for /K>0, got nil")
	}
}
