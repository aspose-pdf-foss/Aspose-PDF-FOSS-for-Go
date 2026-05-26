// SPDX-License-Identifier: MIT

package asposepdf

import "testing"

func TestParseSVGCSS_ClassRule(t *testing.T) {
	rules := parseSVGCSS(`.red { fill: red; stroke: black; }`)
	if len(rules) != 1 {
		t.Fatalf("got %d rules", len(rules))
	}
	if rules[0].properties["fill"] != "red" {
		t.Errorf("fill = %q", rules[0].properties["fill"])
	}
	if rules[0].properties["stroke"] != "black" {
		t.Errorf("stroke = %q", rules[0].properties["stroke"])
	}
	if len(rules[0].selectors) != 1 {
		t.Fatalf("selectors len = %d", len(rules[0].selectors))
	}
	if rules[0].selectors[0].kind != cssSelClass || rules[0].selectors[0].name != "red" {
		t.Errorf("selector = %+v", rules[0].selectors[0])
	}
}

func TestParseSVGCSS_MultipleRules(t *testing.T) {
	rules := parseSVGCSS(`
		.foo { fill: red; }
		#bar { stroke: blue; }
		rect { opacity: 0.5; }
	`)
	if len(rules) != 3 {
		t.Fatalf("got %d", len(rules))
	}
	if rules[1].selectors[0].kind != cssSelID || rules[1].selectors[0].name != "bar" {
		t.Errorf("rules[1] selector = %+v", rules[1].selectors[0])
	}
	if rules[2].selectors[0].kind != cssSelElement || rules[2].selectors[0].name != "rect" {
		t.Errorf("rules[2] selector = %+v", rules[2].selectors[0])
	}
}

func TestParseSVGCSS_GroupedSelector(t *testing.T) {
	rules := parseSVGCSS(`.a, .b, #c { fill: red; }`)
	if len(rules) != 1 {
		t.Fatalf("got %d", len(rules))
	}
	if len(rules[0].selectors) != 3 {
		t.Errorf("expected 3 selectors")
	}
}

func TestParseSVGCSS_Comment(t *testing.T) {
	rules := parseSVGCSS(`/* this is a comment */ .red { fill: red; }`)
	if len(rules) != 1 {
		t.Fatalf("got %d", len(rules))
	}
	if rules[0].selectors[0].name != "red" {
		t.Errorf("got %+v", rules[0].selectors[0])
	}
}

func TestParseSVGCSS_Empty(t *testing.T) {
	rules := parseSVGCSS(``)
	if len(rules) != 0 {
		t.Errorf("got %d", len(rules))
	}
	rules = parseSVGCSS(`   `)
	if len(rules) != 0 {
		t.Errorf("got %d", len(rules))
	}
}
