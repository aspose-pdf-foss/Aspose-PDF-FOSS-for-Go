// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"maps"
	"math"
	"reflect"
	"regexp"
	"slices"
	"sort"
	"strings"
	"unicode/utf8"
)

// PDF/A and PDF/UA rules about the glyphs a document actually draws. A font can
// carry every entry the standards ask for and still fail: its /Widths may
// disagree with the program, or a code may reach no glyph. Only the program can
// tell, so these rules read it, through the machinery the renderer uses to pick
// glyphs. The conditions follow veraPDF's profiles.

// fontWidthTolerance is how far /Widths may stray from the program, in 1/1000 em
// (ISO 19005-1 §6.3.6, as veraPDF applies it).
const fontWidthTolerance = 1.0

// fontIssue is one violation found in a font, named without a PDF/A or PDF/UA
// prefix so either validator can report it under its own codes.
type fontIssue struct{ rule, msg string }

// glyphFont is what the walk over a document's text collects for one font.
type glyphFont struct {
	num      int // object number; 0 for a font written directly into a resource dictionary
	name     string
	dict     pdfDict
	fi       fontInfo
	rf       *renderFont // nil when the font has no program to compare with
	seen     map[uint32]bool
	seenTr3  map[uint32]bool
	notdef   []uint32
	missing  []uint32
	unmapped []uint32
	widths   []glyphWidthMismatch
}

type glyphWidthMismatch struct {
	code         uint32
	dict, actual float64
}

// loadEmbeddedFontProgram attaches the font's embedded program to rf, reporting
// whether there was one that parses. Shared by the renderer and the checks here,
// so both pick glyphs the same way.
func loadEmbeddedFontProgram(objects map[int]*pdfObject, rf *renderFont, descriptor pdfDict) bool {
	if stream, ok := resolveRef(objects, descriptor["/FontFile2"]).(*pdfStream); ok {
		if prog, err := parseTTF(decodedStreamData(stream)); err == nil {
			rf.prog = prog
			if prog.unitsPerEm != 0 {
				rf.em = float64(prog.unitsPerEm)
			}
			return true
		}
	}
	// Embedded classic Type1 outlines (/FontFile: eexec-encrypted PostScript
	// charstrings). Glyphs are keyed by name; gid() resolves code → name → GID.
	if stream, ok := resolveRef(objects, descriptor["/FontFile"]).(*pdfStream); ok {
		len1 := dictGetInt(stream.Dict, "/Length1")
		len2 := dictGetInt(stream.Dict, "/Length2")
		if v := resolveRef(objects, stream.Dict["/Length1"]); v != nil {
			len1 = int(operandFloat(v))
		}
		if v := resolveRef(objects, stream.Dict["/Length2"]); v != nil {
			len2 = int(operandFloat(v))
		}
		if t1 := parseType1(decodedStreamData(stream), len1, len2); t1 != nil {
			rf.t1 = t1
			if t1.unitsPerEm != 0 {
				rf.em = t1.unitsPerEm
			}
			return true
		}
	}
	// Embedded CFF outlines (/FontFile3: Type1C, CIDFontType0C, or an OpenType
	// wrapper). The bytes may be a bare CFF or an sfnt with a 'CFF ' table.
	if stream, ok := resolveRef(objects, descriptor["/FontFile3"]).(*pdfStream); ok {
		if cff, err := parseCFFProgram(decodedStreamData(stream)); err == nil {
			rf.cff = cff
			if cff.unitsPerEm != 0 {
				rf.em = cff.unitsPerEm
			}
			return true
		}
	}
	return false
}

// fontDescriptorOf returns a font's descriptor, through the descendant for a
// composite font, along with that descendant.
func fontDescriptorOf(objects map[int]*pdfObject, font pdfDict) (descriptor, cidFont pdfDict) {
	if dictGetName(font, "/Subtype") == "/Type0" {
		if descs, ok := resolveRefToArray(objects, font["/DescendantFonts"]); ok && len(descs) > 0 {
			cidFont, _ = resolveRefToDict(objects, descs[0])
			descriptor, _ = resolveRefToDict(objects, cidFont["/FontDescriptor"])
		}
		return descriptor, cidFont
	}
	descriptor, _ = resolveRefToDict(objects, font["/FontDescriptor"])
	return descriptor, nil
}

