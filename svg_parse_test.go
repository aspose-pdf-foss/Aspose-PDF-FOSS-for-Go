// SPDX-License-Identifier: MIT

package asposepdf

import (
	"os"
	"testing"
)

func TestParseSVG_MinimalRect(t *testing.T) {
	data, err := os.ReadFile("testdata/svg/rect.svg")
	if err != nil {
		t.Fatal(err)
	}
	svg, err := parseSVGBytes(data)
	if err != nil {
		t.Fatal(err)
	}
	if svg.viewBox == nil || svg.viewBox.w != 100 || svg.viewBox.h != 50 {
		t.Errorf("viewBox = %+v", svg.viewBox)
	}
	if svg.width != 100 || svg.height != 50 {
		t.Errorf("intrinsic = %g × %g", svg.width, svg.height)
	}
	if svg.root == nil || len(svg.root.children) != 1 {
		t.Fatalf("expected 1 child, got %d", len(svg.root.children))
	}
	r, ok := svg.root.children[0].(*svgRect)
	if !ok {
		t.Fatalf("expected *svgRect, got %T", svg.root.children[0])
	}
	if r.x != 10 || r.y != 10 || r.w != 80 || r.h != 30 {
		t.Errorf("rect dims = %g,%g %g×%g", r.x, r.y, r.w, r.h)
	}
	if r.style.fill == nil || r.style.fill.color == nil || r.style.fill.color.R != 1 || r.style.fill.color.G != 0 || r.style.fill.color.B != 0 {
		t.Errorf("fill = %+v", r.style.fill)
	}
}

func TestParseSVG_AllShapes(t *testing.T) {
	data, _ := os.ReadFile("testdata/svg/all_shapes.svg")
	svg, err := parseSVGBytes(data)
	if err != nil {
		t.Fatal(err)
	}
	got := len(svg.root.children)
	if got != 6 {
		t.Fatalf("expected 6 shape children, got %d", got)
	}
	kinds := map[string]int{}
	for _, c := range svg.root.children {
		kinds[c.svgNodeKind()]++
	}
	for _, k := range []string{"rect", "circle", "ellipse", "line", "polyline", "polygon"} {
		if kinds[k] != 1 {
			t.Errorf("expected 1 %s, got %d", k, kinds[k])
		}
	}
}

func TestParseSVG_InvalidXML(t *testing.T) {
	_, err := parseSVGBytes([]byte("<svg><not-closed"))
	if err == nil {
		t.Error("expected error for malformed XML")
	}
}

func TestParseSVG_NotSVGRoot(t *testing.T) {
	_, err := parseSVGBytes([]byte("<html><body></body></html>"))
	if err == nil {
		t.Error("expected error for non-svg root")
	}
}

