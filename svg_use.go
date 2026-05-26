// SPDX-License-Identifier: MIT

package asposepdf

// svgUse is a placeholder before resolveUseReferences (Task 6) replaces it
// with the cloned referent. After resolution, no *svgUse nodes remain in
// the IR tree.
type svgUse struct {
	refID     string
	x, y      float64
	style     svgStyle
	transform *svgMatrix
}

func (*svgUse) svgNodeKind() string { return "use" }

// svgSymbol is a container with its own viewBox. Not rendered directly;
// only referenced via <use href="#symbolId">.
type svgSymbol struct {
	viewBox  *svgViewBox
	children []svgNode
	style    svgStyle
}

func (*svgSymbol) svgNodeKind() string { return "symbol" }
