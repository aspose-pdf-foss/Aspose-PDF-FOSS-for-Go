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
