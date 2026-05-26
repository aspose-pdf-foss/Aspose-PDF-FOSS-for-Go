// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"fmt"
	"math"
)

// emitMarker emits a single marker instance at (x, y) with the given tangent angle.
// The marker's children are rendered inside a q/Q with composed transforms:
// translate to position → optional rotation (if orient=auto) → scale (markerUnits +
// viewBox) → translate by -(refX, refY).
func emitMarker(buf *bytes.Buffer, p *Page, svg *SVG, m *svgMarker, x, y, angleRad, strokeWidth float64) {
	if m == nil {
		return
	}
	buf.WriteString("q\n")

	// Step 1: translate to position
	fmt.Fprintf(buf, "1 0 0 1 %s %s cm\n", formatFloat(x), formatFloat(y))

	// Step 2: rotate if orient=auto
	if m.orient.auto {
		c, s := math.Cos(angleRad), math.Sin(angleRad)
		fmt.Fprintf(buf, "%s %s %s %s 0 0 cm\n",
			formatFloat(c), formatFloat(s), formatFloat(-s), formatFloat(c))
	} else if m.orient.angle != 0 {
		// Fixed angle in degrees
		rad := m.orient.angle * math.Pi / 180
		c, s := math.Cos(rad), math.Sin(rad)
		fmt.Fprintf(buf, "%s %s %s %s 0 0 cm\n",
			formatFloat(c), formatFloat(s), formatFloat(-s), formatFloat(c))
	}

	// Step 3: scale by markerUnits + viewBox mapping
	scaleX, scaleY := 1.0, 1.0
	if m.units == svgMarkerStrokeWidth {
		sw := strokeWidth
		if sw <= 0 {
			sw = 1
		}
		scaleX, scaleY = sw, sw
	}
	if m.viewBox != nil && m.viewBox.w > 0 && m.viewBox.h > 0 {
		scaleX *= m.markerW / m.viewBox.w
		scaleY *= m.markerH / m.viewBox.h
	}
	if scaleX != 1 || scaleY != 1 {
		fmt.Fprintf(buf, "%s 0 0 %s 0 0 cm\n",
			formatFloat(scaleX), formatFloat(scaleY))
	}

	// Step 4: translate by -refX, -refY (anchor adjustment)
	if m.refX != 0 || m.refY != 0 {
		fmt.Fprintf(buf, "1 0 0 1 %s %s cm\n",
			formatFloat(-m.refX), formatFloat(-m.refY))
	}

	// Step 5: render marker's children (each child gets its own q/Q via the renderer)
	renderSVGNodes(buf, p, svg, m.children, defaultSVGStyle())

	buf.WriteString("Q\n")
}

// resolveMarker looks up the marker by id in svg.defs. Returns nil if missing.
func resolveMarker(svg *SVG, id string) *svgMarker {
	if svg == nil || id == "" {
		return nil
	}
	m, _ := svg.defs[id].(*svgMarker)
	return m
}

// emitMarkersForLine adds markers at endpoints of a <line>.
func emitMarkersForLine(buf *bytes.Buffer, p *Page, svg *SVG, l *svgLine) {
	angle := math.Atan2(l.y2-l.y1, l.x2-l.x1)
	sw := l.style.strokeWidth
	if start := resolveMarker(svg, l.style.markerStart); start != nil {
		emitMarker(buf, p, svg, start, l.x1, l.y1, angle, sw)
	}
	if end := resolveMarker(svg, l.style.markerEnd); end != nil {
		emitMarker(buf, p, svg, end, l.x2, l.y2, angle, sw)
	}
}

