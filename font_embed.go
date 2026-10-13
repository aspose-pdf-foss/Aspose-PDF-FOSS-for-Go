// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"sort"
	"strings"
)

// buildFontFile2Stream creates a /FontFile2 stream with the raw TTF bytes,
// compressed via FlateDecode. /Length1 holds the uncompressed length.
func buildFontFile2Stream(f *ttfFont) *pdfStream {
	return buildFontFile2StreamBytes(f.data)
}

// buildFontFile2StreamBytes wraps an arbitrary (full or subset) TTF
// program as a FlateDecode-compressed /FontFile2 stream.
func buildFontFile2StreamBytes(program []byte) *pdfStream {
	var buf bytes.Buffer
	zw := zlib.NewWriter(&buf)
	_, _ = zw.Write(program)
	_ = zw.Close()
	return &pdfStream{
		Dict: pdfDict{
			"/Length1": len(program),
			"/Filter":  pdfName("/FlateDecode"),
		},
		Data: buf.Bytes(),
	}
}

// buildFlateStream wraps raw bytes as a FlateDecode-compressed stream
// (used for the /CIDToGIDMap stream produced by subsetting).
func buildFlateStream(data []byte) *pdfStream {
	var buf bytes.Buffer
	zw := zlib.NewWriter(&buf)
	_, _ = zw.Write(data)
	_ = zw.Close()
	return &pdfStream{
		Dict: pdfDict{"/Filter": pdfName("/FlateDecode")},
		Data: buf.Bytes(),
	}
}

// buildFontFile3Stream wraps the full OpenType (sfnt/OTTO) program of a
// CFF-based font as a FlateDecode-compressed /FontFile3 stream with
// /Subtype /OpenType (ISO 32000-1 §9.9 Table 126). The whole font file is
// embedded (cmap included), which is the most broadly compatible way to
// embed an OpenType-CFF font into a CIDFontType0 descendant.
func buildFontFile3Stream(f *ttfFont) *pdfStream {
	var buf bytes.Buffer
	zw := zlib.NewWriter(&buf)
	_, _ = zw.Write(f.data)
	_ = zw.Close()
	return &pdfStream{
		Dict: pdfDict{
			"/Subtype": pdfName("/OpenType"),
			"/Filter":  pdfName("/FlateDecode"),
		},
		Data: buf.Bytes(),
	}
}

// buildFontDescriptor creates a /FontDescriptor dict referencing the embedded
// font program under fontFileKey (/FontFile2 for TrueType, /FontFile3 for
// OpenType-CFF). symbolic selects the Symbolic (true) or Nonsymbolic (false)
// flag — see the two bullets below; callers pass true for every composite
// (Type0/Identity-H) embedding this function serves, where no /Encoding
// dict exists on the font to conflict with either flag, and for a genuinely
// symbolic simple font (Symbol, ZapfDingbats) that has no /Encoding either.
// pdfa_convert.go's buildEmbeddedSimpleFont passes false for a Standard-14
// Latin substitute, which does carry an /Encoding — see its own comment.
func buildFontDescriptor(f *ttfFont, fontFileKey string, fontFileID int, symbolic bool) pdfDict {
	scale := func(v int16) int {
		// Scale FUnits to 1/1000 em.
		return int(float64(v) * 1000.0 / float64(f.unitsPerEm))
	}
	// Flags (PDF spec Table 123; ISO 32000-1 §9.8.2):
	//   bit 1 (1):       FixedPitch
	//   bit 3 (4):       Symbolic
	//   bit 6 (32):      Nonsymbolic
	//   bit 7 (64):      Italic
	//   bit 19 (262144): ForceBold
	// Symbolic and Nonsymbolic are mutually exclusive (exactly one must be
	// set, ISO 32000-1 Table 123) and govern how a simple TrueType font's
	// codes map to glyphs (§9.6.6.2): a Nonsymbolic font is expected to
	// carry /Encoding (WinAnsi/MacRoman/Differences) and look glyphs up by
	// name through the font's (3,1) Microsoft-Unicode or (1,0) Mac-Roman
	// cmap; a Symbolic font is expected to have NO /Encoding and look codes
	// up directly through its (3,0) symbol or (1,0) cmap. A simple TrueType
	// font that carries /Encoding but is flagged Symbolic violates PDF/A
	// (veraPDF 6.2.11.4.1-2 / 6.2.11.6-3/-4) — GitHub issue #12.
	flags := 0
	if symbolic {
		flags |= 0x4
	} else {
		flags |= 0x20
	}
	if f.isFixedPitch {
		flags |= 0x1
	}
	if f.flagsItalic {
		flags |= 0x40
	}
	if f.flagsBold {
		flags |= 0x40000
	}
	// StemV heuristic: 50 at weight 400, +0.2 per weight unit.
	stemV := 50
	if f.weight > 0 {
		stemV = 50 + int(float64(f.weight-400)*0.2)
		if stemV < 50 {
			stemV = 50
		}
	}
	cap := scale(f.capHeight)
	if cap == 0 {
		cap = scale(f.ascent)
	}
	return pdfDict{
		"/Type":     pdfName("/FontDescriptor"),
		"/FontName": pdfName("/" + f.postScriptName),
		"/Flags":    flags,
		"/FontBBox": pdfArray{
			scale(f.xMin), scale(f.yMin),
			scale(f.xMax), scale(f.yMax),
		},
		"/ItalicAngle": f.italicAngle,
		"/Ascent":      scale(f.ascent),
		"/Descent":     scale(f.descent),
		"/CapHeight":   cap,
		"/StemV":       stemV,
		fontFileKey:    pdfRef{Num: fontFileID},
	}
}

