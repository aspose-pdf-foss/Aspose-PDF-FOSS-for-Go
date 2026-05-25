// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"fmt"
)

// emitLineToBuf writes the PDF content-stream bytes for a stroked line segment
// from→to into buf, without q/Q wrapping and without calling appendToContentStream.
// The caller is responsible for the surrounding q/Q and alpha gs op (if any).
// No-op if style.Width <= 0.
func emitLineToBuf(buf *bytes.Buffer, p *Page, from, to Point, style LineStyle) {
	if style.Width <= 0 {
		return
	}
	buf.WriteString(formatLineStyle(style))
	fmt.Fprintf(buf, "%s %s m\n", formatFloat(from.X), formatFloat(from.Y))
	fmt.Fprintf(buf, "%s %s l\n", formatFloat(to.X), formatFloat(to.Y))
	buf.WriteString("S\n")
}

// emitRectangleToBuf writes the PDF content-stream bytes for an axis-aligned
// rectangle into buf, without q/Q wrapping and without calling appendToContentStream.
// No-op if paintOp returns "".
func emitRectangleToBuf(buf *bytes.Buffer, p *Page, rect Rectangle, style ShapeStyle) {
	op := paintOp(style)
	if op == "" {
		return
	}
	w := rect.URX - rect.LLX
	h := rect.URY - rect.LLY
	buf.WriteString(formatShapeStyle(style))
	fmt.Fprintf(buf, "%s %s %s %s re %s\n",
		formatFloat(rect.LLX), formatFloat(rect.LLY),
		formatFloat(w), formatFloat(h), op)
}

// emitRoundedRectangleToBuf writes the PDF content-stream bytes for a rounded
// rectangle into buf, without q/Q wrapping and without calling appendToContentStream.
// The radius is NOT clamped here — callers should pre-clamp if needed.
func emitRoundedRectangleToBuf(buf *bytes.Buffer, p *Page, rect Rectangle, radius float64, style ShapeStyle) {
	op := paintOp(style)
	if op == "" {
		return
	}
	w := rect.URX - rect.LLX
	h := rect.URY - rect.LLY
	if w <= 0 || h <= 0 {
		return
	}
	r := radius
	if maxR := w / 2; r > maxR {
		r = maxR
	}
	if maxR := h / 2; r > maxR {
		r = maxR
	}

	const halfPi = 3.141592653589793 / 2

	path := NewPath().
		MoveTo(rect.LLX+r, rect.LLY).
		LineTo(rect.URX-r, rect.LLY).
		Arc(rect.URX-r, rect.LLY+r, r, -halfPi, halfPi).
		LineTo(rect.URX, rect.URY-r).
		Arc(rect.URX-r, rect.URY-r, r, 0, halfPi).
		LineTo(rect.LLX+r, rect.URY).
		Arc(rect.LLX+r, rect.URY-r, r, halfPi, halfPi).
		LineTo(rect.LLX, rect.LLY+r).
		Arc(rect.LLX+r, rect.LLY+r, r, 3.141592653589793, halfPi).
		Close()

	buf.WriteString(formatShapeStyle(style))
	buf.WriteString(pathOpsToOperators(path.ops))
	buf.WriteString(op + "\n")
}

// emitCircleToBuf writes the PDF content-stream bytes for a circle into buf,
// without q/Q wrapping and without calling appendToContentStream.
func emitCircleToBuf(buf *bytes.Buffer, p *Page, center Point, radius float64, style ShapeStyle) {
	emitEllipseToBuf(buf, p, center, radius, radius, style)
}

// emitEllipseToBuf writes the PDF content-stream bytes for an axis-aligned
// ellipse into buf, without q/Q wrapping and without calling appendToContentStream.
func emitEllipseToBuf(buf *bytes.Buffer, p *Page, center Point, rx, ry float64, style ShapeStyle) {
	op := paintOp(style)
	if op == "" || rx == 0 || ry == 0 {
		return
	}
	buf.WriteString(formatShapeStyle(style))
	buf.WriteString(ellipsePathOps(center.X, center.Y, rx, ry))
	buf.WriteString(op + "\n")
}

// emitPolylineToBuf writes the PDF content-stream bytes for an open polyline
// into buf, without q/Q wrapping and without calling appendToContentStream.
// No-op if style.Width <= 0 or len(points) < 2.
func emitPolylineToBuf(buf *bytes.Buffer, p *Page, points []Point, style LineStyle) {
	if style.Width <= 0 || len(points) < 2 {
		return
	}
	buf.WriteString(formatLineStyle(style))
	fmt.Fprintf(buf, "%s %s m\n", formatFloat(points[0].X), formatFloat(points[0].Y))
	for _, pt := range points[1:] {
		fmt.Fprintf(buf, "%s %s l\n", formatFloat(pt.X), formatFloat(pt.Y))
	}
	buf.WriteString("S\n")
}

// emitPolygonToBuf writes the PDF content-stream bytes for a closed polygon
// into buf, without q/Q wrapping and without calling appendToContentStream.
// No-op if paintOp returns "" or len(points) < 3.
func emitPolygonToBuf(buf *bytes.Buffer, p *Page, points []Point, style ShapeStyle) {
	op := paintOp(style)
	if op == "" || len(points) < 3 {
		return
	}
	buf.WriteString(formatShapeStyle(style))
	fmt.Fprintf(buf, "%s %s m\n", formatFloat(points[0].X), formatFloat(points[0].Y))
	for _, pt := range points[1:] {
		fmt.Fprintf(buf, "%s %s l\n", formatFloat(pt.X), formatFloat(pt.Y))
	}
	buf.WriteString(" h\n")
	buf.WriteString(op + "\n")
}

// paintOpWithFillRule is like paintOp but respects the SVG/PDF fill-rule:
//
//	"evenodd" → use f* (fill even-odd) and B* (fill+stroke even-odd)
//	anything else → use f (nonzero winding, the PDF/SVG default) and B
//
// Stroke-only is "S" in both cases (fill rule irrelevant).
func paintOpWithFillRule(s ShapeStyle, fillRule string) string {
	stroke := s.LineStyle.Width > 0
	fill := s.FillColor != nil
	evenOdd := fillRule == "evenodd"
	switch {
	case stroke && fill:
		if evenOdd {
			return "B*"
		}
		return "B"
	case stroke:
		return "S"
	case fill:
		if evenOdd {
			return "f*"
		}
		return "f"
	default:
		return ""
	}
}

// emitPathToBuf writes the operators for a Path into buf, parameterized by fill rule.
// fillRule is "evenodd" or anything-else (defaults to nonzero per SVG/PDF spec).
// Does NOT wrap in q/Q — caller controls that.
// No-op if path is nil, has no ops, or style has neither stroke nor fill.
func emitPathToBuf(buf *bytes.Buffer, p *Page, path *Path, style ShapeStyle, fillRule string) {
	if path == nil || len(path.ops) == 0 {
		return
	}
	op := paintOpWithFillRule(style, fillRule)
	if op == "" {
		return
	}
	buf.WriteString(formatShapeStyle(style))
	buf.WriteString(pathOpsToOperators(path.ops))
	buf.WriteString(op + "\n")
}
