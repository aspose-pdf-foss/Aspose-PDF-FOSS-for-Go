// SPDX-License-Identifier: MIT

package asposepdf

import "testing"

// TestShadingDeviceNTintColor checks that a shading whose colour space is
// /DeviceN (or /Separation) runs the function output through the tint transform
// instead of reading the lone component as DeviceGray. 33319-2.pdf paints its
// background with a /DeviceN [/Black] radial shading whose tint runs 0..0.1;
// read as gray that is near-black, but through the CMYK tint transform it is the
// light gray Acrobat shows.
func TestShadingDeviceNTintColor(t *testing.T) {
	type2 := func(c0, c1 pdfArray) pdfDict {
		return pdfDict{"/FunctionType": 2, "/Domain": pdfArray{0.0, 1.0}, "/C0": c0, "/C1": c1, "/N": 1.0}
	}
	shDict := pdfDict{
		"/ShadingType": 3,
		"/Coords":      pdfArray{0.0, 0.0, 0.0, 0.0, 0.0, 1.0},
		"/Domain":      pdfArray{0.0, 1.0},
		"/Extend":      pdfArray{true, true},
		// /Black tint -> DeviceCMYK [0 0 0 tint].
		"/ColorSpace": pdfArray{
			pdfName("/DeviceN"), pdfArray{pdfName("/Black")}, pdfName("/DeviceCMYK"),
			type2(pdfArray{0.0, 0.0, 0.0, 0.0}, pdfArray{0.0, 0.0, 0.0, 1.0}),
		},
		// Shading function: tint runs 0 -> 0.1 across the radius.
		"/Function": type2(pdfArray{0.0}, pdfArray{0.1}),
	}

	s := parseShading(map[int]*pdfObject{}, shDict)
	if s == nil {
		t.Fatal("parseShading returned nil")
	}
	if s.tint == nil {
		t.Fatal("DeviceN colour space did not produce a tint converter")
	}

	// t=0 -> CMYK K=0 -> white; t=1 -> CMYK K=0.1 -> light gray (~230), not the
	// near-black (~25) that reading the tint as DeviceGray would give.
	r0, _, _ := s.colorAt(0)
	if r0 < 250 {
		t.Errorf("colorAt(0) R = %d, want ~255 (white)", r0)
	}
	r1, g1, b1 := s.colorAt(1)
	if r1 < 200 {
		t.Errorf("colorAt(1) = (%d,%d,%d), want light gray (R>200); tint transform not applied", r1, g1, b1)
	}
}
