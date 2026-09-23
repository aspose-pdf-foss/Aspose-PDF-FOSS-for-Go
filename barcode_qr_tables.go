// SPDX-License-Identifier: MIT

package asposepdf

// qrBlockSpec describes how a QR version's data+EC codewords split into
// blocks (ISO/IEC 18004 Table 9), for error-correction level M only — the
// only level this encoder writes (see barcode_qr.go). Some versions split
// data into two groups of differently-sized blocks (g1 = smaller blocks,
// g2 = larger); versions with only one group leave g2Blocks at 0.
type qrBlockSpec struct {
	ecPerBlock       int
	g1Blocks, g1Data int
	g2Blocks, g2Data int
}

func (s qrBlockSpec) totalDataCodewords() int {
	return s.g1Blocks*s.g1Data + s.g2Blocks*s.g2Data
}

// qrBlockTableM is ISO/IEC 18004 Table 9, error-correction level M column,
// indexed by version-1 (versions 1-40). Each row's arithmetic
// (dataCodewords + ecPerBlock*numBlocks) reproduces the version's total
// codeword count independently of this table, which is asserted in
// barcode_qr_test.go as a structural cross-check.
var qrBlockTableM = [40]qrBlockSpec{
	{10, 1, 16, 0, 0},    // 1
	{16, 1, 28, 0, 0},    // 2
	{26, 1, 44, 0, 0},    // 3
	{18, 2, 32, 0, 0},    // 4
	{24, 2, 43, 0, 0},    // 5
	{16, 4, 27, 0, 0},    // 6
	{18, 4, 31, 0, 0},    // 7
	{22, 2, 38, 2, 39},   // 8
	{22, 3, 36, 2, 37},   // 9
	{26, 4, 43, 1, 44},   // 10
	{30, 1, 50, 4, 51},   // 11
	{22, 6, 36, 2, 37},   // 12
	{22, 8, 37, 1, 38},   // 13
	{24, 4, 40, 5, 41},   // 14
	{24, 5, 41, 5, 42},   // 15
	{28, 7, 45, 3, 46},   // 16
	{28, 10, 46, 1, 47},  // 17
	{26, 9, 43, 4, 44},   // 18
	{26, 3, 44, 11, 45},  // 19
	{26, 3, 41, 13, 42},  // 20
	{26, 17, 42, 0, 0},   // 21
	{28, 17, 46, 0, 0},   // 22
	{28, 4, 47, 14, 48},  // 23
	{28, 6, 45, 14, 46},  // 24
	{28, 8, 47, 13, 48},  // 25
	{28, 19, 46, 4, 47},  // 26
	{28, 22, 45, 3, 46},  // 27
	{28, 3, 45, 23, 46},  // 28
	{28, 21, 45, 7, 46},  // 29
	{28, 19, 47, 10, 48}, // 30
	{28, 2, 46, 29, 47},  // 31
	{28, 10, 46, 23, 47}, // 32
	{28, 14, 46, 21, 47}, // 33
	{28, 14, 46, 23, 47}, // 34
	{28, 12, 47, 26, 48}, // 35
	{28, 6, 47, 34, 48},  // 36
	{28, 29, 46, 14, 47}, // 37
	{28, 13, 46, 32, 47}, // 38
	{28, 40, 47, 7, 48},  // 39
	{28, 18, 47, 31, 48}, // 40
}

// qrRemainderBits returns the number of extra zero bits appended after all
// data+EC codeword bits before laying them into the module grid (ISO/IEC
// 18004 Table 1) — some versions' data-module count isn't a whole number of
// bytes.
func qrRemainderBits(version int) int {
	switch {
	case version == 1:
		return 0
	case version <= 6:
		return 7
	case version <= 13:
		return 0
	case version <= 20:
		return 3
	case version <= 27:
		return 4
	case version <= 34:
		return 3
	default: // 35-40
		return 0
	}
}
