// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"testing"
)

// TestAnnotColorFromComponents covers the 1/3/4-number annotation colour arrays
// (DeviceGray / DeviceRGB / DeviceCMYK) and the empty/absent case.
func TestAnnotColorFromComponents(t *testing.T) {
	if c := annotColorFromComponents(pdfArray{0.466667}); c == nil || c.R != c.G || c.G != c.B {
		t.Errorf("1-component gray = %+v, want equal R=G=B", c)
	}
	if c := annotColorFromComponents(pdfArray{0.2, 0.6, 0.466667}); c == nil || c.R != 0.2 {
		t.Errorf("3-component RGB = %+v, want R=0.2", c)
	}
	if c := annotColorFromComponents(pdfArray{0.2, 0.3, 0.7, 0.6}); c == nil {
		t.Error("4-component CMYK returned nil, want a colour")
	}
	if c := annotColorFromComponents(pdfArray{}); c != nil {
		t.Errorf("empty array = %+v, want nil (transparent)", c)
	}
	if c := annotColorFromComponents(nil); c != nil {
		t.Errorf("nil array = %+v, want nil", c)
	}
}

func squareWith(dict pdfDict) *SquareAnnotation {
	dict["/Subtype"] = pdfName("/Square")
	if _, ok := dict["/Rect"]; !ok {
		dict["/Rect"] = pdfArray{0.0, 0.0, 50.0, 100.0}
	}
	return &SquareAnnotation{drawingAnnotationBase: drawingAnnotationBase{
		annotationBase: annotationBase{dict: dict},
	}}
}

// TestSquareInteriorColorComponents checks /IC parsing accepts gray and CMYK,
// not just RGB (34415.pdf fills its first two rectangles with 1- and
// 4-component interior colours).
func TestSquareInteriorColorComponents(t *testing.T) {
	if ic := squareWith(pdfDict{"/IC": pdfArray{0.466667}}).InteriorColor(); ic == nil {
		t.Error("gray /IC returned nil")
	}
	if ic := squareWith(pdfDict{"/IC": pdfArray{0.2, 0.3, 0.7, 0.6}}).InteriorColor(); ic == nil {
		t.Error("CMYK /IC returned nil")
	}
}

// TestSquareAppearanceNoColorIsEmpty checks that a Square with neither /C nor
// /IC paints nothing — no default black border invented by the synthesizer
// (34415.pdf's fourth rectangle has both absent and must render blank).
func TestSquareAppearanceNoColorIsEmpty(t *testing.T) {
	ap := generateSquareAppearance(squareWith(pdfDict{}))
	if ap == nil {
		t.Fatal("generateSquareAppearance returned nil")
	}
	for _, op := range [][]byte{[]byte("\nS\n"), []byte("\nf\n"), []byte("\nB\n")} {
		if bytes.Contains(ap.Data, op) {
			t.Errorf("empty Square appearance contains paint op %q; want no drawing\ncontent=%q", op, ap.Data)
		}
	}

	// Sanity: a Square WITH an interior colour does paint.
	ap2 := generateSquareAppearance(squareWith(pdfDict{"/IC": pdfArray{0.2, 0.6, 0.466667}}))
	if !bytes.Contains(ap2.Data, []byte("\nf\n")) {
		t.Errorf("filled Square missing a fill op; content=%q", ap2.Data)
	}
}
