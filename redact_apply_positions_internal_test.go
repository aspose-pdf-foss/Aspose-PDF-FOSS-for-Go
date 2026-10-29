// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"math"
	"strings"
	"testing"
)

// Removing glyphs must not move the text drawn after them. A show
// operator advances the text matrix for whatever the same text object
// draws next; the rewriter used to drop a fully redacted operator without
// its advance, and to strip leading/trailing kerning numbers from a
// rewritten TJ, so the following text slid left onto the redacted area
// (found while building ReplaceText's ShiftRestOfLine, which shares this
// rewriter).
func TestRedactionKeepsFollowingTextInPlace(t *testing.T) {
	cases := []struct{ name, content, secret, watch string }{
		{"start of a Tj", "BT %F 12 Tf 72 700 Td (SECRET visible) Tj ET", "SECRET", "visible"},
		{"a whole Tj, then another", "BT %F 12 Tf 72 700 Td (Price: ) Tj (SECRET) Tj ( EUR) Tj ET", "SECRET", "EUR"},
		{"end of a TJ, then a Tj", "BT %F 12 Tf 72 700 Td [(keep ) -100 (SECRET)] TJ ( after) Tj ET", "SECRET", "after"},
		{"start of a TJ", "BT %F 12 Tf 72 700 Td [(SECRET) -100 ( shown)] TJ ET", "SECRET", "shown"},
		{"a whole TJ, then a Tj", "BT %F 12 Tf 72 700 Td [(SEC) -50 (RET)] TJ ( next) Tj ET", "SECRET", "next"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			doc := NewDocumentFromFormat(PageFormatA4)
			p, _ := doc.Page(1)
			f, err := p.ensureStandardFontResource("/Helvetica")
			if err != nil {
				t.Fatal(err)
			}
			if err := replacePageContents(p, []byte(strings.ReplaceAll(c.content, "%F", f))); err != nil {
				t.Fatal(err)
			}
			before := findOne(t, p, c.watch)
			if err := p.Annotations().Add(NewRedactAnnotation(p, findOne(t, p, c.secret))); err != nil {
				t.Fatal(err)
			}
			if err := doc.ApplyRedactions(); err != nil {
				t.Fatal(err)
			}
			if after := findOne(t, p, c.watch); math.Abs(after.LLX-before.LLX) > 0.01 {
				t.Errorf("%q moved from x=%.2f to x=%.2f", c.watch, before.LLX, after.LLX)
			}
			var buf bytes.Buffer
			if _, err := doc.WriteTo(&buf); err != nil {
				t.Fatal(err)
			}
			re, err := OpenStream(bytes.NewReader(buf.Bytes()))
			if err != nil {
				t.Fatal(err)
			}
			if found, n := documentContainsBytes(re, "SEC"); found {
				t.Errorf("redacted text still in %d object(s)", n)
			}
		})
	}
}
