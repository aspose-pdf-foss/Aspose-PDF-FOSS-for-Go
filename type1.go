// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"strconv"
)

// type1Font is a parsed embedded Type1 (PostScript) font program — the
// /FontFile form, as opposed to TrueType (/FontFile2) or CFF (/FontFile3).
// Its glyph outlines come from eexec-encrypted Type1 charstrings keyed by
// glyph name; a synthetic GID is assigned to each name so the renderer's
// GID-based glyph path can drive it.
type type1Font struct {
	charstrings map[string][]byte // glyph name → decrypted charstring (lenIV stripped)
	subrs       [][]byte          // decrypted local subroutines
	builtinEnc  [256]string       // built-in /Encoding: code → glyph name
	fontMatrix  [6]float64        // glyph space → text space (default 0.001 scale)
	unitsPerEm  float64           // 1 / fontMatrix[0] (≈ 1000)
	glyphNames  []string          // GID → name
	nameToGID   map[string]uint16
}

// t1Decrypt runs the Type1 eexec/charstring decryption (Adobe Type 1 Font
// Format §7): p_i = c_i ⊕ (R>>8); R = (c_i + R)·c1 + c2. The first skip bytes
// (4 for eexec, lenIV for charstrings) are random and discarded.
func t1Decrypt(cipher []byte, r uint16, skip int) []byte {
	const c1, c2 = 52845, 22719
	out := make([]byte, len(cipher))
	for i, c := range cipher {
		out[i] = c ^ byte(r>>8)
		r = (uint16(c)+r)*c1 + c2
	}
	if skip <= len(out) {
		return out[skip:]
	}
	return nil
}

// parseType1 parses an embedded Type1 program. len1/len2/len3 are the
// /Length1 (clear text), /Length2 (binary eexec) and /Length3 (trailer)
// segment sizes from the /FontFile dict; when zero, the split is found by the
// "eexec" keyword. Returns nil on a malformed program.
func parseType1(data []byte, len1, len2 int) *type1Font {
	if len(data) < 16 {
		return nil
	}
	// Reassemble a PFB (segmented, 0x80-marked) container into a flat stream.
	if data[0] == 0x80 {
		data = pfbToFlat(data)
		len1, len2 = 0, 0
	}

	var clear, binary []byte
	if len1 > 0 && len2 > 0 && len1+len2 <= len(data) {
		clear = data[:len1]
		binary = data[len1 : len1+len2]
	} else {
		idx := bytes.Index(data, []byte("eexec"))
		if idx < 0 {
			return nil
		}
		clear = data[:idx]
		b := data[idx+5:]
		// Skip the whitespace after "eexec".
		for len(b) > 0 && (b[0] == ' ' || b[0] == '\r' || b[0] == '\n' || b[0] == '\t') {
			b = b[1:]
		}
		binary = b
	}

	// The eexec section may be ASCII-hex encoded (the first bytes are all hex
	// digits/whitespace) rather than raw binary.
	if isHexEexec(binary) {
		binary = decodeHexEexec(binary)
	}

	priv := t1Decrypt(binary, 55665, 4)
	if priv == nil {
		return nil
	}

	f := &type1Font{
		charstrings: map[string][]byte{},
		nameToGID:   map[string]uint16{},
		fontMatrix:  [6]float64{0.001, 0, 0, 0.001, 0, 0},
		unitsPerEm:  1000,
	}
	f.parseClearText(clear)
	if fm := parseFontMatrix(clear); fm != [6]float64{} {
		f.fontMatrix = fm
		if fm[0] != 0 {
			f.unitsPerEm = 1 / fm[0]
		}
	}
	lenIV := parseLenIV(priv)
	f.parseSubrs(priv, lenIV)
	f.parseCharStrings(priv, lenIV)
	if len(f.charstrings) == 0 {
		return nil
	}

	// Assign GIDs: .notdef first (if present), then the rest in encounter order.
	if _, ok := f.charstrings[".notdef"]; ok {
		f.addGlyph(".notdef")
	}
	for name := range f.charstrings {
		if name != ".notdef" {
			f.addGlyph(name)
		}
	}
	return f
}

