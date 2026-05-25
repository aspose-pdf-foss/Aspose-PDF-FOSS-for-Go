// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"fmt"
	"strings"
)

// formatLineStyle emits the PDF graphics state operators for stroking with
// the given style: w (width), J (cap), j (join), M (miter limit), d (dash),
// RG (stroke color). Always emits all six for predictable behavior — defaults
// from the surrounding gstate would otherwise leak through `q`.
//
// Returns "" if style.Width <= 0 (caller should not emit a stroke).
func formatLineStyle(s LineStyle) string {
	if s.Width <= 0 {
		return ""
	}
	var buf strings.Builder
	buf.WriteString(fmt.Sprintf("%s w\n", formatFloat(s.Width)))
	buf.WriteString(fmt.Sprintf("%d J\n", int(s.Cap)))
	buf.WriteString(fmt.Sprintf("%d j\n", int(s.Join)))
	if s.MiterLimit > 0 {
		buf.WriteString(fmt.Sprintf("%s M\n", formatFloat(s.MiterLimit)))
	} else {
		buf.WriteString("10 M\n") // PDF default
	}
	if len(s.DashPattern) > 0 {
		parts := make([]string, len(s.DashPattern))
		for i, d := range s.DashPattern {
			parts[i] = formatFloat(d)
		}
		buf.WriteString(fmt.Sprintf("[%s] %s d\n",
			strings.Join(parts, " "), formatFloat(s.DashPhase)))
	} else {
		buf.WriteString("[] 0 d\n")
	}
	c := Color{R: 0, G: 0, B: 0, A: 1}
	if s.Color != nil {
		c = *s.Color
	}
	buf.WriteString(fmt.Sprintf("%s %s %s RG\n",
		formatFloat(c.R), formatFloat(c.G), formatFloat(c.B)))
	return buf.String()
}

// applyAlpha registers an ExtGState if either the stroke color or fill color
// has alpha < 1, and returns the gs op string (with trailing newline) to be
// inserted into the q-block. Returns "" if no transparency is needed.
//
// Uses the most-restrictive alpha among configured colors. If stroke and fill
// have different alpha values, the smaller one is applied to both (single
// ExtGState resource — sufficient for MVP).
func (p *Page) applyAlpha(strokeColor, fillColor *Color) (string, error) {
	a := 1.0
	if strokeColor != nil && strokeColor.A < a {
		a = strokeColor.A
	}
	if fillColor != nil && fillColor.A < a {
		a = fillColor.A
	}
	if a >= 1 {
		return "", nil
	}
	name, err := p.ensureExtGState(a)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s gs\n", name), nil
}

// DrawLine strokes a single line segment from→to with the given style.
// No-op if style.Width <= 0.
//
// Mirrors Aspose.PDF for .NET's Drawing.Line shape.
func (p *Page) DrawLine(from, to Point, style LineStyle) error {
	if style.Width <= 0 {
		return nil
	}
	var buf bytes.Buffer
	buf.WriteString("q\n")
	gsOp, err := p.applyAlpha(style.Color, nil)
	if err != nil {
		return err
	}
	buf.WriteString(gsOp)
	emitLineToBuf(&buf, p, from, to, style)
	buf.WriteString("Q\n")
	return p.appendToContentStream(buf.Bytes())
}

// paintOp returns the PDF painting operator for the given style:
//
//	"S"  — stroke only
//	"f"  — fill only
//	"B"  — stroke + fill
//	""   — neither (caller should skip emission entirely)
func paintOp(s ShapeStyle) string {
	stroke := s.LineStyle.Width > 0
	fill := s.FillColor != nil
	switch {
	case stroke && fill:
		return "B"
	case stroke:
		return "S"
	case fill:
		return "f"
	default:
		return ""
	}
}

// formatFillColor emits a fill-color (rg) op, or "" if color is nil.
func formatFillColor(c *Color) string {
	if c == nil {
		return ""
	}
	return fmt.Sprintf("%s %s %s rg\n",
		formatFloat(c.R), formatFloat(c.G), formatFloat(c.B))
}

// formatShapeStyle emits stroke + fill graphics state ops.
// Returns "" if neither stroke nor fill is configured.
func formatShapeStyle(s ShapeStyle) string {
	op := paintOp(s)
	if op == "" {
		return ""
	}
	var buf strings.Builder
	if s.LineStyle.Width > 0 {
		buf.WriteString(formatLineStyle(s.LineStyle))
	}
	buf.WriteString(formatFillColor(s.FillColor))
	return buf.String()
}

