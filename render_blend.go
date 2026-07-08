// SPDX-License-Identifier: MIT

package asposepdf

import "image"
import "math"

// blendFunc is a separable blend mode B(Cb, Cs): given a backdrop and source
// channel value in [0,1] it returns the blended channel (ISO 32000-1 §11.3.5).
// A nil blendFunc means Normal (plain source-over).
type blendFunc func(cb, cs float64) float64

// nonSepBlend is a non-separable blend mode operating on the whole colour
// (ISO 32000-1 §11.3.5.3): Hue, Saturation, Color, Luminosity.
type nonSepBlend func(cbr, cbg, cbb, csr, csg, csb float64) (float64, float64, float64)

// blendMode is the active blend on the graphics state: at most one of sep / ns
// is non-nil; the zero value is Normal (plain source-over). css carries the
// CSS mix-blend-mode keyword for the SVG export ("" = normal).
type blendMode struct {
	sep blendFunc
	ns  nonSepBlend
	css string
}

// blendModeFor resolves a /BM name to a blendMode (zero value = Normal).
func blendModeFor(name string) blendMode {
	m := blendMode{}
	if f := blendFor(name); f != nil {
		m.sep = f
	} else {
		switch name {
		case "/Hue":
			m.ns = blendHue
		case "/Saturation":
			m.ns = blendSaturation
		case "/Color":
			m.ns = blendColorMode
		case "/Luminosity":
			m.ns = blendLuminosity
		}
	}
	if m.sep != nil || m.ns != nil {
		m.css = cssBlendName(name)
	}
	return m
}

// cssBlendName maps a PDF /BM name to the equivalent CSS mix-blend-mode
// keyword — CSS supports all 15 non-Normal PDF blend modes.
func cssBlendName(name string) string {
	switch name {
	case "/Multiply":
		return "multiply"
	case "/Screen":
		return "screen"
	case "/Overlay":
		return "overlay"
	case "/Darken":
		return "darken"
	case "/Lighten":
		return "lighten"
	case "/ColorDodge":
		return "color-dodge"
	case "/ColorBurn":
		return "color-burn"
	case "/HardLight":
		return "hard-light"
	case "/SoftLight":
		return "soft-light"
	case "/Difference":
		return "difference"
	case "/Exclusion":
		return "exclusion"
	case "/Hue":
		return "hue"
	case "/Saturation":
		return "saturation"
	case "/Color":
		return "color"
	case "/Luminosity":
		return "luminosity"
	}
	return ""
}

// blendApply composites source (sr,sg,sb) over the backdrop pixel at off with
// effective alpha a, applying the blend mode (ISO 32000-1 §11.3.6 for an opaque
// backdrop: C = (1−a)·Cb + a·B(Cb,Cs)).
func blendApply(dst *image.RGBA, off int, sr, sg, sb uint8, a float64, m blendMode) {
	switch {
	case m.ns != nil:
		cbr, cbg, cbb := float64(dst.Pix[off])/255, float64(dst.Pix[off+1])/255, float64(dst.Pix[off+2])/255
		mr, mg, mb := m.ns(cbr, cbg, cbb, float64(sr)/255, float64(sg)/255, float64(sb)/255)
		dst.Pix[off+0] = clamp255((1-a)*cbr + a*mr)
		dst.Pix[off+1] = clamp255((1-a)*cbg + a*mg)
		dst.Pix[off+2] = clamp255((1-a)*cbb + a*mb)
	case m.sep != nil:
		dst.Pix[off+0] = blendChannel(dst.Pix[off+0], sr, a, m.sep)
		dst.Pix[off+1] = blendChannel(dst.Pix[off+1], sg, a, m.sep)
		dst.Pix[off+2] = blendChannel(dst.Pix[off+2], sb, a, m.sep)
	default:
		inv := 1 - a
		dst.Pix[off+0] = uint8(float64(sr)*a + float64(dst.Pix[off+0])*inv + 0.5)
		dst.Pix[off+1] = uint8(float64(sg)*a + float64(dst.Pix[off+1])*inv + 0.5)
		dst.Pix[off+2] = uint8(float64(sb)*a + float64(dst.Pix[off+2])*inv + 0.5)
	}
	inv := 1 - a
	dst.Pix[off+3] = uint8(a*255 + float64(dst.Pix[off+3])*inv + 0.5)
}

func clamp255(v float64) uint8 {
	if v <= 0 {
		return 0
	}
	if v >= 1 {
		return 255
	}
	return uint8(v*255 + 0.5)
}

// --- non-separable colour helpers (ISO 32000-1 §11.3.5.3) ---

func lum(r, g, b float64) float64 { return 0.3*r + 0.59*g + 0.11*b }

