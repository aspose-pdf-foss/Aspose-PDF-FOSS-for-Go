// SPDX-License-Identifier: MIT

package asposepdf

import (
	"encoding/binary"
	"fmt"
)

// CFF (Compact Font Format, Adobe TN #5176) carries glyph outlines as Type2
// charstrings (TN #5177). OpenType-CFF fonts (.otf) and PDF /FontFile3 embeds
// (Subtype /Type1C or /CIDFontType0C) use it instead of the TrueType glyf table.
// This file parses the CFF container and interprets Type2 charstrings into the
// same flattened glyphContour the glyf decoder produces, so the renderer draws
// CFF glyphs through the existing pipeline. Cubic Béziers are flattened in glyph
// units via the shared flattener; the result is on-curve polylines.

// cffFont is a parsed CFF program able to produce glyph outlines by glyph ID.
type cffFont struct {
	charStrings [][]byte
	globalSubrs [][]byte
	localSubrs  [][]byte // non-CID local subrs (or the matched FD's for CID)

	// CID-keyed fonts select a per-glyph Private DICT (and its local subrs)
	// through FDSelect; charset maps glyph ID → CID.
	isCID        bool
	fdSelect     []uint8    // GID → FD index
	fdLocalSubrs [][][]byte // local subrs per FD
	charset      []uint16   // GID → CID (CID-keyed) or SID (name-keyed)
	cidToGID     map[uint16]uint16
	simpleGID    map[uint16]uint16 // code → GID for simple (non-CID) fonts

	unitsPerEm float64
	numGlyphs  int
}

// parseCFFProgram parses /FontFile3 (or .otf) bytes: a bare CFF table, or an
// sfnt/OpenType wrapper ('OTTO') from which the 'CFF ' table is extracted.
func parseCFFProgram(data []byte) (*cffFont, error) {
	if len(data) >= 4 {
		switch binary.BigEndian.Uint32(data[0:4]) {
		case 0x4F54544F, 0x00010000, 0x74727565: // 'OTTO' / sfnt / 'true'
			if cff := tableSlice(data, tableDir(data), "CFF "); cff != nil {
				return parseCFF(cff)
			}
			return nil, fmt.Errorf("parse cff: sfnt wrapper has no 'CFF ' table")
		}
	}
	return parseCFF(data)
}

func parseCFF(data []byte) (*cffFont, error) {
	if len(data) < 4 {
		return nil, fmt.Errorf("parse cff: too small")
	}
	hdrSize := int(data[2])
	if hdrSize < 4 || hdrSize > len(data) {
		return nil, fmt.Errorf("parse cff: bad header size %d", hdrSize)
	}
	pos := hdrSize

	// Name INDEX, Top DICT INDEX, String INDEX, Global Subr INDEX.
	_, pos, err := readCFFIndex(data, pos)
	if err != nil {
		return nil, err
	}
	topDicts, pos2, err := readCFFIndex(data, pos)
	if err != nil || len(topDicts) == 0 {
		return nil, fmt.Errorf("parse cff: missing Top DICT")
	}
	pos = pos2
	_, pos, err = readCFFIndex(data, pos) // String INDEX (unused for outlines)
	if err != nil {
		return nil, err
	}
	gsubrs, _, err := readCFFIndex(data, pos)
	if err != nil {
		return nil, err
	}

	top := parseCFFDict(topDicts[0])
	f := &cffFont{globalSubrs: gsubrs, unitsPerEm: 1000}

	// FontMatrix (12 7) sets units/em; default 0.001 → 1000.
	if fm, ok := top[cffOp(12, 7)]; ok && len(fm) >= 1 && fm[0] != 0 {
		f.unitsPerEm = 1 / fm[0]
	}

	csOff := intOp(top, cffOp(17, 0))
	if csOff <= 0 || csOff >= len(data) {
		return nil, fmt.Errorf("parse cff: bad CharStrings offset")
	}
	f.charStrings, _, err = readCFFIndex(data, csOff)
	if err != nil {
		return nil, err
	}
	f.numGlyphs = len(f.charStrings)

	_, f.isCID = top[cffOp(12, 30)] // ROS operator → CID-keyed

	if f.isCID {
		if err := f.parseCIDParts(data, top); err != nil {
			return nil, err
		}
	} else {
		if priv, ok := top[cffOp(18, 0)]; ok && len(priv) >= 2 {
			f.localSubrs = readPrivateSubrs(data, int(priv[1]), int(priv[0]))
		}
		f.buildSimpleEncoding(data, top)
	}
	return f, nil
}

