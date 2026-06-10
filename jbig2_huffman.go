// SPDX-License-Identifier: MIT

package asposepdf

// JBIG2 Huffman coding support (ITU-T T.88 Annex B): an MSB-first bit reader, a
// canonical-prefix Huffman table builder/decoder, and the 15 standard tables.
// Used by the Huffman symbol-dictionary and text-region paths (SDHUFF/SBHUFF).

// jbig2Reader reads bits MSB-first from a byte slice; it also supports the
// byte-alignment the MMR collective bitmaps need.
type jbig2Reader struct {
	data    []byte
	bytePos int
	bitPos  int // 0..7, counting from the MSB
}

func newJBIG2Reader(data []byte, start int) *jbig2Reader {
	return &jbig2Reader{data: data, bytePos: start}
}

func (r *jbig2Reader) readBit() int {
	if r.bytePos >= len(r.data) {
		return 0
	}
	b := int(r.data[r.bytePos]>>uint(7-r.bitPos)) & 1
	r.bitPos++
	if r.bitPos == 8 {
		r.bitPos = 0
		r.bytePos++
	}
	return b
}

func (r *jbig2Reader) readBits(n int) int {
	v := 0
	for i := 0; i < n; i++ {
		v = v<<1 | r.readBit()
	}
	return v
}

func (r *jbig2Reader) align() {
	if r.bitPos != 0 {
		r.bitPos = 0
		r.bytePos++
	}
}

// huffLine is one table entry: a prefix of prefLen bits, then rangeLen value bits
// added to (or, for isLower, subtracted from) rangeLow. isOOB marks the
// out-of-band line.
type huffLine struct {
	prefLen, rangeLen, rangeLow int
	isLower, isOOB              bool
	code                        int
}

type huffTable struct{ lines []huffLine }

// newHuffTable assigns canonical prefix codes to the lines (ITU-T T.88 §B.3).
func newHuffTable(lines []huffLine) *huffTable {
	maxLen := 0
	for _, l := range lines {
		if l.prefLen > maxLen {
			maxLen = l.prefLen
		}
	}
	lenCount := make([]int, maxLen+1)
	for _, l := range lines {
		if l.prefLen > 0 {
			lenCount[l.prefLen]++
		}
	}
	firstCode := make([]int, maxLen+2)
	for curLen := 1; curLen <= maxLen; curLen++ {
		firstCode[curLen] = (firstCode[curLen-1] + lenCount[curLen-1]) << 1
		cur := firstCode[curLen]
		for i := range lines {
			if lines[i].prefLen == curLen {
				lines[i].code = cur
				cur++
			}
		}
	}
	return &huffTable{lines: lines}
}

// decode reads one value (ITU-T T.88 §B.4); oob is true for the OOB symbol.
func (t *huffTable) decode(r *jbig2Reader) (value int, oob bool) {
	code, length := 0, 0
	for length < 32 {
		code = code<<1 | r.readBit()
		length++
		for _, l := range t.lines {
			if l.prefLen == length && l.code == code {
				if l.isOOB {
					return 0, true
				}
				off := r.readBits(l.rangeLen)
				if l.isLower {
					return l.rangeLow - off, false
				}
				return l.rangeLow + off, false
			}
		}
	}
	return 0, false
}

// Standard Huffman tables B.1–B.15 (ITU-T T.88 Annex B). Each line is
// {prefLen, rangeLen, rangeLow, isLower, isOOB}; a rangeLen of 32 with the
// default sign is an upper range (rangeLow + 32-bit offset), with isLower a
// lower range (rangeLow − 32-bit offset).
var jbig2StdTables = map[int]*huffTable{}