// newGlyphFont resolves a font for the walk.
func newGlyphFont(objects map[int]*pdfObject, num int, dict pdfDict) *glyphFont {
	gf := &glyphFont{num: num, dict: dict, fi: resolveFont(objects, dict), seen: map[uint32]bool{}, seenTr3: map[uint32]bool{}}
	gf.name = dictGetName(dict, "/BaseFont")
	if gf.name == "" {
		gf.name = fmt.Sprintf("object %d", num)
	}
	if dictGetName(dict, "/Subtype") == "/Type3" {
		return gf
	}
	descriptor, cidFont := fontDescriptorOf(objects, dict)
	rf := &renderFont{fi: gf.fi, isType0: gf.fi.isType0, em: 1000}
	if cidFont != nil {
		rf.cidToGID = parseCIDToGIDMap(objects, cidFont["/CIDToGIDMap"])
	}
	if loadEmbeddedFontProgram(objects, rf, descriptor) {
		gf.rf = rf
	}
	return gf
}

// glyphState says what a code selects in the font's program.
func (f *renderFont) glyphState(code uint32) (gid uint16, present, notdef bool) {
	if f.t1 != nil && code < 256 && !f.isType0 {
		name := f.t1Name(code)
		_, present = f.t1.charstrings[name]
		return f.t1.nameToGID[name], present, name == "" || name == ".notdef"
	}
	gid = f.gid(code)
	if gid == 0 {
		// Selects nothing. Only a composite font calls this .notdef by that fact;
		// a simple font's glyph is named by its encoding (encodedNotdef).
		return 0, false, f.isType0
	}
	switch {
	case f.prog != nil:
		present = int(gid) < len(f.prog.glyphWidths)
	case f.cff != nil:
		present = int(gid) < f.cff.numGlyphs
	}
	return gid, present, false
}

// encodedNotdef reports whether a simple font's encoding sends a code to no glyph
// name, which is what makes it .notdef: with no /Encoding, the program alone decides
// what a code selects, and an unmapped code is merely absent (ISO 19005-2 §6.2.11.8
// as veraPDF applies it).
func (gf *glyphFont) encodedNotdef(code uint32) bool {
	if gf.fi.isType0 || code > 255 || gf.rf == nil || gf.rf.t1 != nil {
		return false
	}
	if n := gf.fi.glyphNames[byte(code)]; n != "" {
		return n == ".notdef"
	}
	if _, has := gf.dict["/Encoding"]; !has {
		return false
	}
	r := gf.fi.encoding[code]
	return r < 0x20 || r == 0xFFFD // control codes name no glyph either
}

// programWidth returns a glyph's advance according to the program, in 1/1000 em.
func (f *renderFont) programWidth(gid uint16) (float64, bool) {
	var w float64
	var ok bool
	switch {
	case f.prog != nil:
		if ok = int(gid) < len(f.prog.glyphWidths); ok {
			w = float64(f.prog.glyphWidths[gid])
		}
	case f.cff != nil:
		w, ok = f.cff.glyphAdvance(gid)
	case f.t1 != nil:
		w, ok = f.t1.glyphAdvance(gid)
	}
	if !ok || f.em == 0 {
		return 0, false
	}
	return w / f.em * 1000, true
}

// dictWidth returns the width the font dictionary gives a code (a CID for a
// composite font); false when it gives none, which leaves nothing to compare.
func (gf *glyphFont) dictWidth(objects map[int]*pdfObject, code uint32) (float64, bool) {
	if gf.fi.isType0 {
		if w, ok := gf.fi.cidWidths[uint16(code)]; ok {
			return w, true
		}
		return gf.fi.defaultW, true
	}
	if arr, ok := resolveRefToArray(objects, gf.dict["/Widths"]); ok {
		if i := int(code) - dictGetInt(gf.dict, "/FirstChar"); i >= 0 && i < len(arr) {
			return operandFloat(resolveRef(objects, arr[i])), true
		}
	}
	descriptor, _ := fontDescriptorOf(objects, gf.dict)
	if v, ok := descriptor["/MissingWidth"]; ok {
		return operandFloat(resolveRef(objects, v)), true
	}
	return 0, false
}

