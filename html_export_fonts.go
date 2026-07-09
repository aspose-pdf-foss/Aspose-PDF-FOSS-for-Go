// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"compress/zlib"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"sort"
	"strings"
)

// WOFF font embedding for the HTML exporter (epic pdf-go-rfom, phase 3).
// In HTMLModeText the document's embedded font programs are re-wrapped as
// WOFF1 (zlib-compressed sfnt tables, RFC-free pure Go) and emitted as
// @font-face data: URLs, so the visible text layer renders with the
// document's real faces instead of metric substitutes.
//
// Coverage: TrueType programs (/FontFile2 — simple and CIDFontType2) and
// full-sfnt OpenType programs (/FontFile3 /Subtype /OpenType). For composite
// fonts the sfnt usually carries no cmap (PDF maps CIDs directly), while a
// browser resolves glyphs from Unicode — so a cmap (format 4) is synthesized
// from the font's /ToUnicode CMap composed with /CIDToGIDMap. Missing
// name/OS∕2/post tables (typical in PDF subsets, required by browser font
// sanitizers like OTS) are synthesized too. Bare-CFF (/FontFile3 Type1C /
// CIDFontType0C) and Type1 (/FontFile) programs are left to the substitute +
// width-fitting fallback of phase 2; PDF-embedded fonts are near-always
// already subset by the producer, so no additional glyph trimming is done
// (for documents built with this library, call SubsetFonts before SaveHTML).

// htmlFont is one embeddable font discovered in the exported pages.
type htmlFont struct {
	id      string // CSS family: "ef0", "ef1", …
	class   string // generic fallback family: "sans" / "serif" / "mono"
	bold    bool
	italic  bool
	flavor  uint32            // sfnt version tag of the program
	tbls    map[string][]byte // parsed program tables
	runeGID map[rune]uint16   // ToUnicode ∘ CIDToGIDMap; nil → keep the font's own cmap
	used    map[rune]struct{} // runes drawn with this font (cmap + OS/2 range)
	woff    []byte            // built by finish()
}

// htmlFontSet collects embeddable fonts across the exported pages, deduped
// by font-dict object number, with a per-page fragment-name → font map.
type htmlFontSet struct {
	doc     *Document
	byObj   map[int]*htmlFont            // font dict object number → font (nil = unsupported)
	perPage map[*Page]map[string]*htmlFont // cleanFontName → font, per page
	fonts   []*htmlFont                  // discovery order
}

func newHTMLFontSet(d *Document) *htmlFontSet {
	return &htmlFontSet{
		doc:     d,
		byObj:   make(map[int]*htmlFont),
		perPage: make(map[*Page]map[string]*htmlFont),
	}
}

// pageFonts builds (and caches) the fragment-name → font map for one page,
// walking the page font resources and those of nested Form XObjects.
func (fs *htmlFontSet) pageFonts(p *Page) map[string]*htmlFont {
	if m, ok := fs.perPage[p]; ok {
		return m
	}
	m := make(map[string]*htmlFont)
	fs.perPage[p] = m
	visited := make(map[int]bool)
	fs.collectFromResources(p.pageResources(), m, visited, 0)
	return m
}

