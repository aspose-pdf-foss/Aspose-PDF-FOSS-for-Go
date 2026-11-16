// SPDX-License-Identifier: MIT

package asposepdf

import (
	"math"
	"testing"
)

func TestTextStateInherit(t *testing.T) {
	blue := &Color{B: 1, A: 1}
	seg := TextState{FontSize: 0, ForegroundColor: blue}
	frag := TextState{Font: FontTimesRoman, FontSize: 5, Underline: true}
	got := seg.inherit(frag)
	if got.Font != FontTimesRoman || got.FontSize != 5 || got.ForegroundColor != blue {
		t.Fatalf("inherit = %+v", got)
	}
	if !got.Underline {
		t.Fatal("Underline lost")
	}
	if !(TextState{Underline: true}).inherit(TextState{}).Underline {
		t.Fatal("child Underline lost")
	}
}

func TestTextStateResolvedDefaults(t *testing.T) {
	r := TextState{}.resolved()
	if r.Font != FontHelvetica || r.FontSize != 12 || r.HorizontalScaling != 100 {
		t.Fatalf("resolved = %+v", r)
	}
	if r.ForegroundColor == nil || *r.ForegroundColor != (Color{0, 0, 0, 1}) {
		t.Fatalf("color = %+v", r.ForegroundColor)
	}
}

func TestTextStateMeasureString(t *testing.T) {
	s := TextState{Font: FontHelvetica, FontSize: 10}
	base, err := MeasureText("a b", TextStyle{Font: FontHelvetica, Size: 10})
	if err != nil {
		t.Fatal(err)
	}
	w, err := s.MeasureString("a b")
	if err != nil || math.Abs(w-base) > 1e-9 {
		t.Fatalf("w=%v base=%v err=%v", w, base, err)
	}
	s.HorizontalScaling = 50
	w, _ = s.MeasureString("a b")
	if math.Abs(w-base/2) > 1e-9 {
		t.Fatalf("scaled w=%v", w)
	}
	s = TextState{Font: FontHelvetica, FontSize: 10, CharacterSpacing: 1}
	w, _ = s.MeasureString("a b")
	if math.Abs(w-(base+3)) > 1e-9 {
		t.Fatalf("Tc w=%v", w)
	}
	s = TextState{Font: FontHelvetica, FontSize: 10, WordSpacing: 2}
	w, _ = s.MeasureString("a b")
	if math.Abs(w-(base+2)) > 1e-9 {
		t.Fatalf("Tw w=%v", w)
	}
}

func TestTextStateFontStyle(t *testing.T) {
	s := TextState{Font: FontTimesRoman, FontStyle: FontStyleBold | FontStyleItalic}
	if got := s.style().Font; got != FontTimesBoldItalic {
		t.Fatalf("font = %v", got.BaseFont())
	}
	h := TextState{Font: FontHelvetica, FontStyle: FontStyleItalic}
	if got := h.style().Font.BaseFont(); got != "Helvetica-Oblique" {
		t.Fatalf("font = %v", got)
	}
}

func TestTextStateApplyChangesFrom(t *testing.T) {
	s := TextState{FontSize: 8, Font: FontCourier}
	s.ApplyChangesFrom(TextState{FontSize: 14, StrikeOut: true})
	if s.FontSize != 14 || s.Font != FontCourier || !s.StrikeOut {
		t.Fatalf("%+v", s)
	}
}

type fakeEmbedded struct{}

func (fakeEmbedded) BaseFont() string { return "Times-Fake" }
func (fakeEmbedded) IsEmbedded() bool { return true }