// hasUnicode reports whether a code can be mapped to Unicode (ISO 14289-1
// §7.21.7). raw is the code as written in the string, cid what it selects.
func (gf *glyphFont) hasUnicode(raw, cid uint32) bool {
	if r, ok := gf.fi.toUnicode[uint16(raw)]; ok && r != 0 {
		return true
	}
	if gf.fi.isType0 {
		return gf.fi.cidToUni[uint16(cid)] != 0
	}
	if raw > 255 {
		return false
	}
	if n := gf.fi.glyphNames[byte(raw)]; n != "" && cffGlyphNameRune(n) != 0 {
		return true
	}
	if r := gf.fi.encoding[raw]; r != 0 && r != 0xFFFD {
		return true
	}
	// Nothing in the dictionary: the program's own name for the glyph may do.
	if gf.rf == nil {
		return false
	}
	switch {
	case gf.rf.t1 != nil:
		return cffGlyphNameRune(gf.rf.t1Name(raw)) != 0
	case gf.rf.cff != nil && !gf.rf.cff.isCID:
		if g := gf.rf.gid(raw); int(g) < len(gf.rf.cff.charset) {
			return cffGlyphNameRune(gf.rf.cff.sidName(gf.rf.cff.charset[g])) != 0
		}
	}
	return false
}

// show records every code of a shown string.
func (gf *glyphFont) show(objects map[int]*pdfObject, s string, tr int) {
	visit := func(raw, cid uint32) {
		seen := gf.seen
		if tr == 3 {
			seen = gf.seenTr3
		}
		if seen[cid] {
			return
		}
		seen[cid] = true
		if !gf.hasUnicode(raw, cid) {
			gf.unmapped = append(gf.unmapped, raw)
		}
		if gf.rf == nil {
			return
		}
		gid, present, notdef := gf.rf.glyphState(cid)
		if notdef || gf.encodedNotdef(cid) {
			gf.notdef = append(gf.notdef, raw)
		}
		if tr == 3 {
			return // nothing is painted, so nothing is measured
		}
		if !present {
			gf.missing = append(gf.missing, raw)
			return
		}
		pw, ok := gf.rf.programWidth(gid)
		if !ok {
			return
		}
		if dw, ok := gf.dictWidth(objects, cid); ok && math.Abs(pw-dw) > fontWidthTolerance {
			gf.widths = append(gf.widths, glyphWidthMismatch{code: raw, dict: dw, actual: pw})
		}
	}
	if !gf.fi.isType0 {
		for i := 0; i < len(s); i++ {
			visit(uint32(s[i]), uint32(s[i]))
		}
		return
	}
	if gf.fi.cidCMap != nil {
		b := []byte(s)
		for len(b) > 0 {
			code, cid, n := gf.fi.cidCMap.next(b)
			if n <= 0 {
				return
			}
			visit(code, uint32(cid))
			b = b[n:]
		}
		return
	}
	for i := 0; i+1 < len(s); i += 2 {
		c := uint32(s[i])<<8 | uint32(s[i+1])
		visit(c, c)
	}
}

// glyphFontsInUse walks every content stream and returns what each font draws,
// in object-number order.
func (d *Document) glyphFontsInUse() []*glyphFont {
	type key struct {
		num int
		ptr uintptr
	}
	fonts := map[key]*glyphFont{}
	d.forEachContentStream(func(data []byte, res pdfDict) {
		fontRes, ok := resolveRefToDict(d.objects, res["/Font"])
		if !ok {
			return
		}
		ops, err := parseContentStream(data)
		if err != nil {
			return
		}
		type state struct {
			font *glyphFont
			tr   int
		}
		var cur state
		var stack []state
		show := func(v pdfValue) {
			if s, ok := v.(string); ok && cur.font != nil {
				cur.font.show(d.objects, s, cur.tr)
			}
		}
		for _, op := range ops {
			switch op.Operator {
			case "q":
				stack = append(stack, cur)
			case "Q":
				if n := len(stack); n > 0 {
					cur, stack = stack[n-1], stack[:n-1]
				}
			case "Tr":
				if len(op.Operands) > 0 {
					cur.tr = int(operandFloat(op.Operands[0]))
				}
			case "Tf":
				cur.font = nil
				if len(op.Operands) == 0 {
					continue
				}
				name, ok := op.Operands[0].(pdfName)
				if !ok {
					continue
				}
				val := fontRes[string(name)]
				dict, ok := resolveRefToDict(d.objects, val)
				if !ok {
					continue
				}
				var k key
				if ref, isRef := val.(pdfRef); isRef {
					k.num = ref.Num
				} else {
					k.ptr = reflect.ValueOf(dict).Pointer()
				}
				if fonts[k] == nil {
					fonts[k] = newGlyphFont(d.objects, k.num, dict)
				}
				cur.font = fonts[k]
			case "Tj", "'":
				if len(op.Operands) >= 1 {
					show(op.Operands[0])
				}
			case "\"":
				if len(op.Operands) >= 3 {
					show(op.Operands[2])
				}
			case "TJ":
				if len(op.Operands) >= 1 {
					if arr, ok := op.Operands[0].(pdfArray); ok {
						for _, el := range arr {
							show(el)
						}
					}
				}
			}
		}
	})
	out := slices.Collect(maps.Values(fonts))
	sort.Slice(out, func(i, j int) bool {
		if out[i].num != out[j].num {
			return out[i].num < out[j].num
		}
		return out[i].name < out[j].name
	})
	return out
}

