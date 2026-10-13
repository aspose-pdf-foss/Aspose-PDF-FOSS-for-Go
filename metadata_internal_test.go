// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"testing"
)

// TestInfoNonASCIIUsesTextStringEncoding pins the actual byte encoding
// GitHub issue #13 is about: a round-trip through this library's own
// Save/Open can't tell a correct ISO 32000-1 §7.9.2.2 text string apart
// from raw UTF-8 bytes, since Info() always decodes however SetInfo
// encoded -- only inspecting the stored dict value directly (as a reader
// like pdfinfo, which assumes PDFDocEncoding/UTF-16BE, would see it)
// catches the bug.
func TestInfoNonASCIIUsesTextStringEncoding(t *testing.T) {
	dict := buildInfoDict(DocumentInfo{Title: "Meldergruppenauslösung"})
	v, ok := dict["/Title"].(string)
	if !ok {
		t.Fatal("/Title is not a string value")
	}
	if len(v) < 2 || v[0] != 0xFE || v[1] != 0xFF {
		t.Fatalf("/Title = %x, want a UTF-16BE value starting with the 0xFE 0xFF byte-order mark", v)
	}
	if bytes.Contains([]byte(v), []byte("ö")) {
		t.Error("/Title still contains the raw UTF-8 encoding of 'ö' -- not actually text-string encoded")
	}
}

// TestInfoASCIIStaysPlainLiteral confirms an ASCII Info value is stored
// as a plain literal, not forced through UTF-16BE (which would be
// correct but wasteful and a needless behavior change for the common case).
func TestInfoASCIIStaysPlainLiteral(t *testing.T) {
	dict := buildInfoDict(DocumentInfo{Title: "Plain Title"})
	v, ok := dict["/Title"].(string)
	if !ok || v != "Plain Title" {
		t.Errorf("/Title = %q, want unchanged %q", v, "Plain Title")
	}
}
