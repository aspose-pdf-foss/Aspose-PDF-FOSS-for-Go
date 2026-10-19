// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"strings"
	"testing"
)

// TestWriteValueEscapesInfoCustomKeyNames pins the report's own
// reproduction: writeValue wrote a PDF name verbatim (both as a dict
// value and, critically, as a dict KEY), so a caller-supplied string
// containing a delimiter or whitespace -- a Document.Info custom key,
// here -- ended the name early and the rest became further PDF syntax
// the caller never intended to write. Checked forensically (the raw
// saved bytes), not just via the round-tripped high-level API, since a
// successful round-trip alone would not distinguish "wrote one
// correctly-escaped name" from "happened to write and re-parse two
// coincidentally-matching tokens."
func TestWriteValueEscapesInfoCustomKeyNames(t *testing.T) {
	const maliciousKey = "Dept /Injected (yes) /X"
	d := NewDocument(200, 200)
	d.SetInfo(DocumentInfo{Title: "t", Custom: map[string]string{maliciousKey: "v"}})

	var buf bytes.Buffer
	if _, err := d.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	raw := buf.String()

	// The literal, unescaped injected name "/Injected" must not appear
	// anywhere in the saved file -- if it does, it was written as its
	// own separate token instead of being folded into one escaped name.
	if strings.Contains(raw, "/Injected") {
		t.Errorf("saved file contains an unescaped injected name token:\n%s", raw)
	}
	// The escaped form must be present as one token: "/" (slash, kept
	// escaped-space) + "Dept" + #20 (space) + #2F (/) + "Injected" + ...
	if !strings.Contains(raw, "#2FInjected") {
		t.Errorf("saved file does not contain the expected escaped name; got:\n%s", raw)
	}

	reopened, err := OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	info, err := reopened.Info()
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Custom[maliciousKey]; got != "v" {
		t.Errorf("Custom[%q] = %q after round-trip, want %q (Custom = %#v)", maliciousKey, got, "v", info.Custom)
	}
	if len(info.Custom) != 1 {
		t.Errorf("got %d custom entries after round-trip, want exactly 1: %#v", len(info.Custom), info.Custom)
	}
}

// TestWriteValueEscapesRadioItemExportNames exercises a second real
// caller-facing path the report named: a radio button's Export name
// becomes a /AP/N dict KEY (AddRadioGroup, form.go) via plain Go string
// concatenation ("/" + it.Export), with no escaping of its own --
// relying entirely on the centralized fix in writeValue to be safe. A
// malicious Export containing PDF delimiters must not inject a /AA
// (additional-actions) entry or otherwise corrupt the widget dict.
func TestWriteValueEscapesRadioItemExportNames(t *testing.T) {
	const maliciousExport = "Yes) /AA << /D 1 0 R >> /Dummy (x"
	d := NewDocumentFromFormat(PageFormatA4)
	items := []RadioItem{
		{PageNum: 1, Rect: Rectangle{LLX: 10, LLY: 10, URX: 30, URY: 30}, Export: maliciousExport},
	}
	if _, err := d.Form().AddRadioGroup("choice", items); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	if _, err := d.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	raw := buf.String()

	// An unescaped injection would add a real /AA entry to whatever dict
	// the broken name token ended up inside. No /AA should exist anywhere
	// in this document -- nothing here ever asked for one.
	if strings.Contains(raw, "/AA") {
		t.Errorf("saved file contains an injected /AA entry:\n%s", raw)
	}

	reopened, err := OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	field := reopened.Form().Field("choice")
	if field == nil {
		t.Fatal("radio group field did not survive round-trip")
	}
}

// TestDecodeNameEscapesRoundTrip checks writeNameValue/decodeNameEscapes
// symmetry directly, across every byte class ISO 32000-1 §7.3.5 singles
// out, plus the tolerant handling of a malformed trailing "#".
func TestDecodeNameEscapesRoundTrip(t *testing.T) {
	cases := []string{
		"/Simple",
		"/with space",
		"/with(paren)",
		"/with/slash",
		"/with%percent",
		"/with#hash",
		"/with<angle>bracket",
		"/with[square]bracket",
		"/with{curly}brace",
		"/" + string([]byte{0x00, 0x01, 0x7F}), // control bytes and DEL, all outside the regular range
	}
	for _, name := range cases {
		var buf bytes.Buffer
		if err := writeNameValue(&buf, pdfName(name)); err != nil {
			t.Fatalf("writeNameValue(%q): %v", name, err)
		}
		written := buf.Bytes()
		for _, c := range written[1:] {
			if nameByteNeedsEscape(c) && c != '#' {
				t.Errorf("writeNameValue(%q) wrote raw delimiter/control byte %q in %q", name, c, written)
			}
		}
		decoded := decodeNameEscapes(written)
		if string(decoded) != name {
			t.Errorf("round-trip mismatch: wrote %q, decoded back as %q, want %q", written, decoded, name)
		}
	}
}

// TestDecodeNameEscapesTrailingHashIsLiteral confirms a malformed "#"
// not followed by two hex digits (here, at the very end of the name)
// is kept literal rather than erroring -- this parser's general
// tolerance for non-conformant input, same convention as the rest of
// this codebase's lexer.
func TestDecodeNameEscapesTrailingHashIsLiteral(t *testing.T) {
	got := decodeNameEscapes([]byte("/Trailing#"))
	if string(got) != "/Trailing#" {
		t.Errorf("decodeNameEscapes(%q) = %q, want unchanged", "/Trailing#", got)
	}
	got = decodeNameEscapes([]byte("/Bad#ZZvalue"))
	if string(got) != "/Bad#ZZvalue" {
		t.Errorf("decodeNameEscapes with non-hex digits after # = %q, want unchanged", got)
	}
}

// TestWriteValueRefusesEmptyName matches the report's own suggestion
// ("optionally reject empty names") -- an empty pdfName has no leading
// "/" to anchor the escaping logic on and cannot be valid PDF syntax
// either way, so refusing outright is safer than silently writing
// nothing.
func TestWriteValueRefusesEmptyName(t *testing.T) {
	var buf bytes.Buffer
	if err := writeValue(&buf, pdfName(""), nil, nil); err == nil {
		t.Error("writeValue accepted an empty pdfName")
	}
}