// emitMarkersForPolyline adds start/mid/end markers along a polyline.
func emitMarkersForPolyline(buf *bytes.Buffer, p *Page, svg *SVG, pts []Point, style svgStyle) {
	if len(pts) < 2 {
		return
	}
	sw := style.strokeWidth
	// start
	if start := resolveMarker(svg, style.markerStart); start != nil {
		a := math.Atan2(pts[1].Y-pts[0].Y, pts[1].X-pts[0].X)
		emitMarker(buf, p, svg, start, pts[0].X, pts[0].Y, a, sw)
	}
	// mid (interior vertices; tangent = bisector of incoming and outgoing direction)
	if mid := resolveMarker(svg, style.markerMid); mid != nil {
		for i := 1; i < len(pts)-1; i++ {
			a := math.Atan2(pts[i+1].Y-pts[i-1].Y, pts[i+1].X-pts[i-1].X)
			emitMarker(buf, p, svg, mid, pts[i].X, pts[i].Y, a, sw)
		}
	}
	// end
	if end := resolveMarker(svg, style.markerEnd); end != nil {
		n := len(pts)
		a := math.Atan2(pts[n-1].Y-pts[n-2].Y, pts[n-1].X-pts[n-2].X)
		emitMarker(buf, p, svg, end, pts[n-1].X, pts[n-1].Y, a, sw)
	}
}

// emitMarkersForPath: simplified — uses M and last endpoint with segment-direction
// tangent at endpoints. Curves (C/Q) use the segment direction (slightly off for true
// tangent but acceptable for Phase 3d).
func emitMarkersForPath(buf *bytes.Buffer, p *Page, svg *SVG, ops []svgPathOp, style svgStyle) {
	if len(ops) < 2 {
		return
	}
	// Collect endpoint positions
	var first, last [2]float64
	var firstSet, lastSet bool
	for _, op := range ops {
		switch op.kind {
		case 'M', 'L':
			if !firstSet {
				first[0], first[1] = op.args[0], op.args[1]
				firstSet = true
			}
			last[0], last[1] = op.args[0], op.args[1]
			lastSet = true
		case 'C':
			last[0], last[1] = op.args[4], op.args[5]
			lastSet = true
		case 'Q':
			last[0], last[1] = op.args[2], op.args[3]
			lastSet = true
		}
	}
	if !firstSet || !lastSet {
		return
	}
	sw := style.strokeWidth
	// start: use the first segment's direction (approximate)
	if start := resolveMarker(svg, style.markerStart); start != nil {
		// Find direction from first M to next op
		var nextX, nextY float64
		for i, op := range ops {
			if i == 0 {
				continue
			}
			switch op.kind {
			case 'L':
				nextX, nextY = op.args[0], op.args[1]
			case 'C':
				nextX, nextY = op.args[0], op.args[1] // first control point as proxy
			case 'Q':
				nextX, nextY = op.args[0], op.args[1]
			}
			break
		}
		a := math.Atan2(nextY-first[1], nextX-first[0])
		emitMarker(buf, p, svg, start, first[0], first[1], a, sw)
	}
	if end := resolveMarker(svg, style.markerEnd); end != nil {
		// Find direction into last endpoint
		// Walk ops in reverse to find prev point
		var prevX, prevY float64
		for i := len(ops) - 1; i >= 0; i-- {
			op := ops[i]
			if op.kind == 'L' || op.kind == 'M' {
				if i > 0 {
					prevOp := ops[i-1]
					switch prevOp.kind {
					case 'M', 'L':
						prevX, prevY = prevOp.args[0], prevOp.args[1]
					case 'C':
						prevX, prevY = prevOp.args[4], prevOp.args[5]
					case 'Q':
						prevX, prevY = prevOp.args[2], prevOp.args[3]
					}
				}
				break
			}
			if op.kind == 'C' {
				prevX, prevY = op.args[2], op.args[3] // 2nd control point as proxy
				break
			}
			if op.kind == 'Q' {
				prevX, prevY = op.args[0], op.args[1]
				break
			}
		}
		a := math.Atan2(last[1]-prevY, last[0]-prevX)
		emitMarker(buf, p, svg, end, last[0], last[1], a, sw)
	}
}