// addObject appends a new PDF object to the document and returns its ID.
func (d *Document) addObject(value pdfValue) int {
	id := d.nextID
	d.nextID++
	d.objects[id] = &pdfObject{Num: id, Value: value}
	return id
}

// defaultCIDWidth is the /DW value written for embedded TTFs.
const defaultCIDWidth = 500

// buildWArray builds the /W array for a CIDFontType2 dict.
// Widths equal to defaultCIDWidth are omitted (covered by /DW).
// Runs of identical non-default widths > 5 consecutive glyphs are emitted as
// `cFirst cLast w`; all other non-default widths are grouped in the array form
// `cStart [w1 w2 w3 ...]`.
func buildWArray(f *ttfFont) pdfArray {
	// Scale each glyph's advance to 1/1000 em (rounded).
	widths := make([]int, len(f.glyphWidths))
	for i, w := range f.glyphWidths {
		widths[i] = int(float64(w)*1000.0/float64(f.unitsPerEm) + 0.5)
	}

	var arr pdfArray
	i := 0
	for i < len(widths) {
		if widths[i] == defaultCIDWidth {
			i++
			continue
		}
		j := i + 1
		for j < len(widths) && widths[j] == widths[i] {
			j++
		}
		runLen := j - i
		if runLen > 5 {
			arr = append(arr, i, j-1, widths[i])
			i = j
			continue
		}
		k := i + 1
		for k < len(widths) && widths[k] != defaultCIDWidth {
			lookEnd := k + 1
			for lookEnd < len(widths) && widths[lookEnd] == widths[k] {
				lookEnd++
			}
			if lookEnd-k > 5 {
				break
			}
			k = lookEnd
		}
		seq := make(pdfArray, 0, k-i)
		for g := i; g < k; g++ {
			seq = append(seq, widths[g])
		}
		arr = append(arr, i, seq)
		i = k
	}
	return arr
}

// toUnicodeEntry is one bfchar mapping: a glyph id and the UTF-16BE hex of
// the characters it stands for.
type toUnicodeEntry struct {
	gid uint16
	hex string
}

// buildToUnicodeCMap generates the /ToUnicode CMap stream for the font: one
// bfchar entry per glyph the cmap names — its lowest character, so a glyph
// shared by U+0020 and U+00A0 reads back as a plain space — with the glyphs
// shaping substituted (extra — ligatures and contextual forms, possibly
// several characters each) mapping to the text they were drawn for.
func buildToUnicodeCMap(f *ttfFont, extra map[uint16]string) *pdfStream {
	return writeToUnicodeCMap(toUnicodeEntries(f, nil, extra))
}