// collectFromResources walks one /Resources dict: every /Font entry is
// resolved into an htmlFont (first name wins), and /XObject Form streams are
// recursed into (they carry their own font resources).
func (fs *htmlFontSet) collectFromResources(res pdfDict, m map[string]*htmlFont, visited map[int]bool, depth int) {
	if res == nil || depth > 3 {
		return
	}
	objects := fs.doc.objects
	if fontDict, ok := resolveRefToDict(objects, res["/Font"]); ok {
		names := make([]string, 0, len(fontDict))
		for name := range fontDict {
			names = append(names, name)
		}
		sort.Strings(names) // deterministic first-wins on duplicate base names
		for _, name := range names {
			objNum := 0
			if ref, ok := fontDict[name].(pdfRef); ok {
				objNum = ref.Num
			}
			fd, ok := resolveRefToDict(objects, fontDict[name])
			if !ok {
				continue
			}
			key := cleanFontName(dictGetName(fd, "/BaseFont"))
			if key == "" {
				continue
			}
			if _, dup := m[key]; dup {
				continue
			}
			f, seen := fs.byObj[objNum]
			if !seen || objNum == 0 {
				f = fs.buildFont(fd)
				if objNum != 0 {
					fs.byObj[objNum] = f
				}
				if f != nil {
					f.id = fmt.Sprintf("ef%d", len(fs.fonts))
					fs.fonts = append(fs.fonts, f)
				}
			}
			m[key] = f // nil marks "seen but unsupported"
		}
	}
	if xobjs, ok := resolveRefToDict(objects, res["/XObject"]); ok {
		for _, v := range xobjs {
			ref, isRef := v.(pdfRef)
			if isRef {
				if visited[ref.Num] {
					continue
				}
				visited[ref.Num] = true
			}
			stream, ok := resolveRef(objects, v).(*pdfStream)
			if !ok || dictGetName(stream.Dict, "/Subtype") != "/Form" {
				continue
			}
			if sub, ok := resolveRefToDict(objects, stream.Dict["/Resources"]); ok {
				fs.collectFromResources(sub, m, visited, depth+1)
			}
		}
	}
}

// markUsed records which runes each embedded font actually draws on this
// page, so cmap synthesis and OS/2 ranges cover exactly the exported text.
func (fs *htmlFontSet) markUsed(p *Page, lines []TextLine) {
	m := fs.pageFonts(p)
	for _, line := range lines {
		for _, frag := range line.Fragments {
			f := m[frag.FontName]
			if f == nil {
				continue
			}
			if f.used == nil {
				f.used = make(map[rune]struct{})
			}
			for _, r := range frag.Text {
				f.used[r] = struct{}{}
			}
		}
	}
}

// resolve returns the embedded font for a fragment on page p, or nil when
// the fragment stays on the substitute + width-fitting path (font not
// embeddable, or its WOFF build failed).
func (fs *htmlFontSet) resolve(p *Page, fontName string) *htmlFont {
	f := fs.perPage[p][fontName]
	if f == nil || f.woff == nil {
		return nil
	}
	return f
}

// buildFont extracts the embeddable program behind one font dict. Returns
// nil for unsupported flavours (bare CFF, Type1, no usable Unicode mapping).
func (fs *htmlFontSet) buildFont(fontDict pdfDict) *htmlFont {
	objects := fs.doc.objects
	fi := resolveFont(objects, fontDict)
	f := &htmlFont{
		class:  fontFamilyClass(cleanFontName(fi.name)),
		bold:   fi.bold,
		italic: fi.italic,
	}

	switch dictGetName(fontDict, "/Subtype") {
	case "/Type0":
		// Composite font: the program hangs off the descendant CIDFont.
		enc := dictGetName(fontDict, "/Encoding")
		if enc != "/Identity-H" && enc != "/Identity-V" {
			return nil // predefined/embedded CMap codes ≠ CIDs — not mapped in v1
		}
		if len(fi.toUnicode) == 0 {
			return nil // no Unicode mapping — a browser cmap can't be built
		}
		descArr, ok := resolveRef(objects, fontDict["/DescendantFonts"]).(pdfArray)
		if !ok || len(descArr) == 0 {
			return nil
		}
		desc, ok := resolveRefToDict(objects, descArr[0])
		if !ok {
			return nil
		}
		if !fs.loadProgram(f, desc) {
			return nil
		}
		cidGID := cidToGIDFunc(objects, desc)
		f.runeGID = make(map[rune]uint16, len(fi.toUnicode))
		codes := make([]int, 0, len(fi.toUnicode))
		for code := range fi.toUnicode {
			codes = append(codes, int(code))
		}
		sort.Ints(codes) // deterministic winner when two codes share a rune
		for _, code := range codes {
			r := fi.toUnicode[uint16(code)]
			if r > 0xFFFF { // cmap format 4 is BMP-only
				continue
			}
			if gid := cidGID(uint16(code)); gid != 0 {
				if _, dup := f.runeGID[r]; !dup {
					f.runeGID[r] = gid
				}
			}
		}
		if len(f.runeGID) == 0 {
			return nil
		}
	case "/TrueType":
		// Simple TrueType: the browser maps text through the font's own
		// cmap, so one must be present; the program is kept as-is.
		if !fs.loadProgram(f, fontDict) {
			return nil
		}
		if _, ok := f.tbls["cmap"]; !ok {
			return nil
		}
	default:
		return nil
	}
	return f
}

