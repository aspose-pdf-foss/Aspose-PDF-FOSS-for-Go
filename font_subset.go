// SPDX-License-Identifier: MIT

package asposepdf

import (
	"encoding/binary"
	"fmt"
	"sort"
)

// TrueType composite-glyph component flags (ISO/IEC 14496-22 §5.3.4).
const (
	compArgsAreWords   = 0x0001
	compHaveScale      = 0x0008
	compMoreComponents = 0x0020
	compHaveXYScale    = 0x0040
	compHave2x2        = 0x0080
)

// subsetResult carries the rebuilt font program and the matching
// CIDToGIDMap. CIDs in content streams stay equal to the ORIGINAL glyph
// IDs (the encoders are unchanged), so cidToGID[oldGID] = newGID maps
// each emitted CID to its compact glyph index inside the subset.
type subsetResult struct {
	program  []byte // rebuilt sfnt font program for /FontFile2
	cidToGID []byte // /CIDToGIDMap stream bytes (2 bytes per CID, big-endian)
	oldSize  int    // original font program size
	newSize  int    // rebuilt font program size
}

// subsetTTF rebuilds f.data keeping only the glyphs in used (plus glyph 0
// and the transitive component glyphs of any composite in the set),
// renumbering them into a compact 0..N-1 range. The original glyph IDs
// are preserved as CIDs via the returned CIDToGIDMap, so no content
// stream needs rewriting. Hinting (cvt/fpgm/prep), cmap, OS/2, and name
// are dropped; a minimal post (3.0) is synthesised. Returns an error if
// the font lacks glyf/loca (e.g. a CFF/OpenType outline font).
func subsetTTF(f *ttfFont, used map[uint16]bool) (*subsetResult, error) {
	tables, err := readTableDirectory(f.data)
	if err != nil {
		return nil, err
	}
	glyf := tableSlice(f.data, tables, "glyf")
	if glyf == nil {
		return nil, fmt.Errorf("subset: font has no glyf table (not a TrueType outline font)")
	}
	loca, err := readLoca(f, tables)
	if err != nil {
		return nil, err
	}
	if len(loca) != int(f.numGlyphs)+1 {
		return nil, fmt.Errorf("subset: loca length %d != numGlyphs+1 %d", len(loca), f.numGlyphs+1)
	}

	// Build the keep-set: requested glyphs + glyph 0, expanded over the
	// component references of every composite glyph (transitive closure).
	keep := map[uint16]bool{0: true}
	for g := range used {
		if int(g) < int(f.numGlyphs) {
			keep[g] = true
		}
	}
	if err := expandComposites(glyf, loca, keep); err != nil {
		return nil, err
	}

	// Sorted old glyph IDs → new compact IDs.
	oldGIDs := make([]uint16, 0, len(keep))
	for g := range keep {
		oldGIDs = append(oldGIDs, g)
	}
	sort.Slice(oldGIDs, func(i, j int) bool { return oldGIDs[i] < oldGIDs[j] })
	oldToNew := make(map[uint16]uint16, len(oldGIDs))
	for newGID, oldGID := range oldGIDs {
		oldToNew[oldGID] = uint16(newGID)
	}
	n := len(oldGIDs)

	// Rebuild glyf + loca, remapping composite component glyph indices.
	newGlyf, newLoca := rebuildGlyf(glyf, loca, oldGIDs, oldToNew)

	// Rebuild dependent tables.
	newHead := patchHead(tableSlice(f.data, tables, "head"))
	newMaxp := patchMaxp(tableSlice(f.data, tables, "maxp"), uint16(n))
	newHhea := patchHhea(tableSlice(f.data, tables, "hhea"), uint16(n))
	newHmtx := buildHmtx(f, glyf, loca, oldGIDs)
	newLocaBytes := encodeLocaLong(newLoca)
	newPost := buildPost30(f)

	program := assembleSFNT(map[string][]byte{
		"head": newHead,
		"hhea": newHhea,
		"maxp": newMaxp,
		"hmtx": newHmtx,
		"loca": newLocaBytes,
		"glyf": newGlyf,
		"post": newPost,
	})

	// CIDToGIDMap: indexed by CID (== original glyph ID), 2 bytes each.
	maxOld := oldGIDs[len(oldGIDs)-1]
	cidToGID := make([]byte, (int(maxOld)+1)*2)
	for _, oldGID := range oldGIDs {
		binary.BigEndian.PutUint16(cidToGID[int(oldGID)*2:], oldToNew[oldGID])
	}

	return &subsetResult{
		program:  program,
		cidToGID: cidToGID,
		oldSize:  len(f.data),
		newSize:  len(program),
	}, nil
}

