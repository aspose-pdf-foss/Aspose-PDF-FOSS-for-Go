// SPDX-License-Identifier: MIT

package asposepdf

import (
	"encoding/binary"
	"fmt"
)

// ttfFont holds the parsed fields required for PDF embedding and text measurement.
type ttfFont struct {
	data []byte // raw TTF bytes (written verbatim into /FontFile2)

	// tables is this font's table directory with file-absolute offsets. For a
	// TTC sub-font the directory does not start at byte 12, so glyph reading must
	// use this captured map rather than re-deriving it from the file head.
	tables map[string]tableRecord

	// From head.
	unitsPerEm uint16
	xMin, yMin int16
	xMax, yMax int16

	// From hhea.
	ascent, descent     int16
	numOfLongHorMetrics uint16

	// From maxp.
	numGlyphs uint16

	// From hmtx.
	glyphWidths []uint16 // advanceWidth per glyphID (FUnits)

	// From cmap.
	runeToGlyph map[rune]uint16

	// From OS/2.
	capHeight   int16
	weight      uint16
	flagsBold   bool
	flagsItalic bool

	// From post.
	italicAngle  float64
	isFixedPitch bool

	// From name.
	postScriptName string
	family         string // name ID 1 (font family), e.g. "Arial"
	subfamily      string // name ID 2 (subfamily/style), e.g. "Bold Italic"
}

// tableRecord is an entry in the TTF table directory.
type tableRecord struct {
	offset uint32
	length uint32
}

// parseTTF parses a single TrueType font file and returns the ttfFont ready for
// embedding. Only the tables required for CIDFontType2 / Type0 embedding are
// read. A TrueType Collection ('ttcf') is rejected here — a collection cannot be
// embedded as a single /FontFile2; the font repository parses collections for
// rendering via parseFontCollection instead.
func parseTTF(data []byte) (*ttfFont, error) {
	return parseSFNTAt(data, 0)
}

// parseFontCollection parses every TrueType-outline sub-font of a file. A plain
// single font yields one element; a TrueType Collection ('ttcf', e.g. macOS
// Helvetica.ttc or Windows cambria.ttc) yields one element per sub-font.
// OpenType/CFF sub-fonts (no glyf outlines) fail the sfnt check and are skipped.
func parseFontCollection(data []byte) ([]*ttfFont, error) {
	if len(data) < 12 {
		return nil, fmt.Errorf("parse ttf: file too small (%d bytes)", len(data))
	}
	if binary.BigEndian.Uint32(data[0:4]) != 0x74746366 { // not 'ttcf' → single font
		f, err := parseSFNTAt(data, 0)
		if err != nil {
			return nil, err
		}
		return []*ttfFont{f}, nil
	}
	numFonts := binary.BigEndian.Uint32(data[8:12])
	dirStart := 12
	if numFonts > 0xFFFF || len(data) < dirStart+int(numFonts)*4 {
		return nil, fmt.Errorf("parse ttc: bad offset table (%d fonts)", numFonts)
	}
	var fonts []*ttfFont
	for i := 0; i < int(numFonts); i++ {
		off := binary.BigEndian.Uint32(data[dirStart+i*4 : dirStart+i*4+4])
		if f, err := parseSFNTAt(data, off); err == nil {
			fonts = append(fonts, f) // CFF/OTTO sub-fonts error out and are skipped
		}
	}
	if len(fonts) == 0 {
		return nil, fmt.Errorf("parse ttc: no TrueType-outline sub-fonts")
	}
	return fonts, nil
}