// buildSimpleEncoding builds the code→GID map for a simple (non-CID) CFF font,
// the missing piece that left embedded Type1C fonts (e.g. MinionPro subsets)
// rendering nothing. The CFF charset maps GID→SID (glyph-name id); the Encoding
// maps codes to glyphs. For the predefined Standard encoding (Top DICT /Encoding
// 0, the common case) a code's SID is code−31 across the ASCII range 32–126, so
// reversing the charset (SID→GID) yields code→GID. A custom Encoding table is
// read directly (formats 0/1). ISO 32000-1 §9.6.6.2.
func (f *cffFont) buildSimpleEncoding(data []byte, top map[int][]float64) {
	if csOff := intOp(top, cffOp(15, 0)); csOff > 2 && csOff < len(data) {
		f.charset = readCharset(data, csOff, f.numGlyphs)
	}
	sidToGID := make(map[uint16]uint16, len(f.charset))
	for gid, sid := range f.charset {
		if _, ok := sidToGID[sid]; !ok {
			sidToGID[sid] = uint16(gid)
		}
	}
	f.simpleGID = make(map[uint16]uint16)

	encOff := intOp(top, cffOp(16, 0))
	if encOff > 1 && encOff < len(data) {
		f.readCustomEncoding(data, encOff)
		return
	}
	// Predefined Standard (0) / Expert (1) encoding: map ASCII codes via the
	// SID = code−31 correspondence, then SID→GID through the charset.
	for code := 32; code <= 126; code++ {
		if gid, ok := sidToGID[uint16(code-31)]; ok {
			f.simpleGID[uint16(code)] = gid
		}
	}
}

// readCustomEncoding parses a CFF custom Encoding table (formats 0/1) into the
// code→GID map. Supplements (high bit of the format byte) are ignored.
func (f *cffFont) readCustomEncoding(data []byte, off int) {
	if off >= len(data) {
		return
	}
	format := data[off]
	p := off + 1
	switch format & 0x7f {
	case 0:
		if p >= len(data) {
			return
		}
		nCodes := int(data[p])
		p++
		for i := 0; i < nCodes && p < len(data); i++ {
			f.simpleGID[uint16(data[p])] = uint16(i + 1)
			p++
		}
	case 1:
		if p >= len(data) {
			return
		}
		nRanges := int(data[p])
		p++
		gid := 1
		for r := 0; r < nRanges && p+1 < len(data); r++ {
			first := int(data[p])
			nLeft := int(data[p+1])
			p += 2
			for k := 0; k <= nLeft; k++ {
				f.simpleGID[uint16(first+k)] = uint16(gid)
				gid++
			}
		}
	}
}

// parseCIDParts reads the FDArray, FDSelect and charset of a CID-keyed CFF.
func (f *cffFont) parseCIDParts(data []byte, top map[int][]float64) error {
	if fdaOff := intOp(top, cffOp(12, 36)); fdaOff > 0 && fdaOff < len(data) {
		fdDicts, _, err := readCFFIndex(data, fdaOff)
		if err != nil {
			return err
		}
		for _, fd := range fdDicts {
			d := parseCFFDict(fd)
			var subrs [][]byte
			if priv, ok := d[cffOp(18, 0)]; ok && len(priv) >= 2 {
				subrs = readPrivateSubrs(data, int(priv[1]), int(priv[0]))
			}
			f.fdLocalSubrs = append(f.fdLocalSubrs, subrs)
		}
	}
	if fdsOff := intOp(top, cffOp(12, 37)); fdsOff > 0 && fdsOff < len(data) {
		f.fdSelect = readFDSelect(data, fdsOff, f.numGlyphs)
	}
	if csOff := intOp(top, cffOp(15, 0)); csOff > 2 && csOff < len(data) {
		f.charset = readCharset(data, csOff, f.numGlyphs)
		f.cidToGID = make(map[uint16]uint16, len(f.charset))
		for gid, cid := range f.charset {
			f.cidToGID[cid] = uint16(gid)
		}
	}
	return nil
}

// gidForCID maps a CID to a glyph ID via the charset (identity if no charset).
func (f *cffFont) gidForCID(cid uint16) uint16 {
	if f.cidToGID != nil {
		if gid, ok := f.cidToGID[cid]; ok {
			return gid
		}
		return 0
	}
	return cid
}