// loadProgram reads /FontDescriptor's /FontFile2 (TrueType) or /FontFile3
// /Subtype /OpenType (full sfnt) into f.tbls/f.flavor. Bare CFF and Type1
// programs return false.
func (fs *htmlFontSet) loadProgram(f *htmlFont, fontDict pdfDict) bool {
	fdesc, ok := resolveRefToDict(fs.doc.objects, fontDict["/FontDescriptor"])
	if !ok {
		return false
	}
	var program []byte
	if s, ok := resolveRef(fs.doc.objects, fdesc["/FontFile2"]).(*pdfStream); ok {
		program = decodedStreamData(s)
	} else if s, ok := resolveRef(fs.doc.objects, fdesc["/FontFile3"]).(*pdfStream); ok {
		if dictGetName(s.Dict, "/Subtype") != "/OpenType" {
			return false // bare CFF — no sfnt wrapper to re-wrap
		}
		program = decodedStreamData(s)
	} else {
		return false
	}
	flavor, tbls, err := sfntReadTables(program)
	if err != nil {
		return false
	}
	f.flavor, f.tbls = flavor, tbls
	return true
}

// cidToGIDFunc returns the CID → GID mapping of a descendant CIDFont dict:
// identity when /CIDToGIDMap is absent or /Identity, else the 2-byte-per-CID
// stream lookup per ISO 32000-1 §9.7.4.3.
func cidToGIDFunc(objects map[int]*pdfObject, desc pdfDict) func(uint16) uint16 {
	if s, ok := resolveRef(objects, desc["/CIDToGIDMap"]).(*pdfStream); ok {
		data := decodedStreamData(s)
		return func(cid uint16) uint16 {
			i := int(cid) * 2
			if i+1 >= len(data) {
				return 0
			}
			return binary.BigEndian.Uint16(data[i:])
		}
	}
	return func(cid uint16) uint16 { return cid }
}

// finish builds the WOFF for every used font and returns the @font-face +
// span-class CSS block ("" when nothing embeddable was found). Fonts whose
// build fails are dropped (their spans keep the phase-2 fallback path).
func (fs *htmlFontSet) finish() string {
	var css strings.Builder
	for _, f := range fs.fonts {
		if len(f.used) == 0 {
			continue
		}
		woff, err := f.build()
		if err != nil {
			continue
		}
		f.woff = woff
		weight, style := "normal", "normal"
		if f.bold {
			weight = "bold"
		}
		if f.italic {
			style = "italic"
		}
		fmt.Fprintf(&css, "@font-face { font-family:'%s'; src:url(data:font/woff;base64,%s) format('woff'); font-weight:%s; font-style:%s; }\n",
			f.id, base64.StdEncoding.EncodeToString(woff), weight, style)
		fmt.Fprintf(&css, ".tv span.%s, .fl .%s { font-family:'%s', %s; }\n", f.id, f.id, f.id, familyStack(f.class))
	}
	return css.String()
}

// familyStack returns the metric-substitute CSS stack for a generic class.
func familyStack(class string) string {
	switch class {
	case "serif":
		return "'Times New Roman', Times, serif"
	case "mono":
		return "'Courier New', Courier, monospace"
	}
	return "Arial, Helvetica, sans-serif"
}

