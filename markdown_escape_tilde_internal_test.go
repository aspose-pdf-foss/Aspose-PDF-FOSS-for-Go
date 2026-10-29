// SPDX-License-Identifier: MIT

package asposepdf

import (
	"strings"
	"testing"
)

// pdf-go-tqtb: mdEscapeText left "~" unescaped. MarkdownDiffOutputGenerator
// wraps a deletion in GFM "~~…~~", so deleted text that itself contains
// "~~" closed the strikethrough early and leaked the rest as plain text.
// The output is read back through this library's own GFM parser (which
// passes the CommonMark suite) and must give one strikethrough holding the
// original text.
func TestMarkdownDiffOutputDeletionWithTildes(t *testing.T) {
	for _, deleted := range []string{"a~~b", "~~x~~", "approx ~5", "a~b~c"} {
		md, err := NewMarkdownDiffOutputGenerator().GenerateOutput([]DiffOperation{
			{Operation: OperationEqual, Text: "keep"},
			{Operation: OperationDelete, Text: deleted},
		})
		if err != nil {
			t.Fatal(err)
		}
		got := strings.TrimSpace(mdTestHTML(parseMarkdown(md)))
		want := "<p>keep <del>" + deleted + "</del></p>"
		if got != want {
			t.Errorf("deleted %q: markdown %q parsed as %s, want %s", deleted, md, got, want)
		}
	}
}

// Plain-text export goes through the same escaper: a literal "~~" in
// page text must not turn into strikethrough when the Markdown is read.
func TestMdEscapeTextTildeRoundTrip(t *testing.T) {
	const text = "a~~b~~c and x~y~z"
	got := strings.TrimSpace(mdTestHTML(parseMarkdown(mdEscapeText(text))))
	if want := "<p>" + text + "</p>"; got != want {
		t.Errorf("escaped %q parsed as %s, want %s", text, got, want)
	}
}