// glyphContours interprets glyph gid's Type2 charstring and returns its outline
// as flattened on-curve contours in glyph units (matching ttfFont.glyphContours).
func (f *cffFont) glyphContours(gid uint16) (contours []glyphContour) {
	defer func() {
		if recover() != nil {
			contours = nil
		}
	}()
	if int(gid) >= len(f.charStrings) {
		return nil
	}
	local := f.localSubrs
	if f.isCID && len(f.fdLocalSubrs) > 0 && int(gid) < len(f.fdSelect) {
		if fd := int(f.fdSelect[gid]); fd < len(f.fdLocalSubrs) {
			local = f.fdLocalSubrs[fd]
		}
	}
	in := &t2interp{
		font:   f,
		local:  local,
		lbias:  subrBias(len(local)),
		gbias:  subrBias(len(f.globalSubrs)),
		fl:     newFlattener(f.unitsPerEm / 400), // ~2.5 units at em 1000
		open:   false,
		nStems: 0,
	}
	in.run(f.charStrings[gid], 0)
	if in.open {
		in.fl.close()
	}
	dp := in.fl.path()
	for _, sp := range dp.subs {
		if len(sp.pts) < 2 {
			continue
		}
		c := make(glyphContour, len(sp.pts))
		for i, p := range sp.pts {
			c[i] = glyphPoint{x: p.x, y: p.y, on: true}
		}
		contours = append(contours, c)
	}
	return contours
}

// --- Type2 charstring interpreter ----------------------------------------

type t2interp struct {
	font         *cffFont
	local        [][]byte
	lbias, gbias int
	fl           *flattener
	stack        []float64
	x, y         float64
	nStems       int
	widthParsed  bool
	open         bool
	done         bool
}

func (in *t2interp) run(cs []byte, depth int) {
	if depth > 10 {
		return
	}
	i := 0
	for i < len(cs) && !in.done {
		b0 := cs[i]
		if b0 >= 32 || b0 == 28 { // operand
			v, n := t2Number(cs[i:])
			in.stack = append(in.stack, v)
			i += n
			continue
		}
		i++
		switch b0 {
		case 1, 3, 18, 23: // h/v stem (+hm)
			in.countStems()
		case 19, 20: // hintmask / cntrmask
			in.countStems()
			i += (in.nStems + 7) / 8
		case 21: // rmoveto
			in.takeWidth(2)
			in.moveTo(in.arg(0), in.arg(1))
			in.clear()
		case 22: // hmoveto
			in.takeWidth(1)
			in.moveTo(in.arg(0), 0)
			in.clear()
		case 4: // vmoveto
			in.takeWidth(1)
			in.moveTo(0, in.arg(0))
			in.clear()
		case 5: // rlineto
			for j := 0; j+1 < len(in.stack); j += 2 {
				in.lineTo(in.stack[j], in.stack[j+1])
			}
			in.clear()
		case 6: // hlineto
			in.altLineto(true)
		case 7: // vlineto
			in.altLineto(false)
		case 8: // rrcurveto
			for j := 0; j+5 < len(in.stack); j += 6 {
				in.curve(in.stack[j], in.stack[j+1], in.stack[j+2], in.stack[j+3], in.stack[j+4], in.stack[j+5])
			}
			in.clear()
		case 24: // rcurveline
			j := 0
			for ; j+5 < len(in.stack)-2; j += 6 {
				in.curve(in.stack[j], in.stack[j+1], in.stack[j+2], in.stack[j+3], in.stack[j+4], in.stack[j+5])
			}
			if j+1 < len(in.stack) {
				in.lineTo(in.stack[j], in.stack[j+1])
			}
			in.clear()
		case 25: // rlinecurve
			j := 0
			for ; j+1 < len(in.stack)-6; j += 2 {
				in.lineTo(in.stack[j], in.stack[j+1])
			}
			if j+5 < len(in.stack) {
				in.curve(in.stack[j], in.stack[j+1], in.stack[j+2], in.stack[j+3], in.stack[j+4], in.stack[j+5])
			}
			in.clear()
		case 26: // vvcurveto
			in.vvhh(false)
		case 27: // hhcurveto
			in.vvhh(true)
		case 30: // vhcurveto
			in.vhhv(false)
		case 31: // hvcurveto
			in.vhhv(true)
		case 10: // callsubr
			in.call(in.local, in.lbias, depth)
		case 29: // callgsubr
			in.call(in.font.globalSubrs, in.gbias, depth)
		case 11: // return
			return
		case 14: // endchar
			in.takeWidth(0)
			in.done = true
		case 12: // escape — two-byte operators
			if i < len(cs) {
				b1 := cs[i]
				i++
				in.escape(b1)
			}
		default:
			in.clear() // unknown operator: drop args, keep going
		}
	}
}

