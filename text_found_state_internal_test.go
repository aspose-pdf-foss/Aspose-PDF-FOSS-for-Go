// SPDX-License-Identifier: MIT

package asposepdf

import (
	"math"
	"strings"
	"testing"
)

// foundStatePage is an A4 page with Helvetica as /F0 whose content is c,
// saved and reopened.
func foundStatePage(t *testing.T, c string) (*Document, *Page) {
	t.Helper()
	d := NewDocumentFromFormat(PageFormatA4)
	p := mustPage(t, d)
	if _, err := p.ensureStandardFontResource("/Helvetica"); err != nil {
		t.Fatal(err)
	}
	if err := p.appendToContentStream([]byte(c)); err != nil {
		t.Fatal(err)
	}
	d = reopenDoc(t, d)
	return d, mustPage(t, d)
}

func foundFragments(t *testing.T, p *Page, phrase string) []*TextFragment {
	t.Helper()
	a := NewTextFragmentAbsorber(phrase)
	if err := a.Visit(p); err != nil {
		t.Fatal(err)
	}
	return a.TextFragments()
}

func nearColor(c *Color, r, g, b float64) bool {
	return c != nil && math.Abs(c.R-r) < 1e-6 && math.Abs(c.G-g) < 1e-6 && math.Abs(c.B-b) < 1e-6
}

// A found fragment's TextState reports the spacing, scaling, rendering mode
// and stroking colour the text was drawn with.
func TestFoundTextStateReportsTextState(t *testing.T) {
	_, p := foundStatePage(t, "q BT /F0 12 Tf 1 0 0 1 72 700 Tm 0.5 Tc 2 Tw 80 Tz 0 0.5 1 RG 1 Tr (Stroked words) Tj ET Q\n"+
		"BT /F0 12 Tf 1 0 0 1 72 650 Tm (Plain text) Tj ET\n")
	frs := foundFragments(t, p, "Stroked words")
	if len(frs) != 1 {
		t.Fatalf("%d fragments", len(frs))
	}
	st := frs[0].Segments[0].TextState
	if st.CharacterSpacing != 0.5 || st.WordSpacing != 2 || st.HorizontalScaling != 80 {
		t.Errorf("spacing %v %v %v, want 0.5 2 80", st.CharacterSpacing, st.WordSpacing, st.HorizontalScaling)
	}
	if st.RenderingMode != RenderingStroke || st.Invisible {
		t.Errorf("rendering mode %d, invisible %v", st.RenderingMode, st.Invisible)
	}
	if !nearColor(st.StrokingColor, 0, 0.5, 1) {
		t.Errorf("stroking colour %+v", st.StrokingColor)
	}
	if frs[0].TextState.RenderingMode != RenderingStroke {
		t.Errorf("fragment state mode %d", frs[0].TextState.RenderingMode)
	}

	plain := foundFragments(t, p, "Plain text")[0].TextState
	if plain.HorizontalScaling != 100 || plain.WordSpacing != 0 || plain.RenderingMode != RenderingFill ||
		!nearColor(plain.StrokingColor, 0, 0, 0) {
		t.Errorf("default state %+v", plain)
	}
}

// Invisible text (3 Tr, and 7 Tr, which only clips) is flagged, also when it
// continues a visible run in one text object: extraction keeps one fragment
// (a mode change does not split it), the absorber splits the segments.
func TestFoundTextStateInvisible(t *testing.T) {
	d, p := foundStatePage(t, "BT /F0 12 Tf 1 0 0 1 72 700 Tm 0 Tr (Shown) Tj 3 Tr (Hidden) Tj ET\n"+
		"BT /F0 12 Tf 1 0 0 1 72 650 Tm 7 Tr (Clipped) Tj ET\n")
	lines, err := p.ExtractTextWithLayout()
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) == 0 || lines[0].Text != "ShownHidden" || len(lines[0].Fragments) != 1 {
		t.Fatalf("first line %+v", lines)
	}
	frs := foundFragments(t, p, "ShownHidden")
	if len(frs) != 1 || len(frs[0].Segments) != 2 {
		t.Fatalf("fragments %+v", frs)
	}
	if s := frs[0].Segments; s[0].TextState.Invisible || !s[1].TextState.Invisible || s[1].Text != "Hidden" ||
		s[1].TextState.RenderingMode != RenderingInvisible {
		t.Errorf("segments %q %v, %q %v", s[0].Text, s[0].TextState.Invisible, s[1].Text, s[1].TextState.Invisible)
	}
	clip := foundFragments(t, p, "Clipped")
	if len(clip) != 1 || !clip[0].TextState.Invisible {
		t.Fatalf("7 Tr not invisible: %+v", clip)
	}

	// Emptying the invisible segment removes it and leaves the visible one.
	frs[0].Segments[1].Text = ""
	if err := d.ApplyTextChanges(); err != nil {
		t.Fatal(err)
	}
	txt, err := mustPage(t, reopenDoc(t, d)).ExtractText()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(txt, "Hidden") || !strings.Contains(txt, "Shown") {
		t.Errorf("text after removing the invisible segment: %q", txt)
	}
}

