// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"encoding/hex"
	"strings"
	"unicode/utf16"
)

// parseCMap parses a ToUnicode CMap stream and returns a mapping
// from character codes (glyph IDs) to Unicode runes.
// It handles beginbfchar/endbfchar and beginbfrange/endbfrange sections.
func parseCMap(data []byte) map[uint16]rune {
	m, _ := parseCMapFull(data)
	return m
}

// unicodeMap collects a CMap's destinations: the first character of every
// code, plus the whole sequence for codes that stand for several characters
// (a ligature glyph mapped to "fi").
type unicodeMap struct {
	runes map[uint16]rune
	seqs  map[uint16][]rune
	nulls map[uint16]bool // codes mapped to U+0000, which say nothing of the text
}

func (u *unicodeMap) set(code uint16, dst []rune) {
	if len(dst) == 0 {
		return
	}
	if dst[0] == 0 {
		if u.nulls == nil {
			u.nulls = map[uint16]bool{}
		}
		u.nulls[code] = true
		return
	}
	u.runes[code] = dst[0]
	if len(dst) > 1 {
		if u.seqs == nil {
			u.seqs = map[uint16][]rune{}
		}
		u.seqs[code] = dst
	}
}

// parseCMapFull is parseCMap plus the multi-character destinations, keyed
// to their full sequence (their first character is in the rune map too).
func parseCMapFull(data []byte) (map[uint16]rune, map[uint16][]rune) {
	u := parseUnicodeMap(data)
	return u.runes, u.seqs
}

func parseUnicodeMap(data []byte) *unicodeMap {
	u := &unicodeMap{runes: make(map[uint16]rune)}
	// Normalize CR/CRLF line endings: classic-Mac producers emit CR-only
	// CMaps, which would otherwise arrive as one unsplittable line.
	data = bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
	data = bytes.ReplaceAll(data, []byte("\r"), []byte("\n"))
	lines := bytes.Split(data, []byte("\n"))

	inBfchar := false
	inBfrange := false

	for _, line := range lines {
		s := strings.TrimSpace(string(line))
		if s == "" {
			continue
		}

		if strings.HasSuffix(s, "beginbfchar") {
			inBfchar = true
			continue
		}
		if s == "endbfchar" {
			inBfchar = false
			continue
		}
		if strings.HasSuffix(s, "beginbfrange") {
			inBfrange = true
			continue
		}
		if s == "endbfrange" {
			inBfrange = false
			continue
		}

		if inBfchar {
			parseBfcharLine(s, u)
		}
		if inBfrange {
			parseBfrangeLine(s, u)
		}
	}
	return u
}

// parseBfcharLine parses a line like "<0003> <0020>".
func parseBfcharLine(s string, u *unicodeMap) {
	tokens := extractHexTokens(s)
	if len(tokens) < 2 {
		return
	}
	u.set(decodeHexUint16(tokens[0]), decodeHexRunes(tokens[1]))
}

// parseBfrangeLine parses a line like "<0041> <0043> <0061>"
// or "<0100> <0102> [<0041> <0042> <0043>]".
func parseBfrangeLine(s string, u *unicodeMap) {
	// Check for array form: [...] at the end.
	if idx := strings.Index(s, "["); idx >= 0 {
		// Parse the two hex tokens before the bracket.
		prefix := s[:idx]
		tokens := extractHexTokens(prefix)
		if len(tokens) < 2 {
			return
		}
		start := decodeHexUint16(tokens[0])
		end := decodeHexUint16(tokens[1])
		// Parse array entries.
		arrayPart := s[idx:]
		arrayTokens := extractHexTokens(arrayPart)
		for i, tok := range arrayTokens {
			code := start + uint16(i)
			if code > end {
				break
			}
			u.set(code, decodeHexRunes(tok))
		}
		return
	}

	tokens := extractHexTokens(s)
	if len(tokens) < 3 {
		return
	}
	start := decodeHexUint16(tokens[0])
	end := decodeHexUint16(tokens[1])
	dstStart := decodeHexRune(tokens[2])
	for c := uint32(start); c <= uint32(end); c++ {
		u.runes[uint16(c)] = dstStart + rune(c-uint32(start))
	}
}

// extractHexTokens returns all <hex> tokens from a string.
func extractHexTokens(s string) []string {
	var tokens []string
	for {
		start := strings.IndexByte(s, '<')
		if start < 0 {
			break
		}
		end := strings.IndexByte(s[start:], '>')
		if end < 0 {
			break
		}
		tokens = append(tokens, s[start+1:start+end])
		s = s[start+end+1:]
	}
	return tokens
}

// decodeHexUint16 decodes a hex string to uint16 (e.g., "0003" -> 3).
func decodeHexUint16(s string) uint16 {
	b, err := hex.DecodeString(s)
	if err != nil || len(b) == 0 {
		return 0
	}
	if len(b) == 1 {
		return uint16(b[0])
	}
	return uint16(b[0])<<8 | uint16(b[1])
}

// decodeHexRune decodes a hex string to a rune (e.g., "0041" -> 'A').
// Supplementary-plane targets (>2 bytes) are not supported and return 0.
func decodeHexRune(s string) rune {
	b, err := hex.DecodeString(s)
	if err != nil || len(b) == 0 || len(b) > 2 {
		return 0
	}
	if len(b) == 1 {
		return rune(b[0])
	}
	return rune(uint16(b[0])<<8 | uint16(b[1]))
}

// decodeHexRunes decodes a UTF-16BE hex destination into its characters,
// joining surrogate pairs ("00660069" -> "fi", "D835DC00" -> U+1D400).
func decodeHexRunes(s string) []rune {
	b, err := hex.DecodeString(s)
	if err != nil || len(b) == 0 {
		return nil
	}
	if len(b) == 1 {
		return []rune{rune(b[0])}
	}
	units := make([]uint16, 0, len(b)/2)
	for i := 0; i+1 < len(b); i += 2 {
		units = append(units, uint16(b[i])<<8|uint16(b[i+1]))
	}
	return utf16.Decode(units)
}
