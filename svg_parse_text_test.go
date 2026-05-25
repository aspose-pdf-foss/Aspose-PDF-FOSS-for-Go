// SPDX-License-Identifier: MIT

package asposepdf

import (
	"os"
	"testing"
)

func TestParseSVG_TextBasic(t *testing.T) {
	data, _ := os.ReadFile("testdata/svg/text_basic.svg")
	svg, err := parseSVGBytes(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(svg.root.children) != 1 {
		t.Fatalf("expected 1 child, got %d", len(svg.root.children))
	}
	tn, ok := svg.root.children[0].(*svgText)
	if !ok {
		t.Fatalf("expected *svgText, got %T", svg.root.children[0])
	}
	if len(tn.runs) != 1 {
		t.Fatalf("expected 1 run, got %d", len(tn.runs))
	}
	run := tn.runs[0]
	if run.text != "Hello world" {
		t.Errorf("text = %q", run.text)
	}
	if run.x != 10 || run.y != 50 {
		t.Errorf("position = (%g, %g)", run.x, run.y)
	}
	if run.style.fontFamily != "Arial" {
		t.Errorf("fontFamily = %q", run.style.fontFamily)
	}
	if run.style.fontSize != 14 {
		t.Errorf("fontSize = %g", run.style.fontSize)
	}
}

func TestParseSVG_TextWhitespaceCollapsed(t *testing.T) {
	svg, _ := parseSVGBytes([]byte(`<svg xmlns="http://www.w3.org/2000/svg"><text x="0" y="0">  hello   world  </text></svg>`))
	tn, _ := svg.root.children[0].(*svgText)
	if tn == nil || len(tn.runs) != 1 {
		t.Fatal("expected one run")
	}
	if tn.runs[0].text != "hello world" {
		t.Errorf("text = %q", tn.runs[0].text)
	}
}

func TestParseSVG_TextInheritsGroupFont(t *testing.T) {
	svg, _ := parseSVGBytes([]byte(`<svg xmlns="http://www.w3.org/2000/svg">
		<g font-family="Times" font-size="18">
			<text x="0" y="0">Hi</text>
		</g>
	</svg>`))
	g, _ := svg.root.children[0].(*svgGroup)
	tn, _ := g.children[0].(*svgText)
	if tn == nil {
		t.Fatal("no text node")
	}
	if tn.runs[0].style.fontFamily != "Times" {
		t.Errorf("inherited fontFamily = %q", tn.runs[0].style.fontFamily)
	}
	if tn.runs[0].style.fontSize != 18 {
		t.Errorf("inherited fontSize = %g", tn.runs[0].style.fontSize)
	}
}
