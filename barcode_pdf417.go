// SPDX-License-Identifier: MIT

package asposepdf

import (
	"fmt"
	"math"
)

// PDF417 encoder (ISO/IEC 15438). Scope: byte compaction only — the raw
// UTF-8 bytes of the value, preceded by an ECI 000026 (UTF-8) designator
// when any byte is non-ASCII. Text and numeric compaction are size
// optimisations, not capabilities this omits: any string byte compaction
// can encode. Everything except the codeword-to-bar-pattern table
// (barcode_pdf417_tables.go, generated and cross-verified — see that file)
// is computed: the Reed-Solomon generator polynomial over GF(929) is built
// from its roots 3^1..3^k rather than looked up, and the symbol dimensions
// follow ISO/IEC 15438 Annex Q.

const (
	pdf417LatchByte     = 901 // byte compaction, byte count not a multiple of 6
	pdf417LatchByte6    = 924 // byte compaction, byte count a multiple of 6
	pdf417ECI           = 927 // extended channel interpretation, one codeword follows
	pdf417ECIUTF8       = 26  // ECI 000026: UTF-8
	pdf417Pad           = 900 // padding codeword (text-compaction latch, harmless as padding)
	pdf417MaxCodewords  = 929 // symbol length descriptor + data + padding + error correction
	pdf417StartPattern  = 0x1fea8
	pdf417StopPattern   = 0x3fa29
	pdf417MinRows       = 3
	pdf417MaxRows       = 90
	pdf417MaxCols       = 30
	pdf417RowModules    = 3 // rendered height of one symbol row, in module widths (ISO minimum)
	pdf417Quiet         = 4 // quiet zone on every side, in modules
	pdf417DefaultAspect = 3.0
)

// pdf417HighLevel converts value to its data codewords: an optional UTF-8 ECI
// designator, a byte-compaction latch, then the bytes — every group of six
// packed into five base-900 codewords, any remainder one codeword per byte.
func pdf417HighLevel(value string) []int {
	data := []byte(value)
	var cw []int
	for _, b := range data {
		if b >= 0x80 {
			cw = append(cw, pdf417ECI, pdf417ECIUTF8)
			break
		}
	}
	if len(data)%6 == 0 {
		cw = append(cw, pdf417LatchByte6)
	} else {
		cw = append(cw, pdf417LatchByte)
	}
	i := 0
	for ; len(data)-i >= 6; i += 6 {
		var t uint64
		for j := 0; j < 6; j++ {
			t = t<<8 | uint64(data[i+j])
		}
		var five [5]int
		for j := 4; j >= 0; j-- {
			five[j] = int(t % 900)
			t /= 900
		}
		cw = append(cw, five[:]...)
	}
	for ; i < len(data); i++ {
		cw = append(cw, int(data[i]))
	}
	return cw
}

// pdf417RecommendedLevel is the minimum error-correction (security) level ISO
// recommends for n data codewords (ISO/IEC 15438 §4.10 table).
func pdf417RecommendedLevel(n int) int {
	switch {
	case n <= 40:
		return 2
	case n <= 160:
		return 3
	case n <= 320:
		return 4
	default:
		return 5
	}
}

func pdf417ECCount(level int) int { return 1 << uint(level+1) }

// pdf417Generator returns the monic degree-k generator polynomial over GF(929),
// prod_{i=1..k}(x - 3^i), highest power first (g[0] == 1).
func pdf417Generator(k int) []int {
	g := []int{1}
	root := 1
	for i := 1; i <= k; i++ {
		root = root * 3 % 929
		next := make([]int, len(g)+1)
		for j, c := range g {
			next[j] = (next[j] + c) % 929
			next[j+1] = ((next[j+1]-c*root)%929 + 929) % 929
		}
		g = next
	}
	return g
}

// pdf417ErrorCorrection returns the k error-correction codewords for data:
// the negated remainder of data(x)·x^k divided by the generator polynomial
// (ISO/IEC 15438 §4.10), so the whole codeword polynomial has roots 3^1..3^k.
func pdf417ErrorCorrection(data []int, level int) []int {
	k := pdf417ECCount(level)
	g := pdf417Generator(k)
	rem := make([]int, k)
	for _, d := range data {
		t := (d + rem[0]) % 929
		for j := 0; j < k-1; j++ {
			rem[j] = ((rem[j+1]-t*g[j+1])%929 + 929) % 929
		}
		rem[k-1] = ((-t*g[k])%929 + 929) % 929
	}
	ec := make([]int, k)
	for j, r := range rem {
		ec[j] = (929 - r) % 929
	}
	return ec
}

// pdf417Rows is the row count for m data codewords and k error-correction
// codewords in c columns (ISO/IEC 15438 Annex Q).
func pdf417Rows(m, k, c int) int {
	r := (m+1+k)/c + 1
	if c*r >= m+1+k+c {
		r--
	}
	return r
}

