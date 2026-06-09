// SPDX-License-Identifier: MIT

package asposepdf

import (
	"encoding/binary"
	"fmt"
)

// JBIG2 (ITU-T T.88) bilevel image decoding for the PDF /JBIG2Decode filter.
// Phase 1 covers the embedded-organization segment stream with the arithmetic
// (MQ) coding path: page info, generic regions, symbol dictionaries and text
// regions — the combination scanned-document PDFs almost always use. Out of
// scope (tracked separately): halftone/refinement regions, MMR coding, and the
// Huffman-coded symbol/text variants.
//
// jbig2Decode parses globals (from /DecodeParms /JBIG2Globals, may be nil) plus
// the image stream, composes the page bitmap at width×height, and returns it
// packed 1 bit per pixel, MSB-first, rows byte-aligned, with 0 = black so the
// result feeds a 1-bpc DeviceGray image directly (JBIG2 foreground is inverted).

type jbig2Segment struct {
	number uint32
	typ    int
	refs   []uint32
	page   uint32
	data   []byte
}

// jbig2State carries decoding state across segments in stream order.
type jbig2State struct {
	page      jbig2Bitmap
	pageW     int
	pageH     int
	pageDef   int
	pageCombo int
	symbols   map[uint32][]jbig2Bitmap // exported symbols keyed by segment number
	patterns  map[uint32][]jbig2Bitmap // pattern dictionaries keyed by segment number
	tables    map[uint32]*huffTable    // custom Huffman tables (type-53 segments)
}

func jbig2Decode(stream, globals []byte, width, height int) ([]byte, error) {
	st := &jbig2State{
		symbols:  map[uint32][]jbig2Bitmap{},
		patterns: map[uint32][]jbig2Bitmap{},
		tables:   map[uint32]*huffTable{},
	}

	var segs []jbig2Segment
	if len(globals) > 0 {
		gs, err := jbig2ParseSegments(globals)
		if err == nil {
			segs = append(segs, gs...)
		}
	}
	ms, err := jbig2ParseSegments(stream)
	if err != nil {
		return nil, err
	}
	segs = append(segs, ms...)

	for _, s := range segs {
		st.process(s)
	}

	if st.page == nil {
		// No page-info segment produced a bitmap; fall back to the PDF size.
		st.ensurePage(width, height)
	}
	return jbig2Pack(st.page, width, height), nil
}

// jbig2GlobalsData resolves the optional /JBIG2Globals stream referenced from an
// image's /DecodeParms (a dict, an indirect ref, or an array of them) and returns
// its decoded global-segment bytes, or nil when absent.
func jbig2GlobalsData(objects map[int]*pdfObject, dict pdfDict) []byte {
	var dps []pdfValue
	switch v := resolveRef(objects, dict["/DecodeParms"]).(type) {
	case pdfDict:
		dps = []pdfValue{v}
	case pdfArray:
		dps = v
	default:
		return nil
	}
	for _, dpv := range dps {
		dp, ok := resolveRef(objects, dpv).(pdfDict)
		if !ok {
			continue
		}
		if g, ok := resolveRef(objects, dp["/JBIG2Globals"]).(*pdfStream); ok {
			return decodedStreamData(g)
		}
	}
	return nil
}

