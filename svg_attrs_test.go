// SPDX-License-Identifier: MIT

package asposepdf

import (
	"math"
	"testing"
)

func TestParseSVGColor_Hex3(t *testing.T) {
	c, ok := parseSVGColor("#f00")
	if !ok || c == nil {
		t.Fatalf("parseSVGColor(#f00) returned ok=%v c=%v", ok, c)
	}
	if math.Abs(c.R-1) > 1e-9 || c.G != 0 || c.B != 0 || c.A != 1 {
		t.Errorf("#f00 → %+v, want R=1 G=0 B=0 A=1", c)
	}
}

func TestParseSVGColor_Hex6(t *testing.T) {
	c, _ := parseSVGColor("#80c0FF")
	wantR, wantG, wantB := 128.0/255, 192.0/255, 1.0
	if math.Abs(c.R-wantR) > 1e-9 || math.Abs(c.G-wantG) > 1e-9 || math.Abs(c.B-wantB) > 1e-9 {
		t.Errorf("#80c0FF → %+v, want R=%g G=%g B=%g", c, wantR, wantG, wantB)
	}
}

func TestParseSVGColor_Hex8WithAlpha(t *testing.T) {
	c, _ := parseSVGColor("#ff000080")
	if c.R != 1 || c.G != 0 || c.B != 0 || math.Abs(c.A-128.0/255) > 1e-9 {
		t.Errorf("#ff000080 → %+v, want A=%g", c, 128.0/255)
	}
}

func TestParseSVGColor_RGB(t *testing.T) {
	c, _ := parseSVGColor("rgb(255, 128, 0)")
	if c.R != 1 || math.Abs(c.G-128.0/255) > 1e-9 || c.B != 0 {
		t.Errorf("rgb(255,128,0) → %+v", c)
	}
}

func TestParseSVGColor_RGBPercent(t *testing.T) {
	c, _ := parseSVGColor("rgb(100%, 50%, 0%)")
	if c.R != 1 || math.Abs(c.G-0.5) > 1e-9 || c.B != 0 {
		t.Errorf("rgb(100%%,50%%,0%%) → %+v", c)
	}
}

func TestParseSVGColor_RGBA(t *testing.T) {
	c, _ := parseSVGColor("rgba(0, 255, 0, 0.5)")
	if c.R != 0 || c.G != 1 || c.B != 0 || math.Abs(c.A-0.5) > 1e-9 {
		t.Errorf("rgba(0,255,0,0.5) → %+v", c)
	}
}

func TestParseSVGColor_NamedRed(t *testing.T) {
	c, _ := parseSVGColor("red")
	if c.R != 1 || c.G != 0 || c.B != 0 {
		t.Errorf("red → %+v", c)
	}
}

func TestParseSVGColor_NamedCaseInsensitive(t *testing.T) {
	c, _ := parseSVGColor("SlateBlue")
	wantR, wantG, wantB := 106.0/255, 90.0/255, 205.0/255
	if math.Abs(c.R-wantR) > 1e-9 || math.Abs(c.G-wantG) > 1e-9 || math.Abs(c.B-wantB) > 1e-9 {
		t.Errorf("SlateBlue → %+v", c)
	}
}

func TestParseSVGColor_None(t *testing.T) {
	c, ok := parseSVGColor("none")
	if !ok || c != nil {
		t.Errorf("none → ok=%v c=%v, want ok=true c=nil", ok, c)
	}
}

func TestParseSVGColor_Transparent(t *testing.T) {
	c, ok := parseSVGColor("transparent")
	if !ok || c != nil {
		t.Errorf("transparent → ok=%v c=%v, want ok=true c=nil", ok, c)
	}
}

func TestParseSVGColor_CurrentColor(t *testing.T) {
	c, ok := parseSVGColor("currentColor")
	if !ok || c == nil || c.R != 0 || c.G != 0 || c.B != 0 || c.A != 1 {
		t.Errorf("currentColor → ok=%v c=%+v, want ok=true c=black", ok, c)
	}
}

func TestParseSVGColor_Unrecognized(t *testing.T) {
	c, ok := parseSVGColor("not-a-color")
	if ok || c != nil {
		t.Errorf("garbage → ok=%v c=%v, want ok=false c=nil", ok, c)
	}
}

