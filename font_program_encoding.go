// SPDX-License-Identifier: MIT

package asposepdf

// programEncoding reads a simple font's encoding back from its embedded
// program, for a font whose dictionary has neither /Encoding nor /ToUnicode
// (pdf-go-nw0h): a symbolic TrueType subset as Word writes them, or a Type1C
// or Type1 program relying on its built-in encoding. Each code is taken to the
// glyph the renderer draws for it, and that glyph to Unicode:
//
//   - TrueType: the code reaches the glyph through the (1,0) or (3,0) cmap
//     (ISO 32000-1 §9.6.6.4). The glyph's character is the one a Unicode cmap
//     gives it, else, for a (1,0) key, the MacRoman character of the code (that
//     subtable is keyed by Mac Roman codes) and, for a (3,0) key, the code
//     itself when it is printable ASCII. A (3,0) symbol font says nothing more.
//   - CFF: the built-in encoding gives the glyph, the charset its name, and the
//     Adobe Glyph List (or a uniXXXX name) the character.
//   - Type1: the built-in /Encoding gives the name, then as for CFF.
//
// ok is false when there is no program or it maps no code.
func programEncoding(objects map[int]*pdfObject, fontDict pdfDict) (enc [256]rune, ok bool) {
	descriptor, _ := fontDescriptorOf(objects, fontDict)
	if descriptor == nil {
		return enc, false
	}
	rf := &renderFont{em: 1000}
	if !loadEmbeddedFontProgram(objects, rf, descriptor) {
		return enc, false
	}
	return encodingFromProgram(rf)
}

// encodingFromProgram is programEncoding once the program is loaded into rf.
func encodingFromProgram(rf *renderFont) (enc [256]rune, ok bool) {
	switch {
	case rf.prog != nil:
		enc, ok = trueTypeProgramEncoding(rf.prog)
	case rf.cff != nil && !rf.cff.isCID:
		for c := range 256 {
			g := rf.cff.simpleGID[uint16(c)]
			if g == 0 || int(g) >= len(rf.cff.charset) {
				continue
			}
			if r := cffGlyphNameRune(rf.cff.sidName(rf.cff.charset[g])); r != 0 {
				enc[c], ok = r, true
			}
		}
	case rf.t1 != nil:
		for c := range 256 {
			if r := cffGlyphNameRune(rf.t1.builtinEnc[c]); r != 0 {
				enc[c], ok = r, true
			}
		}
	}
	if !ok {
		return enc, false
	}
	for c := range enc {
		if enc[c] == 0 {
			enc[c] = '�'
		}
	}
	return enc, true
}

// trueTypeProgramEncoding is programEncoding for a TrueType program.
func trueTypeProgramEncoding(prog *ttfFont) (enc [256]rune, ok bool) {
	if len(prog.codeToGlyph) == 0 {
		return enc, false
	}
	// The character a Unicode cmap gives each glyph; the lowest when several
	// do, so the result does not depend on map order.
	byGlyph := map[uint16]rune{}
	for r, g := range prog.runeToGlyph {
		if g == 0 || (r >= 0xE000 && r <= 0xF8FF) {
			continue // private use says nothing about the character
		}
		if cur, seen := byGlyph[g]; !seen || r < cur {
			byGlyph[g] = r
		}
	}
	for c := range 256 {
		var r rune
		if g := prog.codeToGlyph[uint16(c)]; g != 0 {
			r = byGlyph[g]
			if r == 0 && c >= 0x20 {
				r = macRomanEncoding[c]
			}
		} else if g := prog.codeToGlyph[uint16(0xF000|c)]; g != 0 {
			r = byGlyph[g]
			if r == 0 && c >= 0x20 && c < 0x7F {
				r = rune(c)
			}
		}
		if r != 0 && r != 0xFFFD {
			enc[c], ok = r, true
		}
	}
	return enc, ok
}