// parseSFNTAt parses one sfnt (TrueType) font whose table directory starts at
// byte offset off within data. Table records carry file-absolute offsets, so
// the subtable parsers index into the whole slice — which is exactly what lets a
// TTC's shared tables resolve from any sub-font's directory.
func parseSFNTAt(data []byte, off uint32) (*ttfFont, error) {
	if uint32(len(data)) < off+12 {
		return nil, fmt.Errorf("parse ttf: sfnt offset %d out of range", off)
	}
	scaler := binary.BigEndian.Uint32(data[off : off+4])
	if scaler != 0x00010000 && scaler != 0x74727565 { // 'true'
		return nil, fmt.Errorf("parse ttf: not a TrueType file (scaler 0x%08X)", scaler)
	}

	numTables := int(binary.BigEndian.Uint16(data[off+4 : off+6]))
	dir := int(off) + 12
	if len(data) < dir+numTables*16 {
		return nil, fmt.Errorf("parse ttf: truncated table directory")
	}
	tables := make(map[string]tableRecord, numTables)
	for i := 0; i < numTables; i++ {
		rec := dir + i*16
		tag := string(data[rec : rec+4])
		tables[tag] = tableRecord{
			offset: binary.BigEndian.Uint32(data[rec+8 : rec+12]),
			length: binary.BigEndian.Uint32(data[rec+12 : rec+16]),
		}
	}

	// Tables needed for glyph metrics and outlines. cmap/OS-2/post/name are
	// optional: a subsetted CIDFontType2 embedded for rendering (see
	// font_subset.go) drops them — its glyphs are selected via /CIDToGIDMap, not
	// a cmap — so requiring them would make embedded subset fonts unparseable.
	required := []string{"head", "hhea", "hmtx", "maxp"}
	for _, tag := range required {
		if _, ok := tables[tag]; !ok {
			return nil, fmt.Errorf("parse ttf: missing required table %q", tag)
		}
	}

	f := &ttfFont{data: data, tables: tables}

	if err := parseHead(f, tables); err != nil {
		return nil, err
	}
	if err := parseHhea(f, tables); err != nil {
		return nil, err
	}
	if err := parseMaxp(f, tables); err != nil {
		return nil, err
	}
	if err := parseHmtx(f, tables); err != nil {
		return nil, err
	}
	// Optional tables — parse only when present.
	if _, ok := tables["cmap"]; ok {
		if err := parseCmap(f, tables); err != nil {
			return nil, err
		}
	}
	if _, ok := tables["OS/2"]; ok {
		if err := parseOS2(f, tables); err != nil {
			return nil, err
		}
	}
	if _, ok := tables["post"]; ok {
		if err := parsePost(f, tables); err != nil {
			return nil, err
		}
	}
	if _, ok := tables["name"]; ok {
		if err := parseName(f, tables); err != nil {
			return nil, err
		}
	}

	return f, nil
}

// tableSlice returns the bytes of the named table or nil if absent.
func tableSlice(data []byte, tables map[string]tableRecord, tag string) []byte {
	t, ok := tables[tag]
	if !ok {
		return nil
	}
	end := t.offset + t.length
	if end > uint32(len(data)) {
		return nil
	}
	return data[t.offset:end]
}

func parseHead(f *ttfFont, tables map[string]tableRecord) error {
	b := tableSlice(f.data, tables, "head")
	if len(b) < 54 {
		return fmt.Errorf("parse ttf head: too small")
	}
	f.unitsPerEm = binary.BigEndian.Uint16(b[18:20])
	if f.unitsPerEm == 0 {
		return fmt.Errorf("parse ttf head: unitsPerEm is zero")
	}
	f.xMin = int16(binary.BigEndian.Uint16(b[36:38]))
	f.yMin = int16(binary.BigEndian.Uint16(b[38:40]))
	f.xMax = int16(binary.BigEndian.Uint16(b[40:42]))
	f.yMax = int16(binary.BigEndian.Uint16(b[42:44]))
	return nil
}

func parseHhea(f *ttfFont, tables map[string]tableRecord) error {
	b := tableSlice(f.data, tables, "hhea")
	if len(b) < 36 {
		return fmt.Errorf("parse ttf hhea: too small")
	}
	f.ascent = int16(binary.BigEndian.Uint16(b[4:6]))
	f.descent = int16(binary.BigEndian.Uint16(b[6:8]))
	f.numOfLongHorMetrics = binary.BigEndian.Uint16(b[34:36])
	return nil
}

func parseMaxp(f *ttfFont, tables map[string]tableRecord) error {
	b := tableSlice(f.data, tables, "maxp")
	if len(b) < 6 {
		return fmt.Errorf("parse ttf maxp: too small")
	}
	f.numGlyphs = binary.BigEndian.Uint16(b[4:6])
	return nil
}

