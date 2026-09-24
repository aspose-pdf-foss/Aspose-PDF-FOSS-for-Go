// SPDX-License-Identifier: MIT

package asposepdf

import (
	"math"
	"strings"
	"testing"
)

// TestPDF417ErrorCorrectionLevel pins ISO/IEC 15438's recommended security
// levels with hard-coded literals (<=40 -> 2, <=160 -> 3, <=320 -> 4,
// <=863 -> 5) at every boundary, so it cannot agree with a wrong threshold in
// pdf417RecommendedLevel by construction — an earlier version compared the
// decoded level to pdf417RecommendedLevel itself.
func TestPDF417ErrorCorrectionLevel(t *testing.T) {
	for _, tc := range []struct{ n, level int }{
		{1, 2}, {40, 2}, {41, 3}, {160, 3}, {161, 4}, {320, 4}, {321, 5}, {863, 5},
	} {
		if got := pdf417RecommendedLevel(tc.n); got != tc.level {
			t.Errorf("pdf417RecommendedLevel(%d) = %d, want %d", tc.n, got, tc.level)
		}
	}
	// End to end: the level a reader recovers from the row indicators.
	for _, tc := range []struct{ bytes, level int }{
		{5, 2},    // 6 data codewords
		{100, 3},  // 1+5*16+4 = 85
		{200, 4},  // 1+5*33+2 = 168
		{500, 5},  // 1+5*83+2 = 418
		{1050, 4}, // 876 data codewords: level 5 (64) would total 941 > 929, so it steps down to 4
		{1107, 0}, // 924 data codewords: only level 0 leaves a layout (level 1 totals exactly 929, prime)
	} {
		_, _, level, _, _ := decodePDF417Modules(t, mustPDF417(t, strings.Repeat("x", tc.bytes), 0))
		if level != tc.level {
			t.Errorf("%d bytes: security level %d, want %d", tc.bytes, level, tc.level)
		}
	}
}

// TestPDF417CapacityEdge pins the exact capacity edge. 1107 and 1108 ASCII
// bytes fit (at security level 0); 1109 and above do not, because the only
// totals left are 929 (prime, so no cols x rows layout with cols <= 30 exists)
// or beyond. An earlier version rejected 1107 because it stopped lowering the
// security level once the codeword total reached 929 instead of once a layout
// existed.
func TestPDF417CapacityEdge(t *testing.T) {
	for _, n := range []int{1107, 1108} {
		got, _, _, _, _ := decodePDF417Modules(t, mustPDF417(t, strings.Repeat("a", n), 0))
		if len(got) != n {
			t.Errorf("%d-byte round trip returned %d bytes", n, len(got))
		}
	}
	for n := 1109; n <= 1300; n++ {
		if _, err := encodePDF417Modules(strings.Repeat("a", n), 0); err == nil {
			t.Errorf("%d bytes should not fit in one PDF417 symbol", n)
		}
	}
}

// TestPDF417DimensionsWithinLimits sweeps every (data, security level)
// combination: whenever a layout is returned it must respect ISO's limits —
// 1..30 columns, 3..90 rows, at most 929 codewords in total — and hold all
// the codewords. (The 30x31 = 930 layout that once slipped through only
// panicked, at the table lookup, when a value hit it exactly.)
func TestPDF417DimensionsWithinLimits(t *testing.T) {
	for level := 0; level <= 8; level++ {
		k := 1 << uint(level+1)
		for m := 1; m+1+k <= 929; m++ {
			for _, aspect := range []float64{0, 0.5, 3, 20} {
				c, r, err := pdf417Dimensions(m, k, aspect)
				if err != nil {
					continue
				}
				if c < 1 || c > 30 || r < 3 || r > 90 || c*r > 929 || c*r < m+1+k {
					t.Fatalf("m=%d k=%d aspect=%v: layout %dx%d violates the limits", m, k, aspect, c, r)
				}
			}
		}
	}
}

// TestPDF417NonFiniteAspect: a NaN, infinite or non-positive aspect falls back
// to the default instead of leaving the layout search empty.
func TestPDF417NonFiniteAspect(t *testing.T) {
	for _, a := range []float64{math.NaN(), math.Inf(1), math.Inf(-1), -1, 0, 1e-300, 1e300} {
		got, _, _, _, _ := decodePDF417Modules(t, mustPDF417(t, "aspect edge", a))
		if string(got) != "aspect edge" {
			t.Errorf("aspect %v: round trip returned %q", a, got)
		}
	}
}

// TestPDF417QuietZone: ISO requires at least two modules of quiet zone on
// every side; the encoder writes four.
func TestPDF417QuietZone(t *testing.T) {
	m := mustPDF417(t, "quiet zone", 0)
	minX, minY, maxX, maxY := m.Cols, m.Rows, -1, -1
	for y := 0; y < m.Rows; y++ {
		for x := 0; x < m.Cols; x++ {
			if m.Bits[y*m.Cols+x] {
				minX, maxX = min(minX, x), max(maxX, x)
				minY, maxY = min(minY, y), max(maxY, y)
			}
		}
	}
	for name, got := range map[string]int{"left": minX, "top": minY, "right": m.Cols - 1 - maxX, "bottom": m.Rows - 1 - maxY} {
		if got < 4 {
			t.Errorf("%s quiet zone is %d modules, want >= 4", name, got)
		}
	}
}

