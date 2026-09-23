// SPDX-License-Identifier: MIT

package asposepdf

import "fmt"

// QR Code encoder (ISO/IEC 18004). Scope: byte mode only (mode indicator
// 0100), the raw UTF-8 bytes of the value with no ECI designator — the
// same "just encode the bytes" approach most general-purpose QR encoders
// default to for arbitrary text (numeric/alphanumeric mode is a size
// optimisation, not a capability this omits: any string byte mode can
// encode). Error-correction level is fixed at M (~15% recovery); version
// (1-40) is chosen automatically as the smallest that fits the data.
// Reed-Solomon error correction and mask selection are fully algorithmic
// (barcode_qr_gf256.go); the per-version block-size table is the one piece
// of spec data this file depends on (barcode_qr_tables.go).

const qrFormatECIndicatorM = 0 // format-info EC-level field for level M (ISO 18004 Table 25 — M=00, L=01, H=10, Q=11)

// qrMatrix is the working module grid during encoding: bits holds module
// colour (true = dark), reserved marks cells that are function patterns,
// separators, format/version info, or the fixed dark module — never
// touched by data placement or masking.
type qrMatrix struct {
	size     int
	bits     []bool
	reserved []bool
}

func newQRMatrix(size int) *qrMatrix {
	return &qrMatrix{size: size, bits: make([]bool, size*size), reserved: make([]bool, size*size)}
}

func (m *qrMatrix) idx(r, c int) int         { return r*m.size + c }
func (m *qrMatrix) set(r, c int, v bool)     { m.bits[m.idx(r, c)] = v }
func (m *qrMatrix) reserve(r, c int)         { m.reserved[m.idx(r, c)] = true }
func (m *qrMatrix) isReserved(r, c int) bool { return m.reserved[m.idx(r, c)] }

// qrBitWriter accumulates a sequence of MSB-first bits, later packed into
// bytes by bytes().
type qrBitWriter struct{ bits []bool }

func (w *qrBitWriter) writeBits(value, n int) {
	for i := n - 1; i >= 0; i-- {
		w.bits = append(w.bits, (value>>uint(i))&1 != 0)
	}
}

func (w *qrBitWriter) len() int { return len(w.bits) }

func (w *qrBitWriter) bytes() []byte {
	out := make([]byte, len(w.bits)/8)
	for i, bit := range w.bits {
		if bit {
			out[i/8] |= 1 << uint(7-i%8)
		}
	}
	return out
}

// encodeQRModules encodes value (as raw bytes, QR byte mode) into a square
// module matrix with quiet zone. Errors if value is empty or too large for
// version 40 at error-correction level M.
func encodeQRModules(value string) (barcodeModules, error) {
	if value == "" {
		return barcodeModules{}, fmt.Errorf("asposepdf: QR barcode value is empty")
	}
	data := []byte(value)
	version, spec, err := qrChooseVersion(len(data))
	if err != nil {
		return barcodeModules{}, err
	}

	codewords := qrBuildCodewords(data, version, spec)
	bits := qrCodewordsToBits(codewords, version)

	size := 4*version + 17
	m := newQRMatrix(size)
	placeFinderPattern(m, 0, 0)
	placeFinderPattern(m, 0, size-7)
	placeFinderPattern(m, size-7, 0)
	placeTimingPatterns(m)
	reserveFormatAreas(m)
	reserveVersionAreas(m, version)
	placeAlignmentPatterns(m, version)

	if err := placeData(m, bits); err != nil {
		return barcodeModules{}, err
	}

	base := append([]bool(nil), m.bits...)
	bestMask, bestScore := 0, -1
	for mask := 0; mask < 8; mask++ {
		copy(m.bits, base)
		qrApplyMask(m, mask)
		writeFormatInfo(m, mask)
		if version >= 7 {
			writeVersionInfo(m, version)
		}
		score := qrPenaltyScore(m)
		if bestScore < 0 || score < bestScore {
			bestScore = score
			bestMask = mask
		}
	}
	copy(m.bits, base)
	qrApplyMask(m, bestMask)
	writeFormatInfo(m, bestMask)
	if version >= 7 {
		writeVersionInfo(m, version)
	}

	return qrMatrixToModules(m), nil
}

