// SPDX-License-Identifier: MIT

package asposepdf

import "testing"

func TestAnnotHiddenFlags(t *testing.T) {
	objects := map[int]*pdfObject{}
	cases := []struct {
		flag int
		want bool
	}{
		{0, false},  // visible
		{2, true},   // Hidden (bit 2)
		{32, true},  // NoView (bit 6)
		{4, false},  // Print only
		{34, true},  // Hidden|NoView
	}
	for _, c := range cases {
		if got := annotHidden(objects, pdfDict{"/F": c.flag}); got != c.want {
			t.Errorf("annotHidden(/F %d) = %v, want %v", c.flag, got, c.want)
		}
	}
}

func TestNormRect(t *testing.T) {
	if _, ok := normRect([]float64{0, 0, 10}); ok {
		t.Error("short rect accepted")
	}
	if _, ok := normRect([]float64{5, 5, 5, 20}); ok {
		t.Error("zero-width rect accepted")
	}
	r, ok := normRect([]float64{30, 40, 10, 20}) // reversed corners
	if !ok || r != [4]float64{10, 20, 30, 40} {
		t.Errorf("normRect reversed = %v ok=%v, want [10 20 30 40]", r, ok)
	}
}

// TestAnnotAppearanceMatrix maps a 0..100 BBox (identity /Matrix) onto a
// 200×100 rect at offset (10,20); the matrix must scale ×2/×1 and translate.
func TestAnnotAppearanceMatrix(t *testing.T) {
	objects := map[int]*pdfObject{}
	ap := &pdfStream{Dict: pdfDict{"/BBox": pdfArray{0, 0, 100, 100}}}
	m, ok := annotAppearanceMatrix(objects, ap, [4]float64{10, 20, 210, 120})
	if !ok {
		t.Fatal("matrix not computed")
	}
	// BBox corner (0,0) → rect corner (10,20); (100,100) → (210,120).
	x0, y0 := applyPt(m, 0, 0)
	x1, y1 := applyPt(m, 100, 100)
	if x0 != 10 || y0 != 20 || x1 != 210 || y1 != 120 {
		t.Errorf("mapped corners = (%g,%g)-(%g,%g), want (10,20)-(210,120)", x0, y0, x1, y1)
	}
}