// DrawRectangle strokes and/or fills an axis-aligned rectangle.
// No-op if neither stroke (Width > 0) nor fill (FillColor != nil) is set.
//
// Mirrors Aspose.PDF for .NET's Drawing.Rectangle shape.
func (p *Page) DrawRectangle(rect Rectangle, style ShapeStyle) error {
	op := paintOp(style)
	if op == "" {
		return nil
	}
	var buf bytes.Buffer
	buf.WriteString("q\n")
	gsOp, err := p.applyAlpha(style.LineStyle.Color, style.FillColor)
	if err != nil {
		return err
	}
	buf.WriteString(gsOp)
	emitRectangleToBuf(&buf, p, rect, style)
	buf.WriteString("Q\n")
	return p.appendToContentStream(buf.Bytes())
}

// ellipseApproxKappa is the magic constant for cubic Bezier approximation of
// a quarter-circle: k = (4/3)*tan(π/8) = 4*(√2 - 1)/3 ≈ 0.5522847498.
const ellipseApproxKappa = 0.5522847498307933

// ellipsePathOps emits the path-construction operators for an axis-aligned
// ellipse centered at (cx, cy) with horizontal radius rx and vertical radius
// ry. Composed of four cubic Beziers + close (h). The leading space before
// "h" ensures the substring " h\n" is present in the output (matches test
// expectations and aligns with the spacing convention used by other path ops).
func ellipsePathOps(cx, cy, rx, ry float64) string {
	kx := rx * ellipseApproxKappa
	ky := ry * ellipseApproxKappa
	var buf strings.Builder
	// Start at right-most point.
	buf.WriteString(fmt.Sprintf("%s %s m\n",
		formatFloat(cx+rx), formatFloat(cy)))
	// Upper-right quadrant.
	buf.WriteString(fmt.Sprintf("%s %s %s %s %s %s c\n",
		formatFloat(cx+rx), formatFloat(cy+ky),
		formatFloat(cx+kx), formatFloat(cy+ry),
		formatFloat(cx), formatFloat(cy+ry)))
	// Upper-left.
	buf.WriteString(fmt.Sprintf("%s %s %s %s %s %s c\n",
		formatFloat(cx-kx), formatFloat(cy+ry),
		formatFloat(cx-rx), formatFloat(cy+ky),
		formatFloat(cx-rx), formatFloat(cy)))
	// Lower-left.
	buf.WriteString(fmt.Sprintf("%s %s %s %s %s %s c\n",
		formatFloat(cx-rx), formatFloat(cy-ky),
		formatFloat(cx-kx), formatFloat(cy-ry),
		formatFloat(cx), formatFloat(cy-ry)))
	// Lower-right.
	buf.WriteString(fmt.Sprintf("%s %s %s %s %s %s c\n",
		formatFloat(cx+kx), formatFloat(cy-ry),
		formatFloat(cx+rx), formatFloat(cy-ky),
		formatFloat(cx+rx), formatFloat(cy)))
	buf.WriteString(" h\n")
	return buf.String()
}

// DrawCircle strokes and/or fills a circle. Returns error for negative radius.
// No-op if radius is zero or neither stroke nor fill is configured.
//
// Mirrors Aspose.PDF for .NET's Drawing.Circle.
func (p *Page) DrawCircle(center Point, radius float64, style ShapeStyle) error {
	if radius < 0 {
		return fmt.Errorf("draw circle: negative radius %g", radius)
	}
	return p.DrawEllipse(center, radius, radius, style)
}

// DrawEllipse strokes and/or fills an axis-aligned ellipse.
// Returns error for negative semi-axis. No-op if either semi-axis is zero or
// neither stroke nor fill is configured.
//
// Mirrors Aspose.PDF for .NET's Drawing.Ellipse.
func (p *Page) DrawEllipse(center Point, rx, ry float64, style ShapeStyle) error {
	if rx < 0 || ry < 0 {
		return fmt.Errorf("draw ellipse: negative semi-axis (rx=%g, ry=%g)", rx, ry)
	}
	op := paintOp(style)
	if op == "" || rx == 0 || ry == 0 {
		return nil
	}
	var buf bytes.Buffer
	buf.WriteString("q\n")
	gsOp, err := p.applyAlpha(style.LineStyle.Color, style.FillColor)
	if err != nil {
		return err
	}
	buf.WriteString(gsOp)
	emitEllipseToBuf(&buf, p, center, rx, ry, style)
	buf.WriteString("Q\n")
	return p.appendToContentStream(buf.Bytes())
}

