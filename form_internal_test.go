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