func TestTextStateStyledFontBranches(t *testing.T) {
	cases := []struct {
		f    Font
		fs   FontStyle
		want Font
	}{
		{FontTimesRoman, FontStyleBold, FontTimesBold},
		{FontTimesRoman, FontStyleItalic, FontTimesItalic},
		{FontTimesRoman, FontStyleBold | FontStyleItalic, FontTimesBoldItalic},
		{FontHelvetica, FontStyleBold, FontHelveticaBold},
		{FontHelvetica, FontStyleItalic, FontHelveticaOblique},
		{FontHelvetica, FontStyleBold | FontStyleItalic, FontHelveticaBoldOblique},
		{FontCourier, FontStyleBold, FontCourierBold},
		{FontCourier, FontStyleItalic, FontCourierOblique},
		{FontCourier, FontStyleBold | FontStyleItalic, FontCourierBoldOblique},
		{FontCourier, FontStyleRegular, FontCourier},
		{FontSymbol, FontStyleBold, FontSymbol},
		{FontZapfDingbats, FontStyleItalic, FontZapfDingbats},
	}
	for _, c := range cases {
		if got := styledFont(c.f, c.fs); got != c.want {
			t.Errorf("%s/%d -> %s, want %s", c.f.BaseFont(), c.fs, got.BaseFont(), c.want.BaseFont())
		}
	}
	e := fakeEmbedded{}
	if got := styledFont(e, FontStyleBold); got != Font(e) {
		t.Errorf("embedded changed: %v", got)
	}
	if styledFont(nil, FontStyleBold) != nil {
		t.Error("nil font changed")
	}
}

func TestTextStateStyleMapping(t *testing.T) {
	red, blue := &Color{R: 1, A: 1}, &Color{B: 1, A: 1}
	st := TextState{
		Font: FontHelvetica, FontSize: 10, ForegroundColor: red, BackgroundColor: blue,
		Underline: true, StrikeOut: true, Rotation: 30, LineSpacing: 15,
	}.style()
	if st.Size != 10 || st.Color != red || st.Background != blue || !st.Underline || !st.Strikethrough || st.Rotation != 30 {
		t.Fatalf("style = %+v", st)
	}
	if st.LineSpacing != 1.5 {
		t.Fatalf("LineSpacing = %v", st.LineSpacing)
	}
	if st.Invisible {
		t.Fatal("Invisible set")
	}
	if (TextState{FontSize: 10}).style().LineSpacing != 0 {
		t.Fatal("LineSpacing without value")
	}
	if !(TextState{Invisible: true}).style().Invisible {
		t.Fatal("Invisible bool lost")
	}
	if !(TextState{RenderingMode: RenderingInvisible}).style().Invisible {
		t.Fatal("RenderingInvisible lost")
	}
	if (TextState{RenderingMode: RenderingStroke}).style().Invisible {
		t.Fatal("stroke mode invisible")
	}
}

func TestSpacedWidth(t *testing.T) {
	if got := spacedWidth(10, "a b", 1, 2, 0); got != 15 {
		t.Fatalf("tz=0: %v", got)
	}
	if got := spacedWidth(10, "a b", 1, 2, 50); got != 7.5 {
		t.Fatalf("tz=50: %v", got)
	}
}

func TestTextStateInheritAllFields(t *testing.T) {
	c := &Color{A: 1}
	p := TextState{Font: FontCourier, FontSize: 7, FontStyle: FontStyleBold, ForegroundColor: c, BackgroundColor: c, StrokingColor: c,
		RenderingMode: RenderingStroke, CharacterSpacing: 1, WordSpacing: 2, HorizontalScaling: 80, LineSpacing: 9, Rotation: 5,
		StrikeOut: true, Invisible: true}
	g := TextState{}.inherit(p)
	if g != p {
		t.Fatalf("inherit = %+v want %+v", g, p)
	}
	if !(TextState{StrikeOut: true}).inherit(TextState{}).StrikeOut || !(TextState{Invisible: true}).inherit(TextState{}).Invisible {
		t.Fatal("child bool lost")
	}
}

func TestTextStateStyleLineSpacingNeedsSize(t *testing.T) {
	if got := (TextState{LineSpacing: 5}).style().LineSpacing; got != 0 {
		t.Fatalf("LineSpacing = %v", got)
	}
}

func TestSpacedWidthCountsOnlySpaces(t *testing.T) {
	// "aab" has no spaces, "a  " has two: word spacing must count U+0020 only.
	if got := spacedWidth(0, "aab", 0, 1, 100); got != 0 {
		t.Fatalf("no spaces: %v", got)
	}
	if got := spacedWidth(0, "a  ", 0, 1, 100); got != 2 {
		t.Fatalf("two spaces: %v", got)
	}
}