func parseHmtx(f *ttfFont, tables map[string]tableRecord) error {
	b := tableSlice(f.data, tables, "hmtx")
	if f.numGlyphs == 0 {
		return fmt.Errorf("parse ttf hmtx: numGlyphs is zero")
	}
	if f.numOfLongHorMetrics == 0 {
		return fmt.Errorf("parse ttf hmtx: numOfLongHorMetrics is zero")
	}
	if f.numOfLongHorMetrics > f.numGlyphs {
		return fmt.Errorf("parse ttf hmtx: numOfLongHorMetrics (%d) exceeds numGlyphs (%d)", f.numOfLongHorMetrics, f.numGlyphs)
	}
	// The hmtx table has numOfLongHorMetrics 4-byte records (advanceWidth uint16, lsb int16),
	// followed by (numGlyphs - numOfLongHorMetrics) 2-byte records (lsb only); the missing
	// advanceWidth inherits the advanceWidth of the last long record.
	if len(b) < int(f.numOfLongHorMetrics)*4 {
		return fmt.Errorf("parse ttf hmtx: too small")
	}
	widths := make([]uint16, f.numGlyphs)
	var lastAdvance uint16
	for i := uint16(0); i < f.numOfLongHorMetrics; i++ {
		off := int(i) * 4
		w := binary.BigEndian.Uint16(b[off : off+2])
		widths[i] = w
		lastAdvance = w
	}
	for i := f.numOfLongHorMetrics; i < f.numGlyphs; i++ {
		widths[i] = lastAdvance
	}
	f.glyphWidths = widths
	return nil
}

func parseCmap(f *ttfFont, tables map[string]tableRecord) error {
	b := tableSlice(f.data, tables, "cmap")
	if len(b) < 4 {
		return fmt.Errorf("parse ttf cmap: too small")
	}
	numSubtables := int(binary.BigEndian.Uint16(b[2:4]))
	if len(b) < 4+numSubtables*8 {
		return fmt.Errorf("parse ttf cmap: truncated subtable index")
	}

	// Rank candidates: prefer format 12 (full Unicode) > format 4 (BMP only);
	// within a format, prefer Unicode platform (0) > Microsoft platform (3).
	type cand struct {
		priority int
		format   uint16
		offset   uint32
	}
	var best *cand

	for i := 0; i < numSubtables; i++ {
		off := 4 + i*8
		platformID := binary.BigEndian.Uint16(b[off : off+2])
		encodingID := binary.BigEndian.Uint16(b[off+2 : off+4])
		subOffset := binary.BigEndian.Uint32(b[off+4 : off+8])
		if int(subOffset)+4 > len(b) {
			continue
		}
		format := binary.BigEndian.Uint16(b[subOffset : subOffset+2])

		// Skip subtables we can't parse.
		if format != 4 && format != 12 {
			continue
		}

		var pri int
		switch {
		case format == 12 && platformID == 0:
			pri = 1000
		case format == 12 && platformID == 3 && encodingID == 10:
			pri = 900
		case format == 4 && platformID == 0:
			pri = 500
		case format == 4 && platformID == 3 && encodingID == 1:
			pri = 400
		default:
			continue
		}
		if best == nil || pri > best.priority {
			c := cand{priority: pri, format: format, offset: subOffset}
			best = &c
		}
	}
	if best == nil {
		return fmt.Errorf("parse ttf cmap: no supported subtable (need format 4 or 12)")
	}

	m := make(map[rune]uint16)
	switch best.format {
	case 4:
		if err := parseCmapFormat4(b[best.offset:], m); err != nil {
			return fmt.Errorf("parse ttf cmap format 4: %w", err)
		}
	case 12:
		if err := parseCmapFormat12(b[best.offset:], m); err != nil {
			return fmt.Errorf("parse ttf cmap format 12: %w", err)
		}
	}
	f.runeToGlyph = m
	return nil
}

