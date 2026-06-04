// SPDX-License-Identifier: MIT

package asposepdf

import "testing"

// TestRenderShOperatorAxial drives the `sh` operator directly with a manually
// built axial shading (black→white, extended both ways) and checks that the
// whole page becomes a horizontal gradient.
func TestRenderShOperatorAxial(t *testing.T) {
	doc := NewDocument(100, 100)
	p, _ := doc.Page(1)

	fn := pdfDict{
		"/FunctionType": 2,
		"/Domain":       pdfArray{0, 1},
		"/C0":           pdfArray{0.0, 0.0, 0.0},
		"/C1":           pdfArray{1.0, 1.0, 1.0},
		"/N":            1,
	}
	sh := pdfDict{
		"/ShadingType": 2,
		"/ColorSpace":  pdfName("/DeviceRGB"),
		"/Coords":      pdfArray{0.0, 0.0, 100.0, 0.0},
		"/Function":    fn,
		"/Extend":      pdfArray{true, true},
	}
	res := p.pageResources()
	res["/Shading"] = pdfDict{"/Sh1": sh}

	if err := p.appendToContentStream([]byte("/Sh1 sh\n")); err != nil {
		t.Fatalf("appendToContentStream: %v", err)
	}
	img, err := p.RenderImage(RenderOptions{DPI: 72})
	if err != nil {
		t.Fatalf("RenderImage: %v", err)
	}

	left, _, _, _ := img.At(8, 50).RGBA()
	mid, _, _, _ := img.At(50, 50).RGBA()
	right, _, _, _ := img.At(92, 50).RGBA()
	l, m, r := int(left>>8), int(mid>>8), int(right>>8)
	if !(l < 60) {
		t.Errorf("left = %d, want dark (near C0 black)", l)
	}
	if !(r > 195) {
		t.Errorf("right = %d, want light (near C1 white)", r)
	}
	if !(l < m && m < r) {
		t.Errorf("not a left→right ramp: left=%d mid=%d right=%d", l, m, r)
	}
}