func (in *t2interp) escape(b1 byte) {
	switch b1 {
	case 35: // flex
		s := in.stack
		if len(s) >= 12 {
			in.curve(s[0], s[1], s[2], s[3], s[4], s[5])
			in.curve(s[6], s[7], s[8], s[9], s[10], s[11])
		}
	case 34: // hflex
		s := in.stack
		if len(s) >= 7 {
			in.curve(s[0], 0, s[1], s[2], s[3], 0)
			in.curve(s[4], 0, s[5], -s[2], s[6], 0)
		}
	case 36: // hflex1
		s := in.stack
		if len(s) >= 9 {
			in.curve(s[0], s[1], s[2], s[3], s[4], 0)
			in.curve(s[5], 0, s[6], s[7], s[8], -(s[1] + s[3] + s[7]))
		}
	case 37: // flex1
		s := in.stack
		if len(s) >= 11 {
			dx := s[0] + s[2] + s[4] + s[6] + s[8]
			dy := s[1] + s[3] + s[5] + s[7] + s[9]
			in.curve(s[0], s[1], s[2], s[3], s[4], s[5])
			if abs(dx) > abs(dy) {
				in.curve(s[6], s[7], s[8], s[9], s[10], -dy)
			} else {
				in.curve(s[6], s[7], s[8], s[9], -dx, s[10])
			}
		}
	}
	in.clear()
}

// altLineto implements hlineto/vlineto: deltas alternate axis, starting with the
// horizontal axis when startH is true.
func (in *t2interp) altLineto(startH bool) {
	h := startH
	for _, d := range in.stack {
		if h {
			in.lineTo(d, 0)
		} else {
			in.lineTo(0, d)
		}
		h = !h
	}
	in.clear()
}

// vvhh implements vvcurveto (horiz=false) and hhcurveto (horiz=true).
func (in *t2interp) vvhh(horiz bool) {
	s := in.stack
	j := 0
	var d1 float64
	if len(s)%4 == 1 {
		d1 = s[0]
		j = 1
	}
	for ; j+3 < len(s); j += 4 {
		if horiz {
			in.curve(s[j], d1, s[j+1], s[j+2], s[j+3], 0)
		} else {
			in.curve(d1, s[j], s[j+1], s[j+2], 0, s[j+3])
		}
		d1 = 0
	}
	in.clear()
}

// vhhv implements hvcurveto (startH=true) and vhcurveto (startH=false): a chain
// of curves whose tangents alternate between horizontal and vertical, with an
// optional final fifth argument.
func (in *t2interp) vhhv(startH bool) {
	s := in.stack
	h := startH
	j := 0
	for j+3 < len(s) {
		last := j+8 > len(s)
		var df float64
		if last && (len(s)-j) == 5 {
			df = s[j+4]
		}
		if h {
			in.curve(s[j], 0, s[j+1], s[j+2], df, s[j+3])
		} else {
			in.curve(0, s[j], s[j+1], s[j+2], s[j+3], df)
		}
		h = !h
		j += 4
	}
	in.clear()
}

func (in *t2interp) call(subrs [][]byte, bias, depth int) {
	if len(in.stack) == 0 {
		return
	}
	idx := int(in.stack[len(in.stack)-1]) + bias
	in.stack = in.stack[:len(in.stack)-1]
	if idx >= 0 && idx < len(subrs) {
		in.run(subrs[idx], depth+1)
	}
}

// countStems adds the pending stem hints (each is a pair) and consumes them,
// taking a leading width on the first hint operator.
func (in *t2interp) countStems() {
	if !in.widthParsed && len(in.stack)%2 == 1 {
		in.stack = in.stack[1:]
	}
	in.widthParsed = true
	in.nStems += len(in.stack) / 2
	in.clear()
}

// takeWidth strips a leading width argument when the stack holds one more than
// the operator's expected operand count.
func (in *t2interp) takeWidth(expected int) {
	if !in.widthParsed && len(in.stack) > expected {
		in.stack = in.stack[1:]
	}
	in.widthParsed = true
}

