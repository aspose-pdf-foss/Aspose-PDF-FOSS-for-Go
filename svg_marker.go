// SPDX-License-Identifier: MIT

package asposepdf

type svgMarkerOrient struct {
	auto  bool
	angle float64 // when auto == false, fixed angle in degrees
}

type svgMarkerUnits int

const (
	svgMarkerStrokeWidth svgMarkerUnits = 0 // default
	svgMarkerUserSpace   svgMarkerUnits = 1
)

type svgMarker struct {
	viewBox          *svgViewBox
	refX, refY       float64
	markerW, markerH float64
	orient           svgMarkerOrient
	units            svgMarkerUnits
	children         []svgNode
}

func (*svgMarker) svgNodeKind() string { return "marker" }
