// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"os"
	"testing"
)

func TestRenderSVG_BasicShapesProducesContentStream(t *testing.T) {
	data, err := os.ReadFile("testdata/svg/all_shapes.svg")
	if err != nil {
		t.Fatal(err)
	}
	svg, err := parseSVGBytes(data)
	if err != nil {
		t.Fatal(err)
	}
	doc := NewDocumentFromFormat(PageFormatA4)
	p, err := doc.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	if err := renderSVG(p, svg, Rectangle{LLX: 100, LLY: 600, URX: 300, URY: 800}); err != nil {
		t.Fatal(err)
	}
	stream, err := p.contentStreams()
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"q\n", "Q\n", "cm\n", " re "} {
		if !bytes.Contains(stream, []byte(want)) {
			t.Errorf("expected %q in content stream, got:\n%s", want, stream)
		}
	}
}

func TestRenderSVG_CircleEmitsCurves(t *testing.T) {
	svgData := []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="100" height="100" viewBox="0 0 100 100">
		<circle cx="50" cy="50" r="40" fill="blue"/>
	</svg>`)
	svg, err := parseSVGBytes(svgData)
	if err != nil {
		t.Fatal(err)
	}
	doc := NewDocumentFromFormat(PageFormatA4)
	p, err := doc.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	if err := renderSVG(p, svg, Rectangle{LLX: 0, LLY: 0, URX: 200, URY: 200}); err != nil {
		t.Fatal(err)
	}
	stream, err := p.contentStreams()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(stream, []byte(" c\n")) {
		t.Errorf("circle should emit bezier curves: %s", stream)
	}
}

func TestRenderSVG_LineEmitsStroke(t *testing.T) {
	svgData := []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="100" height="100" viewBox="0 0 100 100">
		<line x1="10" y1="10" x2="90" y2="90" stroke="black" stroke-width="2"/>
	</svg>`)
	svg, err := parseSVGBytes(svgData)
	if err != nil {
		t.Fatal(err)
	}
	doc := NewDocumentFromFormat(PageFormatA4)
	p, err := doc.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	if err := renderSVG(p, svg, Rectangle{LLX: 0, LLY: 0, URX: 200, URY: 200}); err != nil {
		t.Fatal(err)
	}
	stream, err := p.contentStreams()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(stream, []byte("S\n")) {
		t.Errorf("line should emit S stroke op: %s", stream)
	}
}

func TestRenderSVG_PolygonEmitsClose(t *testing.T) {
	svgData := []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="100" height="100" viewBox="0 0 100 100">
		<polygon points="50,10 90,90 10,90" fill="orange"/>
	</svg>`)
	svg, err := parseSVGBytes(svgData)
	if err != nil {
		t.Fatal(err)
	}
	doc := NewDocumentFromFormat(PageFormatA4)
	p, err := doc.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	if err := renderSVG(p, svg, Rectangle{LLX: 0, LLY: 0, URX: 200, URY: 200}); err != nil {
		t.Fatal(err)
	}
	stream, err := p.contentStreams()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(stream, []byte(" h\n")) {
		t.Errorf("polygon should close path with h: %s", stream)
	}
}

func TestRenderSVG_PathWithCubicBeziers(t *testing.T) {
	svg, _ := parseSVGBytes([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 100">
		<path d="M 10 10 C 20 0 80 0 90 10 Z" fill="red"/>
	</svg>`))
	doc := NewDocumentFromFormat(PageFormatA4)
	page, _ := doc.Page(1)
	if err := renderSVG(page, svg, Rectangle{LLX: 0, LLY: 0, URX: 200, URY: 200}); err != nil {
		t.Fatal(err)
	}
	stream, err := page.contentStreams()
	if err != nil {
		t.Fatal(err)
	}
	// Expect: m operator (moveto), c operator (curveto), h operator (closepath)
	for _, want := range []string{" m\n", " c\n", "h\n"} {
		if !bytes.Contains(stream, []byte(want)) {
			t.Errorf("missing operator %q in stream:\n%s", want, stream)
		}
	}
}

func TestRenderSVG_PathFillRuleEvenOdd(t *testing.T) {
	svg, _ := parseSVGBytes([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 100">
		<path d="M 10 10 L 50 10 L 50 50 L 10 50 Z" fill="red" fill-rule="evenodd"/>
	</svg>`))
	doc := NewDocumentFromFormat(PageFormatA4)
	page, _ := doc.Page(1)
	_ = renderSVG(page, svg, Rectangle{LLX: 0, LLY: 0, URX: 200, URY: 200})
	stream, _ := page.contentStreams()
	if !bytes.Contains(stream, []byte("f*\n")) && !bytes.Contains(stream, []byte("f* ")) {
		t.Errorf("expected f* (even-odd fill) operator, got:\n%s", stream)
	}
}

func TestRenderSVG_DisplayNoneSkipsShape(t *testing.T) {
	svgData := []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="100" height="100" viewBox="0 0 100 100">
		<rect x="10" y="10" width="80" height="80" fill="red" display="none"/>
	</svg>`)
	svg, err := parseSVGBytes(svgData)
	if err != nil {
		t.Fatal(err)
	}
	doc := NewDocumentFromFormat(PageFormatA4)
	p, err := doc.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	if err := renderSVG(p, svg, Rectangle{LLX: 0, LLY: 0, URX: 200, URY: 200}); err != nil {
		t.Fatal(err)
	}
	stream, err := p.contentStreams()
	if err != nil {
		t.Fatal(err)
	}
	// display=none: no rect op (re) should appear
	if bytes.Contains(stream, []byte(" re ")) {
		t.Errorf("display=none rect should not emit re op: %s", stream)
	}
}
