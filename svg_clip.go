// SPDX-License-Identifier: MIT

package asposepdf

// svgClipPath defines a clipping path; referenced by shapes via clip-path="url(#id)".
type svgClipPath struct {
	units    svgGradientUnits // reuses enum: userSpaceOnUse | objectBoundingBox
	children []svgNode
}

func (*svgClipPath) svgNodeKind() string { return "clipPath" }