// qrChooseVersion returns the smallest version (1-40) whose data-codeword
// capacity at level M fits the byte-mode header (4-bit mode indicator +
// 8- or 16-bit character count, depending on version) plus byteLen bytes.
func qrChooseVersion(byteLen int) (int, qrBlockSpec, error) {
	for v := 1; v <= 40; v++ {
		spec := qrBlockTableM[v-1]
		ccBits := 8
		if v >= 10 {
			ccBits = 16
		}
		needed := 4 + ccBits + byteLen*8
		if spec.totalDataCodewords()*8 >= needed {
			return v, spec, nil
		}
	}
	return 0, qrBlockSpec{}, fmt.Errorf("asposepdf: QR barcode value too large (%d bytes) for version 40 at error-correction level M", byteLen)
}

// qrBuildDataCodewords assembles the mode/length/data/terminator/padding
// bit stream and packs it into exactly spec.totalDataCodewords() bytes
// (ISO/IEC 18004 §7.4.9-7.4.10).
func qrBuildDataCodewords(data []byte, version int, spec qrBlockSpec) []byte {
	w := &qrBitWriter{}
	w.writeBits(0b0100, 4) // byte mode
	ccBits := 8
	if version >= 10 {
		ccBits = 16
	}
	w.writeBits(len(data), ccBits)
	for _, b := range data {
		w.writeBits(int(b), 8)
	}

	totalDataBits := spec.totalDataCodewords() * 8
	term := totalDataBits - w.len()
	if term > 4 {
		term = 4
	}
	if term > 0 {
		w.writeBits(0, term)
	}
	for w.len()%8 != 0 {
		w.writeBits(0, 1)
	}
	pad := [2]int{0xEC, 0x11}
	for i := 0; w.len() < totalDataBits; i++ {
		w.writeBits(pad[i%2], 8)
	}
	return w.bytes()
}

// qrBuildCodewords returns the final, block-interleaved data+EC codeword
// sequence (ISO/IEC 18004 §7.5) ready to be split into bits and placed.
func qrBuildCodewords(data []byte, version int, spec qrBlockSpec) []byte {
	dataCodewords := qrBuildDataCodewords(data, version, spec)

	type block struct{ data, ec []byte }
	var blocks []block
	offset := 0
	addGroup := func(count, size int) {
		for i := 0; i < count; i++ {
			d := dataCodewords[offset : offset+size]
			offset += size
			blocks = append(blocks, block{data: d, ec: qrRSEncode(d, spec.ecPerBlock)})
		}
	}
	addGroup(spec.g1Blocks, spec.g1Data)
	addGroup(spec.g2Blocks, spec.g2Data)

	maxData := spec.g1Data
	if spec.g2Data > maxData {
		maxData = spec.g2Data
	}
	out := make([]byte, 0, offset+spec.ecPerBlock*len(blocks))
	for i := 0; i < maxData; i++ {
		for _, b := range blocks {
			if i < len(b.data) {
				out = append(out, b.data[i])
			}
		}
	}
	for i := 0; i < spec.ecPerBlock; i++ {
		for _, b := range blocks {
			out = append(out, b.ec[i])
		}
	}
	return out
}

// qrCodewordsToBits expands codewords MSB-first into individual bits and
// appends the version's remainder bits (ISO/IEC 18004 Table 1).
func qrCodewordsToBits(codewords []byte, version int) []bool {
	bits := make([]bool, 0, len(codewords)*8+8)
	for _, b := range codewords {
		for i := 7; i >= 0; i-- {
			bits = append(bits, (b>>uint(i))&1 != 0)
		}
	}
	for i := 0; i < qrRemainderBits(version); i++ {
		bits = append(bits, false)
	}
	return bits
}

// placeFinderPattern draws a 7x7 finder pattern (concentric dark/light/dark
// squares) plus its one-module light separator, with top-left corner at
// (r0, c0). Every touched cell (including the separator) is reserved.
func placeFinderPattern(m *qrMatrix, r0, c0 int) {
	for dr := -1; dr <= 7; dr++ {
		for dc := -1; dc <= 7; dc++ {
			r, c := r0+dr, c0+dc
			if r < 0 || r >= m.size || c < 0 || c >= m.size {
				continue
			}
			dark := false
			if dr >= 0 && dr <= 6 && dc >= 0 && dc <= 6 {
				if dr == 0 || dr == 6 || dc == 0 || dc == 6 || (dr >= 2 && dr <= 4 && dc >= 2 && dc <= 4) {
					dark = true
				}
			}
			m.set(r, c, dark)
			m.reserve(r, c)
		}
	}
}

