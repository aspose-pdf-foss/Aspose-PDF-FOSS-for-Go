// SPDX-License-Identifier: MIT

package asposepdf

// GF(256) arithmetic and Reed-Solomon error-correction codeword generation
// for QR Code (ISO/IEC 18004 Annex A). Field generator polynomial
// x^8+x^4+x^3+x^2+1 (0x11D), primitive element 2. Everything here is
// computed algorithmically from that single constant — no per-version
// lookup table, so there is nothing beyond this one polynomial to get
// wrong by transcription.

var qrGFExp [512]byte // doubled so qrGFMul can index exp[log(a)+log(b)] without a modulo
var qrGFLog [256]byte

func init() {
	x := 1
	for i := 0; i < 255; i++ {
		qrGFExp[i] = byte(x)
		qrGFLog[x] = byte(i)
		x <<= 1
		if x&0x100 != 0 {
			x ^= 0x11D
		}
	}
	for i := 255; i < 512; i++ {
		qrGFExp[i] = qrGFExp[i-255]
	}
}

func qrGFMul(a, b byte) byte {
	if a == 0 || b == 0 {
		return 0
	}
	return qrGFExp[int(qrGFLog[a])+int(qrGFLog[b])]
}

// qrRSGeneratorPoly returns the degree-n generator polynomial (coefficients
// low-degree-first is NOT used here; index 0 is the highest-degree
// coefficient, always 1) for n EC codewords: the product over
// i=0..n-1 of (x - 2^i), computed in GF(256) where subtraction is XOR.
func qrRSGeneratorPoly(n int) []byte {
	poly := []byte{1}
	for i := 0; i < n; i++ {
		next := make([]byte, len(poly)+1)
		root := qrGFExp[i]
		for j, c := range poly {
			next[j] ^= c                  // poly * x: same index (array grows by one at the low-degree end)
			next[j+1] ^= qrGFMul(c, root) // poly * root: shifted one index (one degree lower)
		}
		poly = next
	}
	return poly
}

// qrRSEncode returns ecLen error-correction codewords for data, computed as
// the remainder of data (treated as a polynomial, shifted up by ecLen
// degrees) divided by the generator polynomial, via the standard
// XOR-based long-division routine.
func qrRSEncode(data []byte, ecLen int) []byte {
	gen := qrRSGeneratorPoly(ecLen)
	remainder := make([]byte, len(data)+ecLen)
	copy(remainder, data)
	for i := 0; i < len(data); i++ {
		coef := remainder[i]
		if coef == 0 {
			continue
		}
		for j, g := range gen {
			remainder[i+j] ^= qrGFMul(g, coef)
		}
	}
	return remainder[len(data):]
}

// qrRSSyndromesZero reports whether codewords (data followed by EC
// codewords, as laid out on the wire) evaluate to zero at every root
// 2^0..2^(ecLen-1) — the standard Reed-Solomon "no errors detected" check.
// Used by tests as a strong correctness signal on qrRSEncode/qrGFMul: a
// wrong generator-polynomial or field-arithmetic implementation would need
// a coincidence to still produce all-zero syndromes.
func qrRSSyndromesZero(codewords []byte, ecLen int) bool {
	for i := 0; i < ecLen; i++ {
		var acc byte
		root := qrGFExp[i]
		for _, c := range codewords {
			acc = qrGFMul(acc, root) ^ c
		}
		if acc != 0 {
			return false
		}
	}
	return true
}