// toUnicodeEntries builds one entry per glyph (restricted to used when it
// is non-nil): shaped text first, else the lowest character the cmap maps
// to the glyph.
func toUnicodeEntries(f *ttfFont, used map[uint16]bool, extra map[uint16]string) []toUnicodeEntry {
	lowest := make(map[uint16]rune, len(f.runeToGlyph))
	for r, gid := range f.runeToGlyph {
		if used != nil && !used[gid] {
			continue
		}
		if prev, seen := lowest[gid]; !seen || r < prev {
			lowest[gid] = r
		}
	}
	entries := make([]toUnicodeEntry, 0, len(lowest)+len(extra))
	for gid, text := range extra {
		if used == nil || used[gid] {
			entries = append(entries, toUnicodeEntry{gid, stringToUTF16BEHex(text)})
		}
	}
	for gid, r := range lowest {
		if _, shaped := extra[gid]; !shaped {
			entries = append(entries, toUnicodeEntry{gid, runeToUTF16BEHex(r)})
		}
	}
	return entries
}

// writeToUnicodeCMap serializes bfchar entries (sorted by glyph, then
// text, so the stream is deterministic) into a /ToUnicode CMap stream.
func writeToUnicodeCMap(entries []toUnicodeEntry) *pdfStream {
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].gid != entries[j].gid {
			return entries[i].gid < entries[j].gid
		}
		return entries[i].hex < entries[j].hex
	})

	var buf bytes.Buffer
	buf.WriteString("/CIDInit /ProcSet findresource begin\n")
	buf.WriteString("12 dict begin\n")
	buf.WriteString("begincmap\n")
	buf.WriteString("/CIDSystemInfo << /Registry (Adobe) /Ordering (UCS) /Supplement 0 >> def\n")
	buf.WriteString("/CMapName /Adobe-Identity-UCS def\n")
	buf.WriteString("/CMapType 2 def\n")
	buf.WriteString("1 begincodespacerange <0000> <FFFF> endcodespacerange\n")

	for start := 0; start < len(entries); start += 100 {
		end := start + 100
		if end > len(entries) {
			end = len(entries)
		}
		fmt.Fprintf(&buf, "%d beginbfchar\n", end-start)
		for _, e := range entries[start:end] {
			fmt.Fprintf(&buf, "<%04X> <%s>\n", e.gid, e.hex)
		}
		buf.WriteString("endbfchar\n")
	}
	buf.WriteString("endcmap\n")
	buf.WriteString("CMapName currentdict /CMap defineresource pop\n")
	buf.WriteString("end\nend\n")

	return &pdfStream{
		Dict:    pdfDict{},
		Data:    buf.Bytes(),
		Decoded: true,
	}
}

// refreshShapedText folds the glyph names shaping recorded (see
// embeddedFont.mapGlyphText) into each font's /ToUnicode before the document
// is serialized, rebuilding the stream in place so the Type0 font's
// reference stays valid.
func (d *Document) refreshShapedText() {
	for _, ef := range d.embeddedFonts {
		if ef == nil || !ef.textDirty {
			continue
		}
		if type0, ok := d.objects[ef.fontObjectID].Value.(pdfDict); ok {
			if tuRef, ok := type0["/ToUnicode"].(pdfRef); ok {
				if obj, ok := d.objects[tuRef.Num]; ok {
					obj.Value = buildToUnicodeCMap(ef.ttf, ef.glyphText)
				}
			}
		}
		ef.textDirty = false
	}
}

// embedFont adds all required PDF objects for the embedded TTF to doc.objects
// and returns the object ID of the Type0 font dict.
func embedFont(d *Document, f *ttfFont) int {
	fontFile2ID := d.addObject(buildFontFile2Stream(f))
	descriptor := buildFontDescriptor(f, "/FontFile2", fontFile2ID, true)
	descriptorID := d.addObject(descriptor)

	cidDict := pdfDict{
		"/Type":     pdfName("/Font"),
		"/Subtype":  pdfName("/CIDFontType2"),
		"/BaseFont": pdfName("/" + f.postScriptName),
		"/CIDSystemInfo": pdfDict{
			"/Registry":   "Adobe",
			"/Ordering":   "Identity",
			"/Supplement": 0,
		},
		"/FontDescriptor": pdfRef{Num: descriptorID},
		"/CIDToGIDMap":    pdfName("/Identity"),
		"/W":              buildWArray(f),
		"/DW":             defaultCIDWidth,
	}
	cidID := d.addObject(cidDict)

	tuID := d.addObject(buildToUnicodeCMap(f, nil))

	type0 := pdfDict{
		"/Type":            pdfName("/Font"),
		"/Subtype":         pdfName("/Type0"),
		"/BaseFont":        pdfName("/" + f.postScriptName),
		"/Encoding":        pdfName("/Identity-H"),
		"/DescendantFonts": pdfArray{pdfRef{Num: cidID}},
		"/ToUnicode":       pdfRef{Num: tuID},
	}
	return d.addObject(type0)
}