// placeTimingPatterns draws the alternating dark/light strips along row 6
// and column 6, skipping cells already reserved by a finder pattern.
func placeTimingPatterns(m *qrMatrix) {
	for i := 0; i < m.size; i++ {
		if !m.isReserved(6, i) {
			m.set(6, i, i%2 == 0)
			m.reserve(6, i)
		}
		if !m.isReserved(i, 6) {
			m.set(i, 6, i%2 == 0)
			m.reserve(i, 6)
		}
	}
}

// reserveFormatAreas reserves the two 15-bit format-info strips around the
// top-left finder pattern (without writing their bits — writeFormatInfo
// does that once the mask is chosen).
func reserveFormatAreas(m *qrMatrix) {
	for i := 0; i <= 8; i++ {
		m.reserve(8, i)
		m.reserve(i, 8)
	}
	for i := m.size - 8; i < m.size; i++ {
		m.reserve(8, i)
		m.reserve(i, 8)
	}
}

// reserveVersionAreas reserves the two 6x3 version-info blocks (version 7
// and up only).
func reserveVersionAreas(m *qrMatrix, version int) {
	if version < 7 {
		return
	}
	for i := 0; i < 18; i++ {
		a := m.size - 11 + i%3
		b := i / 3
		m.reserve(b, a)
		m.reserve(a, b)
	}
}

// placeAlignmentPatterns draws every alignment pattern for version, skipping
// the three positions that would overlap a finder pattern.
func placeAlignmentPatterns(m *qrMatrix, version int) {
	centers := qrAlignmentCenters(version)
	if len(centers) == 0 {
		return
	}
	first, last := centers[0], centers[len(centers)-1]
	for _, r := range centers {
		for _, c := range centers {
			if (r == first && c == first) || (r == first && c == last) || (r == last && c == first) {
				continue
			}
			placeAlignmentPattern(m, r, c)
		}
	}
}

// placeAlignmentPattern draws a 5x5 alignment pattern centred at (r, c).
func placeAlignmentPattern(m *qrMatrix, r, c int) {
	for dr := -2; dr <= 2; dr++ {
		for dc := -2; dc <= 2; dc++ {
			dark := dr == -2 || dr == 2 || dc == -2 || dc == 2 || (dr == 0 && dc == 0)
			m.set(r+dr, c+dc, dark)
			m.reserve(r+dr, c+dc)
		}
	}
}

// qrAlignmentCenters returns the row/column centre coordinates shared by
// every alignment pattern of version (ISO/IEC 18004 Annex E), computed from
// the module count rather than a per-version lookup table.
func qrAlignmentCenters(version int) []int {
	if version == 1 {
		return nil
	}
	numAlign := version/7 + 2
	size := 4*version + 17
	var step int
	if version == 32 {
		step = 26
	} else {
		step = (version*4 + numAlign*2 + 1) / (numAlign*2 - 2) * 2
	}
	centers := make([]int, numAlign)
	pos := size - 7
	for i := numAlign - 1; i >= 1; i-- {
		centers[i] = pos
		pos -= step
	}
	centers[0] = 6
	return centers
}

// placeData lays the given bits into every non-reserved module in the
// standard zigzag column-pair order (ISO/IEC 18004 §7.7.3), skipping the
// vertical timing column. Errors if the bit count doesn't exactly match the
// number of available data modules — a defensive internal-consistency check
// (a real mismatch would mean the block/remainder-bit tables are wrong).
func placeData(m *qrMatrix, bits []bool) error {
	bitIdx := 0
	upward := true
	for col := m.size - 1; col > 0; col -= 2 {
		if col == 6 {
			col--
		}
		for rowStep := 0; rowStep < m.size; rowStep++ {
			r := rowStep
			if upward {
				r = m.size - 1 - rowStep
			}
			for _, c := range [2]int{col, col - 1} {
				if m.isReserved(r, c) {
					continue
				}
				var bit bool
				if bitIdx < len(bits) {
					bit = bits[bitIdx]
				}
				bitIdx++
				m.set(r, c, bit)
			}
		}
		upward = !upward
	}
	if bitIdx != len(bits) {
		return fmt.Errorf("asposepdf: internal error: QR data-module count (%d) does not match codeword bit count (%d)", bitIdx, len(bits))
	}
	return nil
}

