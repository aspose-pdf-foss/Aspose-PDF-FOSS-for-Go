// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"encoding/ascii85"
	"testing"
)

// encodeASCII85 produces spec-compliant ASCII85 bytes for s, terminated with ~>.
func encodeASCII85(s string) []byte {
	var buf bytes.Buffer
	enc := ascii85.NewEncoder(&buf)
	_, _ = enc.Write([]byte(s))
	_ = enc.Close()
	buf.WriteString("~>")
	return buf.Bytes()
}

// TestDecryptStreamInPlaceASCII85 is the core regression test for pdf-go-2qq.
// Before the fix, ascii85Decode accepted any input bytes without validation, so
// it "decoded" RC4-encrypted stream bytes into garbage and returned Decoded=true.
// decryptStreamInPlace then skipped the stream (early-return on Decoded=true),
// leaving silently corrupted data.  With the fix, ascii85Decode rejects bytes
// outside the valid alphabet, the parser stores Decoded=false, and
// decryptStreamInPlace can correctly decrypt-then-decode.
func TestDecryptStreamInPlaceASCII85(t *testing.T) {
	const plaintext = "BT /F1 12 Tf 50 700 Td (ascii85 round-trip) Tj ET"

	// Step 1: encode plaintext to ASCII85 — the form the stream would have on disk.
	filtered := encodeASCII85(plaintext)

	// Step 2: encrypt with a known document key + object number.
	state := &encryptState{key: []byte("0123456789abcdef")}
	perObjectKey := state.objectKey(5)
	encrypted := make([]byte, len(filtered))
	copy(encrypted, filtered)
	applyRC4(encrypted, perObjectKey)

	// Verify that the encrypted bytes are NOT valid ASCII85 (precondition for
	// the bug to manifest — if they happened to be valid ASCII85 by chance the
	// early-return path would never trigger and there would be no bug).
	if err := validateASCII85(encrypted); err == nil {
		// This is extremely unlikely but possible in theory; skip rather than fail.
		t.Skip("encrypted bytes happen to be valid ASCII85 for this key/plaintext pair; skip")
	}

	// Step 3: build a stream as the parser would produce it post-fix:
	// Decoded=false because validateASCII85 rejected the encrypted bytes.
	streamObj := &pdfStream{
		Dict: pdfDict{
			"/Filter": pdfName("/ASCII85Decode"),
		},
		Data:    encrypted,
		Decoded: false,
	}

	// Step 4: run decryptStreamInPlace — should decrypt RC4, then ASCII85-decode.
	decryptStreamInPlace(streamObj, perObjectKey)

	if !streamObj.Decoded {
		t.Fatal("after decryptStreamInPlace, stream should be Decoded=true")
	}
	if string(streamObj.Data) != plaintext {
		t.Errorf("decrypted+decoded stream mismatch:\ngot:  %q\nwant: %q", streamObj.Data, plaintext)
	}
}

// TestASCII85DecodeRejectsInvalidBytes verifies that ascii85Decode returns an
// error for bytes that lie outside the valid ASCII85 alphabet (0x80–0xFF etc.).
// This is the property that prevents encrypted stream bytes from being silently
// mis-decoded.
func TestASCII85DecodeRejectsInvalidBytes(t *testing.T) {
	invalid := []byte{0x80, 0x90, 0xFF, 0xAA, 0xBB}
	if _, err := ascii85Decode(invalid); err == nil {
		t.Error("ascii85Decode(non-ascii85 bytes) should return an error, got nil")
	}
}

// TestASCII85DecodeAcceptsValidInput ensures the fix did not regress normal
// decoding — valid ASCII85 data must still decode correctly.
func TestASCII85DecodeAcceptsValidInput(t *testing.T) {
	const want = "Hello, ASCII85!"
	encoded := encodeASCII85(want)

	got, err := ascii85Decode(encoded)
	if err != nil {
		t.Fatalf("ascii85Decode(valid input): unexpected error: %v", err)
	}
	if string(got) != want {
		t.Errorf("ascii85Decode round-trip:\ngot:  %q\nwant: %q", got, want)
	}
}