// embedCFFFont embeds an OpenType-CFF (.otf) font as a Type0 composite font
// with a CIDFontType0 descendant and an OpenType /FontFile3, returning the
// object ID of the Type0 font dict. It parallels embedFont (the TrueType
// path) but differs in three places per ISO 32000-1 §9.7.4: the font program
// is /FontFile3 (Subtype OpenType), the descendant is /CIDFontType0, and there
// is no /CIDToGIDMap (for a non-CID-keyed CFF the CID is used directly as the
// glyph index, which matches our Identity-H glyph-ID emission). Caller has
// already rejected CID-keyed CFF.
func embedCFFFont(d *Document, f *ttfFont) int {
	fontFile3ID := d.addObject(buildFontFile3Stream(f))
	descriptor := buildFontDescriptor(f, "/FontFile3", fontFile3ID, true)
	descriptorID := d.addObject(descriptor)

	cidDict := pdfDict{
		"/Type":     pdfName("/Font"),
		"/Subtype":  pdfName("/CIDFontType0"),
		"/BaseFont": pdfName("/" + f.postScriptName),
		"/CIDSystemInfo": pdfDict{
			"/Registry":   "Adobe",
			"/Ordering":   "Identity",
			"/Supplement": 0,
		},
		"/FontDescriptor": pdfRef{Num: descriptorID},
		"/W":              buildWArray(f),
		"/DW":             defaultCIDWidth,
	}
	cidID := d.addObject(cidDict)

	tuID := d.addObject(buildToUnicodeCMap(f, nil))

	type0 := pdfDict{
		"/Type":            pdfName("/Font"),
		"/Subtype":         pdfName("/Type0"),
		"/BaseFont":        pdfName("/" + f.postScriptName),
		"/Encoding":        pdfName("/Identity-H"),
		"/DescendantFonts": pdfArray{pdfRef{Num: cidID}},
		"/ToUnicode":       pdfRef{Num: tuID},
	}
	return d.addObject(type0)
}

// SubsetFonts shrinks every embedded TTF (loaded via LoadFont) to only
// the glyphs actually drawn, replacing each /FontFile2 with a rebuilt
// subset program and switching the CIDFont's /CIDToGIDMap from /Identity
// to a stream that maps the original glyph IDs (still used verbatim as
// CIDs in content streams) to the compact subset glyph IDs. /W and
// /ToUnicode are also trimmed to the used glyphs.
//
// Call this once, AFTER all text has been added and just before Save /
// WriteTo: adding more text that introduces new glyphs after subsetting
// would reference glyphs the subset no longer contains. Fonts with no
// drawn glyphs are skipped. Returns the number of fonts subsetted.
//
// Typical reduction is from ~300-700 KB per embedded font down to a few
// KB, since only the used outlines survive. Mirrors the post-processing
// shape of (*Document).OptimizeImages.
func (d *Document) SubsetFonts() (int, error) {
	count := 0
	for _, ef := range d.embeddedFonts {
		if ef == nil || len(ef.usedGlyphs) == 0 {
			continue
		}
		// CFF (OpenType) subsetting is not supported; the whole /FontFile3
		// is kept. Only TrueType (glyf) fonts are subset here.
		if ef.ttf.cff != nil {
			continue
		}
		res, err := subsetTTF(ef.ttf, ef.usedGlyphs)
		if err != nil {
			return count, fmt.Errorf("subset %s: %w", ef.baseFont, err)
		}
		cidFont, fontFile2ID, ok := d.embeddedFontObjs(ef)
		if !ok {
			continue
		}

		// Replace the embedded font program.
		d.objects[fontFile2ID].Value = buildFontFile2StreamBytes(res.program)

		// /CIDToGIDMap stream (CID == original GID → subset GID).
		cidMapID := d.addObject(buildFlateStream(res.cidToGID))
		cidFont["/CIDToGIDMap"] = pdfRef{Num: cidMapID}

		// Trim /W to the used CIDs.
		cidFont["/W"] = buildSubsetWArray(ef.ttf, ef.usedGlyphs)

		// Trim /ToUnicode to the used glyphs, reusing the existing stream
		// object so the Type0 /ToUnicode ref stays valid.
		if type0, ok := d.objects[ef.fontObjectID].Value.(pdfDict); ok {
			if tuRef, ok := type0["/ToUnicode"].(pdfRef); ok {
				if obj, ok := d.objects[tuRef.Num]; ok {
					obj.Value = buildSubsetToUnicode(ef.ttf, ef.usedGlyphs, ef.glyphText)
				}
			}
		}
		ef.textDirty = false
		count++
	}
	return count, nil
}