// jbig2ParseSegments reads the embedded-organization segment headers (ITU-T T.88
// §7.2) and slices out each segment's data.
func jbig2ParseSegments(data []byte) ([]jbig2Segment, error) {
	var segs []jbig2Segment
	p := 0
	for p+11 <= len(data) {
		segNum := binary.BigEndian.Uint32(data[p:])
		p += 4
		flags := data[p]
		p++
		typ := int(flags & 0x3f)
		pageAssoc4 := flags&0x40 != 0

		if p >= len(data) {
			break
		}
		rtFlags := data[p]
		var refCount int
		if rtFlags>>5 == 7 {
			if p+4 > len(data) {
				break
			}
			refCount = int(binary.BigEndian.Uint32(data[p:]) & 0x1fffffff)
			p += 4
			p += (refCount + 8) / 8 // retention flag bytes
		} else {
			refCount = int(rtFlags >> 5)
			p++
		}

		var refSize int
		switch {
		case segNum <= 256:
			refSize = 1
		case segNum <= 65536:
			refSize = 2
		default:
			refSize = 4
		}
		refs := make([]uint32, 0, refCount)
		for i := 0; i < refCount; i++ {
			if p+refSize > len(data) {
				return segs, fmt.Errorf("jbig2: truncated referred-to segment list")
			}
			var r uint32
			switch refSize {
			case 1:
				r = uint32(data[p])
			case 2:
				r = uint32(binary.BigEndian.Uint16(data[p:]))
			default:
				r = binary.BigEndian.Uint32(data[p:])
			}
			refs = append(refs, r)
			p += refSize
		}

		var page uint32
		if pageAssoc4 {
			if p+4 > len(data) {
				break
			}
			page = binary.BigEndian.Uint32(data[p:])
			p += 4
		} else {
			if p >= len(data) {
				break
			}
			page = uint32(data[p])
			p++
		}

		if p+4 > len(data) {
			break
		}
		dataLen := binary.BigEndian.Uint32(data[p:])
		p += 4
		if dataLen == 0xffffffff {
			return segs, fmt.Errorf("jbig2: unknown-length segment unsupported")
		}
		if p+int(dataLen) > len(data) {
			dataLen = uint32(len(data) - p) // tolerate a short final segment
		}
		segs = append(segs, jbig2Segment{
			number: segNum, typ: typ, refs: refs, page: page,
			data: data[p : p+int(dataLen)],
		})
		p += int(dataLen)
	}
	return segs, nil
}

func (st *jbig2State) ensurePage(w, h int) {
	if st.page != nil {
		return
	}
	st.pageW, st.pageH = w, h
	st.page = newJBIG2Bitmap(w, h)
	if st.pageDef != 0 {
		for i := range st.page {
			for j := range st.page[i] {
				st.page[i][j] = 1
			}
		}
	}
}

func (st *jbig2State) process(s jbig2Segment) {
	switch s.typ {
	case 48: // page info
		st.handlePageInfo(s)
	case 53: // custom Huffman table
		st.handleTableSegment(s)
	case 0: // symbol dictionary
		st.handleSymbolDict(s)
	case 4, 6, 7: // text region (intermediate/immediate/immediate lossless)
		st.handleTextRegion(s)
	case 16: // pattern dictionary
		st.handlePatternDict(s)
	case 20, 22, 23: // halftone region
		st.handleHalftoneRegion(s)
	case 36, 38, 39: // generic region
		st.handleGenericRegion(s)
	case 40, 42, 43: // generic refinement region
		st.handleRefinementRegion(s)
	default:
		// page end / end of stripe / extension: ignore.
	}
}

// gatherCustomTables collects the custom Huffman tables (type-53) referred to by
// a segment, in reference order, for selectors flagged "custom".
func (st *jbig2State) gatherCustomTables(refs []uint32) *customTables {
	c := &customTables{}
	for _, r := range refs {
		if t, ok := st.tables[r]; ok {
			c.tables = append(c.tables, t)
		}
	}
	return c
}

func (st *jbig2State) handlePageInfo(s jbig2Segment) {
	d := s.data
	if len(d) < 17 {
		return
	}
	w := int(binary.BigEndian.Uint32(d[0:]))
	h := int(binary.BigEndian.Uint32(d[4:]))
	flags := d[16]
	st.pageDef = int(flags>>2) & 1
	st.pageCombo = int(flags>>3) & 3
	if h <= 0 || h == int(int32(-1)) || uint32(h) == 0xffffffff {
		h = 0 // unknown height; grown lazily during compositing
	}
	st.pageW = w
	st.pageH = h
	if w > 0 && h > 0 && w < 1<<20 && h < 1<<20 {
		st.page = newJBIG2Bitmap(w, h)
		if st.pageDef != 0 {
			for i := range st.page {
				for j := range st.page[i] {
					st.page[i][j] = 1
				}
			}
		}
	}
}

// gatherInputSymbols collects the exported symbols of all referred-to symbol
// dictionaries, in reference order.
func (st *jbig2State) gatherInputSymbols(refs []uint32) []jbig2Bitmap {
	var in []jbig2Bitmap
	for _, r := range refs {
		if syms, ok := st.symbols[r]; ok {
			in = append(in, syms...)
		}
	}
	return in
}

