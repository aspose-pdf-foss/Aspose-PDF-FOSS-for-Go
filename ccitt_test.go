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

// TestCCITTWhiteMakeupCodes verifies every white make-up code (64..1728) against
// the canonical ITU-T T.4 Table 2 bit patterns. The 1280..1728 entries were
// transcribed wrong originally (0x91..0x97), which only surfaced when a scan used
// a horizontal-mode wide white run — V0-coded all-white rows never hit them — so
// most of the page silently failed to decode. Pin the whole sub-table here.
func TestCCITTWhiteMakeupCodes(t *testing.T) {
	want := map[int]string{
		64: "11011", 128: "10010", 192: "010111", 256: "0110111",
		320: "00110110", 384: "00110111", 448: "01100100", 512: "01100101",
		576: "01101000", 640: "01100111", 704: "011001100", 768: "011001101",
		832: "011010010", 896: "011010011", 960: "011010100", 1024: "011010101",
		1088: "011010110", 1152: "011010111", 1216: "011011000", 1280: "011011001",
		1344: "011011010", 1408: "011011011", 1472: "010011000", 1536: "010011001",
		1600: "010011010", 1664: "011000", 1728: "010011011",
	}
	for run, bits := range want {
		var code uint32
		for _, c := range bits {
			code = code<<1 | uint32(c-'0')
		}
		key := uint32(len(bits))<<16 | code
		if v, ok := whiteCodes[key]; !ok || v != run {
			t.Errorf("white makeup %q (run %d) = %d,%v want %d,true", bits, run, v, ok, run)
		}
	}
}

// TestCCITTDecodeHorizWideWhite decodes a single 1728-column Group 4 row coded in
// horizontal mode with a full-width white run via make-up 1728 — one of the codes
// that was wrong (0x97 instead of 0x9B). Before the fix this run failed to match
// and the row (and everything after it) was lost. White packs as 1, so the whole
// 216-byte row must be 0xFF.
func TestCCITTDecodeHorizWideWhite(t *testing.T) {
	// Horiz(001) + white run 1728 (makeup 010011011 + term 0 00110101) + black 0
	// (0000110111). a1=1728, a2=1728 → whole row white.
	bitstr := "001" + "010011011" + "00110101" + "0000110111"
	var data []byte
	var cur byte
	n := 0
	for _, c := range bitstr {
		cur = cur<<1 | byte(c-'0')
		if n++; n == 8 {
			data = append(data, cur)
			cur, n = 0, 0
		}
	}
	if n > 0 {
		data = append(data, cur<<(8-uint(n)))
	}
	out, err := ccittDecode(data, ccittParams{k: -1, columns: 1728, rows: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 216 {
		t.Fatalf("got %d bytes, want 216 (one 1728-px row)", len(out))
	}
	for i, b := range out {
		if b != 0xFF {
			t.Errorf("byte %d = %#02x, want 0xff (all white)", i, b)
		}
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
