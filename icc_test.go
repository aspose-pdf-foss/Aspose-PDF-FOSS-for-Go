// SPDX-License-Identifier: MIT

package asposepdf

import (
	"math"
	"testing"
)

// The bundled sRGB profile (pdfa_convert.go) must parse, be recognized as
// sRGB-like, and transform ~identically.
func TestICCParseSRGBProfile(t *testing.T) {
	p, err := parseICCProfile(srgbICCProfile())
	if err != nil {
		t.Fatal(err)
	}
	if p.space != "RGB" || p.nComp != 3 {
		t.Fatalf("space=%s n=%d", p.space, p.nComp)
	}
	if !p.srgbLike {
		t.Error("bundled sRGB profile not detected as sRGB-like")
	}
	// Midtone probes: the bundled profile approximates sRGB's piecewise
	// curve with a pure gamma 2.2, so the dark toe legitimately deviates.
	for _, v := range [][3]float64{{0, 0, 0}, {1, 1, 1}, {0.5, 0.5, 0.5}, {0.8, 0.4, 0.6}} {
		r, g, b := p.toSRGB(v[:])
		for i, got := range []float64{r, g, b} {
			if math.Abs(got-v[i]) > 0.02 {
				t.Errorf("sRGB identity: in=%v out=(%f %f %f)", v, r, g, b)
				break
			}
		}
	}
}

// A synthetic AdobeRGB (1998) matrix/TRC profile: pure gamma 563/256,
// AdobeRGB primaries D50-adapted. Saturated green must shift DOWN when
// converted to sRGB (AdobeRGB green is outside sRGB).
func TestICCAdobeRGBShifts(t *testing.T) {
	p := &iccProfile{space: "RGB", nComp: 3}
	p.m = [9]float64{
		0.6097, 0.2053, 0.1492,
		0.3111, 0.6257, 0.0632,
		0.0195, 0.0609, 0.7448,
	}
	g := 563.0 / 256
	for i := range p.trc {
		p.trc[i] = &iccCurve{gamma: g}
	}
	if p.matrixIsSRGB() {
		t.Fatal("AdobeRGB detected as sRGB")
	}
	// Pure AdobeRGB green lies OUTSIDE sRGB — the matrix conversion clips
	// (negative sRGB red). A mid-saturation colour is representable and
	// must actually move: AdobeRGB (0.6, 0.8, 0.3) is more saturated in
	// sRGB terms, so the converted channels differ from a pass-through.
	r, gg, b := p.toSRGB([]float64{0.6, 0.8, 0.3})
	moved := math.Abs(r-0.6) > 0.03 || math.Abs(gg-0.8) > 0.03 || math.Abs(b-0.3) > 0.03
	if !moved {
		t.Errorf("AdobeRGB midtone did not move: (%f %f %f)", r, gg, b)
	}
	if gg < 0.7 {
		t.Errorf("green collapsed: %f", gg)
	}
	// White stays white, black stays black.
	if r, g, b := p.toSRGB([]float64{1, 1, 1}); r < 0.98 || g < 0.98 || b < 0.98 {
		t.Errorf("white drifted: (%f %f %f)", r, g, b)
	}
	if r, g, b := p.toSRGB([]float64{0, 0, 0}); r > 0.02 || g > 0.02 || b > 0.02 {
		t.Errorf("black drifted: (%f %f %f)", r, g, b)
	}
}

// Gray gamma-2.2 profile: mid-gray input must come out near the sRGB
// encoding of 0.5^2.2.
func TestICCGrayTRC(t *testing.T) {
	p := &iccProfile{space: "GRAY", nComp: 1, kTRC: &iccCurve{gamma: 2.2}}
	r, g, b := p.toSRGB([]float64{0.5})
	if r != g || g != b {
		t.Fatalf("gray must stay neutral: (%f %f %f)", r, g, b)
	}
	want := srgbEncode(math.Pow(0.5, 2.2))
	if math.Abs(r-want) > 0.01 {
		t.Errorf("gray 0.5 → %f, want %f", r, want)
	}
}

// Synthetic 2-grid mft2 CMYK→Lab LUT: an identity-ish pipeline where
// K=0,C=M=Y=0 maps to white (L=100) and full K maps to black.
func TestICCLUTCMYK(t *testing.T) {
	grid := 2
	inCh, outCh := 4, 3
	l := &iccLUT{inCh: inCh, outCh: outCh, grid: grid}
	for i := 0; i < inCh; i++ {
		l.inCurves = append(l.inCurves, []float64{0, 1})
	}
	for i := 0; i < outCh; i++ {
		l.outCurves = append(l.outCurves, []float64{0, 1})
	}
	// CLUT: L = (1-K)*(1-max(C,M,Y) fudge) — enough to check corners; the
	// Lab encoding of white is L=0xFF00/0xFFFF.
	n := 1
	for i := 0; i < inCh; i++ {
		n *= grid
	}
	l.clut = make([]float64, n*outCh)
	labL := func(v float64) float64 { return v * 65280 / 65535 }
	labAB := func(v float64) float64 { return (v + 128) / 255 * 65280 / 65535 }
	for idx := 0; idx < n; idx++ {
		// Decode grid coords (last channel fastest).
		rem := idx
		coord := make([]int, inCh)
		for i := inCh - 1; i >= 0; i-- {
			coord[i] = rem % grid
			rem /= grid
		}
		c, m, y, k := coord[0], coord[1], coord[2], coord[3]
		ink := c | m | y | k
		L := 100.0
		if ink == 1 {
			L = 0
		}
		l.clut[idx*outCh+0] = labL(L / 100)
		l.clut[idx*outCh+1] = labAB(0)
		l.clut[idx*outCh+2] = labAB(0)
	}
	p := &iccProfile{space: "CMYK", nComp: 4, pcsLab: true, a2b: l}

	r, g, b := p.toSRGB([]float64{0, 0, 0, 0})
	if r < 0.95 || g < 0.95 || b < 0.95 {
		t.Errorf("CMYK white → (%f %f %f)", r, g, b)
	}
	r, g, b = p.toSRGB([]float64{0, 0, 0, 1})
	if r > 0.05 || g > 0.05 || b > 0.05 {
		t.Errorf("CMYK black → (%f %f %f)", r, g, b)
	}
}
