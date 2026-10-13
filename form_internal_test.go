// SPDX-License-Identifier: MIT

package asposepdf

import "testing"

func TestEncodeFormStringASCII(t *testing.T) {
	got := encodeFormString("plain ASCII")
	if got != "plain ASCII" {
		t.Errorf("ASCII passthrough failed: got %q", got)
	}
}

func TestEncodeFormStringCyrillic(t *testing.T) {
	got := encodeFormString("привет")
	want := "\xFE\xFF" + "\x04\x3F\x04\x40\x04\x38\x04\x32\x04\x35\x04\x42"
	if got != want {
		t.Errorf("Cyrillic encoding mismatch:\ngot:  %x\nwant: %x", got, want)
	}
}

func TestDecodeFormStringRoundTrip(t *testing.T) {
	cases := []string{"hello", "привет", "with\nnewline", "Zéà"}
	for _, in := range cases {
		got := decodeFormString(encodeFormString(in))
		if got != in {
			t.Errorf("round-trip mismatch: in=%q got=%q", in, got)
		}
	}
}

// TestEncodeFormStringInvalidUTF8PassesThrough pins a GitHub issue #13
// compatibility consideration: a non-ASCII Go string that is not valid
// UTF-8 is returned unchanged rather than corrupted by decoding it as
// UTF-8 anyway (every invalid byte would otherwise become U+FFFD via
// `for range`) -- the shape a caller's own pre-encoding workaround (raw
// PDFDocEncoding high bytes, or hand-built UTF-16BE+BOM bytes) takes.
func TestEncodeFormStringInvalidUTF8PassesThrough(t *testing.T) {
	invalid := string([]byte{0xFE, 0xFF, 0x00, 0x41, 0xC0, 0xC1}) // BOM + 'A' + two invalid UTF-8 lead bytes
	got := encodeFormString(invalid)
	if got != invalid {
		t.Errorf("invalid-UTF-8 input was altered: got %x, want %x (unchanged)", got, invalid)
	}
}
