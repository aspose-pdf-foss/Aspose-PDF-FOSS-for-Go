// SPDX-License-Identifier: MIT

package asposepdf

import "testing"

// TestAdobeCMYKToRGB checks the baked Adobe-profile LUT reproduces the process
// colours (matching Acrobat/MuPDF) and interpolates interior colours — in
// particular the purple that the naive (1-C)(1-K) formula rendered too blue.
func TestAdobeCMYKToRGB(t *testing.T) {
	cases := []struct {
		c, m, y, k       float64
		wantR, wantG, wantB uint8
		tol              int
	}{
		{0, 0, 0, 0, 255, 255, 255, 0},   // white (exact grid point)
		{0, 0, 0, 1, 34, 31, 31, 0},      // rich black
		{1, 0, 0, 0, 0, 173, 239, 0},     // process cyan
		{0, 1, 0, 0, 236, 0, 139, 0},     // process magenta
		{0, 0, 1, 0, 255, 241, 0, 0},     // process yellow
		{0.63, 0.80, 0.11, 0.145, 108, 70, 133, 12}, // crowd purple (interpolated)
	}
	for _, c := range cases {
		r, g, b := adobeCMYKToRGB(c.c, c.m, c.y, c.k)
		if absDiff(int(r), int(c.wantR)) > c.tol || absDiff(int(g), int(c.wantG)) > c.tol || absDiff(int(b), int(c.wantB)) > c.tol {
			t.Errorf("CMYK(%.2f,%.2f,%.2f,%.2f) → (%d,%d,%d), want (%d,%d,%d) ±%d",
				c.c, c.m, c.y, c.k, r, g, b, c.wantR, c.wantG, c.wantB, c.tol)
		}
	}

	// A bluish naive purple must now be notably less blue (more purple).
	_, _, b := adobeCMYKToRGB(0.63, 0.80, 0.11, 0.145)
	if b > 160 {
		t.Errorf("purple B channel = %d, expected well below the naive ~194", b)
	}
}

func absDiff(a, b int) int {
	if a < b {
		return b - a
	}
	return a - b
}
