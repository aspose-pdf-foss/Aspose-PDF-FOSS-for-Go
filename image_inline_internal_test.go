// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"strings"
	"testing"
)

// TestParseInlineImageBinaryLength checks that an UNFILTERED inline image is
// consumed by its exact computed byte length, not by scanning for "EI". The raw
// 1-bpp samples here contain a literal "EI" and a "<space>EI" — a scan would
// truncate the image mid-data and swallow the operator that follows. 35862.pdf
// (156 mask bars) exhibited exactly this.
func TestParseInlineImageBinaryLength(t *testing.T) {
	// /W 16 /H 2 /BPC 1 mask → ceil(16/8)=2 bytes/row × 2 = 4 bytes of data.
	// Craft those 4 bytes to include the ASCII for "EI" and " EI".
	data := []byte{'E', 'I', ' ', 'I'} // contains "EI" at 0 and " I" — adversarial
	var buf bytes.Buffer
	buf.WriteString("/W 16 /H 2 /BPC 1 /IM true ID ")
	buf.Write(data)
	buf.WriteString("EI\n0 0 m S") // real terminator, then an operator that must survive

	l := newLexer(buf.Bytes())
	dict, got := parseInlineImage(l)
	if dict == nil {
		t.Fatal("inline image failed to parse")
	}
	if !bytes.Equal(got, data) {
		t.Errorf("data = %v, want %v (length-based consume failed)", got, data)
	}

	// The operator after the real EI must still be lexable.
	tok, err := l.Next()
	if err != nil {
		t.Fatal(err)
	}
	if string(tok.raw) != "0" {
		t.Errorf("after EI, next token = %q; the scan overran the image", tok.raw)
	}
}

// TestInlineRawDataLen spot-checks the byte-length computation for the common
// inline colour spaces (rows byte-aligned).
func TestInlineRawDataLen(t *testing.T) {
	cases := []struct {
		dict pdfDict
		want int
	}{
		{pdfDict{"/Width": 84, "/Height": 8, "/ImageMask": true}, 11 * 8}, // 84 bits → 11 B/row
		{pdfDict{"/Width": 4, "/Height": 4, "/BitsPerComponent": 8, "/ColorSpace": pdfName("/DeviceRGB")}, 4 * 3 * 4},
		{pdfDict{"/Width": 2, "/Height": 2, "/BitsPerComponent": 8, "/ColorSpace": pdfName("/DeviceGray")}, 2 * 2},
		{pdfDict{"/Width": 10, "/Height": 1, "/BitsPerComponent": 8, "/ColorSpace": pdfName("/DeviceCMYK")}, 10 * 4},
		// Filtered → unknown length.
		{pdfDict{"/Width": 4, "/Height": 4, "/BitsPerComponent": 1, "/ImageMask": true, "/Filter": pdfName("/ASCIIHexDecode")}, -1},
		// Unknown colour space → unknown length.
		{pdfDict{"/Width": 4, "/Height": 4, "/BitsPerComponent": 8, "/ColorSpace": pdfName("/SomeResource")}, -1},
	}
	for i, c := range cases {
		if got := inlineRawDataLen(c.dict); got != c.want {
			t.Errorf("case %d: inlineRawDataLen = %d, want %d", i, got, c.want)
		}
	}
}

// TestParseInlineImageAHxTerminator checks that an inline image whose data ends
// with the ASCIIHex '>' EOD marker directly followed by "EI" (no whitespace) is
// parsed without overrunning into the next image. Real producers emit glyph
// masks this way ("...3F>EI"); requiring whitespace before EI dropped them.
func TestParseInlineImageAHxTerminator(t *testing.T) {
	// Two back-to-back inline image masks: the first ends ">EI" (AHx EOD), the
	// second ends "\nEI" (whitespace). Both must parse and keep their boundary.
	content := "/W 2 /H 2 /BPC 1 /F [/AHx] /IM true ID FF00>EI " +
		"/W 2 /H 2 /BPC 1 /F [/AHx] /IM true ID 00FF\nEI Q"

	l := newLexer([]byte(content))

	dict1, data1 := parseInlineImage(l)
	if dict1 == nil {
		t.Fatal("first image (>EI terminator) failed to parse")
	}
	if got := strings.TrimSpace(string(data1)); got != "FF00>" {
		t.Errorf("first image data = %q, want %q (overran into next image)", got, "FF00>")
	}

	// The lexer must now be positioned at the second image, not past it.
	tok, err := l.Next()
	if err != nil {
		t.Fatal(err)
	}
	if string(tok.raw) != "/W" || tok.kind != tokName {
		t.Fatalf("after first EI, next token = %q (kind %d); scan overran the second image", tok.raw, tok.kind)
	}
}
