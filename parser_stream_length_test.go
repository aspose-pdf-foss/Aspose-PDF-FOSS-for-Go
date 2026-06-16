// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"testing"
)

// TestReadStreamDataWrongLength checks that a direct but wrong /Length does not
// truncate the stream: the reader must verify that "endstream" follows the
// claimed length and, when it does not, fall back to scanning for the
// terminator (ISO 32000-1 §7.3.8.1). 36263.pdf ships a content stream declared
// "/Length 1" in front of ~5.7 KB of operators; trusting the 1 blanked the page.
func TestReadStreamDataWrongLength(t *testing.T) {
	body := []byte("  1.0 1.0 1.0 rg\n  30 30 540 690 re B\n")
	raw := append(append([]byte{}, body...), []byte("\nendstream\nendobj")...)

	l := newLexer(raw) // positioned at the start of the stream data
	got, err := readStreamData(l, pdfDict{"/Length": 1})
	if err != nil {
		t.Fatalf("readStreamData: %v", err)
	}
	if !bytes.Equal(got, body) {
		t.Errorf("wrong /Length: got %d bytes %q, want %d bytes", len(got), got, len(body))
	}
}

// TestReadStreamDataCorrectLength confirms the fast path still trusts a correct
// direct /Length verbatim (the "endstream" verification passes).
func TestReadStreamDataCorrectLength(t *testing.T) {
	body := []byte("BT /F1 12 Tf (hi) Tj ET")
	raw := append(append([]byte{}, body...), []byte("\nendstream\nendobj")...)

	l := newLexer(raw)
	got, err := readStreamData(l, pdfDict{"/Length": len(body)})
	if err != nil {
		t.Fatalf("readStreamData: %v", err)
	}
	if !bytes.Equal(got, body) {
		t.Errorf("correct /Length: got %q, want %q", got, body)
	}
}