// pdf417Dimensions picks the column count (and its row count) whose rendered
// width:height ratio — 17c+69+2·quiet modules over 3·rows+2·quiet — is
// closest to aspect (0 → 3:1).
func pdf417Dimensions(m, k int, aspect float64) (cols, rows int, err error) {
	if !(aspect > 0) || math.IsInf(aspect, 0) { // also catches NaN
		aspect = pdf417DefaultAspect
	}
	best := math.Inf(1)
	for c := 1; c <= pdf417MaxCols; c++ {
		r := pdf417Rows(m, k, c)
		if r < pdf417MinRows {
			r = pdf417MinRows
		}
		if r > pdf417MaxRows || c*r > pdf417MaxCodewords {
			continue // a symbol holds at most 929 codewords in total (ISO/IEC 15438 §5.4.2)
		}
		w := float64(17*c + 69 + 2*pdf417Quiet)
		h := float64(pdf417RowModules*r + 2*pdf417Quiet)
		if d := math.Abs(w/h - aspect); d < best {
			best, cols, rows = d, c, r
		}
	}
	if cols == 0 {
		return 0, 0, fmt.Errorf("asposepdf: PDF417 cannot fit %d codewords in %d columns and %d rows", m+1+k, pdf417MaxCols, pdf417MaxRows)
	}
	return cols, rows, nil
}

// encodePDF417Modules encodes value into a PDF417 module grid (quiet zone
// included). aspect is the target width:height of the rendered symbol (0 =
// 3:1); the column count is chosen to approach it. Errors if value is empty
// or too large for one symbol (at most 929 codewords including the symbol
// length descriptor and error correction).
func encodePDF417Modules(value string, aspect float64) (barcodeModules, error) {
	if value == "" {
		return barcodeModules{}, fmt.Errorf("asposepdf: PDF417 barcode value is empty")
	}
	high := pdf417HighLevel(value)
	m := len(high)

	// Start from ISO's recommended security level and step down only as far
	// as needed for the symbol to fit. "Fits" means a real layout exists, not
	// just m+1+k <= 929: when the total is exactly 929 (prime) no cols x rows
	// product with cols <= 30 reaches it, yet one level lower would fit.
	var cols, rows, k, level int
	found := false
	for level = pdf417RecommendedLevel(m); level >= 0; level-- {
		k = pdf417ECCount(level)
		if m+1+k > pdf417MaxCodewords {
			continue
		}
		if c, r, err := pdf417Dimensions(m, k, aspect); err == nil {
			cols, rows, found = c, r, true
			break
		}
	}
	if !found {
		return barcodeModules{}, fmt.Errorf("asposepdf: PDF417 barcode value too large (%d bytes)", len(value))
	}

	pad := 0
	if n := cols*rows - k; n > m+1 {
		pad = n - m - 1
	}
	data := make([]int, 0, m+pad+1)
	data = append(data, m+pad+1) // symbol length descriptor
	data = append(data, high...)
	for i := 0; i < pad; i++ {
		data = append(data, pdf417Pad)
	}
	all := append(data, pdf417ErrorCorrection(data, level)...)

	width := 17*cols + 69
	gridW := width + 2*pdf417Quiet
	gridH := pdf417RowModules*rows + 2*pdf417Quiet
	bits := make([]bool, gridW*gridH)

	idx := 0
	for y := 0; y < rows; y++ {
		cluster := y % 3
		var left, right int
		switch cluster {
		case 0:
			left = 30*(y/3) + (rows-1)/3
			right = 30*(y/3) + (cols - 1)
		case 1:
			left = 30*(y/3) + level*3 + (rows-1)%3
			right = 30*(y/3) + (rows-1)/3
		default:
			left = 30*(y/3) + (cols - 1)
			right = 30*(y/3) + level*3 + (rows-1)%3
		}

		row := make([]bool, 0, width)
		put := func(pattern uint32, n int) {
			for b := n - 1; b >= 0; b-- {
				row = append(row, (pattern>>uint(b))&1 != 0)
			}
		}
		put(pdf417StartPattern, 17)
		put(pdf417Codewords[cluster][left], 17)
		for x := 0; x < cols; x++ {
			put(pdf417Codewords[cluster][all[idx]], 17)
			idx++
		}
		put(pdf417Codewords[cluster][right], 17)
		put(pdf417StopPattern, 18)

		for r := 0; r < pdf417RowModules; r++ {
			gy := pdf417Quiet + y*pdf417RowModules + r
			copy(bits[gy*gridW+pdf417Quiet:], row)
		}
	}
	return barcodeModules{Cols: gridW, Rows: gridH, Bits: bits, Uniform: true}, nil
}