// What AddTextAt draws reads back as drawn, the stroking colour included
// when the mode does not stroke.
func TestFoundTextStateRoundTrip(t *testing.T) {
	d := NewDocumentFromFormat(PageFormatA4)
	p := mustPage(t, d)
	style := TextStyle{Size: 14, StrokeColor: &Color{R: 1, A: 1}, WordSpacing: 3, HorizontalScaling: 90}
	if err := p.AddTextAt("fill only", style, 72, 700); err != nil {
		t.Fatal(err)
	}
	style.Invisible = true
	if err := p.AddTextAt("hidden layer", style, 72, 650); err != nil {
		t.Fatal(err)
	}
	p = mustPage(t, reopenDoc(t, d))
	for _, c := range []struct {
		text      string
		invisible bool
	}{{"fill only", false}, {"hidden layer", true}} {
		frs := foundFragments(t, p, c.text)
		if len(frs) != 1 {
			t.Fatalf("%q: %d fragments", c.text, len(frs))
		}
		st := frs[0].TextState
		if st.Invisible != c.invisible || !nearColor(st.StrokingColor, 1, 0, 0) || st.WordSpacing != 3 || st.HorizontalScaling != 90 {
			t.Errorf("%q: %+v", c.text, st)
		}
	}
}

// sc with four operands is CMYK, so text filled through a DeviceCMYK colour
// space reports its colour.
func TestFoundTextStateCMYKFill(t *testing.T) {
	_, p := foundStatePage(t, "BT /F0 12 Tf 1 0 0 1 72 700 Tm /DeviceCMYK cs 0 1 1 0 sc /DeviceCMYK CS 1 0 1 0 SC (Red text) Tj ET\n")
	frs := foundFragments(t, p, "Red text")
	if len(frs) != 1 {
		t.Fatalf("%d fragments", len(frs))
	}
	if st := frs[0].TextState; !nearColor(st.ForegroundColor, 1, 0, 0) || !nearColor(st.StrokingColor, 0, 1, 0) {
		t.Errorf("fill %+v stroke %+v", st.ForegroundColor, st.StrokingColor)
	}
}

// Text made bold by filling and stroking a regular face reports fill and
// stroke, but a colour edit redraws it bold in the new colour, not stroked
// in the old stroking colour.
func TestFoundFakedBoldColourEdit(t *testing.T) {
	d, p := foundStatePage(t, "BT /F0 12 Tf 1 0 0 1 72 700 Tm 0 0 1 rg 0 0 1 RG 0.4 w 2 Tr (Heavy words) Tj ET\n")
	frs := foundFragments(t, p, "Heavy words")
	if len(frs) != 1 {
		t.Fatalf("%d fragments", len(frs))
	}
	st := frs[0].Segments[0].TextState
	if st.RenderingMode != RenderingFillStroke || st.FontStyle&FontStyleBold == 0 {
		t.Fatalf("faked bold state %+v", st)
	}
	frs[0].Segments[0].TextState.ForegroundColor = &Color{R: 1, A: 1}
	if err := d.ApplyTextChanges(); err != nil {
		t.Fatal(err)
	}
	got := foundFragments(t, mustPage(t, reopenDoc(t, d)), "Heavy words")
	if len(got) != 1 {
		t.Fatalf("%d fragments after the edit", len(got))
	}
	ns := got[0].Segments[0].TextState
	if !nearColor(ns.ForegroundColor, 1, 0, 0) || !nearColor(ns.StrokingColor, 1, 0, 0) || ns.FontStyle&FontStyleBold == 0 {
		t.Errorf("after the edit: fill %+v stroke %+v style %d", ns.ForegroundColor, ns.StrokingColor, ns.FontStyle)
	}
}