func (in *t2interp) moveTo(dx, dy float64) {
	if in.open {
		in.fl.close()
	}
	in.x += dx
	in.y += dy
	in.fl.moveTo(in.x, in.y)
	in.open = true
}

func (in *t2interp) lineTo(dx, dy float64) {
	in.x += dx
	in.y += dy
	in.fl.lineTo(in.x, in.y)
}

func (in *t2interp) curve(dx1, dy1, dx2, dy2, dx3, dy3 float64) {
	c1x, c1y := in.x+dx1, in.y+dy1
	c2x, c2y := c1x+dx2, c1y+dy2
	in.x, in.y = c2x+dx3, c2y+dy3
	in.fl.cubicTo(c1x, c1y, c2x, c2y, in.x, in.y)
}

func (in *t2interp) arg(i int) float64 {
	if i < len(in.stack) {
		return in.stack[i]
	}
	return 0
}
func (in *t2interp) clear() { in.stack = in.stack[:0] }

// --- CFF container primitives --------------------------------------------

// readCFFIndex reads an INDEX at pos and returns its entries and the position
// just past it.
func readCFFIndex(data []byte, pos int) ([][]byte, int, error) {
	if pos+2 > len(data) {
		return nil, pos, fmt.Errorf("parse cff: index out of range")
	}
	count := int(binary.BigEndian.Uint16(data[pos : pos+2]))
	pos += 2
	if count == 0 {
		return nil, pos, nil
	}
	if pos >= len(data) {
		return nil, pos, fmt.Errorf("parse cff: truncated index")
	}
	offSize := int(data[pos])
	pos++
	if offSize < 1 || offSize > 4 {
		return nil, pos, fmt.Errorf("parse cff: bad offSize %d", offSize)
	}
	readOff := func(p int) int {
		v := 0
		for k := 0; k < offSize; k++ {
			v = v<<8 | int(data[p+k])
		}
		return v
	}
	offEnd := pos + (count+1)*offSize
	if offEnd > len(data) {
		return nil, pos, fmt.Errorf("parse cff: truncated offsets")
	}
	offsets := make([]int, count+1)
	for k := 0; k <= count; k++ {
		offsets[k] = readOff(pos + k*offSize)
	}
	base := offEnd - 1 // offsets are 1-based from the byte before the data
	entries := make([][]byte, count)
	for k := 0; k < count; k++ {
		a, b := base+offsets[k], base+offsets[k+1]
		if a < 0 || b > len(data) || a > b {
			return nil, pos, fmt.Errorf("parse cff: bad index entry")
		}
		entries[k] = data[a:b]
	}
	return entries, base + offsets[count], nil
}

// cffOp encodes a DICT operator: one-byte op b0 (escape=-1 means two-byte 12 b1).
func cffOp(b0, b1 int) int {
	if b0 == 12 {
		return 1200 + b1
	}
	return b0
}

// parseCFFDict parses a DICT into operator → operand list.
func parseCFFDict(b []byte) map[int][]float64 {
	d := map[int][]float64{}
	var operands []float64
	i := 0
	for i < len(b) {
		c := b[i]
		switch {
		case c <= 21: // operator
			op := int(c)
			i++
			if c == 12 && i < len(b) {
				op = 1200 + int(b[i])
				i++
			}
			d[op] = append([]float64(nil), operands...)
			operands = operands[:0]
		case c == 28:
			operands = append(operands, float64(int16(binary.BigEndian.Uint16(b[i+1:i+3]))))
			i += 3
		case c == 29:
			operands = append(operands, float64(int32(binary.BigEndian.Uint32(b[i+1:i+5]))))
			i += 5
		case c == 30: // real
			v, n := cffReal(b[i+1:])
			operands = append(operands, v)
			i += 1 + n
		case c >= 32 && c <= 246:
			operands = append(operands, float64(int(c)-139))
			i++
		case c >= 247 && c <= 250:
			operands = append(operands, float64((int(c)-247)*256+int(b[i+1])+108))
			i += 2
		case c >= 251 && c <= 254:
			operands = append(operands, float64(-(int(c)-251)*256-int(b[i+1])-108))
			i += 2
		default:
			i++
		}
	}
	return d
}

