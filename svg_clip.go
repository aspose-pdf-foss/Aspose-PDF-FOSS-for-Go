// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"fmt"
)

// svgClipPath defines a clipping path; referenced by shapes via clip-path="url(#id)".
type svgClipPath struct {
	units    svgGradientUnits // reuses enum: userSpaceOnUse | objectBoundingBox
	children []svgNode
}

func (*svgClipPath) svgNodeKind() string { return "clipPath" }

// emitClipPathInline writes path construction ops for all clip children, followed
// by W (nonzero) + n. The caller has already emitted q; the clip remains active
// until the matching Q.
//
// Note: objectBoundingBox unit handling (for the bbox transform) is the caller's
// responsibility — Phase 3c performs the simpler userSpaceOnUse case here.
func emitClipPathInline(buf *bytes.Buffer, p *Page, cp *svgClipPath) {
	if cp == nil || len(cp.children) == 0 {
		return
	}
	for _, c := range cp.children {
		emitClipChildPath(buf, c)
	}
	buf.WriteString("W\nn\n")
}

// emitClipChildPath writes path construction ops (m/l/c/h/re, NO paint op) for a
// single clip child. Supports rect/circle/ellipse/line/polyline/polygon/path;
// skips others silently.
func emitClipChildPath(buf *bytes.Buffer, n svgNode) {
	switch s := n.(type) {
	case *svgRect:
		fmt.Fprintf(buf, "%s %s %s %s re\n",
			formatFloat(s.x), formatFloat(s.y), formatFloat(s.w), formatFloat(s.h))
	case *svgCircle:
		buf.WriteString(ellipsePathOps(s.cx, s.cy, s.r, s.r))
	case *svgEllipse:
		buf.WriteString(ellipsePathOps(s.cx, s.cy, s.rx, s.ry))
	case *svgLine:
		fmt.Fprintf(buf, "%s %s m\n", formatFloat(s.x1), formatFloat(s.y1))
		fmt.Fprintf(buf, "%s %s l\n", formatFloat(s.x2), formatFloat(s.y2))
	case *svgPolyline:
		if len(s.points) >= 1 {
			fmt.Fprintf(buf, "%s %s m\n", formatFloat(s.points[0].X), formatFloat(s.points[0].Y))
			for _, pt := range s.points[1:] {
				fmt.Fprintf(buf, "%s %s l\n", formatFloat(pt.X), formatFloat(pt.Y))
			}
		}
	case *svgPolygon:
		if len(s.points) >= 1 {
			fmt.Fprintf(buf, "%s %s m\n", formatFloat(s.points[0].X), formatFloat(s.points[0].Y))
			for _, pt := range s.points[1:] {
				fmt.Fprintf(buf, "%s %s l\n", formatFloat(pt.X), formatFloat(pt.Y))
			}
			buf.WriteString("h\n")
		}
	case *svgPath:
		path := NewPath()
		for _, op := range s.commands {
			switch op.kind {
			case 'M':
				path.MoveTo(op.args[0], op.args[1])
			case 'L':
				path.LineTo(op.args[0], op.args[1])
			case 'C':
				path.CurveTo(op.args[0], op.args[1], op.args[2], op.args[3], op.args[4], op.args[5])
			case 'Q':
				path.QuadTo(op.args[0], op.args[1], op.args[2], op.args[3])
			case 'Z':
				path.Close()
			}
		}
		buf.WriteString(pathOpsToOperators(path.ops))
	}
}
