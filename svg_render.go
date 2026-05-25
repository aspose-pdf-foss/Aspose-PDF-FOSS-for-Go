// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"fmt"
)

// renderSVG renders a parsed *SVG into the given Page within rect.
// Best-effort: unsupported nodes are silently skipped (already filtered at parse time).
func renderSVG(p *Page, svg *SVG, rect Rectangle) error {
	if svg == nil || svg.root == nil {
		return nil
	}
	if rect.URX-rect.LLX <= 0 || rect.URY-rect.LLY <= 0 {
		return nil
	}

	var buf bytes.Buffer
	outer := computeViewBoxMatrix(svg.viewBox, svg.width, svg.height, svg.par, rect)

	buf.WriteString("q\n")
	writeCMOperator(&buf, outer)
	renderSVGNodes(&buf, p, svg.root.children, svg.root.style)
	buf.WriteString("Q\n")

	return p.appendToContentStream(buf.Bytes())
}

// writeCMOperator writes a PDF `cm` operator (concat matrix to current CTM) from svgMatrix.
func writeCMOperator(buf *bytes.Buffer, m svgMatrix) {
	fmt.Fprintf(buf, "%s %s %s %s %s %s cm\n",
		formatFloat(m[0]), formatFloat(m[1]),
		formatFloat(m[2]), formatFloat(m[3]),
		formatFloat(m[4]), formatFloat(m[5]))
}

func renderSVGNodes(buf *bytes.Buffer, p *Page, nodes []svgNode, parentStyle svgStyle) {
	for _, n := range nodes {
		renderSVGNode(buf, p, n, parentStyle)
	}
}

func renderSVGNode(buf *bytes.Buffer, p *Page, n svgNode, parentStyle svgStyle) {
	switch node := n.(type) {
	case *svgGroup:
		renderSVGGroup(buf, p, node)
	case *svgRect:
		renderSVGRect(buf, p, node)
	case *svgCircle:
		renderSVGCircle(buf, p, node)
	case *svgEllipse:
		renderSVGEllipse(buf, p, node)
	case *svgLine:
		renderSVGLine(buf, p, node)
	case *svgPolyline:
		renderSVGPolyline(buf, p, node)
	case *svgPolygon:
		renderSVGPolygon(buf, p, node)
	case *svgPath:
		renderSVGPath(buf, p, node)
	}
}

func renderSVGGroup(buf *bytes.Buffer, p *Page, g *svgGroup) {
	if !g.style.display {
		return
	}
	buf.WriteString("q\n")
	if g.transform != nil {
		writeCMOperator(buf, *g.transform)
	}
	if err := applyGroupOpacity(buf, p, g.style); err != nil {
		// best-effort: skip opacity on error
		_ = err
	}
	renderSVGNodes(buf, p, g.children, g.style)
	buf.WriteString("Q\n")
}

func renderSVGRect(buf *bytes.Buffer, p *Page, r *svgRect) {
	if !r.style.display || r.w <= 0 || r.h <= 0 {
		return
	}
	buf.WriteString("q\n")
	if r.transform != nil {
		writeCMOperator(buf, *r.transform)
	}
	style := svgStyleToShapeStyle(r.style)
	rect := Rectangle{LLX: r.x, LLY: r.y, URX: r.x + r.w, URY: r.y + r.h}
	if r.rx > 0 || r.ry > 0 {
		rr := r.rx
		if rr == 0 {
			rr = r.ry
		}
		emitRoundedRectangleToBuf(buf, p, rect, rr, style)
	} else {
		emitRectangleToBuf(buf, p, rect, style)
	}
	buf.WriteString("Q\n")
}

func renderSVGCircle(buf *bytes.Buffer, p *Page, c *svgCircle) {
	if !c.style.display || c.r <= 0 {
		return
	}
	buf.WriteString("q\n")
	if c.transform != nil {
		writeCMOperator(buf, *c.transform)
	}
	emitCircleToBuf(buf, p, Point{X: c.cx, Y: c.cy}, c.r, svgStyleToShapeStyle(c.style))
	buf.WriteString("Q\n")
}

