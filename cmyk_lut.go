// SPDX-License-Identifier: MIT

// Code generated from a 5x5x5x5 DeviceCMYK->sRGB lookup table baked from the
// MuPDF/Adobe default CMYK profile (matches Acrobat far better than the naive
// (1-C)(1-K) formula). Do not edit by hand. Mean error vs the profile ~1.4 RGB.

package asposepdf

import "encoding/base64"

// cmykLUT holds 5*5*5*5 RGB triples for CMYK grid points at {0,.25,.5,.75,1}^4,
// indexed as ((((c*5+m)*5+y)*5+k)*3 + channel).
var cmykLUT = mustDecodeCMYKLUT()

func mustDecodeCMYKLUT() []byte {
	b, err := base64.StdEncoding.DecodeString(cmykLUTB64)
	if err != nil || len(b) != 5*5*5*5*3 {
		return nil
	}
	return b
}

const cmykLUTB64 = "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"

// adobeCMYKToRGB converts a DeviceCMYK colour (components in [0,1]) to sRGB via
// quadrilinear interpolation of the baked Adobe-profile LUT. Falls back to the
// naive conversion if the table failed to load.
func adobeCMYKToRGB(c, m, y, k float64) (uint8, uint8, uint8) {
	if cmykLUT == nil {
		r := (1 - c) * (1 - k)
		g := (1 - m) * (1 - k)
		b := (1 - y) * (1 - k)
		return cmykClamp8(r * 255), cmykClamp8(g * 255), cmykClamp8(b * 255)
	}
	clampUnit := func(v float64) float64 {
		if v < 0 { return 0 }
		if v > 1 { return 1 }
		return v
	}
	axis := func(v float64) (int, float64) {
		f := clampUnit(v) * 4
		i := int(f)
		if i > 3 { i = 3 }
		return i, f - float64(i)
	}
	ic, fc := axis(c); im, fm := axis(m); iy, fy := axis(y); ik, fk := axis(k)
	var rr, gg, bb float64
	for dc := 0; dc < 2; dc++ {
		wc := 1 - fc; if dc == 1 { wc = fc }
		for dm := 0; dm < 2; dm++ {
			wm := 1 - fm; if dm == 1 { wm = fm }
			for dy := 0; dy < 2; dy++ {
				wy := 1 - fy; if dy == 1 { wy = fy }
				for dk := 0; dk < 2; dk++ {
					wk := 1 - fk; if dk == 1 { wk = fk }
					w := wc * wm * wy * wk
					if w == 0 { continue }
					ci := ic + dc; if ci > 4 { ci = 4 }
					mi := im + dm; if mi > 4 { mi = 4 }
					yi := iy + dy; if yi > 4 { yi = 4 }
					ki := ik + dk; if ki > 4 { ki = 4 }
					o := ((((ci*5+mi)*5+yi)*5+ki) * 3)
					rr += w * float64(cmykLUT[o])
					gg += w * float64(cmykLUT[o+1])
					bb += w * float64(cmykLUT[o+2])
				}
			}
		}
	}
	return cmykClamp8(rr), cmykClamp8(gg), cmykClamp8(bb)
}

// cmykClamp8 rounds and clamps a value already in [0,255] to a byte.
func cmykClamp8(v float64) uint8 {
	if v <= 0 {
		return 0
	}
	if v >= 255 {
		return 255
	}
	return uint8(v + 0.5)
}