// fontGlyphIssues reports the glyph-level rules for what the document draws.
// wantUnicode adds the PDF/UA rule that every code maps to Unicode.
func (d *Document) fontGlyphIssues(wantUnicode bool) []fontIssue {
	var out []fontIssue
	for _, gf := range d.glyphFontsInUse() {
		if len(gf.notdef) > 0 {
			slices.Sort(gf.notdef)
			out = append(out, fontIssue{"NOTDEF_GLYPH_USED", fmt.Sprintf("font %s draws code %d with no glyph (.notdef), %d distinct codes in all; rendering through .notdef is forbidden", gf.name, gf.notdef[0], len(gf.notdef))})
		}
		if len(gf.missing) > 0 {
			slices.Sort(gf.missing)
			out = append(out, fontIssue{"GLYPH_MISSING", fmt.Sprintf("font %s draws code %d, which its embedded program does not define, %d distinct codes in all", gf.name, gf.missing[0], len(gf.missing))})
		}
		if len(gf.widths) > 0 {
			w := slices.MinFunc(gf.widths, func(a, b glyphWidthMismatch) int { return int(a.code) - int(b.code) })
			out = append(out, fontIssue{"FONT_WIDTH_MISMATCH", fmt.Sprintf("font %s: code %d has width %g in the font dictionary but %.1f in the embedded program, %d distinct codes in all", gf.name, w.code, w.dict, w.actual, len(gf.widths))})
		}
		if wantUnicode && len(gf.unmapped) > 0 {
			slices.Sort(gf.unmapped)
			out = append(out, fontIssue{"NO_UNICODE_MAPPING", fmt.Sprintf("font %s: code %d has no Unicode mapping, %d distinct codes in all", gf.name, gf.unmapped[0], len(gf.unmapped))})
		}
	}
	return out
}

var (
	reCMapRegistry   = regexp.MustCompile(`/Registry\s*\(([^)]*)\)`)
	reCMapOrdering   = regexp.MustCompile(`/Ordering\s*\(([^)]*)\)`)
	reCMapSupplement = regexp.MustCompile(`/Supplement\s+(\d+)`)
)

// cmapSystemInfo returns the /CIDSystemInfo a composite font's CMap declares,
// and that CMap's name. ok is false when it cannot be read, or for the Identity
// CMaps, which match anything.
func (d *Document) cmapSystemInfo(font pdfDict) (registry, ordering string, supplement int, ok bool) {
	var cm *cidCMap
	switch v := resolveRef(d.objects, font["/Encoding"]).(type) {
	case pdfName:
		if name := string(v); name == "/Identity-H" || name == "/Identity-V" {
			return "", "", 0, false
		}
		cm = predefinedCMap(strings.TrimPrefix(string(v), "/"))
	case *pdfStream:
		if name := dictGetName(v.Dict, "/CMapName"); name == "/Identity-H" || name == "/Identity-V" {
			return "", "", 0, false
		}
		if info, isDict := resolveRefToDict(d.objects, v.Dict["/CIDSystemInfo"]); isDict {
			reg, r := info["/Registry"].(string)
			ord, o := info["/Ordering"].(string)
			return reg, ord, dictGetInt(info, "/Supplement"), r && o
		}
		cm = parseCIDCMap(decodedStreamData(v), predefinedCMap)
	}
	if cm == nil || cm.ordering == "" {
		return "", "", 0, false
	}
	return cm.registry, cm.ordering, cm.supplement, true
}

