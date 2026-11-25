// SPDX-License-Identifier: MIT

package asposepdf

import (
	"fmt"
	"sort"
	"strings"
)

// pageFont is a font already on a page, drawn with again when an edit of
// found text needs no other glyphs (pdf-go-ddpy): a change of colour,
// background, underline, size or, emulated, style redraws the text in the
// document's own font, so it keeps its shapes and widths instead of moving
// to a Standard-14 substitute of another width. It encodes a character with
// the code the font already shows it with and measures it with the font's
// own widths.
//
// Only simple fonts (not Type3) and composite fonts with Identity-H or a
// horizontal CMap are used this way. For a font with an embedded program,
// only characters the edited fragment already showed in it count as covered:
// a subset need not hold the others.
type pageFont struct {
	src      documentFont
	ref      int // the font dictionary's object number
	fi       fontInfo
	embedded bool
	codes    map[rune]string // character -> its code's bytes
	shown    map[rune]bool   // characters the fragment showed in the font
}

func (f *pageFont) BaseFont() string { return f.src.name }
func (f *pageFont) IsEmbedded() bool { return f.embedded }

// newPageFont returns the font of object ref as a pageFont, nil when it
// cannot be drawn with again (no such font, Type3, vertical writing, codes
// that cannot be told apart).
func newPageFont(objects map[int]*pdfObject, src documentFont, ref int) *pageFont {
	o, ok := objects[ref]
	if !ok {
		return nil
	}
	fd, ok := o.Value.(pdfDict)
	if !ok || dictGetName(fd, "/Subtype") == "/Type3" {
		return nil
	}
	fi := resolveFont(objects, fd)
	if !fi.known || fi.vertical || fi.programEncoding {
		return nil
	}
	pf := &pageFont{src: src, ref: ref, fi: fi, embedded: descriptorEmbedded(objects, fd), codes: map[rune]string{}}
	add := func(r rune, code string) {
		if r == 0 || r == '�' {
			return
		}
		if _, ok := pf.codes[r]; !ok {
			pf.codes[r] = code
		}
	}
	switch {
	case !fi.isType0:
		for c := 0; c < 256; c++ {
			if len(fi.toUnicodeSeq[uint16(c)]) > 1 {
				continue // a ligature stands for several characters
			}
			r := fi.encoding[c]
			if tr, ok := fi.toUnicode[uint16(c)]; ok {
				r = tr
			}
			add(r, string([]byte{byte(c)}))
		}
	case fi.cidCMap == nil:
		for _, code := range sortedCodes(fi.toUnicode) {
			if len(fi.toUnicodeSeq[code]) <= 1 {
				add(fi.toUnicode[code], string([]byte{byte(code >> 8), byte(code)}))
			}
		}
		for _, cid := range sortedCodes(fi.cidToUni) {
			add(fi.cidToUni[cid], string([]byte{byte(cid >> 8), byte(cid)}))
		}
	default:
		// A CMap: the codes /ToUnicode names, each as many bytes as the
		// codespace says.
		for _, code := range sortedCodes(fi.toUnicode) {
			if len(fi.toUnicodeSeq[code]) > 1 {
				continue
			}
			for _, b := range [][]byte{{byte(code)}, {byte(code >> 8), byte(code)}} {
				if len(b) == 1 && code > 0xFF {
					continue
				}
				if c, _, n := fi.cidCMap.next(b); n == len(b) && c == uint32(code) {
					add(fi.toUnicode[code], string(b))
					break
				}
			}
		}
	}
	return pf
}

func sortedCodes(m map[uint16]rune) []uint16 {
	codes := make([]uint16, 0, len(m))
	for c := range m {
		codes = append(codes, c)
	}
	sort.Slice(codes, func(i, j int) bool { return codes[i] < codes[j] })
	return codes
}

// covers reports whether every character of text can be drawn in the font:
// it has a code for it and, for an embedded program, the fragment showed it.
// Arabic is never: its glyphs are contextual forms of one character.
func (f *pageFont) covers(text string) bool {
	if bidiHasArabic(text) {
		return false
	}
	for _, r := range text {
		if isDefaultIgnorable(r) {
			continue
		}
		if _, ok := f.codes[r]; !ok {
			return false
		}
		if f.embedded && !f.shown[r] && r != ' ' {
			return false
		}
	}
	return true
}

// twoByte reports whether codes are composite-font codes (Tw does not apply).
func (f *pageFont) twoByte() bool { return f.fi.isType0 }