// applyMask XORs maskID's condition into every non-reserved module.
func qrApplyMask(m *qrMatrix, maskID int) {
	for r := 0; r < m.size; r++ {
		for c := 0; c < m.size; c++ {
			if m.isReserved(r, c) {
				continue
			}
			if qrMaskCondition(maskID, r, c) {
				m.bits[m.idx(r, c)] = !m.bits[m.idx(r, c)]
			}
		}
	}
}

// qrMaskCondition implements the 8 standard mask patterns (ISO/IEC 18004
// Table 10).
func qrMaskCondition(id, r, c int) bool {
	switch id {
	case 0:
		return (r+c)%2 == 0
	case 1:
		return r%2 == 0
	case 2:
		return c%3 == 0
	case 3:
		return (r+c)%3 == 0
	case 4:
		return (r/2+c/3)%2 == 0
	case 5:
		return (r*c)%2+(r*c)%3 == 0
	case 6:
		return ((r*c)%2+(r*c)%3)%2 == 0
	case 7:
		return ((r+c)%2+(r*c)%3)%2 == 0
	}
	return false
}

// qrFormatBits computes the 15-bit format-info value (5 data bits: EC level
// + 3-bit mask, plus a 10-bit BCH remainder, XORed with the fixed mask
// pattern 0x5412) for error-correction level M via GF(2) polynomial
// division against generator 0x537 (ISO/IEC 18004 §7.9 / Annex C).
func qrFormatBits(mask int) int {
	data := (qrFormatECIndicatorM << 3) | mask
	rem := data
	for i := 0; i < 10; i++ {
		rem = (rem << 1) ^ ((rem >> 9) * 0x537)
	}
	bits := (data<<10 | rem) & 0x7FFF
	return bits ^ 0x5412
}

// qrVersionBits computes the 18-bit version-info value (6-bit version + a
// 12-bit BCH remainder against generator 0x1F25, no XOR mask; ISO/IEC 18004
// Annex D). Only meaningful for version >= 7.
func qrVersionBits(version int) int {
	rem := version
	for i := 0; i < 12; i++ {
		rem = (rem << 1) ^ ((rem >> 11) * 0x1F25)
	}
	return version<<12 | rem
}

// writeFormatInfo writes the (unmasked-position) format-info bits into both
// copies around the top-left finder pattern, plus the fixed always-dark
// module (ISO/IEC 18004 Figure 25). Position mapping cross-checked against
// a reference implementation's (x, y) = (column, row) module coordinates —
// an earlier version of this function had every coordinate pair transposed
// (row/col swapped) except the two positions a transpose can't move, which
// a self-mirroring test round-trip could not catch (see barcode_qr_test.go).
func writeFormatInfo(m *qrMatrix, mask int) {
	bits := qrFormatBits(mask)
	size := m.size
	bit := func(i int) bool { return (bits>>uint(i))&1 != 0 }

	for i := 0; i <= 5; i++ {
		m.set(i, 8, bit(i))
	}
	m.set(7, 8, bit(6))
	m.set(8, 8, bit(7))
	m.set(8, 7, bit(8))
	for i := 9; i <= 14; i++ {
		m.set(8, 14-i, bit(i))
	}

	for i := 0; i <= 7; i++ {
		m.set(8, size-1-i, bit(i))
	}
	for i := 8; i <= 14; i++ {
		m.set(size-15+i, 8, bit(i))
	}

	m.set(size-8, 8, true) // always-dark module (overrides bit 7's copy-B slot)
}

