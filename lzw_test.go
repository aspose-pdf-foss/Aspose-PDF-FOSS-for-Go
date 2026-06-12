// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"testing"
)

// TestLZWDecodeSpecExample decodes a classic known LZW vector (clear table,
// literals, a KwKwK code, EOD): 80 0B 60 50 22 0C 0C 85 01 → "-----A---B".
func TestLZWDecodeSpecExample(t *testing.T) {
	enc := []byte{0x80, 0x0B, 0x60, 0x50, 0x22, 0x0C, 0x0C, 0x85, 0x01}
	want := []byte("-----A---B")
	got, err := lzwDecode(enc, 1)
	if err != nil {
		t.Fatalf("lzwDecode: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("lzwDecode = % x, want % x", got, want)
	}
}

// TestLZWDecodeTruncated verifies a stream cut off mid-data returns the bytes
// decoded so far without error (flateDecode-style tolerance).
func TestLZWDecodeTruncated(t *testing.T) {
	enc := []byte{0x80, 0x0B, 0x60, 0x50} // spec example cut short
	got, err := lzwDecode(enc, 1)
	if err != nil {
		t.Fatalf("lzwDecode: %v", err)
	}
	if len(got) == 0 || !bytes.HasPrefix(got, []byte("---")) {
		t.Errorf("lzwDecode truncated = % x, want prefix 2d 2d 2d", got)
	}
}

// TestLZWDecodeInvalidCode verifies a code far beyond the table errors instead
// of panicking.
func TestLZWDecodeInvalidCode(t *testing.T) {
	// 9-bit codes: 0x1FF (511) right after reset is far beyond table size 258.
	enc := []byte{0xFF, 0xFF, 0xFF}
	if _, err := lzwDecode(enc, 1); err == nil {
		t.Error("lzwDecode: expected error for invalid code, got nil")
	}
}

// TestPNGPredictorRGB verifies the PNG predictor honors the bytes-per-pixel
// distance for multi-component samples (42201.pdf: Predictor 15 /Colors 3
// icons failed with a stride mismatch and were skipped).
func TestPNGPredictorRGB(t *testing.T) {
	// One row, filter type 1 (Sub), two RGB8 pixels: the second adds the
	// first per channel (left reference is 3 bytes back, not 1).
	data := []byte{1, 10, 20, 30, 5, 6, 7}
	got, err := applyPNGPredictor(data, 6, 3)
	if err != nil {
		t.Fatalf("applyPNGPredictor: %v", err)
	}
	want := []byte{10, 20, 30, 15, 26, 37}
	if !bytes.Equal(got, want) {
		t.Errorf("got % d, want % d", got, want)
	}
}
