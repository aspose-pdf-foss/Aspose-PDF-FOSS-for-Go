// SPDX-License-Identifier: MIT

package asposepdf

// svgNode is the interface implemented by every IR node.
type svgNode interface {
	svgNodeKind() string
}

// svgMatrix is a 2D affine transform in column-major order:
//
//	[a c e]
//	[b d f]
//	[0 0 1]
//
// stored as [a, b, c, d, e, f].
type svgMatrix [6]float64

// svgViewBox holds the four numbers of <svg viewBox="x y w h">.
type svgViewBox struct {
	x, y, w, h float64
}

// svgPreserveAspect holds the parsed preserveAspectRatio attribute.
// align is one of "none" / "xMinYMin" / "xMidYMin" / "xMaxYMin" / "xMinYMid"
// / "xMidYMid" / "xMaxYMid" / "xMinYMax" / "xMidYMax" / "xMaxYMax".
// meetOrSlice is "meet" (default) or "slice"; ignored when align == "none".
type svgPreserveAspect struct {
	align       string
	meetOrSlice string
}

// svgStyle holds resolved presentation attributes after parent cascade.
type svgStyle struct {
	fill          *svgPaint // was *Color
	stroke        *svgPaint // was *Color
	strokeWidth   float64
	dashArray     []float64
	dashOffset    float64
	lineCap       LineCap
	lineJoin      LineJoin
	miterLimit    float64
	opacity       float64
	fillOpacity   float64
	strokeOpacity float64
	fillRule      string
	display       bool

	// Text-specific (Phase 3b)
	fontFamily string
	fontSize   float64
	bold       bool
	italic     bool
	anchor     svgTextAnchor

	// Phase 3c: clip-path reference (bare id, no #). Empty = no clip.
	clipPath string

	// Phase 3d: mask reference (bare id, no #). Empty = no mask.
	mask string

	// Phase 3d: CSS selector matching support
	cssClasses []string
	cssID      string
}

// defaultSVGStyle returns the SVG initial value per SVG spec §6.2 table.
// fill = black, no stroke, opacity = 1, fillRule = nonzero, display = true.
func defaultSVGStyle() svgStyle {
	return svgStyle{
		fill:          &svgPaint{color: &Color{R: 0, G: 0, B: 0, A: 1}},
		stroke:        nil,
		strokeWidth:   1,
		lineCap:       LineCapButt,
		lineJoin:      LineJoinMiter,
		miterLimit:    4,
		opacity:       1,
		fillOpacity:   1,
		strokeOpacity: 1,
		fillRule:      "nonzero",
		display:       true,

		// Text defaults (Phase 3b)
		fontFamily: "", // empty = inherit / use heuristic
		fontSize:   16, // CSS spec default
		bold:       false,
		italic:     false,
		anchor:     svgTextAnchorStart,
	}
}

// svgPathOp is one normalized path command (absolute coords, expanded shortcut).
type svgPathOp struct {
	kind byte       // 'M', 'L', 'C', 'Q', 'A', 'Z'
	args [7]float64 // command-specific; A uses all 7, M/L use [0..1], C uses [0..5], Q uses [0..3], Z uses none
}

type svgGroup struct {
	transform *svgMatrix
	style     svgStyle
	children  []svgNode
}

func (*svgGroup) svgNodeKind() string { return "g" }

type svgPath struct {
	commands  []svgPathOp
	style     svgStyle
	transform *svgMatrix
}

func (*svgPath) svgNodeKind() string { return "path" }

type svgRect struct {
	x, y, w, h, rx, ry float64
	style              svgStyle
	transform          *svgMatrix
}

func (*svgRect) svgNodeKind() string { return "rect" }

type svgCircle struct {
	cx, cy, r float64
	style     svgStyle
	transform *svgMatrix
}

func (*svgCircle) svgNodeKind() string { return "circle" }

type svgEllipse struct {
	cx, cy, rx, ry float64
	style          svgStyle
	transform      *svgMatrix
}

func (*svgEllipse) svgNodeKind() string { return "ellipse" }

type svgLine struct {
	x1, y1, x2, y2 float64
	style          svgStyle
	transform      *svgMatrix
}

func (*svgLine) svgNodeKind() string { return "line" }

type svgPolyline struct {
	points    []Point
	style     svgStyle
	transform *svgMatrix
}

func (*svgPolyline) svgNodeKind() string { return "polyline" }

type svgPolygon struct {
	points    []Point
	style     svgStyle
	transform *svgMatrix
}

func (*svgPolygon) svgNodeKind() string { return "polygon" }

// SVG is the pre-parsed SVG document.
type SVG struct {
	viewBox   *svgViewBox
	width     float64
	height    float64
	par       svgPreserveAspect
	root      *svgGroup
	gradients map[string]svgGradient // id → gradient definition (collected from <defs>)

	// Phase 3c: generalized definition registry. Any element with `id` attribute
	// (inside <defs> or top-level) is collected here for <use> reference resolution
	// and clip-path lookup. Keys are bare ids (no #).
	defs map[string]svgNode

	// Phase 3d: parsed CSS rules from <style> blocks
	cssRules []cssRule
}
