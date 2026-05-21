// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"fmt"
)

type tokenKind int

const (
	tokInt tokenKind = iota
	tokReal
	tokBool
	tokNull
	tokName
	tokString  // literal (...)
	tokHexStr  // hex <...>
	tokArrayOpen
	tokArrayClose
	tokDictOpen  // <<
	tokDictClose // >>
	tokKeyword   // obj, endobj, stream, endstream, R, xref, trailer, startxref, f, n
	tokEOF
)

type token struct {
	kind tokenKind
	raw  []byte
}

func (t token) String() string { return string(t.raw) }

type lexer struct {
	data []byte
	pos  int
}

func newLexer(data []byte) *lexer { return &lexer{data: data} }
func newLexerAt(data []byte, pos int) *lexer {
	return &lexer{data: data, pos: pos}
}

func (l *lexer) Pos() int { return l.pos }

func isWhitespace(b byte) bool {
	return b == 0 || b == 9 || b == 10 || b == 12 || b == 13 || b == 32
}

func isDelimiter(b byte) bool {
	return isWhitespace(b) || b == '(' || b == ')' || b == '<' || b == '>' ||
		b == '[' || b == ']' || b == '{' || b == '}' || b == '/' || b == '%'
}

func (l *lexer) skipWS() {
	for l.pos < len(l.data) {
		b := l.data[l.pos]
		if b == '%' {
			// comment — skip to end of line
			for l.pos < len(l.data) && l.data[l.pos] != '\n' && l.data[l.pos] != '\r' {
				l.pos++
			}
		} else if isWhitespace(b) {
			l.pos++
		} else {
			break
		}
	}
}

func (l *lexer) Next() (token, error) {
	l.skipWS()
	if l.pos >= len(l.data) {
		return token{kind: tokEOF, raw: nil}, nil
	}

	b := l.data[l.pos]

	// Array
	if b == '[' {
		l.pos++
		return token{kind: tokArrayOpen, raw: []byte{'['}}, nil
	}
	if b == ']' {
		l.pos++
		return token{kind: tokArrayClose, raw: []byte{']'}}, nil
	}

	// Dictionary << or hex string <...>
	if b == '<' {
		if l.pos+1 < len(l.data) && l.data[l.pos+1] == '<' {
			l.pos += 2
			return token{kind: tokDictOpen, raw: []byte("<<")}, nil
		}
		return l.readHexString()
	}
	if b == '>' {
		if l.pos+1 < len(l.data) && l.data[l.pos+1] == '>' {
			l.pos += 2
			return token{kind: tokDictClose, raw: []byte(">>")}, nil
		}
		return token{}, fmt.Errorf("unexpected '>' at %d", l.pos)
	}

	// Literal string
	if b == '(' {
		return l.readLiteralString()
	}

	// Name
	if b == '/' {
		return l.readName()
	}

	// Number or keyword
	if b == '+' || b == '-' || (b >= '0' && b <= '9') || b == '.' {
		return l.readNumber()
	}

	// Keyword / boolean / null
	return l.readKeyword()
}

func (l *lexer) readName() (token, error) {
	start := l.pos
	l.pos++ // skip '/'
	for l.pos < len(l.data) && !isDelimiter(l.data[l.pos]) {
		l.pos++
	}
	return token{kind: tokName, raw: l.data[start:l.pos]}, nil
}

func (l *lexer) readNumber() (token, error) {
	start := l.pos
	isReal := false
	if l.data[l.pos] == '+' || l.data[l.pos] == '-' {
		l.pos++
	}
	for l.pos < len(l.data) && !isDelimiter(l.data[l.pos]) {
		if l.data[l.pos] == '.' {
			isReal = true
		}
		l.pos++
	}
	kind := tokInt
	if isReal {
		kind = tokReal
	}
	return token{kind: kind, raw: l.data[start:l.pos]}, nil
}

func (l *lexer) readKeyword() (token, error) {
	start := l.pos
	for l.pos < len(l.data) && !isDelimiter(l.data[l.pos]) {
		l.pos++
	}
	raw := l.data[start:l.pos]
	switch string(raw) {
	case "true", "false":
		return token{kind: tokBool, raw: raw}, nil
	case "null":
		return token{kind: tokNull, raw: raw}, nil
	default:
		return token{kind: tokKeyword, raw: raw}, nil
	}
}

func (l *lexer) readLiteralString() (token, error) {
	start := l.pos
	l.pos++ // skip '('
	depth := 1
	for l.pos < len(l.data) && depth > 0 {
		b := l.data[l.pos]
		if b == '\\' {
			l.pos += 2
			continue
		}
		if b == '(' {
			depth++
		} else if b == ')' {
			depth--
		}
		l.pos++
	}
	return token{kind: tokString, raw: l.data[start:l.pos]}, nil
}

func (l *lexer) readHexString() (token, error) {
	start := l.pos
	l.pos++ // skip '<'
	for l.pos < len(l.data) && l.data[l.pos] != '>' {
		l.pos++
	}
	if l.pos < len(l.data) {
		l.pos++ // skip '>'
	}
	return token{kind: tokHexStr, raw: l.data[start:l.pos]}, nil
}

// peekKeyword looks ahead without consuming — used to detect "stream" after a dict.
func (l *lexer) peekKeyword() string {
	saved := l.pos
	l.skipWS()
	start := l.pos
	for l.pos < len(l.data) && !isDelimiter(l.data[l.pos]) {
		l.pos++
	}
	kw := string(l.data[start:l.pos])
	l.pos = saved
	return kw
}

// skipLine advances past the current line (skips to after the next \n).
func (l *lexer) skipLine() {
	for l.pos < len(l.data) && l.data[l.pos] != '\n' && l.data[l.pos] != '\r' {
		l.pos++
	}
	if l.pos < len(l.data) && l.data[l.pos] == '\r' {
		l.pos++
	}
	if l.pos < len(l.data) && l.data[l.pos] == '\n' {
		l.pos++
	}
}

// skipToStreamData advances past the "stream\n" (or "stream\r\n") marker.
func (l *lexer) skipToStreamData() {
	idx := bytes.Index(l.data[l.pos:], []byte("stream"))
	if idx < 0 {
		return
	}
	l.pos += idx + len("stream")
	if l.pos < len(l.data) && l.data[l.pos] == '\r' {
		l.pos++
	}
	if l.pos < len(l.data) && l.data[l.pos] == '\n' {
		l.pos++
	}
}
