// SPDX-License-Identifier: MIT

package asposepdf

import "testing"

// TestLexerDelimiterProgress checks that the lexer always advances past a
// delimiter byte that Next does not special-case (')', '{', '}', '%', or stray
// binary). Before the fix readKeyword consumed nothing on such a byte, so
// parseContentStream re-read the same position forever — 33150.pdf has a page
// whose content fails to inflate, leaving raw bytes that hung the renderer.
func TestLexerDelimiterProgress(t *testing.T) {
	for _, b := range []byte{')', '}', '{', '%', 0x80, 0x00, 0xff} {
		l := newLexer([]byte{b, ' ', 'B', 'T'})
		tok, err := l.Next()
		if err == nil && l.pos == 0 {
			t.Errorf("byte %#x: lexer did not advance (pos still 0), token=%q", b, tok.raw)
		}
	}
}

// TestParseContentStreamGarbageTerminates ensures parsing undecodable/garbage
// content returns instead of looping forever. The bytes mix stray delimiters
// with valid operators; a hang would trip the test binary's -timeout.
func TestParseContentStreamGarbageTerminates(t *testing.T) {
	garbage := []byte("BT ) ) } } % stray\n\x80\x81\x82 ( unterminated 100 200 m ET")
	if _, err := parseContentStream(garbage); err != nil {
		_ = err // an error is acceptable; only an infinite loop would fail this
	}
}