// build assembles the final sfnt (synthesized cmap for composite fonts;
// name/OS∕2/post filled in when the PDF subset dropped them) and wraps it
// as WOFF1.
func (f *htmlFont) build() ([]byte, error) {
	tbls := make(map[string][]byte, len(f.tbls)+4)
	for tag, data := range f.tbls {
		tbls[tag] = data
	}
	if f.runeGID != nil {
		cm := make(map[rune]uint16, len(f.used))
		for r := range f.used {
			if gid, ok := f.runeGID[r]; ok {
				cm[r] = gid
			}
		}
		if len(cm) == 0 {
			return nil, fmt.Errorf("html font: no used rune maps to a glyph")
		}
		tbls["cmap"] = buildCmapFormat4(cm)
	}
	if _, ok := tbls["head"]; !ok {
		return nil, fmt.Errorf("html font: no head table")
	}
	if _, ok := tbls["post"]; !ok {
		tbls["post"] = buildMinimalPost(f.italic)
	}
	if _, ok := tbls["name"]; !ok {
		tbls["name"] = buildMinimalName(f.id)
	}
	if _, ok := tbls["OS/2"]; !ok {
		first, last := runeRange(f.used)
		tbls["OS/2"] = buildMinimalOS2(f.bold, f.italic, tbls["hhea"], first, last)
	}
	flavor := f.flavor
	if flavor == 0x74727565 { // 'true' (classic Mac) — browsers only take 1.0/OTTO
		flavor = 0x00010000
	}
	return woffEncode(assembleSFNTFlavor(flavor, tbls))
}

// runeRange returns the lowest and highest BMP rune in the set.
func runeRange(used map[rune]struct{}) (first, last uint16) {
	first = 0xFFFF
	for r := range used {
		if r > 0xFFFF {
			continue
		}
		if uint16(r) < first {
			first = uint16(r)
		}
		if uint16(r) > last {
			last = uint16(r)
		}
	}
	if first > last {
		first, last = 0, 0
	}
	return
}

// sfntReadTables splits an sfnt (TrueType 1.0, 'true', or 'OTTO') into its
// tables. Lenient by design: PDF-embedded subsets routinely lack the tables
// parseTTF requires.
func sfntReadTables(data []byte) (flavor uint32, tbls map[string][]byte, err error) {
	if len(data) < 12 {
		return 0, nil, fmt.Errorf("sfnt: too small (%d bytes)", len(data))
	}
	flavor = binary.BigEndian.Uint32(data[0:4])
	if flavor != 0x00010000 && flavor != 0x74727565 && flavor != 0x4F54544F {
		return 0, nil, fmt.Errorf("sfnt: unsupported version 0x%08X", flavor)
	}
	numTables := int(binary.BigEndian.Uint16(data[4:6]))
	if len(data) < 12+numTables*16 {
		return 0, nil, fmt.Errorf("sfnt: truncated table directory")
	}
	tbls = make(map[string][]byte, numTables)
	for i := 0; i < numTables; i++ {
		rec := data[12+i*16:]
		tag := string(rec[0:4])
		off := int(binary.BigEndian.Uint32(rec[8:12]))
		length := int(binary.BigEndian.Uint32(rec[12:16]))
		if off < 0 || length < 0 || off+length > len(data) {
			return 0, nil, fmt.Errorf("sfnt: table %q out of bounds", tag)
		}
		tbls[tag] = data[off : off+length]
	}
	return flavor, tbls, nil
}