func (st *jbig2State) handleSymbolDict(s jbig2Segment) {
	d := s.data
	if len(d) < 2 {
		return
	}
	flags := binary.BigEndian.Uint16(d[0:])
	p := 2
	sdhuff := flags & 1
	sdrefagg := (flags >> 1) & 1
	template := int(flags>>10) & 3
	rTemplate := int(flags>>12) & 1

	// SDAT adaptive template pixels (only present for the arithmetic path).
	var at []jbig2Point
	if sdhuff == 0 {
		nAT := 4
		if template != 0 {
			nAT = 1
		}
		for i := 0; i < nAT; i++ {
			if p+2 > len(d) {
				return
			}
			at = append(at, jbig2Point{x: int(int8(d[p])), y: int(int8(d[p+1]))})
			p += 2
		}
	}
	// SDRAT refinement adaptive pixels (only when refinement is on and template 0).
	var rat []jbig2Point
	if sdrefagg == 1 && rTemplate == 0 {
		for i := 0; i < 2; i++ {
			if p+2 > len(d) {
				return
			}
			rat = append(rat, jbig2Point{x: int(int8(d[p])), y: int(int8(d[p+1]))})
			p += 2
		}
	}
	if p+8 > len(d) {
		return
	}
	numEx := int(binary.BigEndian.Uint32(d[p:]))
	numNew := int(binary.BigEndian.Uint32(d[p+4:]))
	p += 8
	if numNew < 0 || numNew > 1<<20 {
		return
	}

	input := st.gatherInputSymbols(s.refs)

	if sdhuff == 1 {
		if sdrefagg == 1 {
			return // Huffman + refinement/aggregate: rare; not yet supported.
		}
		custom := st.gatherCustomTables(s.refs)
		dhTable := pickTable(int(flags>>2)&3, map[int]int{0: 4, 1: 5}, custom)
		dwTable := pickTable(int(flags>>4)&3, map[int]int{0: 2, 1: 3}, custom)
		bmTable := pickTable(int(flags>>6)&1, map[int]int{0: 1}, custom)
		r := newJBIG2Reader(d, p)
		st.symbols[s.number] = decodeHuffSymbolDict(r, dhTable, dwTable, bmTable, jbig2StdTables[1], input, numNew, numEx)
		return
	}

	symCodeLen := jbig2Log2(len(input) + numNew)
	ctx := newJBIG2Ctx(symCodeLen)
	dec := newMQDecoder(d, p, len(d))
	exported := dec.decodeSymbolDict(ctx, symbolDictParams{
		template: template, at: at, refagg: sdrefagg == 1, rTemplate: rTemplate,
		rat: rat, symCodeLen: symCodeLen, numNew: numNew, numEx: numEx,
	}, input)
	st.symbols[s.number] = exported
}

