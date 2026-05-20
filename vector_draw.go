package asposepdf

import (
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

// DrawLine strokes a single line segment from→to with the given style.
// No-op if style.Width <= 0.
//
// Mirrors Aspose.PDF for .NET's Drawing.Line shape.
func (p *Page) DrawLine(from, to Point, style LineStyle) error {
	if style.Width <= 0 {
		return nil
	}
	var buf strings.Builder
	buf.WriteString("q\n")
	buf.WriteString(formatLineStyle(style))
	buf.WriteString(fmt.Sprintf("%s %s m\n", formatFloat(from.X), formatFloat(from.Y)))
	buf.WriteString(fmt.Sprintf("%s %s l\n", formatFloat(to.X), formatFloat(to.Y)))
	buf.WriteString("S\n")
	buf.WriteString("Q\n")
	return p.appendToContentStream([]byte(buf.String()))
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
	w := rect.URX - rect.LLX
	h := rect.URY - rect.LLY
	var buf strings.Builder
	buf.WriteString("q\n")
	buf.WriteString(formatShapeStyle(style))
	buf.WriteString(fmt.Sprintf("%s %s %s %s re %s\n",
		formatFloat(rect.LLX), formatFloat(rect.LLY),
		formatFloat(w), formatFloat(h), op))
	buf.WriteString("Q\n")
	return p.appendToContentStream([]byte(buf.String()))
}