func (f *type1Font) addGlyph(name string) {
	if _, ok := f.nameToGID[name]; ok {
		return
	}
	f.nameToGID[name] = uint16(len(f.glyphNames))
	f.glyphNames = append(f.glyphNames, name)
}

// pfbToFlat concatenates the data segments of a PFB container (0x80 0x01 ascii
// / 0x80 0x02 binary / 0x80 0x03 EOF, each with a 4-byte little-endian length).
func pfbToFlat(data []byte) []byte {
	var out []byte
	i := 0
	for i+6 <= len(data) && data[i] == 0x80 {
		t := data[i+1]
		if t == 0x03 {
			break
		}
		n := int(data[i+2]) | int(data[i+3])<<8 | int(data[i+4])<<16 | int(data[i+5])<<24
		i += 6
		if i+n > len(data) {
			n = len(data) - i
		}
		out = append(out, data[i:i+n]...)
		i += n
	}
	return out
}

func isHexEexec(b []byte) bool {
	n := 0
	for _, c := range b {
		if c == ' ' || c == '\r' || c == '\n' || c == '\t' {
			continue
		}
		if !isHexDigit(c) {
			return false
		}
		if n++; n >= 4 {
			return true
		}
	}
	return n > 0
}

func isHexDigit(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

func decodeHexEexec(b []byte) []byte {
	out := make([]byte, 0, len(b)/2)
	var hi int = -1
	for _, c := range b {
		if !isHexDigit(c) {
			continue
		}
		v := hexVal(c)
		if hi < 0 {
			hi = v
		} else {
			out = append(out, byte(hi<<4|v))
			hi = -1
		}
	}
	return out
}

func hexVal(c byte) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'a' && c <= 'f':
		return int(c-'a') + 10
	default:
		return int(c-'A') + 10
	}
}

// parseClearText reads the built-in /Encoding from the clear-text header:
// either "/Encoding StandardEncoding def" or a sequence of
// "dup <code> /<name> put" entries.
func (f *type1Font) parseClearText(clear []byte) {
	if bytes.Contains(clear, []byte("/Encoding StandardEncoding def")) {
		for code, r := range standardEncoding {
			if r != 0 && r != 0xFFFD {
				f.builtinEnc[code] = runeToStdGlyphName(r)
			}
		}
	}
	// Custom encoding: dup <code> /<name> put
	for _, m := range reType1Dup.FindAllSubmatch(clear, -1) {
		code, err := strconv.Atoi(string(m[1]))
		if err == nil && code >= 0 && code < 256 {
			f.builtinEnc[code] = string(m[2])
		}
	}
}

// parseSubrs reads "/Subrs N array" then "dup i len RD <bytes> NP" entries.
func (f *type1Font) parseSubrs(priv []byte, lenIV int) {
	i := bytes.Index(priv, []byte("/Subrs"))
	if i < 0 {
		return
	}
	rest := priv[i:]
	// Pre-size the slice from the declared count when sane.
	if m := reType1SubrsCount.FindSubmatch(rest); m != nil {
		if n, err := strconv.Atoi(string(m[1])); err == nil && n >= 0 && n < 1<<16 {
			f.subrs = make([][]byte, n)
		}
	}
	pos := 0
	for {
		rel := bytes.Index(rest[pos:], []byte("dup "))
		if rel < 0 {
			break
		}
		p := pos + rel + 4
		idx, p2, ok := readInt(rest, p)
		if !ok {
			pos = p
			continue
		}
		ln, p3, ok := readInt(rest, p2)
		if !ok {
			pos = p2
			continue
		}
		data, p4, ok := readBinaryAfterRD(rest, p3, ln)
		if !ok {
			pos = p3
			// Stop once we reach CharStrings.
			if bytes.HasPrefix(bytes.TrimSpace(rest[pos:]), []byte("ND")) {
				break
			}
			continue
		}
		if idx >= 0 {
			if idx >= len(f.subrs) {
				grown := make([][]byte, idx+1)
				copy(grown, f.subrs)
				f.subrs = grown
			}
			f.subrs[idx] = t1Decrypt(data, 4330, lenIV)
		}
		pos = p4
		if bytes.HasPrefix(bytes.TrimSpace(rest[pos:]), []byte("/CharStrings")) {
			break
		}
	}
}