func init() {
	mk := func(n int, lines []huffLine) { jbig2StdTables[n] = newHuffTable(lines) }
	L := func(p, rl, lo int) huffLine { return huffLine{prefLen: p, rangeLen: rl, rangeLow: lo} }
	low := func(p, rl, lo int) huffLine { return huffLine{prefLen: p, rangeLen: rl, rangeLow: lo, isLower: true} }
	oob := func(p int) huffLine { return huffLine{prefLen: p, isOOB: true} }

	mk(1, []huffLine{L(1, 4, 0), L(2, 8, 16), L(3, 16, 272), L(3, 32, 65808)})
	mk(2, []huffLine{L(1, 0, 0), L(2, 0, 1), L(3, 0, 2), L(4, 3, 3), L(5, 6, 11), L(6, 32, 75), oob(6)})
	mk(3, []huffLine{L(8, 8, -256), L(1, 0, 0), L(2, 0, 1), L(3, 0, 2), L(4, 3, 3), L(5, 6, 11), low(8, 32, -257), L(7, 32, 75), oob(6)})
	mk(4, []huffLine{L(1, 0, 1), L(2, 0, 2), L(3, 0, 3), L(4, 3, 4), L(5, 6, 12), L(5, 32, 76)})
	mk(5, []huffLine{L(7, 8, -255), L(1, 0, 1), L(2, 0, 2), L(3, 0, 3), L(4, 3, 4), L(5, 6, 12), low(7, 32, -256), L(6, 32, 76)})
	mk(6, []huffLine{
		L(5, 10, -2048), L(4, 9, -1024), L(4, 8, -512), L(4, 7, -256), L(5, 6, -128), L(5, 5, -64), L(4, 5, -32),
		L(2, 7, 0), L(3, 7, 128), L(3, 8, 256), L(4, 9, 512), L(4, 10, 1024), low(6, 32, -2049), L(6, 32, 2048),
	})
	mk(7, []huffLine{
		L(4, 9, -1024), L(3, 8, -512), L(4, 7, -256), L(5, 6, -128), L(5, 5, -64), L(4, 5, -32), L(4, 5, 0),
		L(5, 5, 32), L(5, 6, 64), L(4, 7, 128), L(3, 8, 256), L(3, 9, 512), L(3, 10, 1024), low(5, 32, -1025), L(5, 32, 2048),
	})
	mk(8, []huffLine{
		L(8, 3, -15), L(9, 1, -7), L(8, 1, -5), L(9, 0, -3), L(7, 0, -2), L(4, 0, -1), L(2, 1, 0), L(5, 0, 2),
		L(6, 0, 3), L(3, 4, 4), L(6, 1, 20), L(4, 4, 22), L(4, 5, 38), L(5, 6, 70), L(5, 7, 134), L(6, 7, 262),
		L(7, 8, 390), L(6, 10, 646), low(9, 32, -16), L(9, 32, 1670), oob(2),
	})
	mk(9, []huffLine{
		L(8, 4, -31), L(9, 2, -15), L(8, 2, -11), L(9, 1, -7), L(7, 1, -5), L(4, 1, -3), L(3, 1, -1), L(3, 1, 1),
		L(5, 1, 3), L(6, 1, 5), L(3, 5, 7), L(6, 2, 39), L(4, 5, 43), L(4, 6, 75), L(5, 7, 139), L(5, 8, 267),
		L(6, 8, 523), L(7, 9, 779), L(6, 11, 1291), low(9, 32, -32), L(9, 32, 3339), oob(2),
	})
	mk(10, []huffLine{
		L(7, 4, -21), L(8, 0, -5), L(7, 0, -4), L(5, 0, -3), L(2, 2, -2), L(5, 0, 2), L(6, 0, 3), L(7, 0, 4),
		L(8, 0, 5), L(2, 6, 6), L(5, 5, 70), L(6, 5, 102), L(6, 6, 134), L(6, 7, 198), L(6, 8, 326), L(6, 9, 582),
		L(6, 10, 1094), L(7, 11, 2118), low(8, 32, -22), L(8, 32, 4166), oob(2),
	})
	mk(11, []huffLine{
		L(1, 0, 1), L(2, 1, 2), L(4, 0, 4), L(4, 1, 5), L(5, 1, 7), L(5, 2, 9), L(6, 2, 13), L(7, 2, 17),
		L(7, 3, 21), L(7, 4, 29), L(7, 5, 45), L(7, 6, 77), L(7, 32, 141),
	})
	mk(12, []huffLine{
		L(1, 0, 1), L(2, 0, 2), L(3, 1, 3), L(5, 0, 5), L(5, 1, 6), L(6, 1, 8), L(7, 0, 10), L(7, 1, 11),
		L(7, 2, 13), L(7, 3, 17), L(7, 4, 25), L(8, 5, 41), L(8, 32, 73),
	})
	mk(13, []huffLine{
		L(1, 0, 1), L(3, 0, 2), L(4, 0, 3), L(5, 0, 4), L(4, 1, 5), L(3, 3, 7), L(6, 1, 15), L(6, 2, 17),
		L(6, 3, 21), L(6, 4, 29), L(6, 5, 45), L(7, 6, 77), L(7, 32, 141),
	})
	mk(14, []huffLine{L(3, 0, -2), L(3, 0, -1), L(1, 0, 0), L(3, 0, 1), L(3, 0, 2)})
	mk(15, []huffLine{
		L(7, 4, -24), L(6, 2, -8), L(5, 1, -4), L(4, 0, -2), L(3, 0, -1), L(1, 0, 0), L(3, 0, 1), L(4, 0, 2),
		L(5, 1, 3), L(6, 2, 5), L(7, 4, 9), low(7, 32, -25), L(7, 32, 25),
	})
}
