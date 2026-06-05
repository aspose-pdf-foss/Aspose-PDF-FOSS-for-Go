// SPDX-License-Identifier: MIT

package asposepdf

import "math"

// blendFunc is a separable blend mode B(Cb, Cs): given a backdrop and source
// channel value in [0,1] it returns the blended channel (ISO 32000-1 §11.3.5).
// A nil blendFunc means Normal (plain source-over).
type blendFunc func(cb, cs float64) float64

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