func clipColor(r, g, b float64) (float64, float64, float64) {
	l := lum(r, g, b)
	n := math.Min(r, math.Min(g, b))
	x := math.Max(r, math.Max(g, b))
	if n < 0 && l != n {
		r = l + (r-l)*l/(l-n)
		g = l + (g-l)*l/(l-n)
		b = l + (b-l)*l/(l-n)
	}
	if x > 1 && x != l {
		r = l + (r-l)*(1-l)/(x-l)
		g = l + (g-l)*(1-l)/(x-l)
		b = l + (b-l)*(1-l)/(x-l)
	}
	return r, g, b
}

func setLum(r, g, b, l float64) (float64, float64, float64) {
	d := l - lum(r, g, b)
	return clipColor(r+d, g+d, b+d)
}

func sat(r, g, b float64) float64 {
	return math.Max(r, math.Max(g, b)) - math.Min(r, math.Min(g, b))
}

// setSat rescales (r,g,b) to saturation s while preserving the colours' order.
func setSat(r, g, b, s float64) (float64, float64, float64) {
	c := [3]float64{r, g, b}
	mn, md, mx := 0, 1, 2
	if c[mn] > c[md] {
		mn, md = md, mn
	}
	if c[md] > c[mx] {
		md, mx = mx, md
	}
	if c[mn] > c[md] {
		mn, md = md, mn
	}
	var out [3]float64
	if c[mx] > c[mn] {
		out[md] = (c[md] - c[mn]) * s / (c[mx] - c[mn])
		out[mx] = s
	}
	return out[0], out[1], out[2]
}

func blendHue(cbr, cbg, cbb, csr, csg, csb float64) (float64, float64, float64) {
	r, g, b := setSat(csr, csg, csb, sat(cbr, cbg, cbb))
	return setLum(r, g, b, lum(cbr, cbg, cbb))
}

func blendSaturation(cbr, cbg, cbb, csr, csg, csb float64) (float64, float64, float64) {
	r, g, b := setSat(cbr, cbg, cbb, sat(csr, csg, csb))
	return setLum(r, g, b, lum(cbr, cbg, cbb))
}

func blendColorMode(cbr, cbg, cbb, csr, csg, csb float64) (float64, float64, float64) {
	return setLum(csr, csg, csb, lum(cbr, cbg, cbb))
}

func blendLuminosity(cbr, cbg, cbb, csr, csg, csb float64) (float64, float64, float64) {
	return setLum(cbr, cbg, cbb, lum(csr, csg, csb))
}

// blendFor maps a /BM blend-mode name to its function, or nil for Normal /
// Compatible / non-separable / unsupported modes (which fall back to src-over).
func blendFor(name string) blendFunc {
	switch name {
	case "/Multiply":
		return func(cb, cs float64) float64 { return cb * cs }
	case "/Screen":
		return blendScreen
	case "/Overlay":
		return func(cb, cs float64) float64 { return blendHardLight(cs, cb) }
	case "/Darken":
		return math.Min
	case "/Lighten":
		return math.Max
	case "/ColorDodge":
		return blendColorDodge
	case "/ColorBurn":
		return blendColorBurn
	case "/HardLight":
		return blendHardLight
	case "/SoftLight":
		return blendSoftLight
	case "/Difference":
		return func(cb, cs float64) float64 { return math.Abs(cb - cs) }
	case "/Exclusion":
		return func(cb, cs float64) float64 { return cb + cs - 2*cb*cs }
	default:
		return nil
	}
}

func blendScreen(cb, cs float64) float64 { return cb + cs - cb*cs }

func blendHardLight(cb, cs float64) float64 {
	if cs <= 0.5 {
		return cb * (2 * cs)
	}
	return blendScreen(cb, 2*cs-1)
}

func blendColorDodge(cb, cs float64) float64 {
	if cb == 0 {
		return 0
	}
	if cs >= 1 {
		return 1
	}
	return math.Min(1, cb/(1-cs))
}

func blendColorBurn(cb, cs float64) float64 {
	if cb >= 1 {
		return 1
	}
	if cs <= 0 {
		return 0
	}
	return 1 - math.Min(1, (1-cb)/cs)
}

func blendSoftLight(cb, cs float64) float64 {
	if cs <= 0.5 {
		return cb - (1-2*cs)*cb*(1-cb)
	}
	var d float64
	if cb <= 0.25 {
		d = ((16*cb-12)*cb + 4) * cb
	} else {
		d = math.Sqrt(cb)
	}
	return cb + (2*cs-1)*(d-cb)
}

// blendChannel combines a source channel over a backdrop channel (both 0..255)
// with effective alpha a, using blend B (nil → Normal): the ISO 32000-1 §11.3.6
// result for an opaque backdrop, C = (1-a)·Cb + a·B(Cb,Cs).
func blendChannel(dst, src uint8, a float64, b blendFunc) uint8 {
	cb := float64(dst) / 255
	cs := float64(src) / 255
	mixed := cs
	if b != nil {
		mixed = b(cb, cs)
	}
	return uint8(((1-a)*cb+a*mixed)*255 + 0.5)
}
