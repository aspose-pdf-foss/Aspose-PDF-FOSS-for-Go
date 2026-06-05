// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"compress/zlib"
	"testing"
)

// TestFlateDecodeTolerantChecksum checks that DEFLATE data with a corrupt
// trailing Adler-32 checksum still yields the decompressed bytes rather than
// being discarded — real-world PDFs (e.g. 30066.pdf) ship such streams, and
// throwing the data away blanks the whole page.
func TestFlateDecodeTolerantChecksum(t *testing.T) {
	want := []byte("BT /F1 12 Tf (hello world) Tj ET\n")
	var buf bytes.Buffer
	zw := zlib.NewWriter(&buf)
	zw.Write(want)
	zw.Close()
	enc := buf.Bytes()

	// Corrupt the last 4 bytes (the Adler-32 checksum).
	for i := len(enc) - 4; i < len(enc); i++ {
		enc[i] ^= 0xFF
	}

	got, err := flateDecode(enc)
	if err != nil {
		t.Fatalf("flateDecode returned error on bad checksum: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("decoded %q, want %q", got, want)
	}
}

// TestFlateDecodeRejectsBadHeader checks that a non-DEFLATE (e.g. random or
// still-encrypted) stream is still rejected rather than inflated into garbage —
// the decryption pipeline relies on this.
func TestFlateDecodeRejectsBadHeader(t *testing.T) {
	if _, err := flateDecode([]byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06}); err == nil {
		t.Error("expected error decoding non-DEFLATE bytes, got nil")
	}
}