// DrawPolyline strokes an open polyline (first and last points are NOT
// connected). No fill — even if one were specified, an open path has
// ambiguous fill semantics. Errors if len(points) < 2.
// No-op if style.Width <= 0.
//
// Mirrors Aspose.PDF for .NET's Drawing.Polyline.
func (p *Page) DrawPolyline(points []Point, style LineStyle) error {
	if len(points) < 2 {
		return fmt.Errorf("draw polyline: need >= 2 points, got %d", len(points))
	}
	if style.Width <= 0 {
		return nil
	}
	var buf bytes.Buffer
	buf.WriteString("q\n")
	gsOp, err := p.applyAlpha(style.Color, nil)
	if err != nil {
		return err
	}
	buf.WriteString(gsOp)
	emitPolylineToBuf(&buf, p, points, style)
	buf.WriteString("Q\n")
	return p.appendToContentStream(buf.Bytes())
}

// pathOpsToOperators converts a Path's internal ops into a PDF content stream
// fragment of path-construction operators (m, l, c, h). Does NOT emit a
// paint operator — the caller appends "S", "f", or "B" as appropriate.
func pathOpsToOperators(ops []pathOp) string {
	var buf strings.Builder
	for _, op := range ops {
		switch op.kind {
		case pathOpMoveTo:
			buf.WriteString(fmt.Sprintf("%s %s m\n", formatFloat(op.x), formatFloat(op.y)))
		case pathOpLineTo:
			buf.WriteString(fmt.Sprintf("%s %s l\n", formatFloat(op.x), formatFloat(op.y)))
		case pathOpCurveTo:
			buf.WriteString(fmt.Sprintf("%s %s %s %s %s %s c\n",
				formatFloat(op.c1x), formatFloat(op.c1y),
				formatFloat(op.c2x), formatFloat(op.c2y),
				formatFloat(op.x), formatFloat(op.y)))
		case pathOpClose:
			buf.WriteString(" h\n") // leading space for consistency with other shape methods
		}
	}
	return buf.String()
}

// DrawPath strokes and/or fills the previously-built path. Errors if path is
// nil. No-op if path has no operations or style is empty.
//
// Mirrors Aspose.PDF for .NET's Drawing.GraphicsPath / Drawing.Curve.
func (p *Page) DrawPath(path *Path, style ShapeStyle) error {
	if path == nil {
		return fmt.Errorf("draw path: nil path")
	}
	if len(path.ops) == 0 {
		return nil
	}
	op := paintOp(style)
	if op == "" {
		return nil
	}
	var buf bytes.Buffer
	buf.WriteString("q\n")
	gsOp, err := p.applyAlpha(style.LineStyle.Color, style.FillColor)
	if err != nil {
		return err
	}
	buf.WriteString(gsOp)
	emitPathToBuf(&buf, p, path, style, "") // "" → nonzero winding (Phase 1 default)
	buf.WriteString("Q\n")
	return p.appendToContentStream(buf.Bytes())
}

// DrawRoundedRectangle strokes and/or fills an axis-aligned rectangle with
// rounded corners of the given radius. The radius is clamped to half the
// shorter side. Returns error for negative radius.
//
// Implemented as a Path: 4 straight edges + 4 quarter-arc corners.
//
// Mirrors Aspose.PDF for .NET's Drawing.RoundedRectangle.
func (p *Page) DrawRoundedRectangle(rect Rectangle, radius float64, style ShapeStyle) error {
	if radius < 0 {
		return fmt.Errorf("draw rounded rectangle: negative radius %g", radius)
	}
	op := paintOp(style)
	if op == "" {
		return nil
	}
	w := rect.URX - rect.LLX
	h := rect.URY - rect.LLY
	if w <= 0 || h <= 0 {
		return nil
	}
	var buf bytes.Buffer
	buf.WriteString("q\n")
	gsOp, err := p.applyAlpha(style.LineStyle.Color, style.FillColor)
	if err != nil {
		return err
	}
	buf.WriteString(gsOp)
	emitRoundedRectangleToBuf(&buf, p, rect, radius, style)
	buf.WriteString("Q\n")
	return p.appendToContentStream(buf.Bytes())
}

// DrawPolygon strokes and/or fills a closed polygon (last point connects back
// to the first via `h`). Errors if len(points) < 3. No-op if neither stroke
// nor fill is configured.
//
// Mirrors Aspose.PDF for .NET's Drawing.Polygon.
func (p *Page) DrawPolygon(points []Point, style ShapeStyle) error {
	if len(points) < 3 {
		return fmt.Errorf("draw polygon: need >= 3 points, got %d", len(points))
	}
	op := paintOp(style)
	if op == "" {
		return nil
	}
	var buf bytes.Buffer
	buf.WriteString("q\n")
	gsOp, err := p.applyAlpha(style.LineStyle.Color, style.FillColor)
	if err != nil {
		return err
	}
	buf.WriteString(gsOp)
	emitPolygonToBuf(&buf, p, points, style)
	buf.WriteString("Q\n")
	return p.appendToContentStream(buf.Bytes())
}
