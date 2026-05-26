// SPDX-License-Identifier: MIT

package asposepdf

import (
	"os"
	"testing"
)

func TestParseSVG_ClipPathStoredInDefs(t *testing.T) {
	data, _ := os.ReadFile("testdata/svg/clippath_circle.svg")
	svg, _ := parseSVGBytes(data)
	cp, ok := svg.defs["circle-clip"].(*svgClipPath)
	if !ok {
		t.Fatalf("defs[circle-clip] = %T", svg.defs["circle-clip"])
	}
	if len(cp.children) != 1 {
		t.Errorf("expected 1 clip child, got %d", len(cp.children))
	}
	if _, ok := cp.children[0].(*svgCircle); !ok {
		t.Errorf("clip child[0] = %T, want *svgCircle", cp.children[0])
	}
}

func TestParseSVG_ClipPathObjectBoundingBox(t *testing.T) {
	svg, _ := parseSVGBytes([]byte(`<svg xmlns="http://www.w3.org/2000/svg">
		<defs>
			<clipPath id="bbox-clip" clipPathUnits="objectBoundingBox">
				<rect x="0.1" y="0.1" width="0.8" height="0.8"/>
			</clipPath>
		</defs>
	</svg>`))
	cp, _ := svg.defs["bbox-clip"].(*svgClipPath)
	if cp == nil {
		t.Fatal("clipPath not stored")
	}
	if cp.units != svgGradientObjectBBox {
		t.Errorf("expected objectBoundingBox units, got %v", cp.units)
	}
}