// compositeFontsInUse lists the used Type0 fonts in object-number order, with
// their descendants.
func (d *Document) compositeFontsInUse() (fonts []pdfDict, cidFonts []pdfDict) {
	used := d.pdfaUsedFonts()
	for _, id := range slices.Sorted(maps.Keys(d.objects)) {
		font, ok := d.objects[id].Value.(pdfDict)
		if !ok || !used[id] || dictGetName(font, "/Subtype") != "/Type0" {
			continue
		}
		if _, cid := fontDescriptorOf(d.objects, font); cid != nil {
			fonts, cidFonts = append(fonts, font), append(cidFonts, cid)
		}
	}
	return fonts, cidFonts
}

// fontDictIssues reports what the used fonts' dictionaries get wrong, as far as
// PDF/A and PDF/UA share the rules. part is the PDF/A part; 2 for PDF/UA, which
// follows PDF/A-2 here.
func (d *Document) fontDictIssues(part int) []fontIssue {
	var out []fontIssue
	used := d.pdfaUsedFonts()
	for _, id := range slices.Sorted(maps.Keys(used)) {
		obj := d.objects[id]
		if obj == nil {
			continue
		}
		if font, ok := obj.Value.(pdfDict); ok && dictGetName(font, "/Type") != "/Font" {
			out = append(out, fontIssue{"FONT_NO_TYPE", fmt.Sprintf("font %s (object %d) has no /Type /Font", dictGetName(font, "/BaseFont"), id)})
		}
	}
	fonts, cids := d.compositeFontsInUse()
	for i, font := range fonts {
		name := dictGetName(font, "/BaseFont")
		enc, isName := resolveRef(d.objects, font["/Encoding"]).(pdfName)
		if part == 1 && isName && enc != "/Identity-H" && enc != "/Identity-V" {
			out = append(out, fontIssue{"CMAP_NOT_EMBEDDED", fmt.Sprintf("font %s uses the CMap %s, which is not embedded; PDF/A-1 requires every CMap but Identity to be", name, enc)})
		}
		reg, ord, sup, ok := d.cmapSystemInfo(font)
		if !ok {
			continue
		}
		info, _ := resolveRefToDict(d.objects, cids[i]["/CIDSystemInfo"])
		fReg, _ := info["/Registry"].(string)
		fOrd, _ := info["/Ordering"].(string)
		if fReg != reg || fOrd != ord || (part > 1 && dictGetInt(info, "/Supplement") > sup) {
			out = append(out, fontIssue{"CIDSYSTEMINFO_MISMATCH", fmt.Sprintf("font %s: CIDFont is %s-%s-%d, its CMap %s-%s-%d", name, fReg, fOrd, dictGetInt(info, "/Supplement"), reg, ord, sup)})
		}
	}
	for _, id := range slices.Sorted(maps.Keys(used)) {
		obj := d.objects[id]
		if obj == nil {
			continue
		}
		if font, ok := obj.Value.(pdfDict); ok {
			out = append(out, d.simpleFontIssues(font, part)...)
		}
	}
	return out
}

// simpleFontIssues covers /CharSet on a Type1 subset and the encoding rules for
// a TrueType font.
func (d *Document) simpleFontIssues(font pdfDict, part int) []fontIssue {
	var out []fontIssue
	descriptor, _ := resolveRefToDict(d.objects, font["/FontDescriptor"])
	name := dictGetName(font, "/BaseFont")
	switch dictGetName(font, "/Subtype") {
	case "/Type1", "/MMType1":
		if !reSubsetName.MatchString(strings.TrimPrefix(name, "/")) {
			return nil
		}
		want := d.programGlyphNames(descriptor)
		if want == nil {
			return nil
		}
		cs, has := resolveRef(d.objects, descriptor["/CharSet"]).(string)
		if !has && part > 1 {
			return nil
		}
		if !has || !slices.Equal(parseCharSet(cs), want) {
			out = append(out, fontIssue{"CHARSET_INCOMPLETE", fmt.Sprintf("Type1 subset %s: /CharSet does not list exactly the glyphs of the embedded program", name)})
		}
	case "/TrueType":
		symbolic := dictGetInt(descriptor, "/Flags")&4 != 0
		_, hasEnc := font["/Encoding"]
		if symbolic && hasEnc {
			out = append(out, fontIssue{"TRUETYPE_ENCODING", fmt.Sprintf("symbolic TrueType font %s has an /Encoding; it must have none", name)})
		}
		if part != 1 {
			return out
		}
		if !symbolic && !d.plainTrueTypeEncoding(font) {
			out = append(out, fontIssue{"TRUETYPE_ENCODING", fmt.Sprintf("non-symbolic TrueType font %s needs MacRomanEncoding or WinAnsiEncoding with no /Differences in PDF/A-1", name)})
		}
		if st, ok := resolveRef(d.objects, descriptor["/FontFile2"]).(*pdfStream); ok && symbolic && ttfCmapCount(decodedStreamData(st)) > 1 {
			out = append(out, fontIssue{"TRUETYPE_CMAPS", fmt.Sprintf("symbolic TrueType font %s must have exactly one cmap subtable in PDF/A-1", name)})
		}
	}
	return out
}