// readTableDirectory parses an sfnt table directory into tag→record.
func readTableDirectory(data []byte) (map[string]tableRecord, error) {
	if len(data) < 12 {
		return nil, fmt.Errorf("subset: font too small")
	}
	numTables := int(binary.BigEndian.Uint16(data[4:6]))
	if len(data) < 12+numTables*16 {
		return nil, fmt.Errorf("subset: truncated table directory")
	}
	tables := make(map[string]tableRecord, numTables)
	for i := 0; i < numTables; i++ {
		off := 12 + i*16
		tag := string(data[off : off+4])
		tables[tag] = tableRecord{
			offset: binary.BigEndian.Uint32(data[off+8 : off+12]),
			length: binary.BigEndian.Uint32(data[off+12 : off+16]),
		}
	}
	return tables, nil
}

// readLoca returns the glyph offsets array (len numGlyphs+1) honouring
// head.indexToLocFormat (0 = short/×2, 1 = long).
func readLoca(f *ttfFont, tables map[string]tableRecord) ([]uint32, error) {
	head := tableSlice(f.data, tables, "head")
	if len(head) < 52 {
		return nil, fmt.Errorf("subset: head table too small")
	}
	longFormat := int16(binary.BigEndian.Uint16(head[50:52])) != 0
	b := tableSlice(f.data, tables, "loca")
	if b == nil {
		return nil, fmt.Errorf("subset: font has no loca table")
	}
	count := int(f.numGlyphs) + 1
	out := make([]uint32, count)
	if longFormat {
		if len(b) < count*4 {
			return nil, fmt.Errorf("subset: loca (long) too small")
		}
		for i := 0; i < count; i++ {
			out[i] = binary.BigEndian.Uint32(b[i*4:])
		}
	} else {
		if len(b) < count*2 {
			return nil, fmt.Errorf("subset: loca (short) too small")
		}
		for i := 0; i < count; i++ {
			out[i] = uint32(binary.BigEndian.Uint16(b[i*2:])) * 2
		}
	}
	return out, nil
}

// glyphBytes returns the raw glyf bytes for glyph gid, or nil for an
// empty glyph (loca[gid] == loca[gid+1]).
func glyphBytes(glyf []byte, loca []uint32, gid uint16) []byte {
	start, end := loca[gid], loca[gid+1]
	if end <= start || int(end) > len(glyf) {
		return nil
	}
	return glyf[start:end]
}

// expandComposites grows keep to include the component glyphs referenced
// by any composite glyph already in keep, transitively.
func expandComposites(glyf []byte, loca []uint32, keep map[uint16]bool) error {
	work := make([]uint16, 0, len(keep))
	for g := range keep {
		work = append(work, g)
	}
	for len(work) > 0 {
		g := work[len(work)-1]
		work = work[:len(work)-1]
		gb := glyphBytes(glyf, loca, g)
		if len(gb) < 10 {
			continue
		}
		if int16(binary.BigEndian.Uint16(gb[0:2])) >= 0 {
			continue // simple glyph, no components
		}
		eachComponent(gb, func(_ int, comp uint16) {
			if !keep[comp] {
				keep[comp] = true
				work = append(work, comp)
			}
		})
	}
	return nil
}

// eachComponent iterates a composite glyph's components, invoking fn with
// the byte offset of the component's glyphIndex field and its value.
func eachComponent(gb []byte, fn func(glyphIndexOff int, glyphIndex uint16)) {
	off := 10 // past the glyph header
	for off+4 <= len(gb) {
		flags := binary.BigEndian.Uint16(gb[off:])
		glyphIndexOff := off + 2
		comp := binary.BigEndian.Uint16(gb[off+2:])
		fn(glyphIndexOff, comp)
		off += 4 // flags + glyphIndex
		if flags&compArgsAreWords != 0 {
			off += 4
		} else {
			off += 2
		}
		switch {
		case flags&compHaveScale != 0:
			off += 2
		case flags&compHaveXYScale != 0:
			off += 4
		case flags&compHave2x2 != 0:
			off += 8
		}
		if flags&compMoreComponents == 0 {
			break
		}
	}
}

