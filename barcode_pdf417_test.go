// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"strings"
	"testing"
)

// pdf417RunWidths splits a pattern given as nbits module bits (MSB first,
// starting with a bar) into its alternating bar/space run widths.
func pdf417RunWidths(v uint32, nbits int) []int {
	var runs []int
	prev := (v >> uint(nbits-1)) & 1
	n := 0
	for b := nbits - 1; b >= 0; b-- {
		bit := (v >> uint(b)) & 1
		if bit == prev {
			n++
			continue
		}
		runs = append(runs, n)
		prev, n = bit, 1
	}
	return append(runs, n)
}

func pdf417RunsEqual(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestPDF417TableStructure re-verifies the committed codeword table against
// the ISO/IEC 15438 rules every entry must satisfy — 17 modules, four bars
// and four spaces each 1..6 wide, cluster value (b1-b2+b3-b4) mod 9 equal to
// 0/3/6 for clusters 0/1/2, no duplicates within a cluster — and that the
// start/stop patterns are the ISO 8-1-1-1-1-1-1-3 / 7-1-1-3-1-1-1-2-1 ones.
// The table itself was extracted mechanically from ZXing and matched entry
// for entry against zint's independent copy (tools/genpdf417); a corrupted
// or shifted entry would break at least one of these.
func TestPDF417TableStructure(t *testing.T) {
	for c := 0; c < 3; c++ {
		seen := map[uint32]bool{}
		for k, v := range pdf417Codewords[c] {
			if v>>17 != 0 || v>>16 != 1 || v&1 != 0 {
				t.Fatalf("cluster %d cw %d: %#x is not 17 bits starting with a bar and ending with a space", c, k, v)
			}
			r := pdf417RunWidths(v, 17)
			if len(r) != 8 {
				t.Fatalf("cluster %d cw %d: %#x has %d runs, want 8", c, k, v, len(r))
			}
			for _, w := range r {
				if w < 1 || w > 6 {
					t.Fatalf("cluster %d cw %d: run width %d outside 1..6", c, k, w)
				}
			}
			if kv := ((r[0]-r[2]+r[4]-r[6])%9 + 9) % 9; kv != c*3 {
				t.Fatalf("cluster %d cw %d: cluster value %d, want %d", c, k, kv, c*3)
			}
			if seen[v] {
				t.Fatalf("cluster %d cw %d: duplicate pattern %#x", c, k, v)
			}
			seen[v] = true
		}
	}
	if got := pdf417RunWidths(pdf417StartPattern, 17); !pdf417RunsEqual(got, []int{8, 1, 1, 1, 1, 1, 1, 3}) {
		t.Errorf("start pattern runs = %v, want 8-1-1-1-1-1-1-3", got)
	}
	if got := pdf417RunWidths(pdf417StopPattern, 18); !pdf417RunsEqual(got, []int{7, 1, 1, 3, 1, 1, 1, 2, 1}) {
		t.Errorf("stop pattern runs = %v, want 7-1-1-3-1-1-1-2-1", got)
	}
}

// TestPDF417GeneratorMatchesISO checks the computed Reed-Solomon generator
// polynomials against ISO/IEC 15438 Annex F, Tables F.1-F.5 (levels 0-4;
// coefficients listed constant term first, as the standard prints them).
func TestPDF417GeneratorMatchesISO(t *testing.T) {
	iso := [][]int{
		{27, 917},
		{522, 568, 723, 809},
		{237, 308, 436, 284, 646, 653, 428, 379},
		{274, 562, 232, 755, 599, 524, 801, 132, 295, 116, 442, 428, 295, 42, 176, 65},
		{361, 575, 922, 525, 176, 586, 640, 321, 536, 742, 677, 742, 687, 284, 193, 517,
			273, 494, 263, 147, 593, 800, 571, 320, 803, 133, 231, 390, 685, 330, 63, 410},
	}
	for level, want := range iso {
		k := pdf417ECCount(level)
		g := pdf417Generator(k)
		if len(want) != k {
			t.Fatalf("level %d: reference has %d coefficients, want %d", level, len(want), k)
		}
		if g[0] != 1 {
			t.Errorf("level %d: generator not monic", level)
		}
		for i := 0; i < k; i++ {
			if g[k-i] != want[i] {
				t.Errorf("level %d: coefficient %d = %d, want %d", level, i, g[k-i], want[i])
				break
			}
		}
	}
}

// pdf417ByteCompactionDecode decodes the byte-compaction sections of a
// codeword stream (data codewords after the symbol length descriptor) the way
// a reader does — ported from the reference decoder's byteCompaction /
// mode-dispatch logic, not from the encoder: ECI 927 nn, latches 901/924,
// groups of five base-900 codewords -> six bytes, trailing single bytes,
// trailing pad 900s ignored.
func pdf417ByteCompactionDecode(t *testing.T, cw []int) (out []byte, eci int) {
	t.Helper()
	i := 0
	for i < len(cw) {
		switch code := cw[i]; {
		case code == 927:
			if i+1 >= len(cw) {
				t.Fatal("truncated ECI")
			}
			eci = cw[i+1]
			i += 2
		case code == 901 || code == 924:
			mode := code
			i++
			if i >= len(cw) || cw[i] >= 900 {
				t.Fatalf("byte-compaction latch %d at %d is followed by no data (a stray latch where padding 900 belongs?)", mode, i-1)
			}
			for i < len(cw) && cw[i] < 900 {
				var value uint64
				count := 0
				for {
					value = 900*value + uint64(cw[i])
					i++
					count++
					if count >= 5 || i >= len(cw) || cw[i] >= 900 {
						break
					}
				}
				if count == 5 && (mode == 924 || (i < len(cw) && cw[i] < 900)) {
					for j := 0; j < 6; j++ {
						out = append(out, byte(value>>(8*uint(5-j))))
					}
				} else {
					i -= count
					for i < len(cw) && cw[i] < 900 {
						out = append(out, byte(cw[i]))
						i++
					}
				}
			}
		case code == 900:
			i++ // padding
		default:
			t.Fatalf("unexpected codeword %d at %d (encoder emits only byte compaction)", code, i)
		}
	}
	return out, eci
}

// decodePDF417Modules is an independent test-only decoder written from the
// reference decoder's semantics rather than from the encoder: it finds the
// symbol, checks every row's start/stop pattern, reads each 17-module
// codeword back through the (verified) table with its cluster required to be
// row mod 3, reads the row indicators exactly as the reference decoder
// interprets them (row number, column count, row count, security level —
// all of which must agree across every row and with the geometry), checks
// the symbol length descriptor, and verifies the Reed-Solomon codewords by
// evaluating the whole codeword polynomial at 3^1..3^k over GF(929).
func decodePDF417Modules(t *testing.T, mat barcodeModules) (payload []byte, eci int, level, cols, rows int) {
	t.Helper()
	at := func(x, y int) bool { return mat.Bits[y*mat.Cols+x] }

	minX, minY, maxX, maxY := mat.Cols, mat.Rows, -1, -1
	for y := 0; y < mat.Rows; y++ {
		for x := 0; x < mat.Cols; x++ {
			if at(x, y) {
				minX, maxX = min(minX, x), max(maxX, x)
				minY, maxY = min(minY, y), max(maxY, y)
			}
		}
	}
	width, height := maxX-minX+1, maxY-minY+1
	if (width-69)%17 != 0 || height%3 != 0 {
		t.Fatalf("symbol %dx%d modules is not 17c+69 wide and a multiple of 3 tall", width, height)
	}
	cols, rows = (width-69)/17, height/3
	if rows < 3 || rows > 90 || cols < 1 || cols > 30 {
		t.Fatalf("decoded geometry %d cols x %d rows outside ISO limits", cols, rows)
	}

	reverse := map[uint32][2]int{}
	for c := 0; c < 3; c++ {
		for k, v := range pdf417Codewords[c] {
			reverse[v] = [2]int{c, k}
		}
	}
	bitsAt := func(y, x0, n int) uint32 {
		var v uint32
		for i := 0; i < n; i++ {
			v <<= 1
			if at(minX+x0+i, y) {
				v |= 1
			}
		}
		return v
	}

	var upper, lower, ecl, colsMeta = -1, -1, -1, -1
	setOnce := func(dst *int, v int, what string, row int) {
		if *dst != -1 && *dst != v {
			t.Fatalf("row %d: %s = %d disagrees with earlier rows (%d)", row, what, v, *dst)
		}
		*dst = v
	}

	var all []int
	for y := 0; y < rows; y++ {
		gy := minY + y*3
		for r := 1; r < 3; r++ { // the three module rows of a symbol row are identical
			for x := 0; x < width; x++ {
				if at(minX+x, gy) != at(minX+x, gy+r) {
					t.Fatalf("symbol row %d: module rows differ at x=%d", y, x)
				}
			}
		}
		if bitsAt(gy, 0, 17) != pdf417StartPattern {
			t.Fatalf("row %d: bad start pattern", y)
		}
		if bitsAt(gy, width-18, 18) != pdf417StopPattern {
			t.Fatalf("row %d: bad stop pattern", y)
		}

		read := func(slot int) int {
			v := bitsAt(gy, 17+17*slot, 17)
			ck, ok := reverse[v]
			if !ok {
				t.Fatalf("row %d slot %d: pattern %#x is not in the codeword table", y, slot, v)
			}
			if ck[0] != y%3 {
				t.Fatalf("row %d slot %d: codeword from cluster %d, want %d", y, slot, ck[0], y%3)
			}
			return ck[1]
		}

		indicator := func(v int, right bool) {
			if row := (v/30)*3 + y%3; row != y {
				t.Fatalf("row %d: indicator %d encodes row %d", y, v, row)
			}
			c := y % 3
			if right {
				c = (c + 2) % 3
			}
			val := v % 30
			switch c {
			case 0:
				setOnce(&upper, val*3+1, "row count (upper part)", y)
			case 1:
				setOnce(&ecl, val/3, "security level", y)
				setOnce(&lower, val%3, "row count (lower part)", y)
			case 2:
				setOnce(&colsMeta, val+1, "column count", y)
			}
		}
		indicator(read(0), false)
		for x := 0; x < cols; x++ {
			all = append(all, read(1+x))
		}
		indicator(read(1+cols), true)
	}
	if colsMeta != cols {
		t.Fatalf("indicators say %d columns, geometry says %d", colsMeta, cols)
	}
	if upper+lower != rows {
		t.Fatalf("indicators say %d rows, geometry says %d", upper+lower, rows)
	}
	level = ecl

	k := 1 << uint(level+1)
	n := len(all)
	if sld := all[0]; sld != n-k {
		t.Fatalf("symbol length descriptor %d, want %d (all codewords %d - %d error-correction)", sld, n-k, n, k)
	}
	root := 1
	for j := 1; j <= k; j++ {
		root = root * 3 % 929
		acc := 0
		for _, c := range all {
			acc = (acc*root + c) % 929
		}
		if acc != 0 {
			t.Fatalf("Reed-Solomon syndrome at 3^%d is %d, want 0", j, acc)
		}
	}

	payload, eci = pdf417ByteCompactionDecode(t, all[1:all[0]])
	return payload, eci, level, cols, rows
}

func TestPDF417RoundTrip(t *testing.T) {
	cases := []string{
		"A", "AB", "ABC", "ABCD", "ABCDE", "ABCDEF", "ABCDEFG", // every byte-count residue mod 6
		"HELLO WORLD",
		"https://example.com/sku/123?x=1&y=2",
		"The quick brown fox jumps over the lazy dog.",
		strings.Repeat("PDF417 stress test data. ", 30),
		"Привет, мир — héllo 日本語", // non-ASCII: needs the UTF-8 ECI
		strings.Repeat("0123456789", 40),
		"\x00\x01\xff binary-ish",
	}
	for _, s := range cases {
		for _, aspect := range []float64{0, 1, 2, 5, 12} {
			mat, err := encodePDF417Modules(s, aspect)
			if err != nil {
				t.Fatalf("encodePDF417Modules(%.20q..., %v): %v", s, aspect, err)
			}
			got, eci, _, _, _ := decodePDF417Modules(t, mat)
			if !bytes.Equal(got, []byte(s)) {
				t.Errorf("aspect %v: round trip mismatch for %.20q...: got %.20q...", aspect, s, got)
			}
			wantECI := 0
			if !isASCII(s) {
				wantECI = 26
			}
			if eci != wantECI {
				t.Errorf("%.20q...: ECI = %d, want %d", s, eci, wantECI)
			}
		}
	}
}

func TestPDF417AspectSteersColumns(t *testing.T) {
	msg := strings.Repeat("aspect ratio ", 20)
	_, _, _, narrowCols, _ := decodePDF417Modules(t, mustPDF417(t, msg, 0.6))
	_, _, _, wideCols, _ := decodePDF417Modules(t, mustPDF417(t, msg, 8))
	if narrowCols >= wideCols {
		t.Errorf("narrow aspect gave %d columns, wide gave %d; want narrow < wide", narrowCols, wideCols)
	}
}

func mustPDF417(t *testing.T, s string, aspect float64) barcodeModules {
	t.Helper()
	m, err := encodePDF417Modules(s, aspect)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestPDF417RejectsEmpty(t *testing.T) {
	if _, err := encodePDF417Modules("", 0); err == nil {
		t.Error("expected an error for empty input")
	}
}
