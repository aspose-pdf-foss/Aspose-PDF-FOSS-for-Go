// SPDX-License-Identifier: MIT

package asposepdf

import (
	"math"
	"testing"
)

// TestRenderBlendMultiply checks the Multiply blend mode: a red rectangle drawn
// over a green backdrop with /BM /Multiply multiplies to black (green·red = 0),
// whereas the backdrop elsewhere stays green.
func TestRenderBlendMultiply(t *testing.T) {
	doc := NewDocument(100, 100)
	p, _ := doc.Page(1)
	p.pageResources()["/ExtGState"] = pdfDict{"/GS1": pdfDict{"/BM": pdfName("/Multiply")}}

	content := "0 1 0 rg 0 0 100 100 re f\n" + // green backdrop
		"/GS1 gs\n" + // blend mode = Multiply
		"1 0 0 rg 20 20 60 60 re f\n" // red rect → green·red = black
	if err := p.appendToContentStream([]byte(content)); err != nil {
		t.Fatal(err)
	}
	img, err := p.RenderImage(RenderOptions{DPI: 72})
	if err != nil {
		t.Fatal(err)
	}

	rgb := func(x, y int) (int, int, int) {
		r, g, b, _ := img.At(x, y).RGBA()
		return int(r >> 8), int(g >> 8), int(b >> 8)
	}
	if r, g, b := rgb(50, 50); r > 40 || g > 40 || b > 40 { // overlap → black
		t.Errorf("Multiply overlap = (%d,%d,%d), want ~black", r, g, b)
	}
	if r, g, b := rgb(5, 95); g < 200 || r > 60 || b > 60 { // backdrop → green
		t.Errorf("backdrop = (%d,%d,%d), want green", r, g, b)
	}
}

// TestBlendFuncs spot-checks a few separable blend functions at known values.
func TestBlendFuncs(t *testing.T) {
	cases := []struct {
		mode       string
		cb, cs, want float64
	}{
		{"/Multiply", 0.5, 0.4, 0.2},
		{"/Screen", 0.5, 0.5, 0.75},
		{"/Darken", 0.3, 0.7, 0.3},
		{"/Lighten", 0.3, 0.7, 0.7},
		{"/Difference", 0.6, 0.2, 0.4},
		{"/Exclusion", 0.5, 0.5, 0.5},
	}
	for _, c := range cases {
		b := blendFor(c.mode)
		if b == nil {
			t.Errorf("%s: no blend func", c.mode)
			continue
		}
		if got := b(c.cb, c.cs); got < c.want-1e-9 || got > c.want+1e-9 {
			t.Errorf("%s(%g,%g) = %g, want %g", c.mode, c.cb, c.cs, got, c.want)
		}
	}
	if blendFor("/Normal") != nil {
		t.Error("/Normal should map to nil (plain src-over)")
	}
}

// TestNonSeparableBlend checks the defining invariants of the non-separable
// modes: Luminosity gives the result the source's luminosity; Color keeps the
// backdrop's luminosity.
func TestNonSeparableBlend(t *testing.T) {
	r, g, b := blendLuminosity(0.2, 0.4, 0.6, 0.5, 0.5, 0.5)
	if math.Abs(lum(r, g, b)-0.5) > 1e-6 {
		t.Errorf("Luminosity result lum = %g, want 0.5 (source's)", lum(r, g, b))
	}
	r, g, b = blendColorMode(0.2, 0.4, 0.6, 1, 0, 0)
	if math.Abs(lum(r, g, b)-lum(0.2, 0.4, 0.6)) > 1e-6 {
		t.Errorf("Color result lum = %g, want %g (backdrop's)", lum(r, g, b), lum(0.2, 0.4, 0.6))
	}
	for _, m := range []string{"/Hue", "/Saturation", "/Color", "/Luminosity"} {
		if blendModeFor(m).ns == nil {
			t.Errorf("%s not resolved as a non-separable mode", m)
		}
	}
}
