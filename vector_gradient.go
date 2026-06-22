// SPDX-License-Identifier: MIT

package asposepdf

import "fmt"

// GradientStop is one colour stop in a gradient, positioned at Offset
// (0 at the gradient's start, 1 at its end). Stops should be supplied in
// ascending Offset order.
type GradientStop struct {
	Offset float64
	Color  Color
}

// Gradient is a fill that varies colour across a shape: either a
// LinearGradient or a RadialGradient. Assign one to ShapeStyle.FillGradient
// to fill a DrawRectangle / DrawRoundedRectangle / DrawCircle / DrawEllipse
// / DrawPolygon / DrawPath shape with it.
//
// Coordinates are PDF user space (points; origin at the page bottom-left,
// Y up) — the same space as the shape being filled. Implemented as PDF
// axial (Type 2) and radial (Type 3) shading patterns. Limitations mirror
// the SVG renderer: the spread method is pad (the end colours extend past
// the gradient vector) and per-stop alpha is not rendered (shadings are
// opaque DeviceRGB).
type Gradient interface {
	isGradient()
}

// LinearGradient interpolates colour along the line from (X1, Y1) to
// (X2, Y2). Use NewLinearGradient for the common case.
type LinearGradient struct {
	X1, Y1, X2, Y2 float64
	Stops          []GradientStop
}

func (LinearGradient) isGradient() {}

// RadialGradient interpolates colour from the focal point (FX, FY) out to
// the circle centred at (CX, CY) with radius R. NewRadialGradient sets the
// focal point to the centre (the common case); set FX/FY explicitly for an
// off-centre highlight.
type RadialGradient struct {
	CX, CY, R, FX, FY float64
	Stops             []GradientStop
}

func (RadialGradient) isGradient() {}

// NewLinearGradient builds a linear gradient from (x1, y1) to (x2, y2)
// with the given stops.
func NewLinearGradient(x1, y1, x2, y2 float64, stops ...GradientStop) LinearGradient {
	return LinearGradient{X1: x1, Y1: y1, X2: x2, Y2: y2, Stops: stops}
}

// NewRadialGradient builds a radial gradient on the circle centred at
// (cx, cy) with radius r, focal point at the centre.
func NewRadialGradient(cx, cy, r float64, stops ...GradientStop) RadialGradient {
	return RadialGradient{CX: cx, CY: cy, R: r, FX: cx, FY: cy, Stops: stops}
}

// resolveShapeGradient registers the PDF shading pattern for a style's
// FillGradient (if any) and records its resource name in style.FillPattern
// so the existing pattern-fill emission path paints with it. No-op when
// there is no gradient or a pattern name is already set. Coordinates are
// used verbatim (identity matrix) because the public API works directly in
// PDF user space.
func (p *Page) resolveShapeGradient(style *ShapeStyle) error {
	// A tiling-pattern fill also resolves to a /Pattern resource; handle it here
	// so every Draw* call site picks it up (it sets FillPattern, which then
	// short-circuits the gradient path below).
	if err := p.resolveShapeTiling(style); err != nil {
		return err
	}
	if style.FillGradient == nil || style.FillPattern != "" {
		return nil
	}
	grad, err := toInternalGradient(style.FillGradient)
	if err != nil {
		return err
	}
	name, err := p.ensurePatternResource(grad, matrixIdentity())
	if err != nil {
		return err
	}
	style.FillPattern = name
	return nil
}

// toInternalGradient converts a public Gradient into the internal
// svgGradient the shading machinery consumes. Per-stop alpha is carried as
// opacity but is not rendered by the RGB shading (documented limitation).
func toInternalGradient(g Gradient) (svgGradient, error) {
	switch v := g.(type) {
	case LinearGradient:
		stops, err := toInternalStops(v.Stops)
		if err != nil {
			return nil, err
		}
		return &svgLinearGradient{
			x1: v.X1, y1: v.Y1, x2: v.X2, y2: v.Y2,
			stops: stops, units: svgGradientUserSpace, spread: svgSpreadPad,
		}, nil
	case RadialGradient:
		stops, err := toInternalStops(v.Stops)
		if err != nil {
			return nil, err
		}
		return &svgRadialGradient{
			cx: v.CX, cy: v.CY, r: v.R, fx: v.FX, fy: v.FY,
			stops: stops, units: svgGradientUserSpace, spread: svgSpreadPad,
		}, nil
	default:
		return nil, fmt.Errorf("gradient: unsupported type %T", g)
	}
}

// toInternalStops converts public stops, rejecting an empty list.
func toInternalStops(stops []GradientStop) ([]svgGradientStop, error) {
	if len(stops) == 0 {
		return nil, fmt.Errorf("gradient: at least one stop is required")
	}
	out := make([]svgGradientStop, len(stops))
	for i, s := range stops {
		c := s.Color
		op := c.A
		if op == 0 && c == (Color{}) {
			op = 1 // a fully-zero Color means "unset"; treat as opaque black
		}
		out[i] = svgGradientStop{offset: s.Offset, color: &c, opacity: op}
	}
	return out, nil
}
