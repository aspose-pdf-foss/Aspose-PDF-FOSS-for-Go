// SPDX-License-Identifier: MIT

package asposepdf

import (
	"os"
	"testing"
)

func TestParseSVG_MaskStoredInDefs(t *testing.T) {
	data, _ := os.ReadFile("testdata/svg/mask_circle.svg")
	svg, err := parseSVGBytes(data)
	if err != nil {
		t.Fatal(err)
	}
	m, ok := svg.defs["m1"].(*svgMask)
	if !ok {
		t.Fatalf("defs[m1] = %T", svg.defs["m1"])
	}
	if len(m.children) != 2 {
		t.Errorf("expected 2 children, got %d", len(m.children))
	}
	if m.units != svgGradientObjectBBox {
		t.Errorf("default maskUnits should be objectBoundingBox, got %v", m.units)
	}
}

func TestParseSVG_MaskUserSpaceUnits(t *testing.T) {
	svg, err := parseSVGBytes([]byte(`<svg xmlns="http://www.w3.org/2000/svg">
		<defs><mask id="m2" maskUnits="userSpaceOnUse"><rect width="50" height="50" fill="white"/></mask></defs>
	</svg>`))
	if err != nil {
		t.Fatal(err)
	}
	m, ok := svg.defs["m2"].(*svgMask)
	if !ok {
		t.Fatal("mask not stored in defs")
	}
	if m.units != svgGradientUserSpace {
		t.Errorf("expected userSpaceOnUse, got %v", m.units)
	}
}

func TestParseSVG_MaskContentUnitsObjectBBox(t *testing.T) {
	svg, err := parseSVGBytes([]byte(`<svg xmlns="http://www.w3.org/2000/svg">
		<defs><mask id="m3" maskContentUnits="objectBoundingBox"><rect width="1" height="1" fill="white"/></mask></defs>
	</svg>`))
	if err != nil {
		t.Fatal(err)
	}
	m, ok := svg.defs["m3"].(*svgMask)
	if !ok {
		t.Fatal("mask not stored in defs")
	}
	if m.contentUnits != svgGradientObjectBBox {
		t.Errorf("expected objectBoundingBox, got %v", m.contentUnits)
	}
}

func TestParseSVG_MaskDefaultContentUnits(t *testing.T) {
	svg, err := parseSVGBytes([]byte(`<svg xmlns="http://www.w3.org/2000/svg">
		<defs><mask id="m4"><rect width="50" height="50" fill="white"/></mask></defs>
	</svg>`))
	if err != nil {
		t.Fatal(err)
	}
	m, ok := svg.defs["m4"].(*svgMask)
	if !ok {
		t.Fatal("mask not stored in defs")
	}
	if m.contentUnits != svgGradientUserSpace {
		t.Errorf("default maskContentUnits should be userSpaceOnUse, got %v", m.contentUnits)
	}
}

func TestParseSVG_MaskTopLevelElement(t *testing.T) {
	svg, err := parseSVGBytes([]byte(`<svg xmlns="http://www.w3.org/2000/svg">
		<mask id="m5">
			<circle cx="50" cy="50" r="30" fill="black"/>
		</mask>
	</svg>`))
	if err != nil {
		t.Fatal(err)
	}
	m, ok := svg.defs["m5"].(*svgMask)
	if !ok {
		t.Fatal("top-level mask not stored in defs")
	}
	if len(m.children) != 1 {
		t.Errorf("expected 1 child, got %d", len(m.children))
	}
}