func TestParseSVGLength_Unitless(t *testing.T) {
	v, _ := parseSVGLength("42")
	if v != 42 {
		t.Errorf("42 → %g", v)
	}
}

func TestParseSVGLength_Px(t *testing.T) {
	v, _ := parseSVGLength("100px")
	if v != 100 {
		t.Errorf("100px → %g", v)
	}
}

func TestParseSVGLength_Pt(t *testing.T) {
	v, _ := parseSVGLength("10pt")
	if v != 10 {
		t.Errorf("10pt → %g", v)
	}
}

func TestParseSVGLength_Pc(t *testing.T) {
	v, _ := parseSVGLength("1pc")
	if v != 12 {
		t.Errorf("1pc → %g, want 12", v)
	}
}

func TestParseSVGLength_In(t *testing.T) {
	v, _ := parseSVGLength("1in")
	if v != 72 {
		t.Errorf("1in → %g, want 72", v)
	}
}

func TestParseSVGLength_Mm(t *testing.T) {
	v, _ := parseSVGLength("10mm")
	want := 10 * 72 / 25.4
	if math.Abs(v-want) > 1e-9 {
		t.Errorf("10mm → %g, want %g", v, want)
	}
}

func TestParseSVGLength_Cm(t *testing.T) {
	v, _ := parseSVGLength("1cm")
	want := 72 / 2.54
	if math.Abs(v-want) > 1e-9 {
		t.Errorf("1cm → %g, want %g", v, want)
	}
}

func TestParseSVGLength_Decimal(t *testing.T) {
	v, _ := parseSVGLength("3.14")
	if math.Abs(v-3.14) > 1e-9 {
		t.Errorf("3.14 → %g", v)
	}
}

func TestParseSVGLength_Negative(t *testing.T) {
	v, _ := parseSVGLength("-5")
	if v != -5 {
		t.Errorf("-5 → %g", v)
	}
}

func TestParseSVGLength_ScientificNotation(t *testing.T) {
	v, _ := parseSVGLength("1e2")
	if v != 100 {
		t.Errorf("1e2 → %g", v)
	}
}

func TestParseSVGLength_UnsupportedUnitFallsBackToZero(t *testing.T) {
	v, ok := parseSVGLength("10em")
	if ok || v != 0 {
		t.Errorf("10em → v=%g ok=%v, want 0/false", v, ok)
	}
}

func TestParseSVGLength_Garbage(t *testing.T) {
	v, ok := parseSVGLength("not-a-number")
	if ok || v != 0 {
		t.Errorf("garbage → v=%g ok=%v", v, ok)
	}
}

func TestParseSVGNumber_Basic(t *testing.T) {
	v, ok := parseSVGNumber("3.14")
	if !ok || math.Abs(v-3.14) > 1e-9 {
		t.Errorf("3.14 → %g ok=%v", v, ok)
	}
}

func TestParseSVGNumber_Garbage(t *testing.T) {
	_, ok := parseSVGNumber("not-a-number")
	if ok {
		t.Errorf("expected false for garbage")
	}
}

func TestParseSVGPaint_PlainColor(t *testing.T) {
	p, ok := parseSVGPaint("red")
	if !ok || p == nil || p.color == nil || p.gradRef != "" {
		t.Errorf("red → %+v ok=%v", p, ok)
	}
	if p.color.R != 1 {
		t.Errorf("red.R = %g", p.color.R)
	}
}

func TestParseSVGPaint_URLReference(t *testing.T) {
	p, ok := parseSVGPaint("url(#myGrad)")
	if !ok || p == nil || p.color != nil || p.gradRef != "myGrad" {
		t.Errorf("url(#myGrad) → %+v ok=%v", p, ok)
	}
}

func TestParseSVGPaint_URLWithWhitespace(t *testing.T) {
	p, _ := parseSVGPaint("url( #abc )")
	if p == nil || p.gradRef != "abc" {
		t.Errorf("url( #abc ) → %+v", p)
	}
}

func TestParseSVGPaint_None(t *testing.T) {
	p, ok := parseSVGPaint("none")
	if !ok || p != nil {
		t.Errorf("none → %+v ok=%v, want nil/true", p, ok)
	}
}

func TestParseSVGPaint_Garbage(t *testing.T) {
	_, ok := parseSVGPaint("not-a-thing")
	if ok {
		t.Error("garbage should fail")
	}
}
