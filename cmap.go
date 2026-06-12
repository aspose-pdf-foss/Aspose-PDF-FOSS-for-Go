// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"encoding/hex"
	"strings"
)

// parseCMap parses a ToUnicode CMap stream and returns a mapping
// from character codes (glyph IDs) to Unicode runes.
// It handles beginbfchar/endbfchar and beginbfrange/endbfrange sections.
func parseCMap(data []byte) map[uint16]rune {
	m := make(map[uint16]rune)
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
			parseBfcharLine(s, m)
		}
		if inBfrange {
			parseBfrangeLine(s, m)
		}
	}
	return m
}

// parseBfcharLine parses a line like "<0003> <0020>".
func parseBfcharLine(s string, m map[uint16]rune) {
	tokens := extractHexTokens(s)
	if len(tokens) < 2 {
		return
	}
	src := decodeHexUint16(tokens[0])
	dst := decodeHexRune(tokens[1])
	if dst != 0 {
		m[src] = dst
	}
}

// parseBfrangeLine parses a line like "<0041> <0043> <0061>"
// or "<0100> <0102> [<0041> <0042> <0043>]".
func parseBfrangeLine(s string, m map[uint16]rune) {
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
			r := decodeHexRune(tok)
			if r != 0 {
				m[code] = r
			}
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
		m[uint16(c)] = dstStart + rune(c-uint32(start))
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