// parseCmapFormat4 handles segmented BMP coverage (Unicode code points <= U+FFFF).
func parseCmapFormat4(b []byte, m map[rune]uint16) error {
	if len(b) < 14 {
		return fmt.Errorf("too small")
	}
	segCountX2 := int(binary.BigEndian.Uint16(b[6:8]))
	segCount := segCountX2 / 2
	if segCount == 0 {
		return nil
	}
	// Layout after the 14-byte header:
	//   endCode[segCount] (uint16)
	//   reservedPad uint16
	//   startCode[segCount] uint16
	//   idDelta[segCount] int16
	//   idRangeOffset[segCount] uint16
	//   glyphIdArray[...] uint16 (remainder)
	needed := 14 + 8*segCount + 2
	if len(b) < needed {
		return fmt.Errorf("truncated")
	}
	endOff := 14
	startOff := endOff + 2*segCount + 2 // skip endCode + reservedPad
	deltaOff := startOff + 2*segCount
	rangeOff := deltaOff + 2*segCount

	for i := 0; i < segCount; i++ {
		endCode := binary.BigEndian.Uint16(b[endOff+2*i : endOff+2*i+2])
		startCode := binary.BigEndian.Uint16(b[startOff+2*i : startOff+2*i+2])
		idDelta := int16(binary.BigEndian.Uint16(b[deltaOff+2*i : deltaOff+2*i+2]))
		idRangeOffsetPos := rangeOff + 2*i
		idRangeOffset := binary.BigEndian.Uint16(b[idRangeOffsetPos : idRangeOffsetPos+2])

		for c := uint32(startCode); c <= uint32(endCode); c++ {
			var gid uint16
			if idRangeOffset == 0 {
				gid = uint16(int32(c) + int32(idDelta))
			} else {
				off := int(idRangeOffsetPos) + int(idRangeOffset) + int(c-uint32(startCode))*2
				if off+2 > len(b) {
					continue
				}
				val := binary.BigEndian.Uint16(b[off : off+2])
				if val != 0 {
					gid = uint16(int32(val) + int32(idDelta))
				}
			}
			if gid != 0 && c <= 0x10FFFF {
				m[rune(c)] = gid
			}
		}
	}
	return nil
}

// parseCmapFormat12 handles segmented coverage including supplementary planes.
func parseCmapFormat12(b []byte, m map[rune]uint16) error {
	if len(b) < 16 {
		return fmt.Errorf("too small")
	}
	numGroups := binary.BigEndian.Uint32(b[12:16])
	if len(b) < 16+int(numGroups)*12 {
		return fmt.Errorf("truncated")
	}
	for i := uint32(0); i < numGroups; i++ {
		off := 16 + int(i)*12
		startChar := binary.BigEndian.Uint32(b[off : off+4])
		endChar := binary.BigEndian.Uint32(b[off+4 : off+8])
		startGlyphID := binary.BigEndian.Uint32(b[off+8 : off+12])
		for c := startChar; c <= endChar && c <= 0x10FFFF; c++ {
			gid := startGlyphID + (c - startChar)
			if gid < 0x10000 {
				m[rune(c)] = uint16(gid)
			}
		}
	}
	return nil
}

// glyphID returns the glyph index for r, or 0 (.notdef) if unmapped.
func (f *ttfFont) glyphID(r rune) uint16 {
	return f.runeToGlyph[r]
}

func parseOS2(f *ttfFont, tables map[string]tableRecord) error {
	b := tableSlice(f.data, tables, "OS/2")
	if len(b) < 78 {
		return fmt.Errorf("parse ttf OS/2: too small")
	}
	f.weight = binary.BigEndian.Uint16(b[4:6])
	fsSelection := binary.BigEndian.Uint16(b[62:64])
	f.flagsItalic = fsSelection&0x01 != 0
	f.flagsBold = fsSelection&0x20 != 0
	// sCapHeight is at offset 88 in OS/2 version 2+. Version 0/1 omits it;
	// fall back to ~70% of ascent (PDF FontDescriptor viewers use this for layout).
	if len(b) >= 90 {
		f.capHeight = int16(binary.BigEndian.Uint16(b[88:90]))
	}
	if f.capHeight == 0 {
		f.capHeight = f.ascent * 7 / 10
	}
	return nil
}