// codeWidth is the advance of the glyph a code selects, in 1/1000 em.
func (f *pageFont) codeWidth(code string) float64 {
	if !f.fi.isType0 {
		return f.fi.widths[code[0]]
	}
	cid := uint16(0)
	if f.fi.cidCMap != nil {
		_, cid, _ = f.fi.cidCMap.next([]byte(code))
	} else if len(code) == 2 {
		cid = uint16(code[0])<<8 | uint16(code[1])
	}
	if w, ok := f.fi.cidWidths[cid]; ok {
		return w
	}
	return f.fi.defaultW
}

func (f *pageFont) widthFn(size float64) widthFn {
	return func(r rune) float64 {
		code, ok := f.codes[r]
		if !ok || isDefaultIgnorable(r) {
			return 0
		}
		return f.codeWidth(code) / 1000 * size
	}
}

// encode writes s as a hex string of the font's codes; characters without
// one are left out (covers is checked before drawing).
func (f *pageFont) encode(s string) string {
	var b strings.Builder
	b.WriteByte('<')
	for _, r := range s {
		for _, c := range []byte(f.codes[r]) {
			fmt.Fprintf(&b, "%02X", c)
		}
	}
	b.WriteByte('>')
	return b.String()
}

// ascentFactor is the font's ascender per em (0.8 when it states none).
func (f *pageFont) ascentFactor() float64 {
	if f.fi.ascent > 0 {
		return f.fi.ascent / 1000
	}
	return 0.8
}

// ensurePageFontResource names f in p's resources: the name already bound to
// its object, else a new one.
func (p *Page) ensurePageFontResource(f *pageFont) (string, error) {
	pd := p.pageDict()
	if pd == nil {
		return "", fmt.Errorf("add text: page has no dict")
	}
	res := p.pageResources()
	if res == nil {
		res = pdfDict{}
		pd["/Resources"] = res
	}
	fonts, _ := resolveRef(p.doc.objects, res["/Font"]).(pdfDict)
	if fonts == nil {
		fonts = pdfDict{}
		res["/Font"] = fonts
	}
	for _, name := range sortedDictKeys(fonts) {
		if r, ok := fonts[name].(pdfRef); ok && r.Num == f.ref {
			return name, nil
		}
	}
	name := nextFontName(fonts)
	fonts[name] = pdfRef{Num: f.ref}
	return name, nil
}

func sortedDictKeys(d pdfDict) []string {
	keys := make([]string, 0, len(d))
	for k := range d {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// emulatedStyle returns how style s is drawn in f: bold the font does not
// have as fill and stroke at a thirtieth of an em, italic as a 12 degree
// slant. Returns false when s takes away a style the font has (an upright
// face cannot be made of a bold one).
func (f *pageFont) emulatedStyle(s FontStyle) (bold, italic, ok bool) {
	wantB, wantI := s&FontStyleBold != 0, s&FontStyleItalic != 0
	if (f.fi.bold && !wantB) || (f.fi.italic && !wantI) {
		return false, false, false
	}
	return wantB && !f.fi.bold, wantI && !f.fi.italic, true
}

// shownIn returns the characters of the snapshot's segments drawn in a font
// whose object is ref.
func shownIn(snap fragmentSnapshot, ref int) map[rune]bool {
	shown := map[rune]bool{}
	for _, s := range snap.segments {
		if df, ok := s.state.Font.(documentFont); ok && df.ref == ref {
			for _, r := range s.text {
				shown[r] = true
			}
		}
	}
	return shown
}

// pageFont returns the page font df names, with the characters the
// snapshot's fragment showed in it, or nil when there is none to draw with.
func (rf *replaceFonts) pageFont(d *Document, df documentFont, snap *fragmentSnapshot) *pageFont {
	if df.ref == 0 || snap == nil {
		return nil
	}
	if rf.pageFonts == nil {
		rf.pageFonts = map[int]*pageFont{}
	}
	base, ok := rf.pageFonts[df.ref]
	if !ok {
		base = newPageFont(d.objects, df, df.ref)
		rf.pageFonts[df.ref] = base
	}
	if base == nil {
		return nil
	}
	pf := *base
	pf.src = df
	pf.shown = shownIn(*snap, df.ref)
	return &pf
}

// documentFontOf returns st with a page font it is drawn in replaced by the
// documentFont it stands for, as the fragment was found.
func documentFontOf(st TextState) TextState {
	if pf, ok := st.Font.(*pageFont); ok {
		st.Font = pf.src
	}
	return st
}

// missing returns the first character of text covers rejects ("" when none).
func (f *pageFont) missing(text string) string {
	for _, r := range text {
		if !f.covers(string(r)) {
			return string(r)
		}
	}
	return ""
}
