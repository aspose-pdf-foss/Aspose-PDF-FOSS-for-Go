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
	renderSVGNodes(&buf, p, svg, svg.root.children, svg.root.style)
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

func renderSVGNodes(buf *bytes.Buffer, p *Page, svg *SVG, nodes []svgNode, parentStyle svgStyle) {
	for _, n := range nodes {
		renderSVGNode(buf, p, svg, n, parentStyle)
	}
}

func renderSVGNode(buf *bytes.Buffer, p *Page, svg *SVG, n svgNode, parentStyle svgStyle) {
	switch node := n.(type) {
	case *svgGroup:
		renderSVGGroup(buf, p, svg, node)
	case *svgRect:
		renderSVGRect(buf, p, svg, node)
	case *svgCircle:
		renderSVGCircle(buf, p, svg, node)
	case *svgEllipse:
		renderSVGEllipse(buf, p, svg, node)
	case *svgLine:
		renderSVGLine(buf, p, svg, node)
	case *svgPolyline:
		renderSVGPolyline(buf, p, svg, node)
	case *svgPolygon:
		renderSVGPolygon(buf, p, svg, node)
	case *svgPath:
		renderSVGPath(buf, p, svg, node)
	case *svgText:
		renderSVGText(buf, p, svg, node)
	case *svgImage:
		renderSVGImage(buf, p, svg, node)
	}
}

func renderSVGGroup(buf *bytes.Buffer, p *Page, svg *SVG, g *svgGroup) {
	if !g.style.display {
		return
	}
	buf.WriteString("q\n")
	if g.transform != nil {
		writeCMOperator(buf, *g.transform)
	}
	applyClipPath(buf, p, svg, g.style)
	applyMask(buf, p, svg, g.style, nil)
	if err := applyGroupOpacity(buf, p, g.style); err != nil {
		// best-effort: skip opacity on error
		_ = err
	}
	renderSVGNodes(buf, p, svg, g.children, g.style)
	buf.WriteString("Q\n")
}