// buildCmapFormat4 synthesizes a cmap table with a single (3,1) Windows
// Unicode BMP format-4 subtable. Segments are split so that GIDs stay
// contiguous within each (pure idDelta encoding, no idRangeOffset arrays).
func buildCmapFormat4(m map[rune]uint16) []byte {
	runes := make([]int, 0, len(m))
	for r := range m {
		runes = append(runes, int(r))
	}
	sort.Ints(runes)

	type seg struct{ start, end, delta uint16 }
	var segs []seg
	for i := 0; i < len(runes); {
		start := runes[i]
		gid := m[rune(start)]
		j := i + 1
		for j < len(runes) && runes[j] == runes[j-1]+1 && m[rune(runes[j])] == m[rune(runes[j-1])]+1 &&
			runes[j] < 0xFFFF {
			j++
		}
		segs = append(segs, seg{uint16(start), uint16(runes[j-1]), gid - uint16(start)})
		i = j
	}
	segs = append(segs, seg{0xFFFF, 0xFFFF, 1}) // required terminator

	segCount := len(segs)
	pow2, exp := 1, 0
	for pow2*2 <= segCount {
		pow2 *= 2
		exp++
	}

	var sub []byte
	put16 := func(v uint16) { sub = append(sub, byte(v>>8), byte(v)) }
	put16(4) // format
	put16(uint16(16 + segCount*8))
	put16(0) // language
	put16(uint16(segCount * 2))
	put16(uint16(pow2 * 2))
	put16(uint16(exp))
	put16(uint16((segCount - pow2) * 2))
	for _, s := range segs {
		put16(s.end)
	}
	put16(0) // reservedPad
	for _, s := range segs {
		put16(s.start)
	}
	for _, s := range segs {
		put16(s.delta)
	}
	for range segs {
		put16(0) // idRangeOffset — pure delta segments
	}

	table := make([]byte, 12, 12+len(sub))
	binary.BigEndian.PutUint16(table[2:], 1)  // one encoding record
	binary.BigEndian.PutUint16(table[4:], 3)  // platform: Windows
	binary.BigEndian.PutUint16(table[6:], 1)  // encoding: Unicode BMP
	binary.BigEndian.PutUint32(table[8:], 12) // subtable offset
	return append(table, sub...)
}

// buildMinimalPost synthesizes a version 3.0 post table (no glyph names).
func buildMinimalPost(italic bool) []byte {
	b := make([]byte, 32)
	binary.BigEndian.PutUint32(b[0:], 0x00030000)
	if italic {
		angle := int32(-12 * 65536) // nominal italic angle, 16.16 fixed
		binary.BigEndian.PutUint32(b[4:], uint32(angle))
	}
	return b
}

// buildMinimalName synthesizes a format-0 name table with the Windows
// Unicode family/subfamily/full/PostScript records browsers expect.
func buildMinimalName(family string) []byte {
	type rec struct {
		id  uint16
		val string
	}
	recs := []rec{{1, family}, {2, "Regular"}, {4, family}, {6, family}}
	var storage []byte
	var dir []byte
	put16 := func(b []byte, off int, v uint16) { binary.BigEndian.PutUint16(b[off:], v) }
	for _, r := range recs {
		var utf16 []byte
		for _, c := range r.val {
			utf16 = append(utf16, byte(c>>8), byte(c))
		}
		e := make([]byte, 12)
		put16(e, 0, 3)      // platform: Windows
		put16(e, 2, 1)      // encoding: Unicode BMP
		put16(e, 4, 0x409)  // language: en-US
		put16(e, 6, r.id)   // name ID
		put16(e, 8, uint16(len(utf16)))
		put16(e, 10, uint16(len(storage)))
		dir = append(dir, e...)
		storage = append(storage, utf16...)
	}
	hdr := make([]byte, 6)
	put16(hdr, 2, uint16(len(recs)))
	put16(hdr, 4, uint16(6+len(dir)))
	return append(append(hdr, dir...), storage...)
}

// buildMinimalOS2 synthesizes a version-4 OS/2 table with metrics derived
// from hhea and weight/style from the font descriptor flags.
func buildMinimalOS2(bold, italic bool, hhea []byte, firstChar, lastChar uint16) []byte {
	var asc, desc int16 = 750, -250
	if len(hhea) >= 8 {
		asc = int16(binary.BigEndian.Uint16(hhea[4:6]))
		desc = int16(binary.BigEndian.Uint16(hhea[6:8]))
	}
	b := make([]byte, 96)
	put16 := func(off int, v uint16) { binary.BigEndian.PutUint16(b[off:], v) }
	put16(0, 4) // version
	weight := uint16(400)
	fsSelection := uint16(0x40) // REGULAR
	if bold {
		weight = 700
		fsSelection = 0x20
	}
	if italic {
		fsSelection = (fsSelection &^ 0x40) | 0x01
	}
	put16(4, weight)
	put16(6, 5) // usWidthClass: medium
	copy(b[58:62], "PDFG")
	put16(62, fsSelection)
	put16(64, firstChar)
	put16(66, lastChar)
	put16(68, uint16(asc))
	put16(70, uint16(desc))
	winDesc := -desc
	if winDesc < 0 {
		winDesc = 0
	}
	put16(74, uint16(asc))     // usWinAscent
	put16(76, uint16(winDesc)) // usWinDescent
	binary.BigEndian.PutUint32(b[78:], 1) // ulCodePageRange1: Latin 1
	put16(92, 0x20) // usBreakChar: space
	return b
}

