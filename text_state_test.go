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