// plainTrueTypeEncoding reports whether a font's encoding is a bare
// MacRomanEncoding or WinAnsiEncoding.
func (d *Document) plainTrueTypeEncoding(font pdfDict) bool {
	base := func(n string) bool { return n == "/MacRomanEncoding" || n == "/WinAnsiEncoding" }
	switch v := resolveRef(d.objects, font["/Encoding"]).(type) {
	case pdfName:
		return base(string(v))
	case pdfDict:
		_, diff := v["/Differences"]
		return !diff && base(dictGetName(v, "/BaseEncoding"))
	}
	return false
}

// ttfCmapCount returns the number of encoding records in an sfnt's cmap table.
func ttfCmapCount(data []byte) int {
	b := tableSlice(data, tableDir(data), "cmap")
	if len(b) < 4 {
		return 0
	}
	return int(binary.BigEndian.Uint16(b[2:4]))
}

var reSubsetName = regexp.MustCompile(`^[A-Z]{6}\+`)

// programGlyphNames returns the sorted glyph names of a Type1 or Type1C program,
// without .notdef; nil when the descriptor has no such program.
func (d *Document) programGlyphNames(descriptor pdfDict) []string {
	rf := &renderFont{em: 1000}
	if !loadEmbeddedFontProgram(d.objects, rf, descriptor) {
		return nil
	}
	var names []string
	switch {
	case rf.t1 != nil:
		names = slices.Clone(rf.t1.glyphNames)
	case rf.cff != nil && !rf.cff.isCID:
		for gid := 1; gid < rf.cff.numGlyphs && gid < len(rf.cff.charset); gid++ {
			names = append(names, rf.cff.sidName(rf.cff.charset[gid]))
		}
	default:
		return nil
	}
	names = slices.DeleteFunc(names, func(n string) bool { return n == ".notdef" || n == "" })
	sort.Strings(names)
	return names
}

// parseCharSet splits a /CharSet string ("/a/b/c") into sorted glyph names,
// without .notdef.
func parseCharSet(s string) []string {
	names := strings.FieldsFunc(s, func(r rune) bool { return r == '/' || r == ' ' || r == '\t' || r == '\r' || r == '\n' })
	names = slices.DeleteFunc(names, func(n string) bool { return n == ".notdef" })
	sort.Strings(names)
	return names
}

// fixFontDicts repairs the font dictionaries where nothing about the document's
// appearance depends on it: the /Type of a used font, a Type1 subset's /CharSet,
// and a CIDFont whose /Supplement is ahead of its CMap's.
func (d *Document) fixFontDicts(part int) {
	used := d.pdfaUsedFonts()
	for _, id := range slices.Sorted(maps.Keys(used)) {
		obj := d.objects[id]
		if obj == nil {
			continue
		}
		font, ok := obj.Value.(pdfDict)
		if !ok {
			continue
		}
		font["/Type"] = pdfName("/Font")
		if st := dictGetName(font, "/Subtype"); st != "/Type1" && st != "/MMType1" {
			continue
		}
		descriptor, ok := resolveRefToDict(d.objects, font["/FontDescriptor"])
		if !ok || !reSubsetName.MatchString(strings.TrimPrefix(dictGetName(font, "/BaseFont"), "/")) {
			continue
		}
		_, has := descriptor["/CharSet"]
		if !has && part > 1 {
			continue
		}
		if names := d.programGlyphNames(descriptor); names != nil {
			descriptor["/CharSet"] = "/" + strings.Join(names, "/")
		}
	}
	fonts, cids := d.compositeFontsInUse()
	for i, font := range fonts {
		reg, ord, sup, ok := d.cmapSystemInfo(font)
		if !ok {
			continue
		}
		info, _ := resolveRefToDict(d.objects, cids[i]["/CIDSystemInfo"])
		if info["/Registry"] == reg && info["/Ordering"] == ord && dictGetInt(info, "/Supplement") > sup {
			// A font of a later supplement holds every CID of the earlier one.
			cids[i]["/CIDSystemInfo"] = pdfDict{"/Registry": reg, "/Ordering": ord, "/Supplement": sup}
		}
	}
}