func renderSVGEllipse(buf *bytes.Buffer, p *Page, e *svgEllipse) {
	if !e.style.display || e.rx <= 0 || e.ry <= 0 {
		return
	}
	buf.WriteString("q\n")
	if e.transform != nil {
		writeCMOperator(buf, *e.transform)
	}
	emitEllipseToBuf(buf, p, Point{X: e.cx, Y: e.cy}, e.rx, e.ry, svgStyleToShapeStyle(e.style))
	buf.WriteString("Q\n")
}

func renderSVGLine(buf *bytes.Buffer, p *Page, l *svgLine) {
	if !l.style.display {
		return
	}
	buf.WriteString("q\n")
	if l.transform != nil {
		writeCMOperator(buf, *l.transform)
	}
	emitLineToBuf(buf, p, Point{X: l.x1, Y: l.y1}, Point{X: l.x2, Y: l.y2}, svgStyleToLineStyle(l.style))
	buf.WriteString("Q\n")
}

func renderSVGPolyline(buf *bytes.Buffer, p *Page, pl *svgPolyline) {
	if !pl.style.display || len(pl.points) < 2 {
		return
	}
	buf.WriteString("q\n")
	if pl.transform != nil {
		writeCMOperator(buf, *pl.transform)
	}
	emitPolylineToBuf(buf, p, pl.points, svgStyleToLineStyle(pl.style))
	buf.WriteString("Q\n")
}

func renderSVGPolygon(buf *bytes.Buffer, p *Page, pg *svgPolygon) {
	if !pg.style.display || len(pg.points) < 3 {
		return
	}
	buf.WriteString("q\n")
	if pg.transform != nil {
		writeCMOperator(buf, *pg.transform)
	}
	emitPolygonToBuf(buf, p, pg.points, svgStyleToShapeStyle(pg.style))
	buf.WriteString("Q\n")
}

// renderSVGPath renders an SVG <path> element by converting its normalized
// svgPathOps (M/L/C/Q/Z) into a Phase 1 Path and delegating to emitPathToBuf.
// The fill-rule from sp.style.fillRule is forwarded ("evenodd" → f*/B*).
func renderSVGPath(buf *bytes.Buffer, p *Page, sp *svgPath) {
	if !sp.style.display || len(sp.commands) == 0 {
		return
	}
	buf.WriteString("q\n")
	if sp.transform != nil {
		writeCMOperator(buf, *sp.transform)
	}
	// Build a Phase 1 Path from svgPathOps for reuse of emitPathToBuf.
	path := NewPath()
	for _, op := range sp.commands {
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
	emitPathToBuf(buf, p, path, svgStyleToShapeStyle(sp.style), sp.style.fillRule)
	buf.WriteString("Q\n")
}

// applyGroupOpacity emits a `/GSx gs` operator if the group has opacity < 1.
// Returns any error from ensureExtGState.
func applyGroupOpacity(buf *bytes.Buffer, p *Page, s svgStyle) error {
	if s.opacity < 1 {
		gsName, err := p.ensureExtGState(s.opacity)
		if err != nil {
			return err
		}
		fmt.Fprintf(buf, "/%s gs\n", gsName)
	}
	return nil
}

// svgStyleToShapeStyle maps the resolved SVG cascade into Phase 1's ShapeStyle.
func svgStyleToShapeStyle(s svgStyle) ShapeStyle {
	ss := ShapeStyle{LineStyle: svgStyleToLineStyle(s)}
	if s.fill != nil {
		c := *s.fill
		c.A *= s.fillOpacity
		ss.FillColor = &c
	}
	return ss
}

// svgStyleToLineStyle maps stroke-related svgStyle fields into Phase 1's LineStyle.
// Returns Width=0 (no stroke) when stroke color is nil.
func svgStyleToLineStyle(s svgStyle) LineStyle {
	ls := LineStyle{
		Width:       s.strokeWidth,
		DashPattern: s.dashArray,
		DashPhase:   s.dashOffset,
		Cap:         s.lineCap,
		Join:        s.lineJoin,
		MiterLimit:  s.miterLimit,
	}
	if s.stroke != nil {
		c := *s.stroke
		c.A *= s.strokeOpacity
		ls.Color = &c
	} else {
		ls.Width = 0 // signal "no stroke"
	}
	return ls
}