func renderSVGRect(buf *bytes.Buffer, p *Page, svg *SVG, r *svgRect) {
	if !r.style.display || r.w <= 0 || r.h <= 0 {
		return
	}
	buf.WriteString("q\n")
	if r.transform != nil {
		writeCMOperator(buf, *r.transform)
	}
	applyClipPath(buf, p, svg, r.style)
	applyMask(buf, p, svg, r.style, r)
	style := svgStyleToShapeStyle(r.style)
	if name := resolveGradientFill(p, svg, r.style.fill, r); name != "" {
		style.FillPattern = name
	}
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

func renderSVGCircle(buf *bytes.Buffer, p *Page, svg *SVG, c *svgCircle) {
	if !c.style.display || c.r <= 0 {
		return
	}
	buf.WriteString("q\n")
	if c.transform != nil {
		writeCMOperator(buf, *c.transform)
	}
	applyClipPath(buf, p, svg, c.style)
	applyMask(buf, p, svg, c.style, c)
	style := svgStyleToShapeStyle(c.style)
	if name := resolveGradientFill(p, svg, c.style.fill, c); name != "" {
		style.FillPattern = name
	}
	emitCircleToBuf(buf, p, Point{X: c.cx, Y: c.cy}, c.r, style)
	buf.WriteString("Q\n")
}

func renderSVGEllipse(buf *bytes.Buffer, p *Page, svg *SVG, e *svgEllipse) {
	if !e.style.display || e.rx <= 0 || e.ry <= 0 {
		return
	}
	buf.WriteString("q\n")
	if e.transform != nil {
		writeCMOperator(buf, *e.transform)
	}
	applyClipPath(buf, p, svg, e.style)
	applyMask(buf, p, svg, e.style, e)
	style := svgStyleToShapeStyle(e.style)
	if name := resolveGradientFill(p, svg, e.style.fill, e); name != "" {
		style.FillPattern = name
	}
	emitEllipseToBuf(buf, p, Point{X: e.cx, Y: e.cy}, e.rx, e.ry, style)
	buf.WriteString("Q\n")
}

func renderSVGLine(buf *bytes.Buffer, p *Page, svg *SVG, l *svgLine) {
	if !l.style.display {
		return
	}
	buf.WriteString("q\n")
	if l.transform != nil {
		writeCMOperator(buf, *l.transform)
	}
	applyClipPath(buf, p, svg, l.style)
	applyMask(buf, p, svg, l.style, l)
	emitLineToBuf(buf, p, Point{X: l.x1, Y: l.y1}, Point{X: l.x2, Y: l.y2}, svgStyleToLineStyle(l.style))
	buf.WriteString("Q\n")
}

func renderSVGPolyline(buf *bytes.Buffer, p *Page, svg *SVG, pl *svgPolyline) {
	if !pl.style.display || len(pl.points) < 2 {
		return
	}
	buf.WriteString("q\n")
	if pl.transform != nil {
		writeCMOperator(buf, *pl.transform)
	}
	applyClipPath(buf, p, svg, pl.style)
	applyMask(buf, p, svg, pl.style, pl)
	emitPolylineToBuf(buf, p, pl.points, svgStyleToLineStyle(pl.style))
	buf.WriteString("Q\n")
}

func renderSVGPolygon(buf *bytes.Buffer, p *Page, svg *SVG, pg *svgPolygon) {
	if !pg.style.display || len(pg.points) < 3 {
		return
	}
	buf.WriteString("q\n")
	if pg.transform != nil {
		writeCMOperator(buf, *pg.transform)
	}
	applyClipPath(buf, p, svg, pg.style)
	applyMask(buf, p, svg, pg.style, pg)
	style := svgStyleToShapeStyle(pg.style)
	if name := resolveGradientFill(p, svg, pg.style.fill, pg); name != "" {
		style.FillPattern = name
	}
	emitPolygonToBuf(buf, p, pg.points, style)
	buf.WriteString("Q\n")
}

// renderSVGPath renders an SVG <path> element by converting its normalized
// svgPathOps (M/L/C/Q/Z) into a Phase 1 Path and delegating to emitPathToBuf.
// The fill-rule from sp.style.fillRule is forwarded ("evenodd" → f*/B*).
func renderSVGPath(buf *bytes.Buffer, p *Page, svg *SVG, sp *svgPath) {
	if !sp.style.display || len(sp.commands) == 0 {
		return
	}
	buf.WriteString("q\n")
	if sp.transform != nil {
		writeCMOperator(buf, *sp.transform)
	}
	applyClipPath(buf, p, svg, sp.style)
	applyMask(buf, p, svg, sp.style, sp)
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
	style := svgStyleToShapeStyle(sp.style)
	if name := resolveGradientFill(p, svg, sp.style.fill, sp); name != "" {
		style.FillPattern = name
	}
	emitPathToBuf(buf, p, path, style, sp.style.fillRule)
	buf.WriteString("Q\n")
}

// applyClipPath, when the style has a non-empty clipPath ref, looks up the
// clipPath in svg.defs and emits its path construction + W + n into buf.
// The caller has already emitted q\n; the clip is active until the matching Q\n.
func applyClipPath(buf *bytes.Buffer, p *Page, svg *SVG, style svgStyle) {
	if style.clipPath == "" || svg == nil {
		return
	}
	cp, ok := svg.defs[style.clipPath].(*svgClipPath)
	if !ok || cp == nil {
		return // best-effort: missing or wrong type — render unclipped
	}
	emitClipPathInline(buf, p, cp)
}

// applyGroupOpacity emits a `/GSx gs` operator if the group has opacity < 1.
// Returns any error from ensureExtGState.
//
// Note: ensureExtGState returns the name WITH a leading slash (e.g. "/GS0"),
// so we emit it as-is — prepending another "/" would produce a malformed
// "//GS0" token that Acrobat rejects.
func applyGroupOpacity(buf *bytes.Buffer, p *Page, s svgStyle) error {
	if s.opacity < 1 {
		gsName, err := p.ensureExtGState(s.opacity)
		if err != nil {
			return err
		}
		fmt.Fprintf(buf, "%s gs\n", gsName)
	}
	return nil
}

// svgStyleToShapeStyle maps the resolved SVG cascade into Phase 1's ShapeStyle.
func svgStyleToShapeStyle(s svgStyle) ShapeStyle {
	ss := ShapeStyle{LineStyle: svgStyleToLineStyle(s)}
	if s.fill != nil && s.fill.color != nil {
		c := *s.fill.color
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
	if s.stroke != nil && s.stroke.color != nil {
		c := *s.stroke.color
		c.A *= s.strokeOpacity
		ls.Color = &c
	} else {
		ls.Width = 0 // signal "no stroke"
	}
	return ls
}
