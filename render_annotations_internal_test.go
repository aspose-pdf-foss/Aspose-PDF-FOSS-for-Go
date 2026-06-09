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

// TestAppearanceStreamASSelection verifies /AS is authoritative when the /AP/N
// is a state subdictionary: the named state's stream renders, and an /AS that
// names a state with no stream (e.g. an off checkbox whose /N holds only the
// on-states) renders nothing rather than falling back to an on-appearance.
func TestAppearanceStreamASSelection(t *testing.T) {
	objects := map[int]*pdfObject{}
	on := &pdfStream{Dict: pdfDict{"/BBox": pdfArray{0, 0, 10, 10}}}

	// /N has only an on-state "/1"; an off widget carries /AS /Off.
	off := pdfDict{
		"/AS": pdfName("/Off"),
		"/AP": pdfDict{"/N": pdfDict{"/1": on}},
	}
	if s := appearanceStream(objects, off); s != nil {
		t.Error("off widget (/AS /Off, no /Off stream) drew an appearance, want nil")
	}

	// Same dict but /AS names the present state → that stream renders.
	checked := pdfDict{
		"/AS": pdfName("/1"),
		"/AP": pdfDict{"/N": pdfDict{"/1": on}},
	}
	if s := appearanceStream(objects, checked); s != on {
		t.Errorf("on widget (/AS /1) = %v, want the /1 stream", s)
	}

	// No /AS at all → fall back to any available state stream.
	noAS := pdfDict{"/AP": pdfDict{"/N": pdfDict{"/1": on}}}
	if s := appearanceStream(objects, noAS); s != on {
		t.Errorf("no /AS fallback = %v, want the /1 stream", s)
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
