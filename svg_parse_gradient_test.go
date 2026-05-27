// SPDX-License-Identifier: MIT

package asposepdf

import (
	"encoding/xml"
	"math"
	"os"
	"strings"
	"testing"
)

func parseStopElement(t *testing.T, xmlStr string) svgGradientStop {
	t.Helper()
	d := xml.NewDecoder(strings.NewReader(xmlStr))
	for {
		tok, err := d.Token()
		if err != nil {
			t.Fatal(err)
		}
		if start, ok := tok.(xml.StartElement); ok {
			return parseSVGGradientStop(d, start)
		}
	}
}

func TestParseSVGStop_BasicOffsetAndColor(t *testing.T) {
	s := parseStopElement(t, `<stop offset="0.5" stop-color="red"/>`)
	if math.Abs(s.offset-0.5) > 1e-9 || s.color == nil || s.color.R != 1 || s.opacity != 1 {
		t.Errorf("got %+v color=%+v", s, s.color)
	}
}

func TestParseSVGStop_OffsetPercent(t *testing.T) {
	s := parseStopElement(t, `<stop offset="75%" stop-color="blue"/>`)
	if math.Abs(s.offset-0.75) > 1e-9 {
		t.Errorf("offset = %g", s.offset)
	}
}

func TestParseSVGStop_OpacityFromAttribute(t *testing.T) {
	s := parseStopElement(t, `<stop offset="0" stop-color="green" stop-opacity="0.5"/>`)
	if math.Abs(s.opacity-0.5) > 1e-9 {
		t.Errorf("opacity = %g", s.opacity)
	}
}

func TestParseSVGStop_StyleAttribute(t *testing.T) {
	s := parseStopElement(t, `<stop offset="0" style="stop-color:red;stop-opacity:0.3"/>`)
	if s.color == nil || s.color.R != 1 {
		t.Errorf("color = %+v", s.color)
	}
	if math.Abs(s.opacity-0.3) > 1e-9 {
		t.Errorf("opacity = %g", s.opacity)
	}
}

func TestParseSVGStop_DefaultsWhenAbsent(t *testing.T) {
	s := parseStopElement(t, `<stop offset="0"/>`)
	if s.color == nil || s.color.R != 0 || s.color.G != 0 || s.color.B != 0 {
		t.Errorf("default color = %+v", s.color)
	}
	if s.opacity != 1 {
		t.Errorf("default opacity = %g", s.opacity)
	}
}