func TestParseSVG_NoViewBox(t *testing.T) {
	svg, err := parseSVGBytes([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="50" height="30"/>`))
	if err != nil {
		t.Fatal(err)
	}
	if svg.viewBox != nil {
		t.Errorf("viewBox should be nil")
	}
	if svg.width != 50 || svg.height != 30 {
		t.Errorf("intrinsic = %g × %g", svg.width, svg.height)
	}
}

func TestParseSVG_GroupInheritance(t *testing.T) {
	data, _ := os.ReadFile("testdata/svg/nested_groups.svg")
	svg, _ := parseSVGBytes(data)
	if len(svg.root.children) != 1 {
		t.Fatalf("expected 1 top-level group, got %d", len(svg.root.children))
	}
	outer, _ := svg.root.children[0].(*svgGroup)
	if outer.style.fill == nil || outer.style.fill.color == nil || outer.style.fill.color.R != 1 {
		t.Errorf("outer fill = %+v, want red", outer.style.fill)
	}
	r, _ := outer.children[0].(*svgRect)
	if r.style.fill == nil || r.style.fill.color == nil || r.style.fill.color.R != 1 ||
		r.style.stroke == nil || r.style.stroke.color == nil || r.style.stroke.color.B != 1 {
		t.Errorf("rect inheritance failed: fill=%+v stroke=%+v", r.style.fill, r.style.stroke)
	}
	inner, _ := outer.children[1].(*svgGroup)
	if inner.style.opacity != 0.5 {
		t.Errorf("inner opacity = %g", inner.style.opacity)
	}
	if inner.style.fill == nil || inner.style.fill.color == nil || inner.style.fill.color.R != 1 {
		t.Errorf("inner inherited fill should be red")
	}
	innerRect, _ := inner.children[1].(*svgRect)
	// Green is parsed from hex #008000 which is RGB(0, 128, 255) → normalized to (0, 0.5019..., 0)
	if innerRect.style.fill == nil || innerRect.style.fill.color == nil ||
		innerRect.style.fill.color.R != 0 || innerRect.style.fill.color.G == 0 || innerRect.style.fill.color.B != 0 {
		t.Errorf("rect override fill should be green, got %+v", innerRect.style.fill)
	}
	if innerRect.style.stroke == nil || innerRect.style.stroke.color == nil || innerRect.style.stroke.color.B != 1 {
		t.Errorf("rect should inherit stroke=blue, got %+v", innerRect.style.stroke)
	}
}

func TestParseSVG_SkipsUnsupportedElements(t *testing.T) {
	data, _ := os.ReadFile("testdata/svg/with_unsupported.svg")
	svg, err := parseSVGBytes(data)
	if err != nil {
		t.Fatal(err)
	}
	rects := 0
	for _, c := range svg.root.children {
		if c.svgNodeKind() == "rect" {
			rects++
		}
	}
	if rects != 2 {
		t.Errorf("expected 2 rects, got %d (total children: %d)", rects, len(svg.root.children))
	}
}

func TestParseSVG_GradientRefFallbacksToFill(t *testing.T) {
	data, _ := os.ReadFile("testdata/svg/with_unsupported.svg")
	svg, _ := parseSVGBytes(data)
	r0, _ := svg.root.children[0].(*svgRect)
	if r0.style.fill == nil || r0.style.fill.color == nil ||
		(r0.style.fill.color.R != 0 || r0.style.fill.color.G != 0 || r0.style.fill.color.B != 0) {
		t.Errorf("gradient-ref rect fill = %+v, want black fallback", r0.style.fill)
	}
}

func TestParseSVG_IgnoresForeignNamespaceAttrs(t *testing.T) {
	data, _ := os.ReadFile("testdata/svg/with_namespaces.svg")
	svg, err := parseSVGBytes(data)
	if err != nil {
		t.Fatal(err)
	}
	r, _ := svg.root.children[0].(*svgRect)
	if r == nil || r.style.fill == nil || r.style.fill.color == nil || r.style.fill.color.R != 1 {
		t.Errorf("inkscape namespace shouldn't break red fill: %+v", r)
	}
}

func TestParseSVG_TransformOnGroup(t *testing.T) {
	data, _ := os.ReadFile("testdata/svg/with_transforms.svg")
	svg, _ := parseSVGBytes(data)
	g, _ := svg.root.children[0].(*svgGroup)
	if g.transform == nil {
		t.Fatal("expected group to have transform")
	}
	if g.transform[4] != 10 || g.transform[5] != 20 {
		t.Errorf("translate(10,20) → %v", *g.transform)
	}
}

func TestParseSVG_TransformOnShape(t *testing.T) {
	data, _ := os.ReadFile("testdata/svg/with_transforms.svg")
	svg, _ := parseSVGBytes(data)
	g, _ := svg.root.children[0].(*svgGroup)
	r, _ := g.children[0].(*svgRect)
	if r.transform == nil {
		t.Fatal("expected rect to have own transform")
	}
}

func TestParseSVG_TransformOnPath(t *testing.T) {
	data, _ := os.ReadFile("testdata/svg/with_transforms.svg")
	svg, _ := parseSVGBytes(data)
	p, _ := svg.root.children[1].(*svgPath)
	if p.transform == nil {
		t.Fatal("expected path to have transform")
	}
	if p.transform[0] != 2 || p.transform[3] != 2 {
		t.Errorf("scale(2) → %v", *p.transform)
	}
}
