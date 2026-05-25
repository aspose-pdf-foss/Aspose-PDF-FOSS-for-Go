// SPDX-License-Identifier: MIT

package asposepdf

// svgGradient is the interface for both linear and radial gradients.
type svgGradient interface {
	gradientKind() string
}

// svgGradientStop is one stop in a gradient (offset 0..1, resolved color, opacity 0..1).
type svgGradientStop struct {
	offset  float64
	color   *Color  // resolved at parse time; never nil
	opacity float64 // multiplied into final alpha; default 1.0
}

type svgGradientUnits int

const (
	svgGradientUserSpace  svgGradientUnits = 0 // userSpaceOnUse (default)
	svgGradientObjectBBox svgGradientUnits = 1 // objectBoundingBox
)

type svgSpreadMethod int

const (
	svgSpreadPad svgSpreadMethod = 0 // default; only one supported in Phase 3a
)

type svgLinearGradient struct {
	x1, y1, x2, y2 float64
	stops           []svgGradientStop
	units           svgGradientUnits
	spread          svgSpreadMethod
	transform       *svgMatrix
}

func (*svgLinearGradient) gradientKind() string { return "linearGradient" }

type svgRadialGradient struct {
	cx, cy, r, fx, fy float64
	stops             []svgGradientStop
	units             svgGradientUnits
	spread            svgSpreadMethod
	transform         *svgMatrix
}

func (*svgRadialGradient) gradientKind() string { return "radialGradient" }

// svgPaint is a tagged union for fill/stroke values: solid color OR gradient ref.
// Both fields nil/empty means "none"/"transparent" (the caller should treat as no paint).
type svgPaint struct {
	color   *Color // non-nil → plain color
	gradRef string // non-empty → url(#id) reference; resolved at render time
}