// rebuildGlyf concatenates the kept glyphs in new-GID order, remapping
// composite component indices through oldToNew, and returns the new glyf
// bytes plus the new loca offsets (len N+1). Each glyph is padded to a
// 2-byte boundary.
func rebuildGlyf(glyf []byte, loca []uint32, oldGIDs []uint16, oldToNew map[uint16]uint16) ([]byte, []uint32) {
	var out []byte
	newLoca := make([]uint32, len(oldGIDs)+1)
	for i, oldGID := range oldGIDs {
		newLoca[i] = uint32(len(out))
		gb := glyphBytes(glyf, loca, oldGID)
		if len(gb) == 0 {
			continue // empty glyph: offsets stay equal
		}
		// Copy so we can remap composite component indices in place.
		buf := make([]byte, len(gb))
		copy(buf, gb)
		if int16(binary.BigEndian.Uint16(buf[0:2])) < 0 {
			eachComponent(buf, func(glyphIndexOff int, comp uint16) {
				if nv, ok := oldToNew[comp]; ok {
					binary.BigEndian.PutUint16(buf[glyphIndexOff:], nv)
				}
			})
		}
		out = append(out, buf...)
		if len(out)%2 != 0 {
			out = append(out, 0)
		}
	}
	newLoca[len(oldGIDs)] = uint32(len(out))
	return out, newLoca
}

// encodeLocaLong serialises offsets as a long-format loca table.
func encodeLocaLong(offsets []uint32) []byte {
	b := make([]byte, len(offsets)*4)
	for i, o := range offsets {
		binary.BigEndian.PutUint32(b[i*4:], o)
	}
	return b
}

// patchHead clones head and forces long loca format (indexToLocFormat=1)
// and zeroes checkSumAdjustment (recomputed during final assembly).
func patchHead(head []byte) []byte {
	b := append([]byte(nil), head...)
	if len(b) >= 52 {
		binary.BigEndian.PutUint16(b[50:52], 1)
	}
	if len(b) >= 12 {
		binary.BigEndian.PutUint32(b[8:12], 0)
	}
	return b
}

// patchMaxp clones maxp and sets numGlyphs.
func patchMaxp(maxp []byte, numGlyphs uint16) []byte {
	b := append([]byte(nil), maxp...)
	if len(b) >= 6 {
		binary.BigEndian.PutUint16(b[4:6], numGlyphs)
	}
	return b
}

// patchHhea clones hhea and sets numberOfHMetrics.
func patchHhea(hhea []byte, numHMetrics uint16) []byte {
	b := append([]byte(nil), hhea...)
	if len(b) >= 36 {
		binary.BigEndian.PutUint16(b[34:36], numHMetrics)
	}
	return b
}

// buildHmtx writes a full hmtx (numberOfHMetrics == numGlyphs) for the
// subset: advanceWidth from the parsed widths, leftSideBearing from the
// glyph's xMin (0 for empty glyphs).
func buildHmtx(f *ttfFont, glyf []byte, loca []uint32, oldGIDs []uint16) []byte {
	b := make([]byte, len(oldGIDs)*4)
	for i, oldGID := range oldGIDs {
		var adv uint16
		if int(oldGID) < len(f.glyphWidths) {
			adv = f.glyphWidths[oldGID]
		}
		var lsb int16
		if gb := glyphBytes(glyf, loca, oldGID); len(gb) >= 4 {
			lsb = int16(binary.BigEndian.Uint16(gb[2:4])) // glyph header xMin
		}
		binary.BigEndian.PutUint16(b[i*4:], adv)
		binary.BigEndian.PutUint16(b[i*4+2:], uint16(lsb))
	}
	return b
}

