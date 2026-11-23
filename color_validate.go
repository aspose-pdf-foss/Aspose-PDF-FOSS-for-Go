// SPDX-License-Identifier: MIT

package asposepdf

import "fmt"

// checkColor rejects a colour with a component that cannot be written as a
// PDF number (NaN or ±Inf). A nil colour is fine. Shared by the text and
// vector drawing calls (GitHub issue #24 point 4): before this, such a
// component reached the content stream as the literal text "NaN" (a NaN
// alpha drew the colour opaque instead).
func checkColor(context, field string, c *Color) error {
	if c == nil {
		return nil
	}
	if nonFinite(c.R) || nonFinite(c.G) || nonFinite(c.B) || nonFinite(c.A) {
		return fmt.Errorf("%s: %s components must be finite, got {R:%v G:%v B:%v A:%v}", context, field, c.R, c.G, c.B, c.A)
	}
	return nil
}

// checkTextStyleValues rejects the TextStyle values that cannot be written
// as PDF numbers or make no sense: a non-finite or negative LineSpacing (a
// negative one stacked the lines upwards out of the rectangle), a
// non-finite CharSpacing, WordSpacing or HorizontalScaling (written as Tc,
// Tw and Tz; GitHub issue #25 point 1) and a non-finite component of Color,
// Background or StrokeColor.
func checkTextStyleValues(context string, style TextStyle) error {
	for _, f := range []struct {
		name string
		v    float64
	}{{"character spacing", style.CharSpacing}, {"word spacing", style.WordSpacing}, {"horizontal scaling", style.HorizontalScaling}} {
		if nonFinite(f.v) {
			return fmt.Errorf("%s: %s must be finite, got %v", context, f.name, f.v)
		}
	}
	if nonFinite(style.LineSpacing) {
		return fmt.Errorf("%s: line spacing must be finite, got %v", context, style.LineSpacing)
	}
	if style.LineSpacing < 0 {
		return fmt.Errorf("%s: line spacing must be non-negative, got %g", context, style.LineSpacing)
	}
	if err := checkColor(context, "color", style.Color); err != nil {
		return err
	}
	if err := checkColor(context, "background", style.Background); err != nil {
		return err
	}
	return checkColor(context, "stroke color", style.StrokeColor)
}

// checkShapeColors rejects a non-finite component in a shape's stroke
// colour, fill colour or gradient stop colours.
func checkShapeColors(context string, style ShapeStyle) error {
	if err := checkColor(context, "line color", style.LineStyle.Color); err != nil {
		return err
	}
	if err := checkColor(context, "fill color", style.FillColor); err != nil {
		return err
	}
	var stops []GradientStop
	switch g := style.FillGradient.(type) {
	case LinearGradient:
		stops = g.Stops
	case RadialGradient:
		stops = g.Stops
	}
	for i := range stops {
		if err := checkColor(context, fmt.Sprintf("gradient stop %d color", i), &stops[i].Color); err != nil {
			return err
		}
	}
	return nil
}
