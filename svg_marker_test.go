// SPDX-License-Identifier: MIT

package asposepdf

import (
	"os"
	"testing"
)

func TestParseSVG_MarkerParsed(t *testing.T) {
	data, _ := os.ReadFile("testdata/svg/marker_arrow.svg")
	svg, err := parseSVGBytes(data)
	if err != nil {
		t.Fatal(err)
	}
	m, ok := svg.defs["arr"].(*svgMarker)
	if !ok {
		t.Fatalf("defs[arr] = %T", svg.defs["arr"])
	}
	if !m.orient.auto {
		t.Error("orient should be auto")
	}
	if m.refX != 10 || m.refY != 5 {
		t.Errorf("ref = (%g,%g)", m.refX, m.refY)
	}
	if m.markerW != 10 || m.markerH != 10 {
		t.Errorf("marker size = %g×%g", m.markerW, m.markerH)
	}
	if m.viewBox == nil || m.viewBox.w != 10 || m.viewBox.h != 10 {
		t.Errorf("viewBox = %+v", m.viewBox)
	}
	if len(m.children) != 1 {
		t.Errorf("expected 1 child (path), got %d", len(m.children))
	}
}

func TestParseSVG_MarkerFixedAngle(t *testing.T) {
	svg, _ := parseSVGBytes([]byte(`<svg xmlns="http://www.w3.org/2000/svg">
		<defs><marker id="m" orient="45"><circle cx="0" cy="0" r="2"/></marker></defs>
	</svg>`))
	m, _ := svg.defs["m"].(*svgMarker)
	if m == nil {
		t.Fatal("marker not stored")
	}
	if m.orient.auto {
		t.Error("should not be auto")
	}
	if m.orient.angle != 45 {
		t.Errorf("angle = %g", m.orient.angle)
	}
}

func TestParseSVG_MarkerUserSpaceUnits(t *testing.T) {
	svg, _ := parseSVGBytes([]byte(`<svg xmlns="http://www.w3.org/2000/svg">
		<defs><marker id="m" markerUnits="userSpaceOnUse"><circle cx="0" cy="0" r="2"/></marker></defs>
	</svg>`))
	m, _ := svg.defs["m"].(*svgMarker)
	if m == nil {
		t.Fatal("marker not stored")
	}
	if m.units != svgMarkerUserSpace {
		t.Errorf("units = %v, want userSpaceOnUse", m.units)
	}
}