func parsePost(f *ttfFont, tables map[string]tableRecord) error {
	b := tableSlice(f.data, tables, "post")
	if len(b) < 32 {
		return fmt.Errorf("parse ttf post: too small")
	}
	// italicAngle is a Fixed (signed 16.16 fraction) at offset 4.
	raw := int32(binary.BigEndian.Uint32(b[4:8]))
	f.italicAngle = float64(raw) / 65536.0
	// isFixedPitch is a uint32 at offset 12.
	f.isFixedPitch = binary.BigEndian.Uint32(b[12:16]) != 0
	return nil
}

// parseName extracts the PostScript name (nameID 6). Falls back to Full Name
// (nameID 4) with spaces replaced by dashes if nameID 6 is absent.
func parseName(f *ttfFont, tables map[string]tableRecord) error {
	b := tableSlice(f.data, tables, "name")
	if len(b) < 6 {
		return fmt.Errorf("parse ttf name: too small")
	}
	count := int(binary.BigEndian.Uint16(b[2:4]))
	storageOffset := int(binary.BigEndian.Uint16(b[4:6]))
	if len(b) < 6+count*12 {
		return fmt.Errorf("parse ttf name: truncated record array")
	}

	var psName, fullName string
	for i := 0; i < count; i++ {
		rec := b[6+i*12:]
		platformID := binary.BigEndian.Uint16(rec[0:2])
		encodingID := binary.BigEndian.Uint16(rec[2:4])
		nameID := binary.BigEndian.Uint16(rec[6:8])
		length := int(binary.BigEndian.Uint16(rec[8:10]))
		offset := int(binary.BigEndian.Uint16(rec[10:12]))

		if nameID != 1 && nameID != 2 && nameID != 4 && nameID != 6 {
			continue
		}
		start := storageOffset + offset
		end := start + length
		if end > len(b) {
			continue
		}
		raw := b[start:end]

		var decoded string
		switch {
		case platformID == 3 && encodingID == 1: // Microsoft Unicode BMP (UTF-16BE)
			decoded = decodeUTF16BE(raw)
		case platformID == 0: // Unicode (UTF-16BE)
			decoded = decodeUTF16BE(raw)
		case platformID == 1 && encodingID == 0: // Mac Roman (ASCII-safe subset)
			decoded = string(raw)
		default:
			continue
		}

		if nameID == 6 && psName == "" {
			psName = decoded
		}
		if nameID == 4 && fullName == "" {
			fullName = decoded
		}
		if nameID == 1 && f.family == "" {
			f.family = decoded
		}
		if nameID == 2 && f.subfamily == "" {
			f.subfamily = decoded
		}
	}

	if psName == "" && fullName == "" {
		return fmt.Errorf("parse ttf name: no PostScript name or Full Name found")
	}
	if psName == "" {
		// Fallback: replace spaces with dashes in Full Name.
		psName = ""
		for _, r := range fullName {
			if r == ' ' {
				psName += "-"
			} else {
				psName += string(r)
			}
		}
	}
	f.postScriptName = psName
	return nil
}

// decodeUTF16BE decodes a UTF-16BE byte sequence to a Go string.
// Invalid bytes yield U+FFFD.
func decodeUTF16BE(b []byte) string {
	if len(b)%2 != 0 {
		// Trim trailing odd byte.
		b = b[:len(b)-1]
	}
	runes := make([]rune, 0, len(b)/2)
	for i := 0; i+1 < len(b); i += 2 {
		u := uint32(b[i])<<8 | uint32(b[i+1])
		// Surrogate pair handling.
		if u >= 0xD800 && u <= 0xDBFF && i+3 < len(b) {
			low := uint32(b[i+2])<<8 | uint32(b[i+3])
			if low >= 0xDC00 && low <= 0xDFFF {
				cp := 0x10000 + ((u - 0xD800) << 10) + (low - 0xDC00)
				runes = append(runes, rune(cp))
				i += 2
				continue
			}
		}
		runes = append(runes, rune(u))
	}
	return string(runes)
}