// pdfaCheckFontConformance reports the font rules that need the embedded program
// or the fonts' dictionaries.
func (d *Document) pdfaCheckFontConformance(format PDFAFormat, r *PDFAValidationReport) {
	for _, is := range d.fontDictIssues(format.part()) {
		r.add(is.rule, is.msg)
	}
	for _, is := range d.fontGlyphIssues(false) {
		if is.rule == "NOTDEF_GLYPH_USED" && format.part() == 1 {
			continue // PDF/A-1 has no rule against .notdef itself
		}
		r.add(is.rule, is.msg)
	}
}

// signatureStale reports a signature whose /ByteRange no longer spans the file
// (ISO 19005-2 §6.4.3): one the document was converted out from under, or that
// never covered the whole of the file it came from.
func (d *Document) signatureStale() bool {
	found := false
	d.pdfaWalkDicts(func(dict pdfDict) {
		if found || !isSignatureDict(dict) {
			return
		}
		br, ok := resolveRef(d.objects, dict["/ByteRange"]).(pdfArray)
		if !ok || len(br) != 4 {
			return
		}
		if d.signaturesRewritten || (d.source != nil && int(operandFloat(br[2])+operandFloat(br[3])) != len(d.source)) {
			found = true
		}
	})
	return found
}

// hasSignature reports whether the document carries a signature value.
func (d *Document) hasSignature() bool {
	found := false
	d.pdfaWalkDicts(func(dict pdfDict) {
		found = found || isSignatureDict(dict)
	})
	return found
}

// fixOutOfRangeIntegers repairs integers a producer wrote as unsigned 32-bit values
// (a /Descent of 4294967076 means -220), which PDF/A's limits rule out.
func (d *Document) fixOutOfRangeIntegers() {
	wrap := func(v pdfValue) pdfValue {
		if n, ok := pdfIntValue(v); ok && n > math.MaxInt32 && n <= math.MaxUint32 {
			return int(n - 1<<32)
		}
		return v
	}
	var walk func(v pdfValue)
	walk = func(v pdfValue) {
		switch x := v.(type) {
		case pdfDict:
			for k, e := range x {
				x[k] = wrap(e)
				walk(e)
			}
		case pdfArray:
			for i, e := range x {
				x[i] = wrap(e)
				walk(e)
			}
		case *pdfStream:
			walk(x.Dict)
		}
	}
	for _, id := range slices.Sorted(maps.Keys(d.objects)) {
		walk(d.objects[id].Value)
	}
}

func pdfIntValue(v pdfValue) (int64, bool) {
	switch n := v.(type) {
	case int:
		return int64(n), true
	case int64:
		return n, true
	}
	return 0, false
}

// pdfaCheckLimits reports the implementation limits a file can exceed (ISO 19005-1
// §6.1.12, ISO 19005-2 §6.1.13), and for part 2 and later a name that is not UTF-8
// (ISO 19005-2 §6.1.8).
func (d *Document) pdfaCheckLimits(format PDFAFormat, r *PDFAValidationReport) {
	badInt, badReal, badName := false, false, ""
	number := func(v pdfValue) {
		if n, ok := pdfIntValue(v); ok && (n > math.MaxInt32 || n < math.MinInt32) {
			badInt = true
		}
		if f, ok := v.(float64); ok && format.part() == 1 && math.Abs(f) > 32767 {
			badReal = true
		}
	}
	var walk func(v pdfValue)
	walk = func(v pdfValue) {
		number(v)
		switch x := v.(type) {
		case pdfDict:
			for _, k := range slices.Sorted(maps.Keys(x)) {
				walk(x[k])
			}
		case pdfArray:
			for _, e := range x {
				walk(e)
			}
		case *pdfStream:
			walk(x.Dict)
		}
	}
	for _, id := range slices.Sorted(maps.Keys(d.objects)) {
		walk(d.objects[id].Value)
	}
	for _, id := range slices.Sorted(maps.Keys(d.pdfaUsedFonts())) {
		if font, ok := d.objects[id].Value.(pdfDict); ok && badName == "" {
			if n := dictGetName(font, "/BaseFont"); !utf8.ValidString(n) {
				badName = n
			}
		}
	}
	d.pdfaWalkDicts(func(dict pdfDict) {
		res, ok := resolveRefToDict(d.objects, dict["/Resources"])
		if !ok || badName != "" {
			return
		}
		for _, cat := range slices.Sorted(maps.Keys(res)) {
			if names, ok := resolveRefToDict(d.objects, res[cat]); ok {
				for _, n := range slices.Sorted(maps.Keys(names)) {
					if !utf8.ValidString(n) {
						badName = n
						return
					}
				}
			}
		}
	})
	if format.part() == 1 {
		d.forEachContentStream(func(data []byte, _ pdfDict) {
			ops, err := parseContentStream(data)
			if err != nil {
				return
			}
			for _, op := range ops {
				for _, v := range op.Operands {
					number(v)
				}
			}
		})
	}
	if badInt {
		r.add("NUMBER_OUT_OF_RANGE", "the document has an integer outside the 32-bit range PDF/A allows")
	}
	if badReal {
		r.add("NUMBER_OUT_OF_RANGE", "the document has a real number outside the ±32767 PDF/A-1 allows")
	}
	if badName != "" && format.part() > 1 {
		r.add("NAME_NOT_UTF8", fmt.Sprintf("the name %q is not valid UTF-8", badName))
	}
}