func TestParseSVG_LinearGradientCollected(t *testing.T) {
	data, _ := os.ReadFile("testdata/svg/linear_gradient.svg")
	svg, err := parseSVGBytes(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(svg.gradients) != 1 {
		t.Fatalf("gradients count = %d", len(svg.gradients))
	}
	g, ok := svg.gradients["grad1"].(*svgLinearGradient)
	if !ok {
		t.Fatalf("type = %T", svg.gradients["grad1"])
	}
	if g.x1 != 0 || g.x2 != 100 || g.y1 != 0 || g.y2 != 0 {
		t.Errorf("coords = (%g,%g)-(%g,%g)", g.x1, g.y1, g.x2, g.y2)
	}
	if len(g.stops) != 2 {
		t.Errorf("stops count = %d", len(g.stops))
	}
	if g.stops[0].color.R != 1 || g.stops[1].color.B != 1 {
		t.Errorf("stop colors wrong: %+v", g.stops)
	}
}

func TestParseSVG_RadialGradientCollected(t *testing.T) {
	data, _ := os.ReadFile("testdata/svg/radial_gradient.svg")
	svg, _ := parseSVGBytes(data)
	g, ok := svg.gradients["grad2"].(*svgRadialGradient)
	if !ok {
		t.Fatalf("type = %T", svg.gradients["grad2"])
	}
	if g.cx != 50 || g.r != 50 {
		t.Errorf("radial coords wrong: cx=%g r=%g", g.cx, g.r)
	}
	if len(g.stops) != 3 {
		t.Errorf("stops = %d", len(g.stops))
	}
	if g.units != svgGradientUserSpace {
		t.Errorf("units = %v", g.units)
	}
	// gradientTransform="matrix(1 0 0 1 0 0)" IS identity — transform should be nil
	if g.transform != nil {
		t.Errorf("expected nil transform for identity matrix, got %v", g.transform)
	}
}

// Regression: gradient coords accept '%' (parses as a 0..1 fraction) and
// gradientUnits defaults to objectBoundingBox per SVG 1.1 §13.2.2 / §13.2.3.
// Before the fix, parseSVGLength rejected '%' (returning 0) and the default
// unit was userSpaceOnUse — so the common idiom <radialGradient cx="50%"
// cy="50%" r="50%"> (no explicit gradientUnits) parsed as a degenerate
// (0, 0, 0) gradient and the whole shape rendered in the extended last-stop
// colour.
func TestParseSVG_GradientPercentInBBoxMode(t *testing.T) {
	svg, err := parseSVGBytes([]byte(`<?xml version="1.0"?>
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 100">
  <defs>
    <radialGradient id="g" cx="50%" cy="50%" r="50%" fx="30%" fy="30%">
      <stop offset="0%" stop-color="white"/>
      <stop offset="100%" stop-color="blue"/>
    </radialGradient>
  </defs>
</svg>`))
	if err != nil {
		t.Fatal(err)
	}
	g, ok := svg.gradients["g"].(*svgRadialGradient)
	if !ok {
		t.Fatalf("type = %T", svg.gradients["g"])
	}
	// Default units must be objectBoundingBox (SVG spec default).
	if g.units != svgGradientObjectBBox {
		t.Errorf("units = %v, want objectBoundingBox (zero-value default was userSpaceOnUse before fix)", g.units)
	}
	// Percent coords resolve to 0..1 fractions; bbox matrix scales at render time.
	const eps = 1e-9
	if math.Abs(g.cx-0.5) > eps || math.Abs(g.cy-0.5) > eps {
		t.Errorf("cx,cy = (%g, %g), want (0.5, 0.5) — '50%%' should be parsed", g.cx, g.cy)
	}
	if math.Abs(g.r-0.5) > eps {
		t.Errorf("r = %g, want 0.5", g.r)
	}
	if math.Abs(g.fx-0.3) > eps || math.Abs(g.fy-0.3) > eps {
		t.Errorf("fx,fy = (%g, %g), want (0.3, 0.3)", g.fx, g.fy)
	}
}

// userSpaceOnUse with % values resolves against the SVG viewport per
// SVG 1.1 §7.10: x-axis uses width, y-axis uses height, radius uses
// sqrt((w²+h²)/2).
func TestParseSVG_GradientPercentInUserSpace(t *testing.T) {
	svg, err := parseSVGBytes([]byte(`<?xml version="1.0"?>
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 200 100">
  <defs>
    <linearGradient id="g" x1="0%" y1="50%" x2="100%" y2="50%" gradientUnits="userSpaceOnUse">
      <stop offset="0" stop-color="red"/>
      <stop offset="1" stop-color="blue"/>
    </linearGradient>
  </defs>
</svg>`))
	if err != nil {
		t.Fatal(err)
	}
	g := svg.gradients["g"].(*svgLinearGradient)
	const eps = 1e-9
	// viewport is 200x100 (from viewBox); x% scales by 200, y% scales by 100.
	if math.Abs(g.x1-0) > eps || math.Abs(g.y1-50) > eps {
		t.Errorf("(x1,y1) = (%g, %g), want (0, 50)", g.x1, g.y1)
	}
	if math.Abs(g.x2-200) > eps || math.Abs(g.y2-50) > eps {
		t.Errorf("(x2,y2) = (%g, %g), want (200, 50)", g.x2, g.y2)
	}
}

func TestParseSVG_RectWithGradientFillRef(t *testing.T) {
	data, _ := os.ReadFile("testdata/svg/linear_gradient.svg")
	svg, _ := parseSVGBytes(data)
	r, _ := svg.root.children[0].(*svgRect)
	if r == nil || r.style.fill == nil || r.style.fill.gradRef != "grad1" {
		t.Errorf("rect fill = %+v", r.style.fill)
	}
}

// Verify math import is used (suppress "imported and not used" if refactored away)
var _ = math.Abs
