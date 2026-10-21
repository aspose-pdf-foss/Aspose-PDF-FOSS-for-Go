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
	// Checked here, the single entry point behind AddSVG/AddSVGFromStream/
	// AddSVGObject, not only in appendToContentStream below: node
	// rendering registers images/fonts/gradients/masks into /Resources
	// well before this function ever reaches its own
	// appendToContentStream call (GitHub issue #20 point 2,
	// shadowhunter1470-svg).
	if err := p.checkCanvasNotFrozen(); err != nil {
		return err
	}

	var buf bytes.Buffer
	outer := computeViewBoxMatrix(svg.viewBox, svg.width, svg.height, svg.par, rect)

	buf.WriteString("q\n")
	writeCMOperator(&buf, outer)
	renderSVGNodes(&buf, p, svg, svg.root.children, svg.root.style, outer)
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

// `ctm` is the cumulative SVG-to-page transform in effect at the caller. Each
// renderer composes its own `transform=` attribute into this before passing it
// to gradient resolution — PDF Type 2 (shading) patterns are interpreted in
// the page's initial coordinate system (ISO 32000-1 §8.7.4.5.1), so /Matrix
// has to encode the full path from pattern coords → device, not just the
// gradientTransform piece.
func renderSVGNodes(buf *bytes.Buffer, p *Page, svg *SVG, nodes []svgNode, parentStyle svgStyle, ctm svgMatrix) {
	for _, n := range nodes {
		renderSVGNode(buf, p, svg, n, parentStyle, ctm)
	}
}

func renderSVGNode(buf *bytes.Buffer, p *Page, svg *SVG, n svgNode, parentStyle svgStyle, ctm svgMatrix) {
	switch node := n.(type) {
	case *svgGroup:
		renderSVGGroup(buf, p, svg, node, ctm)
	case *svgRect:
		renderSVGRect(buf, p, svg, node, ctm)
	case *svgCircle:
		renderSVGCircle(buf, p, svg, node, ctm)
	case *svgEllipse:
		renderSVGEllipse(buf, p, svg, node, ctm)
	case *svgLine:
		renderSVGLine(buf, p, svg, node, ctm)
	case *svgPolyline:
		renderSVGPolyline(buf, p, svg, node, ctm)
	case *svgPolygon:
		renderSVGPolygon(buf, p, svg, node, ctm)
	case *svgPath:
		renderSVGPath(buf, p, svg, node, ctm)
	case *svgText:
		renderSVGText(buf, p, svg, node, ctm)
	case *svgImage:
		renderSVGImage(buf, p, svg, node, ctm)
	}
}

// composeCTM returns ctm × transform if transform is non-nil, else ctm.
// Used to build the cumulative SVG-to-page transform inside each render fn
// (the same transform we just emitted via `cm`) so we can pass it to gradient
// resolution.
func composeCTM(ctm svgMatrix, transform *svgMatrix) svgMatrix {
	if transform == nil {
		return ctm
	}
	return matrixMul(ctm, *transform)
}

func renderSVGGroup(buf *bytes.Buffer, p *Page, svg *SVG, g *svgGroup, ctm svgMatrix) {
	if !g.style.display {
		return
	}
	buf.WriteString("q\n")
	if g.transform != nil {
		writeCMOperator(buf, *g.transform)
	}
	applyClipPath(buf, p, svg, g.style)
	applyMask(buf, p, svg, g.style, nil)
	applySVGFilter(buf, p, svg, g.style, nil)
	if err := applyGroupOpacity(buf, p, g.style); err != nil {
		// best-effort: skip opacity on error
		_ = err
	}
	renderSVGNodes(buf, p, svg, g.children, g.style, composeCTM(ctm, g.transform))
	buf.WriteString("Q\n")
}

