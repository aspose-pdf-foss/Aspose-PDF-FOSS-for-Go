// SPDX-License-Identifier: MIT

package asposepdf

// svgMask defines a mask; referenced by shapes via mask="url(#id)".
// maskUnits determines the coordinate system for the mask container (default: objectBoundingBox).
// maskContentUnits determines the coordinate system for children (default: userSpaceOnUse).
type svgMask struct {
	units        svgGradientUnits // maskUnits: default objectBoundingBox
	contentUnits svgGradientUnits // maskContentUnits: default userSpaceOnUse
	children     []svgNode
}

func (*svgMask) svgNodeKind() string { return "mask" }