// woffEncode wraps an assembled sfnt as WOFF1 (W3C WOFF 1.0): 44-byte
// header, 20-byte-per-table directory in the sfnt's tag order, and each
// table zlib-compressed (kept raw when compression does not shrink it).
func woffEncode(sfnt []byte) ([]byte, error) {
	if len(sfnt) < 12 {
		return nil, fmt.Errorf("woff: sfnt too small")
	}
	flavor := binary.BigEndian.Uint32(sfnt[0:4])
	numTables := int(binary.BigEndian.Uint16(sfnt[4:6]))
	if len(sfnt) < 12+numTables*16 {
		return nil, fmt.Errorf("woff: truncated sfnt directory")
	}

	type entry struct {
		tag      []byte
		checksum uint32
		orig     []byte
		comp     []byte
	}
	entries := make([]entry, numTables)
	totalSfntSize := uint32(12 + numTables*16)
	for i := 0; i < numTables; i++ {
		rec := sfnt[12+i*16:]
		off := int(binary.BigEndian.Uint32(rec[8:12]))
		length := int(binary.BigEndian.Uint32(rec[12:16]))
		if off < 0 || length < 0 || off+length > len(sfnt) {
			return nil, fmt.Errorf("woff: table %q out of bounds", string(rec[0:4]))
		}
		orig := sfnt[off : off+length]
		var zbuf bytes.Buffer
		zw, _ := zlib.NewWriterLevel(&zbuf, zlib.BestCompression)
		if _, err := zw.Write(orig); err != nil {
			return nil, err
		}
		if err := zw.Close(); err != nil {
			return nil, err
		}
		comp := zbuf.Bytes()
		if len(comp) >= len(orig) {
			comp = orig
		}
		entries[i] = entry{
			tag:      append([]byte(nil), rec[0:4]...),
			checksum: binary.BigEndian.Uint32(rec[4:8]),
			orig:     orig,
			comp:     comp,
		}
		totalSfntSize += uint32((length + 3) &^ 3)
	}

	dirSize := 44 + numTables*20
	var body []byte
	offsets := make([]uint32, numTables)
	for i, e := range entries {
		offsets[i] = uint32(dirSize + len(body))
		body = append(body, e.comp...)
		for len(body)%4 != 0 {
			body = append(body, 0)
		}
	}

	out := make([]byte, 44, dirSize+len(body))
	binary.BigEndian.PutUint32(out[0:], 0x774F4646) // 'wOFF'
	binary.BigEndian.PutUint32(out[4:], flavor)
	binary.BigEndian.PutUint32(out[8:], uint32(dirSize+len(body)))
	binary.BigEndian.PutUint16(out[12:], uint16(numTables))
	binary.BigEndian.PutUint32(out[16:], totalSfntSize)
	binary.BigEndian.PutUint16(out[20:], 1) // majorVersion
	// minorVersion, meta*, priv* stay 0.
	for i, e := range entries {
		rec := make([]byte, 20)
		copy(rec[0:4], e.tag)
		binary.BigEndian.PutUint32(rec[4:], offsets[i])
		binary.BigEndian.PutUint32(rec[8:], uint32(len(e.comp)))
		binary.BigEndian.PutUint32(rec[12:], uint32(len(e.orig)))
		binary.BigEndian.PutUint32(rec[16:], e.checksum)
		out = append(out, rec...)
	}
	return append(out, body...), nil
}
