// SPDX-License-Identifier: MIT

package asposepdf

import "fmt"

// CCITTFaxDecode (ITU-T T.4 / T.6) decompresses Group 3/4 fax data, the codec
// PDF uses for 1-bit scanned images and image masks (e.g. logos). This file
// implements Group 4 (pure two-dimensional, /K < 0) — by far the most common in
// PDF — plus Group 3 1-D (/K = 0). Mixed 1-D/2-D Group 3 (/K > 0) is not yet
// handled. Output is packed 1 bit per pixel, MSB-first, one row per scanline,
// 0 = white unless /BlackIs1 is true.

// ccittParams are the relevant /DecodeParms entries.
type ccittParams struct {
	k         int
	columns   int
	rows      int
	blackIs1  bool
	byteAlign bool
}

// ccittFilter reads the /DecodeParms (and the image /Height as a fallback for
// /Rows) and runs the decoder.
func ccittFilter(data []byte, parms, streamDict pdfDict) ([]byte, error) {
	p := ccittParams{columns: 1728}
	if parms != nil {
		if v, ok := parms["/K"]; ok {
			p.k = toInt(v)
		}
		if v, ok := parms["/Columns"]; ok {
			p.columns = toInt(v)
		}
		if v, ok := parms["/Rows"]; ok {
			p.rows = toInt(v)
		}
		p.blackIs1 = toBool(parms["/BlackIs1"])
		p.byteAlign = toBool(parms["/EncodedByteAlign"])
	}
	if p.rows <= 0 && streamDict != nil {
		if v, ok := streamDict["/Height"]; ok {
			p.rows = toInt(v)
		}
	}
	return ccittDecode(data, p)
}

// toBool coerces a pdfValue to bool (PDF boolean).
func toBool(v pdfValue) bool {
	b, _ := v.(bool)
	return b
}

// ccittDecode decompresses CCITT fax data into packed 1-bpp rows.
func ccittDecode(data []byte, p ccittParams) ([]byte, error) {
	if p.columns <= 0 {
		p.columns = 1728
	}
	if p.k > 0 {
		return nil, fmt.Errorf("CCITTFaxDecode: mixed Group 3 2-D (/K=%d) unsupported", p.k)
	}
	br := &ccittBits{data: data}
	cols := p.columns
	rowBytes := (cols + 7) / 8
	var out []byte

	// Reference line: changing-element positions, terminated by two cols. The
	// imaginary line above the first row is all white.
	ref := []int{cols, cols}
	rowsDone := 0
	for {
		if p.rows > 0 && rowsDone >= p.rows {
			break
		}
		if p.byteAlign {
			br.align()
		}
		cur, ok := ccittDecodeRow(br, ref, cols, p.k)
		if !ok {
			break
		}
		// Emit the row from its changing elements (runs alternate white→black…).
		row := make([]byte, rowBytes)
		color := byte(0) // 0 = white
		pos := 0
		for _, c := range cur {
			if c > cols {
				c = cols
			}
			if color == 1 {
				for x := pos; x < c; x++ {
					row[x>>3] |= 0x80 >> uint(x&7)
				}
			}
			pos = c
			color ^= 1
			if pos >= cols {
				break
			}
		}
		// PDF convention: a set bit is black only when /BlackIs1; default the
		// packed "1" we wrote for black must become 0 (black) unless BlackIs1.
		if !p.blackIs1 {
			for i := range row {
				row[i] = ^row[i]
			}
		}
		out = append(out, row...)
		rowsDone++
		ref = append(cur[:len(cur):len(cur)], cols, cols)
		if p.rows <= 0 && br.eof() {
			break
		}
		if rowsDone > 1<<20 { // safety
			break
		}
	}
	return out, nil
}

// ccittDecodeRow decodes one scanline into a slice of changing-element columns.
// For Group 4 (k<0) it uses 2-D modes against the reference line; for Group 3
// 1-D (k==0) it reads alternating white/black runs directly.
func ccittDecodeRow(br *ccittBits, ref []int, cols, k int) ([]int, bool) {
	if br.eof() {
		return nil, false
	}
	var cur []int
	a0 := -1
	color := 0 // 0 white, 1 black

	if k == 0 { // Group 3 one-dimensional
		// G3 lines are delimited by EOL codes (000000000001), optionally
		// preceded by fill bits and possibly several in a row (a leading EOL,
		// and the 6-EOL RTC at end). Consume them before reading runs;
		// without this the very first row hits an EOL where a white run is
		// expected and decoding aborts to a black page (39646.pdf).
		for br.skipEOL() {
		}
		if br.eof() {
			return nil, false
		}
		pos := 0
		for pos < cols {
			run, ok := ccittRun(br, color)
			if !ok {
				if pos == 0 {
					return nil, false
				}
				break
			}
			pos += run
			if pos > cols {
				pos = cols
			}
			cur = append(cur, pos)
			color ^= 1
		}
		return cur, true
	}

	for a0 < cols {
		mode, ok := ccittMode(br)
		if !ok {
			if len(cur) == 0 {
				return nil, false
			}
			break
		}
		b1, b2 := ccittB1B2(ref, a0, color, cols)
		switch mode {
		case modePass:
			a0 = b2
		case modeHoriz:
			r1, ok1 := ccittRun(br, color)
			r2, ok2 := ccittRun(br, color^1)
			if !ok1 || !ok2 {
				return cur, len(cur) > 0
			}
			start := a0
			if start < 0 {
				start = 0
			}
			a1 := start + r1
			a2 := a1 + r2
			if a1 > cols {
				a1 = cols
			}
			if a2 > cols {
				a2 = cols
			}
			cur = append(cur, a1, a2)
			a0 = a2
		case modeV0, modeVR1, modeVR2, modeVR3, modeVL1, modeVL2, modeVL3:
			a1 := b1 + ccittVDelta(mode)
			if a1 < 0 {
				a1 = 0
			}
			if a1 > cols {
				a1 = cols
			}
			cur = append(cur, a1)
			a0 = a1
			color ^= 1
		default:
			return cur, len(cur) > 0
		}
	}
	return cur, true
}