func (st *jbig2State) handleTextRegion(s jbig2Segment) {
	d := s.data
	if len(d) < 17+2 {
		return
	}
	rw := int(binary.BigEndian.Uint32(d[0:]))
	rh := int(binary.BigEndian.Uint32(d[4:]))
	rx := int(binary.BigEndian.Uint32(d[8:]))
	ry := int(binary.BigEndian.Uint32(d[12:]))
	regionCombo := int(d[16]) & 7
	p := 17

	flags := binary.BigEndian.Uint16(d[p:])
	p += 2
	sbhuff := flags & 1
	sbrefine := (flags >> 1) & 1
	logStrips := int(flags>>2) & 3
	refCorner := int(flags>>4) & 3
	transposed := (flags>>6)&1 == 1
	combOp := int(flags>>7) & 3
	defPixel := int(flags>>9) & 1
	dsOffset := int(flags>>10) & 0x1f
	if dsOffset > 15 {
		dsOffset -= 32 // signed 5-bit
	}
	rTemplate := int(flags>>15) & 1

	var huffFlags uint16
	if sbhuff == 1 {
		if p+2 > len(d) {
			return
		}
		huffFlags = binary.BigEndian.Uint16(d[p:])
		p += 2
	}
	var rat []jbig2Point
	if sbrefine == 1 && rTemplate == 0 {
		for i := 0; i < 2; i++ {
			if p+2 > len(d) {
				return
			}
			rat = append(rat, jbig2Point{x: int(int8(d[p])), y: int(int8(d[p+1]))})
			p += 2
		}
	}
	if p+4 > len(d) {
		return
	}
	numInstances := int(binary.BigEndian.Uint32(d[p:]))
	p += 4
	if numInstances < 0 || numInstances > 1<<24 {
		return
	}

	symbols := st.gatherInputSymbols(s.refs)

	if sbhuff == 1 {
		custom := st.gatherCustomTables(s.refs)
		fsTable := pickTable(int(huffFlags)&3, map[int]int{0: 6, 1: 7}, custom)
		dsTable := pickTable(int(huffFlags>>2)&3, map[int]int{0: 8, 1: 9, 2: 10}, custom)
		dtTable := pickTable(int(huffFlags>>4)&3, map[int]int{0: 11, 1: 12, 2: 13}, custom)
		if sbrefine == 1 {
			return // Huffman + refinement text regions: rare; not yet supported.
		}
		r := newJBIG2Reader(d, p)
		symIDTable := buildSymbolIDTable(r, len(symbols))
		region := decodeHuffTextRegion(r, symbols, huffTextParams{
			width: rw, height: rh, numInstances: numInstances,
			logStrips: logStrips, refCorner: refCorner, transposed: transposed,
			combOp: combOp, dsOffset: dsOffset, defPixel: defPixel,
			symIDTable: symIDTable, fsTable: fsTable, dsTable: dsTable, dtTable: dtTable,
		})
		st.composite(region, rx, ry, regionCombo)
		return
	}

	symCodeLen := jbig2Log2(len(symbols))
	if symCodeLen < 1 {
		symCodeLen = 1
	}
	ctx := newJBIG2Ctx(symCodeLen)
	dec := newMQDecoder(d, p, len(d))
	region := dec.decodeTextRegion(ctx, symbols, textRegionParams{
		width: rw, height: rh, numInstances: numInstances, symCodeLen: symCodeLen,
		logStrips: logStrips, refCorner: refCorner, transposed: transposed,
		combOp: combOp, dsOffset: dsOffset, defPixel: defPixel,
		refine: sbrefine == 1, rTemplate: rTemplate, rat: rat,
	})
	st.composite(region, rx, ry, regionCombo)
}

func (st *jbig2State) handleGenericRegion(s jbig2Segment) {
	d := s.data
	if len(d) < 17+1 {
		return
	}
	rw := int(binary.BigEndian.Uint32(d[0:]))
	rh := int(binary.BigEndian.Uint32(d[4:]))
	rx := int(binary.BigEndian.Uint32(d[8:]))
	ry := int(binary.BigEndian.Uint32(d[12:]))
	regionCombo := int(d[16]) & 7
	p := 17

	gflags := d[p]
	p++
	mmr := gflags & 1
	template := int(gflags>>1) & 3
	tpgdon := (gflags>>3)&1 == 1
	var at []jbig2Point
	if mmr == 0 {
		nAT := 4
		if template != 0 {
			nAT = 1
		}
		for i := 0; i < nAT; i++ {
			if p+2 > len(d) {
				return
			}
			at = append(at, jbig2Point{x: int(int8(d[p])), y: int(int8(d[p+1]))})
			p += 2
		}
	}
	if rw <= 0 || rh <= 0 || rw > 1<<20 || rh > 1<<20 {
		return
	}
	var region jbig2Bitmap
	if mmr == 1 {
		region = jbig2MMRDecode(d[p:], rw, rh)
	} else {
		ctx := newJBIG2Ctx(1)
		dec := newMQDecoder(d, p, len(d))
		region = decodeGenericBitmap(rw, rh, template, tpgdon, nil, at, dec, ctx.gb)
	}
	st.composite(region, rx, ry, regionCombo)
}

