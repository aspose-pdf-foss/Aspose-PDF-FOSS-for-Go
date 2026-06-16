// SPDX-License-Identifier: MIT

package asposepdf

import (
	"strings"
	"testing"
)

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
