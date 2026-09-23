// SPDX-License-Identifier: MIT

package asposepdf

import (
	"strings"
	"testing"
)

// qrTotalCodewordsByVersion is ISO/IEC 18004's version-only (not tied to any
// error-correction level) total-codeword-count table, independently
// transcribed from qrBlockTableM. Every row of qrBlockTableM must reproduce
// the matching entry here (dataCodewords + ecPerBlock*numBlocks), which is
// the cross-check this package leans on for confidence in that table (see
// its doc comment) since there is no reference QR decoder available to
// validate against instead.
var qrTotalCodewordsByVersion = [40]int{
	26, 44, 70, 100, 134, 172, 196, 242, 292, 346,
	404, 466, 532, 581, 655, 733, 815, 901, 991, 1085,
	1156, 1258, 1364, 1474, 1588, 1706, 1828, 1921, 2051, 2185,
	2323, 2465, 2611, 2761, 2876, 3034, 3196, 3362, 3532, 3706,
}

func TestQRBlockTableArithmetic(t *testing.T) {
	for i, spec := range qrBlockTableM {
		numBlocks := spec.g1Blocks + spec.g2Blocks
		total := spec.totalDataCodewords() + spec.ecPerBlock*numBlocks
		want := qrTotalCodewordsByVersion[i]
		if total != want {
			t.Errorf("version %d: data(%d)+ec(%d*%d)=%d, want total %d",
				i+1, spec.totalDataCodewords(), spec.ecPerBlock, numBlocks, total, want)
		}
	}
}

// qrBitReader reads MSB-first bits from a byte slice, used only by the
// test-only decoder below.
type qrBitReader struct {
	data []byte
	pos  int
}

func (r *qrBitReader) read(n int) int {
	v := 0
	for i := 0; i < n; i++ {
		byteIdx := r.pos / 8
		bitIdx := 7 - r.pos%8
		bit := (r.data[byteIdx] >> uint(bitIdx)) & 1
		v = v<<1 | int(bit)
		r.pos++
	}
	return v
}

// readQRDataBits mirrors placeData's zigzag traversal to read module values
// back out in the same order they were written — the same traversal order
// any QR decoder needs, since it's dictated by the symbol format, not by
// this encoder's implementation choices.
func readQRDataBits(m *qrMatrix) []bool {
	var bits []bool
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
				bits = append(bits, m.bits[m.idx(r, c)])
			}
		}
		upward = !upward
	}
	return bits
}