// pdfaCheckAttachedPDFA reports what PDF/A-2 forbids of attachments: a file that is
// not itself PDF/A-1 or PDF/A-2 (ISO 19005-2 §6.8). This reads the attachment's own
// claim; it does not validate it. PDF/A-3 is the part that allows any file.
func (d *Document) pdfaCheckAttachedPDFA(r *PDFAValidationReport) {
	reported := map[*pdfStream]bool{}
	d.pdfaWalkDicts(func(dict pdfDict) {
		ef, ok := resolveRefToDict(d.objects, dict["/EF"])
		if !ok {
			return
		}
		st, ok := resolveRef(d.objects, ef["/F"]).(*pdfStream)
		if !ok || reported[st] || attachmentClaimsPDFA12(decodedStreamData(st)) {
			return
		}
		reported[st] = true
		name, _ := resolveRef(d.objects, dict["/F"]).(string)
		r.add("EMBEDDED_FILE_NOT_PDFA", fmt.Sprintf("embedded file %q is not PDF/A-1 or PDF/A-2, as PDF/A-2 requires; PDF/A-3 allows any file", name))
	})
}

// pdfaAnnotSubtypesPart1 are the annotation types PDF/A-1 allows (ISO 19005-1
// §6.5.2); later parts forbid only pdfaAnnotSubtypesForbidden, and unknown types.
var (
	pdfaAnnotSubtypesPart1 = map[string]bool{
		"/Text": true, "/Link": true, "/FreeText": true, "/Line": true, "/Square": true, "/Circle": true,
		"/Highlight": true, "/Underline": true, "/Squiggly": true, "/StrikeOut": true, "/Stamp": true,
		"/Ink": true, "/Popup": true, "/Widget": true, "/PrinterMark": true, "/TrapNet": true,
	}
	pdfaAnnotSubtypesForbidden = map[string]bool{"/3D": true, "/Sound": true, "/Screen": true, "/Movie": true}
)

// pdfaCheckAnnotationTypes reports annotation types the part does not allow. A
// conversion cannot repair this without removing what the annotation carries.
func (d *Document) pdfaCheckAnnotationTypes(format PDFAFormat, r *PDFAValidationReport) {
	reported := map[string]bool{}
	d.pdfaEachAnnotation(func(ad pdfDict) {
		sub := dictGetName(ad, "/Subtype")
		bad := pdfaAnnotSubtypesForbidden[sub]
		if format.part() == 1 {
			bad = !pdfaAnnotSubtypesPart1[sub]
		}
		if bad && !reported[sub] {
			reported[sub] = true
			r.add("ANNOTATION_TYPE_FORBIDDEN", fmt.Sprintf("the document has a %s annotation, which PDF/A-%d does not allow", sub, format.part()))
		}
	})
}

func attachmentClaimsPDFA12(data []byte) bool {
	doc, err := OpenStream(bytes.NewReader(data))
	if err != nil {
		return false
	}
	raw, err := doc.XMPRaw()
	if err != nil {
		return false
	}
	part := xmpPDFAPart(string(raw))
	return part == "1" || part == "2"
}
