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

func TestMatchSVGCSS_ClassMatch(t *testing.T) {
	rules := parseSVGCSS(`.red { fill: red; }`)
	s := defaultSVGStyle()
	s.cssClasses = []string{"red"}
	matchSVGCSS(&s, rules, "rect")
	if s.fill == nil || s.fill.color == nil || s.fill.color.R != 1 {
		t.Errorf("class match failed: %+v", s.fill)
	}
}

func TestMatchSVGCSS_IDMatch(t *testing.T) {
	rules := parseSVGCSS(`#special { fill: blue; }`)
	s := defaultSVGStyle()
	s.cssID = "special"
	matchSVGCSS(&s, rules, "rect")
	if s.fill == nil || s.fill.color == nil || s.fill.color.B != 1 {
		t.Errorf("id match failed: %+v", s.fill)
	}
}

func TestMatchSVGCSS_ElementMatch(t *testing.T) {
	rules := parseSVGCSS(`rect { stroke: black; stroke-width: 2; }`)
	s := defaultSVGStyle()
	matchSVGCSS(&s, rules, "rect")
	if s.stroke == nil || s.stroke.color == nil {
		t.Errorf("element match failed (stroke = %+v)", s.stroke)
	}
	if s.strokeWidth != 2 {
		t.Errorf("strokeWidth = %g, want 2", s.strokeWidth)
	}
}

func TestMatchSVGCSS_SpecificityIDWinsOverClass(t *testing.T) {
	// Class .red gives red; id #special gives green. ID wins.
	rules := parseSVGCSS(`.red { fill: red; } #special { fill: green; }`)
	s := defaultSVGStyle()
	s.cssClasses = []string{"red"}
	s.cssID = "special"
	matchSVGCSS(&s, rules, "rect")
	// "green" → rgb(0, 128, 0) → G ≈ 0.5019607843...
	if s.fill == nil || s.fill.color == nil || s.fill.color.G < 0.4 || s.fill.color.R > 0.1 {
		t.Errorf("id should win, got %+v", s.fill.color)
	}
}

func TestMatchSVGCSS_DocumentOrderWithinSameSpecificity(t *testing.T) {
	// Two class rules: later wins.
	rules := parseSVGCSS(`.red { fill: red; } .red { fill: blue; }`)
	s := defaultSVGStyle()
	s.cssClasses = []string{"red"}
	matchSVGCSS(&s, rules, "rect")
	if s.fill == nil || s.fill.color == nil || s.fill.color.B != 1 || s.fill.color.R != 0 {
		t.Errorf("later rule should win, got %+v", s.fill.color)
	}
}

func TestMatchSVGCSS_NoMatchNoChange(t *testing.T) {
	rules := parseSVGCSS(`.red { fill: red; }`)
	s := defaultSVGStyle()
	// No classes/id → no match
	matchSVGCSS(&s, rules, "rect")
	// Default fill is black, should stay black
	if s.fill == nil || s.fill.color == nil || s.fill.color.R != 0 {
		t.Errorf("no match should leave default, got %+v", s.fill.color)
	}
}