// TestPDF417GeneratorRoots checks every level's generator polynomial
// algebraically — monic, degree k, and vanishing at 3^1..3^k over GF(929) —
// which is what makes the codeword polynomial divisible by it. Levels 0-4 are
// additionally pinned to the ISO tables in TestPDF417GeneratorMatchesISO; the
// coefficients of all nine levels were also compared once against zint's copy
// of Annex F.
func TestPDF417GeneratorRoots(t *testing.T) {
	for level := 0; level <= 8; level++ {
		k := 1 << uint(level+1)
		g := pdf417Generator(k)
		if len(g) != k+1 || g[0] != 1 {
			t.Fatalf("level %d: generator has %d coefficients, leading %d", level, len(g), g[0])
		}
		root := 1
		for j := 1; j <= k; j++ {
			root = root * 3 % 929
			acc := 0
			for _, c := range g {
				acc = (acc*root + c) % 929
			}
			if acc != 0 {
				t.Fatalf("level %d: g(3^%d) = %d, want 0", level, j, acc)
			}
		}
	}
}

// TestPDF417RenderedGeometry renders a PDF417 field through the page
// rasterizer and compares the raster with the module grid module by module —
// the check that pins orientation (row 0 at the top), the fit-and-centre
// scaling, and the rectangle's aspect ratio reaching the encoder. The module
// grid is located in the raster by its dark-pixel bounding box, not by
// re-deriving drawBarcodeMatrix's formulas.
func TestPDF417RenderedGeometry(t *testing.T) {
	for _, rect := range []Rectangle{
		{LLX: 50, LLY: 600, URX: 450, URY: 700}, // wide
		{LLX: 50, LLY: 400, URX: 200, URY: 700}, // tall
		{LLX: 50, LLY: 300, URX: 350, URY: 560}, // squarish
	} {
		const value = "orientation check: row zero must be at the top of the raster"
		const dpi = 288
		doc := NewDocumentFromFormat(PageFormatA4)
		if _, err := doc.Form().AddBarcodeField(1, rect, "bc", BarcodePDF417, value); err != nil {
			t.Fatal(err)
		}
		page, err := doc.Page(1)
		if err != nil {
			t.Fatal(err)
		}
		img, err := page.RenderImage(RenderOptions{DPI: dpi})
		if err != nil {
			t.Fatal(err)
		}
		dark := func(x, y int) bool {
			r, g, b, _ := img.At(x, y).RGBA()
			return r < 0x8000 && g < 0x8000 && b < 0x8000
		}
		bnd := img.Bounds()
		px0, py0, px1, py1 := bnd.Max.X, bnd.Max.Y, -1, -1
		for y := bnd.Min.Y; y < bnd.Max.Y; y++ {
			for x := bnd.Min.X; x < bnd.Max.X; x++ {
				if dark(x, y) {
					px0, px1 = min(px0, x), max(px1, x)
					py0, py1 = min(py0, y), max(py1, y)
				}
			}
		}
		if px1 < 0 {
			t.Fatalf("rect %v: nothing painted", rect)
		}

		mat, err := encodePDF417Modules(value, (rect.URX-rect.LLX)/(rect.URY-rect.LLY))
		if err != nil {
			t.Fatal(err)
		}
		mx0, my0, mx1, my1 := mat.Cols, mat.Rows, -1, -1
		for y := 0; y < mat.Rows; y++ {
			for x := 0; x < mat.Cols; x++ {
				if mat.Bits[y*mat.Cols+x] {
					mx0, mx1 = min(mx0, x), max(mx1, x)
					my0, my1 = min(my0, y), max(my1, y)
				}
			}
		}
		symW, symH := mx1-mx0+1, my1-my0+1
		bw, bh := float64(px1-px0+1), float64(py1-py0+1)
		if d := (bw/bh)/(float64(symW)/float64(symH)) - 1; d > 0.02 || d < -0.02 {
			t.Errorf("rect %v: raster aspect %.3f differs from the module grid's %.3f", rect, bw/bh, float64(symW)/float64(symH))
		}
		for y := 0; y < symH; y++ {
			for x := 0; x < symW; x++ {
				sx := px0 + int((float64(x)+0.5)*bw/float64(symW))
				sy := py0 + int((float64(y)+0.5)*bh/float64(symH))
				if dark(sx, sy) != mat.Bits[(my0+y)*mat.Cols+mx0+x] {
					t.Fatalf("rect %v: module (%d,%d) does not match the raster (orientation or scaling is wrong)", rect, x, y)
				}
			}
		}

		// Inside the rectangle and centred in it.
		pageH := float64(bnd.Dy())
		rx0, rx1 := rect.LLX*dpi/72, rect.URX*dpi/72
		ry0, ry1 := pageH-rect.URY*dpi/72, pageH-rect.LLY*dpi/72
		if float64(px0) < rx0-2 || float64(px1) > rx1+2 || float64(py0) < ry0-2 || float64(py1) > ry1+2 {
			t.Errorf("rect %v: symbol overflows the rectangle", rect)
		}
		cx, cy := float64(px0+px1)/2, float64(py0+py1)/2
		if math.Abs(cx-(rx0+rx1)/2) > 3 || math.Abs(cy-(ry0+ry1)/2) > 3 {
			t.Errorf("rect %v: symbol is not centred (centre %.0f,%.0f)", rect, cx, cy)
		}
	}
}