// cffReal decodes a DICT real (BCD nibble) number; returns value and bytes used.
func cffReal(b []byte) (float64, int) {
	var s []byte
	n := 0
	for n < len(b) {
		hi, lo := b[n]>>4, b[n]&0xf
		n++
		done := false
		for _, nib := range []byte{hi, lo} {
			switch {
			case nib <= 9:
				s = append(s, '0'+nib)
			case nib == 0xa:
				s = append(s, '.')
			case nib == 0xb:
				s = append(s, 'E')
			case nib == 0xc:
				s = append(s, 'E', '-')
			case nib == 0xe:
				s = append(s, '-')
			case nib == 0xf:
				done = true
			}
			if done {
				break
			}
		}
		if done {
			break
		}
	}
	var v float64
	fmt.Sscanf(string(s), "%g", &v)
	return v, n
}

func intOp(d map[int][]float64, op int) int {
	if v, ok := d[op]; ok && len(v) > 0 {
		return int(v[len(v)-1])
	}
	return 0
}

// readPrivateSubrs reads the local Subrs INDEX referenced by a Private DICT at
// [off, off+size).
func readPrivateSubrs(data []byte, off, size int) [][]byte {
	if off <= 0 || off+size > len(data) {
		return nil
	}
	priv := parseCFFDict(data[off : off+size])
	subrsRel := intOp(priv, cffOp(19, 0))
	if subrsRel <= 0 {
		return nil
	}
	subrs, _, err := readCFFIndex(data, off+subrsRel)
	if err != nil {
		return nil
	}
	return subrs
}

// readFDSelect reads the FDSelect structure (format 0 or 3) → GID-indexed FD.
func readFDSelect(data []byte, off, nGlyphs int) []uint8 {
	if off >= len(data) {
		return nil
	}
	out := make([]uint8, nGlyphs)
	switch data[off] {
	case 0:
		for g := 0; g < nGlyphs && off+1+g < len(data); g++ {
			out[g] = data[off+1+g]
		}
	case 3:
		if off+3 > len(data) {
			return out
		}
		nRanges := int(binary.BigEndian.Uint16(data[off+1 : off+3]))
		p := off + 3
		for r := 0; r < nRanges && p+4 < len(data); r++ {
			first := int(binary.BigEndian.Uint16(data[p : p+2]))
			fd := data[p+2]
			next := int(binary.BigEndian.Uint16(data[p+3 : p+5]))
			for g := first; g < next && g < nGlyphs; g++ {
				out[g] = fd
			}
			p += 3
		}
	}
	return out
}

// readCharset reads the charset (formats 0/1/2) → GID-indexed CID/SID. Glyph 0
// (.notdef) is always 0 and not stored in the table.
func readCharset(data []byte, off, nGlyphs int) []uint16 {
	if off >= len(data) {
		return nil
	}
	out := make([]uint16, nGlyphs)
	format := data[off]
	p := off + 1
	gid := 1
	switch format {
	case 0:
		for ; gid < nGlyphs && p+1 < len(data); gid++ {
			out[gid] = binary.BigEndian.Uint16(data[p : p+2])
			p += 2
		}
	case 1, 2:
		for gid < nGlyphs && p+2 < len(data) {
			first := binary.BigEndian.Uint16(data[p : p+2])
			p += 2
			var nLeft int
			if format == 1 {
				nLeft = int(data[p])
				p++
			} else {
				nLeft = int(binary.BigEndian.Uint16(data[p : p+2]))
				p += 2
			}
			for k := 0; k <= nLeft && gid < nGlyphs; k++ {
				out[gid] = first + uint16(k)
				gid++
			}
		}
	}
	return out
}

// t2Number decodes a Type2 charstring operand; returns value and bytes consumed.
func t2Number(b []byte) (float64, int) {
	c := b[0]
	switch {
	case c == 28:
		return float64(int16(binary.BigEndian.Uint16(b[1:3]))), 3
	case c < 247: // 32..246
		return float64(int(c) - 139), 1
	case c < 251: // 247..250
		return float64((int(c)-247)*256 + int(b[1]) + 108), 2
	case c < 255: // 251..254
		return float64(-(int(c)-251)*256 - int(b[1]) - 108), 2
	default: // 255: 16.16 fixed
		return float64(int32(binary.BigEndian.Uint32(b[1:5]))) / 65536, 5
	}
}

// subrBias is the Type2 subroutine index bias for a subr count.
func subrBias(n int) int {
	switch {
	case n < 1240:
		return 107
	case n < 33900:
		return 1131
	default:
		return 32768
	}
}

func abs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}