// handleTableSegment decodes a custom Huffman table segment (ITU-T T.88 §7.4.13 /
// Annex B.2) into a huffTable keyed by segment number.
func (st *jbig2State) handleTableSegment(s jbig2Segment) {
	d := s.data
	if len(d) < 9 {
		return
	}
	flags := d[0]
	oob := flags&1 != 0
	htps := int((flags>>1)&7) + 1 // prefix-size bits
	htrs := int((flags>>4)&7) + 1 // range-size bits
	low := int(int32(binary.BigEndian.Uint32(d[1:])))
	high := int(int32(binary.BigEndian.Uint32(d[5:])))
	r := newJBIG2Reader(d, 9)

	var lines []huffLine
	cur := low
	for cur < high {
		prefLen := r.readBits(htps)
		rangeLen := r.readBits(htrs)
		lines = append(lines, huffLine{prefLen: prefLen, rangeLen: rangeLen, rangeLow: cur})
		cur += 1 << uint(rangeLen)
		if len(lines) > 1<<16 {
			return
		}
	}
	// Lower and upper range lines.
	lowerLen := r.readBits(htps)
	lines = append(lines, huffLine{prefLen: lowerLen, rangeLen: 32, rangeLow: low - 1, isLower: true})
	upperLen := r.readBits(htps)
	lines = append(lines, huffLine{prefLen: upperLen, rangeLen: 32, rangeLow: high})
	if oob {
		oobLen := r.readBits(htps)
		lines = append(lines, huffLine{prefLen: oobLen, isOOB: true})
	}
	st.tables[s.number] = newHuffTable(lines)
}

// handleRefinementRegion decodes a standalone generic refinement region (ITU-T
// T.88 §7.4.7) that refines the page content already under its rectangle.
func (st *jbig2State) handleRefinementRegion(s jbig2Segment) {
	d := s.data
	if len(d) < 18 {
		return
	}
	rw := int(binary.BigEndian.Uint32(d[0:]))
	rh := int(binary.BigEndian.Uint32(d[4:]))
	rx := int(binary.BigEndian.Uint32(d[8:]))
	ry := int(binary.BigEndian.Uint32(d[12:]))
	p := 17
	gflags := d[p]
	p++
	rTemplate := int(gflags & 1)
	var rat []jbig2Point
	if rTemplate == 0 {
		for i := 0; i < 2; i++ {
			if p+2 > len(d) {
				return
			}
			rat = append(rat, jbig2Point{x: int(int8(d[p])), y: int(int8(d[p+1]))})
			p += 2
		}
	}
	if rw <= 0 || rh <= 0 || rw > 1<<20 || rh > 1<<20 {
		return
	}
	// Reference is the current page content under the region rectangle.
	ref := newJBIG2Bitmap(rw, rh)
	for y := 0; y < rh && y < len(ref); y++ {
		py := ry + y
		if py < 0 || py >= len(st.page) {
			continue
		}
		for x := 0; x < rw; x++ {
			px := rx + x
			if px >= 0 && px < len(st.page[py]) {
				ref[y][x] = st.page[py][px]
			}
		}
	}
	ctx := newJBIG2Ctx(1)
	dec := newMQDecoder(d, p, len(d))
	region := decodeRefinement(rw, rh, rTemplate, ref, 0, 0, rat, dec, ctx.gr)
	st.composite(region, rx, ry, 4) // replace
}

// composite draws a region bitmap onto the page at (x,y), growing the page if its
// height was left unknown by the page-info segment.
func (st *jbig2State) composite(region jbig2Bitmap, x, y, op int) {
	if len(region) == 0 {
		return
	}
	needH := y + len(region)
	if st.page == nil {
		st.pageH = needH
		st.ensurePage(st.pageW, st.pageH)
	} else if needH > len(st.page) {
		for len(st.page) < needH {
			st.page = append(st.page, make([]byte, st.pageW))
		}
		st.pageH = len(st.page)
	}
	jbig2Blit(st.page, region, x, y, op)
}

// jbig2Pack converts the bilevel page bitmap to packed 1-bpp rows (MSB-first),
// cropped/padded to width×height, inverting JBIG2 foreground (1=black) to the
// DeviceGray convention (sample 0 = black).
func jbig2Pack(page jbig2Bitmap, width, height int) []byte {
	if width <= 0 {
		width = 0
		if len(page) > 0 {
			width = len(page[0])
		}
	}
	if height <= 0 {
		height = len(page)
	}
	rowBytes := (width + 7) / 8
	out := make([]byte, rowBytes*height)
	for y := 0; y < height; y++ {
		var row []byte
		if y < len(page) {
			row = page[y]
		}
		for x := 0; x < width; x++ {
			fg := byte(0)
			if x < len(row) {
				fg = row[x]
			}
			if fg == 0 { // background → white → sample bit 1
				out[y*rowBytes+x/8] |= 0x80 >> uint(x%8)
			}
		}
	}
	return out
}
