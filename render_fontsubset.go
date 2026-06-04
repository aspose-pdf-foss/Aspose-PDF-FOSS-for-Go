// SPDX-License-Identifier: MIT

package asposepdf

import "fmt"

// subsetFontToRunes shrinks a TrueType font to the glyphs needed for the given
// runes (plus .notdef and any composite components), used to keep the bundled
// substitute fonts small. Unlike subsetTTF (the embedding subsetter, which
// renumbers glyphs and drops the cmap), this keeps the original glyph IDs and
// the cmap intact — so rune→GID lookup still works for rendering — and only
// empties the glyf entries of unused glyphs. The bulky glyf table shrinks to a
// few hundred outlines while loca/hmtx/maxp/cmap stay valid.
func subsetFontToRunes(data []byte, runes []rune) ([]byte, error) {
	f, err := parseTTF(data)
	if err != nil {
		return nil, err
	}
	tables, err := readTableDirectory(data)
	if err != nil {
		return nil, err
	}
	loca, err := readLoca(f, tables)
	if err != nil {
		return nil, err
	}
	glyf := tableSlice(data, tables, "glyf")
	if glyf == nil {
		return nil, fmt.Errorf("subset: font has no glyf table")
	}

	keep := map[uint16]bool{0: true} // .notdef
	for _, r := range runes {
		if gid := f.glyphID(r); gid != 0 {
			keep[gid] = true
		}
	}
	if err := expandComposites(glyf, loca, keep); err != nil {
		return nil, err
	}

	newGlyf, newLoca := buildSparseGlyf(glyf, loca, keep, f.numGlyphs)
	tbls := map[string][]byte{
		"cmap": tableSlice(data, tables, "cmap"),
		"head": patchHead(tableSlice(data, tables, "head")), // force long loca
		"hhea": tableSlice(data, tables, "hhea"),
		"hmtx": tableSlice(data, tables, "hmtx"),
		"maxp": tableSlice(data, tables, "maxp"),
		"name": tableSlice(data, tables, "name"),
		"OS/2": tableSlice(data, tables, "OS/2"),
		"post": buildPost30(f), // synthetic post 3.0 (drops bulky glyph names)
		"glyf": newGlyf,
		"loca": encodeLocaLong(newLoca),
	}
	for name, b := range tbls {
		if b == nil {
			return nil, fmt.Errorf("subset: missing table %q", name)
		}
	}
	return assembleSFNT(tbls), nil
}

// buildSparseGlyf rebuilds glyf keeping the original glyph IDs: kept glyphs copy
// their outline bytes, dropped glyphs become empty (zero-length) entries.
func buildSparseGlyf(glyf []byte, loca []uint32, keep map[uint16]bool, numGlyphs uint16) ([]byte, []uint32) {
	var out []byte
	newLoca := make([]uint32, int(numGlyphs)+1)
	for gid := uint16(0); gid < numGlyphs; gid++ {
		newLoca[gid] = uint32(len(out))
		if keep[gid] {
			out = append(out, glyphBytes(glyf, loca, gid)...)
			if len(out)%2 != 0 {
				out = append(out, 0) // align to 2 bytes
			}
		}
	}
	newLoca[numGlyphs] = uint32(len(out))
	return out, newLoca
}

// winAnsiRunes is the rune set the bundled substitute fonts are subset to:
// ASCII, Latin-1 + Latin Extended-A, and the common punctuation/symbols that
// WinAnsi (cp1252) and typical PDFs use.
func winAnsiRunes() []rune {
	var rs []rune
	for r := rune(0x20); r <= 0x7E; r++ { // ASCII
		rs = append(rs, r)
	}
	for r := rune(0xA0); r <= 0x17F; r++ { // Latin-1 + Latin Extended-A
		rs = append(rs, r)
	}
	for r := rune(0x2010); r <= 0x2027; r++ { // dashes, quotes, bullet, ellipsis
		rs = append(rs, r)
	}
	rs = append(rs, 0x20AC, 0x2122, 0x02C6, 0x02DC, 0x0152, 0x0153, 0x0160, 0x0161, 0x0178, 0x017D, 0x017E)
	return rs
}
