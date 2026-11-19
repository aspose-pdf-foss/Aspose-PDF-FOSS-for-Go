// SPDX-License-Identifier: MIT

package asposepdf

import "testing"

// wordAt returns where word starts on p: the left edge of its match and the
// baseline of the line it is on (the match's descender is less than 4pt at
// 12pt).
func wordAt(t *testing.T, p *Page, word string) (x, y float64) {
	t.Helper()
	m, err := p.SearchText(word)
	if err != nil || len(m) != 1 {
		t.Fatalf("%q: %d matches, %v", word, len(m), err)
	}
	return m[0].Rect.LLX, m[0].Rect.LLY
}

func nearBaseline(lly, y float64) bool { return lly < y && lly > y-4 }

// A segment given a Position is drawn there; a segment after it without one
// follows on from its end.
func TestAppendTextSegmentPosition(t *testing.T) {
	d := NewDocumentFromFormat(PageFormatA4)
	p := mustPage(t, d)
	f := NewTextFragment("alpha")
	f.Position = Position{XIndent: 100, YIndent: 600}
	f.TextState = TextState{Font: FontCourier, FontSize: 12}
	moved := NewTextSegment("bravo")
	moved.Position = Position{XIndent: 300, YIndent: 400}
	f.Segments = append(f.Segments, moved, NewTextSegment("charlie"))
	if err := NewTextBuilder(p).AppendText(f); err != nil {
		t.Fatal(err)
	}
	got := reopenDoc(t, d)
	gp := mustPage(t, got)
	// Courier 12: 5 characters are 36pt wide.
	for _, w := range []struct {
		text string
		x, y float64
	}{{"alpha", 100, 600}, {"bravo", 300, 400}, {"charlie", 336, 400}} {
		if x, y := wordAt(t, gp, w.text); !nearPt(x, w.x) || !nearBaseline(y, w.y) {
			t.Errorf("%s at (%.1f, %.1f), want (%.0f, %.0f)", w.text, x, y, w.x, w.y)
		}
	}
	if s := f.Segments[1].Position; !nearPt(s.XIndent, 300) || !nearPt(s.YIndent, 400) {
		t.Errorf("recorded position of the moved segment %+v", s)
	}
}

// Moving a segment of a drawn fragment, or adding one with a Position, takes
// effect when the changes are applied; the recorded positions of the other
// segments are not mistaken for chosen ones.
func TestApplyDrawnSegmentPosition(t *testing.T) {
	d := NewDocumentFromFormat(PageFormatA4)
	p := mustPage(t, d)
	f := NewTextFragment("alpha")
	f.Position = Position{XIndent: 100, YIndent: 600}
	f.TextState = TextState{Font: FontCourier, FontSize: 12}
	f.Segments = append(f.Segments, NewTextSegment("bravo"), NewTextSegment("charlie"))
	if err := NewTextBuilder(p).AppendText(f); err != nil {
		t.Fatal(err)
	}
	f.Segments[1].Position = Position{XIndent: 50, YIndent: 300}
	extra := NewTextSegment("delta")
	extra.Position = Position{XIndent: 400, YIndent: 200}
	f.Segments = append(f.Segments, extra)
	got := reopenDoc(t, d)
	gp := mustPage(t, got)
	for _, w := range []struct {
		text string
		x, y float64
	}{{"alpha", 100, 600}, {"bravo", 50, 300}, {"charlie", 86, 300}, {"delta", 400, 200}} {
		if x, y := wordAt(t, gp, w.text); !nearPt(x, w.x) || !nearBaseline(y, w.y) {
			t.Errorf("%s at (%.1f, %.1f), want (%.0f, %.0f)", w.text, x, y, w.x, w.y)
		}
	}
}

// Moving the first segment of a found fragment moves it.
func TestApplyFoundSegmentPosition(t *testing.T) {
	d := openBytes(t, applyDocBytes(t))
	f := absorbOne(t, d, "second line")
	x0, y0 := f.Position.XIndent, f.Position.YIndent
	f.Segments[0].Position = Position{XIndent: x0 + 30, YIndent: y0 - 20}
	got := openBytes(t, writeBytes(t, d))
	g := absorbOne(t, got, "second line")
	if !nearPt(g.Position.XIndent, x0+30) || !nearPt(g.Position.YIndent, y0-20) {
		t.Errorf("at %+v, want (%.1f, %.1f)", g.Position, x0+30, y0-20)
	}
	if absorbCount(t, got, "second line") != 1 {
		t.Error("the old text stayed")
	}
}

// Moving one segment of a drawn fragment, and nothing else, is an edit too;
// the segment following on from it moves with it.
func TestApplyDrawnSegmentMoveOnly(t *testing.T) {
	d := NewDocumentFromFormat(PageFormatA4)
	p := mustPage(t, d)
	f := NewTextFragment("alpha")
	f.Position = Position{XIndent: 100, YIndent: 600}
	f.TextState = TextState{Font: FontCourier, FontSize: 12}
	f.Segments = append(f.Segments, NewTextSegment("bravo"), NewTextSegment("charlie"))
	if err := NewTextBuilder(p).AppendText(f); err != nil {
		t.Fatal(err)
	}
	f.Segments[1].Position = Position{XIndent: 250, YIndent: 450}
	gp := mustPage(t, reopenDoc(t, d))
	if x, y := wordAt(t, gp, "bravo"); !nearPt(x, 250) || !nearBaseline(y, 450) {
		t.Errorf("bravo at (%.1f, %.1f), want (250, 450)", x, y)
	}
	if x, y := wordAt(t, gp, "charlie"); !nearPt(x, 286) || !nearBaseline(y, 450) {
		t.Errorf("charlie at (%.1f, %.1f), want (286, 450)", x, y)
	}
}
