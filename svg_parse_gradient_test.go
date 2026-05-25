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