// embeddedFontObjs resolves an embedded font's CIDFont dict and the
// object ID of its /FontFile2 stream by walking Type0 → DescendantFonts
// → FontDescriptor → FontFile2.
func (d *Document) embeddedFontObjs(ef *embeddedFont) (cidFont pdfDict, fontFile2ID int, ok bool) {
	type0, ok := d.objects[ef.fontObjectID].Value.(pdfDict)
	if !ok {
		return nil, 0, false
	}
	desc, ok := type0["/DescendantFonts"].(pdfArray)
	if !ok || len(desc) == 0 {
		return nil, 0, false
	}
	cidRef, ok := desc[0].(pdfRef)
	if !ok {
		return nil, 0, false
	}
	cidObj, ok := d.objects[cidRef.Num]
	if !ok {
		return nil, 0, false
	}
	cidFont, ok = cidObj.Value.(pdfDict)
	if !ok {
		return nil, 0, false
	}
	fdRef, ok := cidFont["/FontDescriptor"].(pdfRef)
	if !ok {
		return nil, 0, false
	}
	fdObj, ok := d.objects[fdRef.Num]
	if !ok {
		return nil, 0, false
	}
	fd, ok := fdObj.Value.(pdfDict)
	if !ok {
		return nil, 0, false
	}
	ff2Ref, ok := fd["/FontFile2"].(pdfRef)
	if !ok {
		return nil, 0, false
	}
	return cidFont, ff2Ref.Num, true
}

// sortedUsedGlyphs returns the used glyph IDs in ascending order.
func sortedUsedGlyphs(used map[uint16]bool) []uint16 {
	gids := make([]uint16, 0, len(used))
	for g := range used {
		gids = append(gids, g)
	}
	sort.Slice(gids, func(i, j int) bool { return gids[i] < gids[j] })
	return gids
}

// buildSubsetWArray emits a /W array with one "cid [width]" entry per
// used glyph (CIDs stay equal to the original glyph IDs). Compact for the
// sparse glyph sets a subset produces.
func buildSubsetWArray(f *ttfFont, used map[uint16]bool) pdfArray {
	var arr pdfArray
	for _, gid := range sortedUsedGlyphs(used) {
		if int(gid) >= len(f.glyphWidths) {
			continue
		}
		w := int(float64(f.glyphWidths[gid])*1000.0/float64(f.unitsPerEm) + 0.5)
		arr = append(arr, int(gid), pdfArray{w})
	}
	return arr
}

// buildSubsetToUnicode regenerates the /ToUnicode CMap for only the used
// glyphs. CIDs (== original glyph IDs) map back to their Unicode runes, and
// the used glyphs shaping produced map to their recorded text (extra).
func buildSubsetToUnicode(f *ttfFont, used map[uint16]bool, extra map[uint16]string) *pdfStream {
	if used == nil {
		used = map[uint16]bool{}
	}
	return writeToUnicodeCMap(toUnicodeEntries(f, used, extra))
}

// stringToUTF16BEHex renders s as big-endian UTF-16 in uppercase hex.
func stringToUTF16BEHex(s string) string {
	var b strings.Builder
	for _, r := range s {
		b.WriteString(runeToUTF16BEHex(r))
	}
	return b.String()
}

// runeToUTF16BEHex renders r as big-endian UTF-16 in uppercase hex, with
// surrogate pairs for supplementary characters (> U+FFFF).
func runeToUTF16BEHex(r rune) string {
	if r <= 0xFFFF {
		return fmt.Sprintf("%04X", uint16(r))
	}
	v := uint32(r) - 0x10000
	high := 0xD800 + (v >> 10)
	low := 0xDC00 + (v & 0x3FF)
	return fmt.Sprintf("%04X%04X", high, low)
}