func renderSVGRect(buf *bytes.Buffer, p *Page, svg *SVG, r *svgRect, ctm svgMatrix) {
	if !r.style.display || r.w <= 0 || r.h <= 0 {
		return
	}
	nodeCTM := composeCTM(ctm, r.transform)
	buf.WriteString("q\n")
	if r.transform != nil {
		writeCMOperator(buf, *r.transform)
	}
	applyClipPath(buf, p, svg, r.style)
	applyMask(buf, p, svg, r.style, r)
	applySVGFilter(buf, p, svg, r.style, r)
	_ = applyGroupOpacity(buf, p, r.style)
	style := svgStyleToShapeStyle(r.style)
	if name := resolveGradientFill(p, svg, r.style.fill, r, nodeCTM); name != "" {
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

func renderSVGCircle(buf *bytes.Buffer, p *Page, svg *SVG, c *svgCircle, ctm svgMatrix) {
	if !c.style.display || c.r <= 0 {
		return
	}
	nodeCTM := composeCTM(ctm, c.transform)
	buf.WriteString("q\n")
	if c.transform != nil {
		writeCMOperator(buf, *c.transform)
	}
	applyClipPath(buf, p, svg, c.style)
	applyMask(buf, p, svg, c.style, c)
	applySVGFilter(buf, p, svg, c.style, c)
	_ = applyGroupOpacity(buf, p, c.style)
	style := svgStyleToShapeStyle(c.style)
	if name := resolveGradientFill(p, svg, c.style.fill, c, nodeCTM); name != "" {
		style.FillPattern = name
	}
	emitCircleToBuf(buf, p, Point{X: c.cx, Y: c.cy}, c.r, style)
	buf.WriteString("Q\n")
}

func renderSVGEllipse(buf *bytes.Buffer, p *Page, svg *SVG, e *svgEllipse, ctm svgMatrix) {
	if !e.style.display || e.rx <= 0 || e.ry <= 0 {
		return
	}
	nodeCTM := composeCTM(ctm, e.transform)
	buf.WriteString("q\n")
	if e.transform != nil {
		writeCMOperator(buf, *e.transform)
	}
	applyClipPath(buf, p, svg, e.style)
	applyMask(buf, p, svg, e.style, e)
	applySVGFilter(buf, p, svg, e.style, e)
	_ = applyGroupOpacity(buf, p, e.style)
	style := svgStyleToShapeStyle(e.style)
	if name := resolveGradientFill(p, svg, e.style.fill, e, nodeCTM); name != "" {
		style.FillPattern = name
	}
	emitEllipseToBuf(buf, p, Point{X: e.cx, Y: e.cy}, e.rx, e.ry, style)
	buf.WriteString("Q\n")
}

func renderSVGLine(buf *bytes.Buffer, p *Page, svg *SVG, l *svgLine, ctm svgMatrix) {
	if !l.style.display {
		return
	}
	_ = composeCTM(ctm, l.transform) // gradient unused on stroke-only line, but kept for symmetry
	buf.WriteString("q\n")
	if l.transform != nil {
		writeCMOperator(buf, *l.transform)
	}
	applyClipPath(buf, p, svg, l.style)
	applyMask(buf, p, svg, l.style, l)
	applySVGFilter(buf, p, svg, l.style, l)
	_ = applyGroupOpacity(buf, p, l.style)
	emitLineToBuf(buf, p, Point{X: l.x1, Y: l.y1}, Point{X: l.x2, Y: l.y2}, svgStyleToLineStyle(l.style))
	buf.WriteString("Q\n")
	emitMarkersForLine(buf, p, svg, l)
}

func renderSVGPolyline(buf *bytes.Buffer, p *Page, svg *SVG, pl *svgPolyline, ctm svgMatrix) {
	if !pl.style.display || len(pl.points) < 2 {
		return
	}
	_ = composeCTM(ctm, pl.transform)
	buf.WriteString("q\n")
	if pl.transform != nil {
		writeCMOperator(buf, *pl.transform)
	}
	applyClipPath(buf, p, svg, pl.style)
	applyMask(buf, p, svg, pl.style, pl)
	applySVGFilter(buf, p, svg, pl.style, pl)
	_ = applyGroupOpacity(buf, p, pl.style)
	emitPolylineToBuf(buf, p, pl.points, svgStyleToLineStyle(pl.style))
	buf.WriteString("Q\n")
	emitMarkersForPolyline(buf, p, svg, pl.points, pl.style)
}

func renderSVGPolygon(buf *bytes.Buffer, p *Page, svg *SVG, pg *svgPolygon, ctm svgMatrix) {
	if !pg.style.display || len(pg.points) < 3 {
		return
	}
	nodeCTM := composeCTM(ctm, pg.transform)
	buf.WriteString("q\n")
	if pg.transform != nil {
		writeCMOperator(buf, *pg.transform)
	}
	applyClipPath(buf, p, svg, pg.style)
	applyMask(buf, p, svg, pg.style, pg)
	applySVGFilter(buf, p, svg, pg.style, pg)
	_ = applyGroupOpacity(buf, p, pg.style)
	style := svgStyleToShapeStyle(pg.style)
	if name := resolveGradientFill(p, svg, pg.style.fill, pg, nodeCTM); name != "" {
		style.FillPattern = name
	}
	emitPolygonToBuf(buf, p, pg.points, style)
	buf.WriteString("Q\n")
	emitMarkersForPolyline(buf, p, svg, pg.points, pg.style)
}

// renderSVGPath renders an SVG <path> element by converting its normalized
// svgPathOps (M/L/C/Q/Z) into a Phase 1 Path and delegating to emitPathToBuf.
// The fill-rule from sp.style.fillRule is forwarded ("evenodd" → f*/B*).
func renderSVGPath(buf *bytes.Buffer, p *Page, svg *SVG, sp *svgPath, ctm svgMatrix) {
	if !sp.style.display || len(sp.commands) == 0 {
		return
	}
	nodeCTM := composeCTM(ctm, sp.transform)
	buf.WriteString("q\n")
	if sp.transform != nil {
		writeCMOperator(buf, *sp.transform)
	}
	applyClipPath(buf, p, svg, sp.style)
	applyMask(buf, p, svg, sp.style, sp)
	applySVGFilter(buf, p, svg, sp.style, sp)
	_ = applyGroupOpacity(buf, p, sp.style)
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
	if name := resolveGradientFill(p, svg, sp.style.fill, sp, nodeCTM); name != "" {
		style.FillPattern = name
	}
	emitPathToBuf(buf, p, path, style, sp.style.fillRule)
	buf.WriteString("Q\n")
	emitMarkersForPath(buf, p, svg, sp.commands, sp.style)
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
