// SPDX-License-Identifier: MIT

package asposepdf

type svgTextAnchor int

const (
	svgTextAnchorStart  svgTextAnchor = 0 // default (left of x)
	svgTextAnchorMiddle svgTextAnchor = 1
	svgTextAnchorEnd    svgTextAnchor = 2
)

// svgTextRun is a single contiguous text run at an absolute position.
// One <text> element produces one or more runs (one per <tspan> + leading/trailing
// CharData of the parent text element).
type svgTextRun struct {
	text  string
	x, y  float64
	style svgStyle // resolved style (font, fill, etc.)
}

// svgText is the IR node for an SVG <text> element.
type svgText struct {
	runs      []svgTextRun
	style     svgStyle // root-level style of the <text> element
	transform *svgMatrix
}

func (*svgText) svgNodeKind() string { return "text" }