// buildPost30 synthesises a version 3.0 post table (no glyph names),
// carrying the italic angle and fixed-pitch flag so the subset stays a
// valid sfnt without the original (often large) name-bearing post table.
func buildPost30(f *ttfFont) []byte {
	b := make([]byte, 32)
	binary.BigEndian.PutUint32(b[0:], 0x00030000) // version 3.0
	italic := int32(f.italicAngle * 65536.0)
	binary.BigEndian.PutUint32(b[4:], uint32(italic))
	// underlinePosition (8), underlineThickness (10) left 0.
	if f.isFixedPitch {
		binary.BigEndian.PutUint32(b[12:], 1)
	}
	// min/max mem usage (16..32) left 0.
	return b
}

// assembleSFNT builds a complete TrueType file from the given tables,
// writing a sorted table directory with correct offsets and checksums
// and patching head.checkSumAdjustment.
func assembleSFNT(tbls map[string][]byte) []byte {
	return assembleSFNTFlavor(0x00010000, tbls) // sfnt version 1.0 (TrueType outlines)
}

// assembleSFNTFlavor is assembleSFNT with an explicit sfnt version tag —
// 0x4F54544F ('OTTO') for CFF-outline fonts (WOFF re-wrapping keeps the
// original flavor).
func assembleSFNTFlavor(flavor uint32, tbls map[string][]byte) []byte {
	tags := make([]string, 0, len(tbls))
	for tag := range tbls {
		tags = append(tags, tag)
	}
	sort.Strings(tags)

	numTables := len(tags)
	searchRange, entrySelector, rangeShift := sfntSearchParams(numTables)

	var hdr []byte
	put16 := func(v uint16) { hdr = append(hdr, byte(v>>8), byte(v)) }
	put32 := func(v uint32) { hdr = append(hdr, byte(v>>24), byte(v>>16), byte(v>>8), byte(v)) }
	put32(flavor)
	put16(uint16(numTables))
	put16(searchRange)
	put16(entrySelector)
	put16(rangeShift)

	// Table data starts after directory; each table 4-byte aligned.
	dataStart := 12 + numTables*16
	offsets := make(map[string]uint32, numTables)
	var body []byte
	off := dataStart
	for _, tag := range tags {
		offsets[tag] = uint32(off)
		t := tbls[tag]
		body = append(body, t...)
		for len(body)%4 != 0 {
			body = append(body, 0)
		}
		off = dataStart + len(body)
	}

	// Directory records (tag, checksum, offset, length).
	for _, tag := range tags {
		var rec [16]byte
		copy(rec[0:4], tag)
		binary.BigEndian.PutUint32(rec[4:8], tableChecksum(tbls[tag]))
		binary.BigEndian.PutUint32(rec[8:12], offsets[tag])
		binary.BigEndian.PutUint32(rec[12:16], uint32(len(tbls[tag])))
		hdr = append(hdr, rec[:]...)
	}

	file := append(hdr, body...)

	// head.checkSumAdjustment = 0xB1B0AFBA - checksum(whole file).
	if headOff, ok := offsets["head"]; ok && int(headOff)+12 <= len(file) {
		adj := 0xB1B0AFBA - tableChecksum(file)
		binary.BigEndian.PutUint32(file[headOff+8:headOff+12], adj)
	}
	return file
}

// sfntSearchParams computes the binary-search hint fields for the table
// directory header per the OpenType spec.
func sfntSearchParams(numTables int) (searchRange, entrySelector, rangeShift uint16) {
	pow2 := 1
	exp := 0
	for pow2*2 <= numTables {
		pow2 *= 2
		exp++
	}
	searchRange = uint16(pow2 * 16)
	entrySelector = uint16(exp)
	rangeShift = uint16(numTables*16) - searchRange
	return
}

// tableChecksum sums a table as big-endian uint32 words, zero-padding a
// trailing partial word (OpenType table-checksum algorithm).
func tableChecksum(b []byte) uint32 {
	var sum uint32
	for i := 0; i < len(b); i += 4 {
		var word uint32
		for j := 0; j < 4; j++ {
			word <<= 8
			if i+j < len(b) {
				word |= uint32(b[i+j])
			}
		}
		sum += word
	}
	return sum
}
