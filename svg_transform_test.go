// SPDX-License-Identifier: MIT

package asposepdf

import (
	"math"
	"testing"
)

func almostEqualM(t *testing.T, got, want svgMatrix, tol float64) {
	t.Helper()
	for i := range got {
		if math.Abs(got[i]-want[i]) > tol {
			t.Errorf("matrix mismatch at [%d]: got %g want %g\n  full got=%v want=%v", i, got[i], want[i], got, want)
			return
		}
	}
}

func TestMatrixIdentity(t *testing.T) {
	almostEqualM(t, matrixIdentity(), svgMatrix{1, 0, 0, 1, 0, 0}, 0)
}

func TestParseSVGTransform_Translate(t *testing.T) {
	m, ok := parseSVGTransform("translate(10, 20)")
	if !ok { t.Fatal("parse failed") }
	almostEqualM(t, m, svgMatrix{1, 0, 0, 1, 10, 20}, 1e-9)
}

func TestParseSVGTransform_TranslateSingleArg(t *testing.T) {
	m, _ := parseSVGTransform("translate(15)")
	almostEqualM(t, m, svgMatrix{1, 0, 0, 1, 15, 0}, 1e-9)
}

func TestParseSVGTransform_Scale(t *testing.T) {
	m, _ := parseSVGTransform("scale(2)")
	almostEqualM(t, m, svgMatrix{2, 0, 0, 2, 0, 0}, 1e-9)
}

func TestParseSVGTransform_ScaleXY(t *testing.T) {
	m, _ := parseSVGTransform("scale(2, 3)")
	almostEqualM(t, m, svgMatrix{2, 0, 0, 3, 0, 0}, 1e-9)
}

func TestParseSVGTransform_Rotate(t *testing.T) {
	m, _ := parseSVGTransform("rotate(90)")
	// cos90=0 sin90=1
	almostEqualM(t, m, svgMatrix{0, 1, -1, 0, 0, 0}, 1e-9)
}

func TestParseSVGTransform_RotateAroundPoint(t *testing.T) {
	m, _ := parseSVGTransform("rotate(90, 10, 20)")
	// equivalent to translate(10,20) rotate(90) translate(-10,-20):
	//   [0 1 -1 0  30  10]
	almostEqualM(t, m, svgMatrix{0, 1, -1, 0, 30, 10}, 1e-9)
}

func TestParseSVGTransform_Matrix(t *testing.T) {
	m, _ := parseSVGTransform("matrix(1, 2, 3, 4, 5, 6)")
	almostEqualM(t, m, svgMatrix{1, 2, 3, 4, 5, 6}, 1e-9)
}

func TestParseSVGTransform_Composite(t *testing.T) {
	// translate(10,20) scale(2) — point (1,1) → first scale → (2,2) → translate → (12, 22)
	m, _ := parseSVGTransform("translate(10, 20) scale(2)")
	// Composite: [2 0 0 2 10 20]
	almostEqualM(t, m, svgMatrix{2, 0, 0, 2, 10, 20}, 1e-9)
}

func TestParseSVGTransform_SkewX(t *testing.T) {
	m, _ := parseSVGTransform("skewX(45)")
	// matrix [1 0 tan(45) 1 0 0] = [1 0 1 1 0 0]
	almostEqualM(t, m, svgMatrix{1, 0, 1, 1, 0, 0}, 1e-9)
}

func TestParseSVGTransform_Empty(t *testing.T) {
	m, ok := parseSVGTransform("")
	if !ok { t.Fatal("empty should be identity, not failure") }
	almostEqualM(t, m, matrixIdentity(), 0)
}

func TestParseSVGTransform_Garbage(t *testing.T) {
	_, ok := parseSVGTransform("foo(1,2)")
	if ok { t.Error("expected garbage to fail") }
}