// decodeQRModules is an independent test-only decoder: it recomputes the
// function-pattern geometry from the module count (as any real decoder
// does — that geometry isn't carried in the symbol), extracts and
// interprets the format-info bits, unmasks, reads codewords back in
// zigzag order, de-interleaves per the block table, verifies every block's
// Reed-Solomon syndromes are zero (a strong signal: a masking, zigzag,
// interleaving, or GF(256)/generator-polynomial bug would need a
// coincidence to still produce all-zero syndromes), and finally parses the
// byte-mode payload. It shares low-risk pure-geometry/pure-formula helpers
// with the encoder (finder/timing/alignment placement, the mask
// conditions, the block-size table) — those must agree by definition on
// both sides of any QR implementation — while the extraction, unmasking,
// de-interleaving, and payload framing are written fresh.
func decodeQRModules(t *testing.T, mat barcodeModules) []byte {
	t.Helper()
	if !mat.Square {
		t.Fatal("decodeQRModules: matrix is not square (QR)")
	}
	const quiet = 4
	size := mat.Cols - quiet*2
	version := (size - 17) / 4
	if 4*version+17 != size {
		t.Fatalf("module count %d is not 17+4*version for any version", size)
	}

	raw := make([]bool, size*size)
	for r := 0; r < size; r++ {
		for c := 0; c < size; c++ {
			raw[r*size+c] = mat.Bits[(r+quiet)*mat.Cols+(c+quiet)]
		}
	}

	m := newQRMatrix(size)
	placeFinderPattern(m, 0, 0)
	placeFinderPattern(m, 0, size-7)
	placeFinderPattern(m, size-7, 0)
	placeTimingPatterns(m)
	reserveFormatAreas(m)
	reserveVersionAreas(m, version)
	placeAlignmentPatterns(m, version)
	copy(m.bits, raw) // overwrite synthetic function pixels with the actual scanned image; m.reserved is unaffected

	// Format-info extraction is written independently from writeFormatInfo,
	// against the (x, y) = (column, row) module-coordinate convention a
	// reference implementation documents, rather than by mirroring that
	// function's (row, col) calls — a self-mirroring reader here is exactly
	// what let an earlier, fully transposed writeFormatInfo pass every
	// test while writing every format bit to the wrong physical module.
	xy := func(x, y int) bool { return m.bits[m.idx(y, x)] } // (col, row) -> stored (row, col)
	fmtBits := 0
	for i := 0; i <= 5; i++ {
		if xy(8, i) {
			fmtBits |= 1 << uint(i)
		}
	}
	if xy(8, 7) {
		fmtBits |= 1 << 6
	}
	if xy(8, 8) {
		fmtBits |= 1 << 7
	}
	if xy(7, 8) {
		fmtBits |= 1 << 8
	}
	for i := 9; i <= 14; i++ {
		if xy(14-i, 8) {
			fmtBits |= 1 << uint(i)
		}
	}
	unmasked := fmtBits ^ 0x5412
	data := unmasked >> 10 // top 5 bits: 2-bit EC indicator, 3-bit mask ID
	maskID := data & 0b111
	ecIndicator := (data >> 3) & 0b11
	if ecIndicator != qrFormatECIndicatorM {
		t.Fatalf("format info EC indicator = %d, want %d (level M)", ecIndicator, qrFormatECIndicatorM)
	}

	for r := 0; r < size; r++ {
		for c := 0; c < size; c++ {
			if m.isReserved(r, c) {
				continue
			}
			if qrMaskCondition(maskID, r, c) {
				m.bits[m.idx(r, c)] = !m.bits[m.idx(r, c)]
			}
		}
	}

	dataBits := readQRDataBits(m)
	rem := qrRemainderBits(version)
	if rem > len(dataBits) {
		t.Fatalf("remainder bits (%d) exceed data bits (%d)", rem, len(dataBits))
	}
	dataBits = dataBits[:len(dataBits)-rem]
	if len(dataBits)%8 != 0 {
		t.Fatalf("data bit count %d is not a multiple of 8", len(dataBits))
	}
	codewords := make([]byte, len(dataBits)/8)
	for i := range codewords {
		var b byte
		for j := 0; j < 8; j++ {
			if dataBits[i*8+j] {
				b |= 1 << uint(7-j)
			}
		}
		codewords[i] = b
	}

	spec := qrBlockTableM[version-1]
	numBlocks := spec.g1Blocks + spec.g2Blocks
	blockLen := make([]int, numBlocks)
	for i := 0; i < spec.g1Blocks; i++ {
		blockLen[i] = spec.g1Data
	}
	for i := 0; i < spec.g2Blocks; i++ {
		blockLen[spec.g1Blocks+i] = spec.g2Data
	}
	maxData := spec.g1Data
	if spec.g2Data > maxData {
		maxData = spec.g2Data
	}

	blockData := make([][]byte, numBlocks)
	pos := 0
	for i := 0; i < maxData; i++ {
		for b := 0; b < numBlocks; b++ {
			if i < blockLen[b] {
				blockData[b] = append(blockData[b], codewords[pos])
				pos++
			}
		}
	}
	blockEC := make([][]byte, numBlocks)
	for i := 0; i < spec.ecPerBlock; i++ {
		for b := 0; b < numBlocks; b++ {
			blockEC[b] = append(blockEC[b], codewords[pos])
			pos++
		}
	}
	if pos != len(codewords) {
		t.Fatalf("de-interleaving consumed %d codewords, have %d", pos, len(codewords))
	}

	var allData []byte
	for b := 0; b < numBlocks; b++ {
		full := append(append([]byte{}, blockData[b]...), blockEC[b]...)
		if !qrRSSyndromesZero(full, spec.ecPerBlock) {
			t.Fatalf("block %d: Reed-Solomon syndromes are not all zero", b)
		}
		allData = append(allData, blockData[b]...)
	}

	br := &qrBitReader{data: allData}
	mode := br.read(4)
	if mode != 0b0100 {
		t.Fatalf("mode indicator = %04b, want 0100 (byte mode)", mode)
	}
	ccBits := 8
	if version >= 10 {
		ccBits = 16
	}
	length := br.read(ccBits)
	if length > len(allData) {
		t.Fatalf("declared length %d exceeds available data codewords %d", length, len(allData))
	}
	payload := make([]byte, length)
	for i := range payload {
		payload[i] = byte(br.read(8))
	}
	return payload
}

func TestQRRoundTrip(t *testing.T) {
	cases := []string{
		"HELLO",
		"https://example.com/",
		"The quick brown fox jumps over the lazy dog.",
		strings.Repeat("QR code stress test data. ", 30), // forces a multi-block, higher version
		"日本語テキスト",                                        // non-ASCII UTF-8 bytes
	}
	for _, s := range cases {
		mat, err := encodeQRModules(s)
		if err != nil {
			t.Fatalf("encodeQRModules(%.20q...): %v", s, err)
		}
		got := decodeQRModules(t, mat)
		if string(got) != s {
			t.Errorf("round trip mismatch for %.20q...: got %.20q...", s, got)
		}
	}
}

func TestQRRejectsEmpty(t *testing.T) {
	if _, err := encodeQRModules(""); err == nil {
		t.Error("expected an error for empty input")
	}
}

func TestQRChooseVersionMonotonic(t *testing.T) {
	prevVersion := 0
	for n := 1; n <= 400; n += 7 {
		v, spec, err := qrChooseVersion(n)
		if err != nil {
			t.Fatalf("qrChooseVersion(%d): %v", n, err)
		}
		if v < prevVersion {
			t.Errorf("qrChooseVersion(%d) = %d, went backwards from %d", n, v, prevVersion)
		}
		ccBits := 8
		if v >= 10 {
			ccBits = 16
		}
		if spec.totalDataCodewords()*8 < 4+ccBits+n*8 {
			t.Errorf("qrChooseVersion(%d) = %d: capacity too small", n, v)
		}
		prevVersion = v
	}
}

func TestQRTooLargeErrors(t *testing.T) {
	huge := strings.Repeat("x", 5000)
	if _, err := encodeQRModules(huge); err == nil {
		t.Error("expected an error for data exceeding version 40 capacity")
	}
}