// writeVersionInfo writes the 18-bit version-info value into its two 6x3
// blocks (version >= 7 only).
func writeVersionInfo(m *qrMatrix, version int) {
	bits := qrVersionBits(version)
	size := m.size
	for i := 0; i < 18; i++ {
		bit := (bits>>uint(i))&1 != 0
		a := size - 11 + i%3
		b := i / 3
		m.set(b, a, bit)
		m.set(a, b, bit)
	}
}

// qrPenaltyScore sums the four standard mask-evaluation penalties (ISO/IEC
// 18004 §7.8.3). Lower is better; used only to pick among 8 equally valid
// masks, so an imperfect score never affects correctness — every mask
// produces a fully scannable symbol once its format bits are written.
func qrPenaltyScore(m *qrMatrix) int {
	size := m.size
	score := 0

	for r := 0; r < size; r++ {
		row := r
		score += qrRunPenalty(func(i int) bool { return m.bits[m.idx(row, i)] }, size)
	}
	for c := 0; c < size; c++ {
		col := c
		score += qrRunPenalty(func(i int) bool { return m.bits[m.idx(i, col)] }, size)
	}

	for r := 0; r < size-1; r++ {
		for c := 0; c < size-1; c++ {
			v := m.bits[m.idx(r, c)]
			if m.bits[m.idx(r, c+1)] == v && m.bits[m.idx(r+1, c)] == v && m.bits[m.idx(r+1, c+1)] == v {
				score += 3
			}
		}
	}

	for r := 0; r < size; r++ {
		row := r
		score += qrFinderPenalty(func(i int) bool { return m.bits[m.idx(row, i)] }, size)
	}
	for c := 0; c < size; c++ {
		col := c
		score += qrFinderPenalty(func(i int) bool { return m.bits[m.idx(i, col)] }, size)
	}

	dark := 0
	for _, b := range m.bits {
		if b {
			dark++
		}
	}
	percent := dark * 100 / (size * size)
	prev := (percent / 5) * 5
	next := prev + 5
	dPrev, dNext := absInt(prev-50), absInt(next-50)
	worst := dPrev
	if dNext < worst {
		worst = dNext
	}
	score += (worst / 5) * 10

	return score
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// qrRunPenalty implements N1: 3 points for a same-colour run of exactly 5
// modules plus 1 for every module beyond that, along one row/column.
func qrRunPenalty(at func(int) bool, n int) int {
	score := 0
	runLen := 1
	for i := 1; i < n; i++ {
		if at(i) == at(i-1) {
			runLen++
			continue
		}
		if runLen >= 5 {
			score += runLen - 5 + 3
		}
		runLen = 1
	}
	if runLen >= 5 {
		score += runLen - 5 + 3
	}
	return score
}

var (
	qrFinderPenaltyPattern       = [11]bool{true, false, true, true, true, false, true, false, false, false, false}
	qrFinderPenaltyPatternMirror = [11]bool{false, false, false, false, true, false, true, true, true, false, true}
)

// qrFinderPenalty implements N3: 40 points for every occurrence, along one
// row/column, of the finder-pattern-like ratio 1:1:3:1:1 with four
// consecutive light modules on one side.
func qrFinderPenalty(at func(int) bool, n int) int {
	score := 0
	for i := 0; i+11 <= n; i++ {
		if qrMatchesPattern(at, i, qrFinderPenaltyPattern) || qrMatchesPattern(at, i, qrFinderPenaltyPatternMirror) {
			score += 40
		}
	}
	return score
}

func qrMatchesPattern(at func(int) bool, offset int, pattern [11]bool) bool {
	for j, want := range pattern {
		if at(offset+j) != want {
			return false
		}
	}
	return true
}

// qrMatrixToModules wraps the finished matrix with a 4-module quiet zone
// (ISO/IEC 18004 §6.3.8) and converts it to the shared barcodeModules type.
func qrMatrixToModules(m *qrMatrix) barcodeModules {
	const quiet = 4
	size := m.size
	total := size + quiet*2
	bits := make([]bool, total*total)
	for r := 0; r < size; r++ {
		for c := 0; c < size; c++ {
			if m.bits[m.idx(r, c)] {
				bits[(r+quiet)*total+(c+quiet)] = true
			}
		}
	}
	return barcodeModules{Cols: total, Rows: total, Bits: bits, Square: true}
}