// parseCharStrings reads "/CharStrings N dict" then "/name len RD <bytes> ND".
func (f *type1Font) parseCharStrings(priv []byte, lenIV int) {
	i := bytes.Index(priv, []byte("/CharStrings"))
	if i < 0 {
		return
	}
	rest := priv[i+len("/CharStrings"):]
	pos := 0
	for {
		rel := bytes.IndexByte(rest[pos:], '/')
		if rel < 0 {
			break
		}
		p := pos + rel + 1
		name, p2 := readName(rest, p)
		if name == "" {
			pos = p
			continue
		}
		ln, p3, ok := readInt(rest, p2)
		if !ok {
			pos = p2
			continue
		}
		data, p4, ok := readBinaryAfterRD(rest, p3, ln)
		if !ok {
			pos = p3
			continue
		}
		f.charstrings[name] = t1Decrypt(data, 4330, lenIV)
		pos = p4
	}
}

// readInt reads an optional-sign integer skipping leading whitespace.
func readInt(b []byte, p int) (int, int, bool) {
	for p < len(b) && isT1Space(b[p]) {
		p++
	}
	start := p
	if p < len(b) && (b[p] == '-' || b[p] == '+') {
		p++
	}
	d := p
	for p < len(b) && b[p] >= '0' && b[p] <= '9' {
		p++
	}
	if p == d {
		return 0, start, false
	}
	n, err := strconv.Atoi(string(b[start:p]))
	if err != nil {
		return 0, start, false
	}
	return n, p, true
}

// readName reads a glyph name (up to whitespace), the leading '/' already consumed.
func readName(b []byte, p int) (string, int) {
	start := p
	for p < len(b) && !isT1Space(b[p]) && b[p] != '/' && b[p] != '{' && b[p] != '(' {
		p++
	}
	return string(b[start:p]), p
}

// readBinaryAfterRD expects an RD or -| operator (the Type1 binary-read
// token), one separating space, then ln raw bytes.
func readBinaryAfterRD(b []byte, p, ln int) ([]byte, int, bool) {
	for p < len(b) && isT1Space(b[p]) {
		p++
	}
	switch {
	case bytes.HasPrefix(b[p:], []byte("RD ")), bytes.HasPrefix(b[p:], []byte("-| ")):
		p += 3
	default:
		return nil, p, false
	}
	if p+ln > len(b) {
		return nil, p, false
	}
	return b[p : p+ln], p + ln, true
}

func isT1Space(c byte) bool {
	return c == ' ' || c == '\r' || c == '\n' || c == '\t' || c == 0
}

func parseLenIV(priv []byte) int {
	if m := reType1LenIV.FindSubmatch(priv); m != nil {
		if n, err := strconv.Atoi(string(m[1])); err == nil && n >= 0 && n < 64 {
			return n
		}
	}
	return 4
}

func parseFontMatrix(clear []byte) [6]float64 {
	m := reType1FontMatrix.FindSubmatch(clear)
	if m == nil {
		return [6]float64{}
	}
	fields := bytes.Fields(m[1])
	if len(fields) != 6 {
		return [6]float64{}
	}
	var out [6]float64
	for i, fld := range fields {
		v, err := strconv.ParseFloat(string(fld), 64)
		if err != nil {
			return [6]float64{}
		}
		out[i] = v
	}
	return out
}
