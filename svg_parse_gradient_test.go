// SPDX-License-Identifier: MIT

package asposepdf

import (
	"encoding/xml"
	"math"
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