// ccittB1B2 returns b1 (first changing element on the reference line right of a0
// with colour opposite to the current colour) and b2 (the next one).
func ccittB1B2(ref []int, a0, color, cols int) (int, int) {
	// Changing elements in ref alternate starting with the first being a white→
	// black transition (index 0 has colour "black to its right"). The colour to
	// the right of ref[i] is (i odd ? white : black); b1 must be a transition to
	// the colour opposite the current coding colour, i.e. its index parity must
	// make the element a changing element of colour != color.
	i := 0
	for i < len(ref) && ref[i] <= a0 {
		i++
	}
	// ref[i] is the first changing element strictly right of a0. Ensure its
	// colour (parity) is opposite to current: element index parity even means a
	// transition from white→black (b1 for current white). If parity mismatches,
	// step one more.
	if (i & 1) != color {
		i++
	}
	b1 := cols
	if i < len(ref) {
		b1 = ref[i]
	}
	b2 := cols
	if i+1 < len(ref) {
		b2 = ref[i+1]
	}
	return b1, b2
}

// ccitt 2-D mode constants.
const (
	modePass = iota
	modeHoriz
	modeV0
	modeVR1
	modeVR2
	modeVR3
	modeVL1
	modeVL2
	modeVL3
)

func ccittVDelta(mode int) int {
	switch mode {
	case modeVR1:
		return 1
	case modeVR2:
		return 2
	case modeVR3:
		return 3
	case modeVL1:
		return -1
	case modeVL2:
		return -2
	case modeVL3:
		return -3
	}
	return 0 // V0
}

// ccittMode reads a 2-D mode code.
func ccittMode(br *ccittBits) (int, bool) {
	if br.bit() == 1 {
		return modeV0, true // 1
	}
	// 0…
	if br.bit() == 1 { // 01x
		if br.bit() == 1 {
			return modeVR1, true // 011
		}
		return modeVL1, true // 010
	}
	if br.bit() == 1 { // 001
		return modeHoriz, true
	}
	if br.bit() == 1 { // 0001
		return modePass, true
	}
	if br.bit() == 1 { // 00001x
		if br.bit() == 1 {
			return modeVR2, true // 000011
		}
		return modeVL2, true // 000010
	}
	if br.bit() == 1 { // 000001x
		if br.bit() == 1 {
			return modeVR3, true // 0000011
		}
		return modeVL3, true // 0000010
	}
	return 0, false // extension / EOL / end of data
}

// ccittRun reads a complete run length (makeup codes accumulate, terminating
// code < 64 ends it) for the given colour (0 white, 1 black).
func ccittRun(br *ccittBits, color int) (int, bool) {
	total := 0
	for {
		var v int
		var ok bool
		if color == 0 {
			v, ok = ccittReadCode(br, whiteCodes)
		} else {
			v, ok = ccittReadCode(br, blackCodes)
		}
		if !ok {
			return 0, false
		}
		total += v
		if v < 64 { // terminating code
			return total, true
		}
		// makeup code (>=64): keep reading.
		if total > 1<<20 {
			return 0, false
		}
	}
}

// ccittReadCode matches the bit stream against a Huffman table, reading up to 14
// bits. The table maps key = (nbits<<16 | code) → run length.
func ccittReadCode(br *ccittBits, table map[uint32]int) (int, bool) {
	code := uint32(0)
	for n := 1; n <= 14; n++ {
		if br.eof() {
			return 0, false
		}
		code = code<<1 | uint32(br.bit())
		if v, ok := table[uint32(n)<<16|code]; ok {
			return v, true
		}
	}
	return 0, false
}

// ccittBits is an MSB-first bit reader over the encoded data.
type ccittBits struct {
	data []byte
	pos  int  // byte index
	bitn uint // bits consumed in current byte (0..7)
}

func (b *ccittBits) eof() bool { return b.pos >= len(b.data) }

func (b *ccittBits) bit() int {
	if b.pos >= len(b.data) {
		return 0
	}
	v := int(b.data[b.pos]>>(7-b.bitn)) & 1
	b.bitn++
	if b.bitn == 8 {
		b.bitn = 0
		b.pos++
	}
	return v
}

func (b *ccittBits) align() {
	if b.bitn != 0 {
		b.bitn = 0
		b.pos++
	}
}

// skipEOL consumes a single EOL code at the current position and reports
// whether one was present. An EOL is at least 11 zero bits (fill bits add
// more) terminated by a 1; no valid white/black run code carries 11 leading
// zeros, so the test is unambiguous. If the bits at the cursor are not an EOL,
// the cursor is restored and false is returned.
func (b *ccittBits) skipEOL() bool {
	savePos, saveBit := b.pos, b.bitn
	zeros := 0
	for !b.eof() {
		if b.bit() == 1 {
			if zeros >= 11 {
				return true
			}
			b.pos, b.bitn = savePos, saveBit
			return false
		}
		zeros++
		if zeros > 64 { // runaway guard (padding / trailing zeros)
			b.pos, b.bitn = savePos, saveBit
			return false
		}
	}
	b.pos, b.bitn = savePos, saveBit
	return false
}
