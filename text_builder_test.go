// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"math"
	"strings"
	"testing"
)

func reopenDoc(t *testing.T, d *Document) *Document {
	t.Helper()
	var buf bytes.Buffer
	if _, err := d.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	r, err := OpenStream(&buf)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func mustPage(t *testing.T, d *Document) *Page {
	t.Helper()
	p, err := d.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func findFrag(t *testing.T, p *Page, word string) TextFragment {
	t.Helper()
	lines, err := p.ExtractTextWithLayout()
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range lines {
		for _, f := range l.Fragments {
			if strings.Contains(f.Text, word) {
				return f
			}
		}
	}
	t.Fatalf("no fragment with %q", word)
	return TextFragment{}
}

func buildSegmentsDoc(t *testing.T) (*Document, *TextFragment) {
	t.Helper()
	d := NewDocumentFromFormat(PageFormatA4)
	p := mustPage(t, d)
	f := NewTextFragment("segment1")
	f.Position = Position{XIndent: 100, YIndent: 600}
	f.TextState = TextState{Font: FontTimesRoman, FontSize: 5,
		BackgroundColor: &Color{R: 0.5, G: 0.5, B: 0.5, A: 1}, ForegroundColor: &Color{R: 1, A: 1}}
	s2 := NewTextSegment("segment2")
	s2.TextState = TextState{ForegroundColor: &Color{B: 1, A: 1}}
	s3 := NewTextSegment("segment3")
	s3.TextState = TextState{Font: FontCourier, FontSize: 12}
	f.Segments = append(f.Segments, s2, s3)
	if err := NewTextBuilder(p).AppendText(f); err != nil {
		t.Fatal(err)
	}
	return d, f
}

func TestAppendTextSegments(t *testing.T) {
	d, f := buildSegmentsDoc(t)
	if f.Text != "segment1segment2segment3" {
		t.Fatalf("Text = %q", f.Text)
	}
	p := mustPage(t, reopenDoc(t, d))
	get := func(q string) TextMatch {
		m, err := p.SearchText(q, SearchOptions{SearchForTextRelatedGraphics: true})
		if err != nil || len(m) != 1 {
			t.Fatalf("%q: %v %v", q, m, err)
		}
		return m[0]
	}
	m1, m2, m3 := get("segment1"), get("segment2"), get("segment3")
	if math.Abs(m2.Rect.LLX-m1.Rect.URX) > 0.6 {
		t.Fatalf("segment2 not right after segment1: %v vs %v", m2.Rect, m1.Rect)
	}
	if f2 := findFrag(t, p, "segment2"); !strings.Contains(f2.FontName, "Times") || f2.FontSize != 5 {
		t.Fatalf("segment2 frag %+v", f2)
	}
	if f3 := findFrag(t, p, "segment3"); !strings.Contains(f3.FontName, "Courier") || f3.FontSize != 12 {
		t.Fatalf("segment3 frag %+v", f3)
	}
	// colour: segment2 paints blue pixels inside its box, segment1 none
	img, err := p.RenderImage(RenderOptions{DPI: 144})
	if err != nil {
		t.Fatal(err)
	}
	blueIn := func(r Rectangle) bool {
		ph := 842.0
		for y := int((ph - r.URY) * 2); y <= int((ph-r.LLY)*2); y++ {
			for x := int(r.LLX * 2); x <= int(r.URX*2); x++ {
				cr, cg, cb, _ := img.At(x, y).RGBA()
				if cb > 0xC000 && cr < 0x4000 && cg < 0x4000 {
					return true
				}
			}
		}
		return false
	}
	if !blueIn(m2.Rect) || blueIn(m1.Rect) {
		t.Fatal("segment2 should be blue and segment1 not")
	}
	if m1.BackgroundColor == nil || m1.BackgroundColor.R != 0.5 {
		t.Fatalf("background = %v", m1.BackgroundColor)
	}
	if m3.BackgroundColor == nil {
		t.Fatal("segment3 lost inherited background")
	}
	if f.Segments[1].Rectangle.LLX <= f.Segments[0].Rectangle.LLX || f.Segments[0].Position.YIndent != 600 {
		t.Fatalf("segment positions %+v %+v", f.Segments[0], f.Segments[1])
	}
	if len(d.boundFragments) != 1 || f.bind == nil || len(f.bind.drawn) != 3 {
		t.Fatal("not bound")
	}
}

func TestAppendTextDefaultPosition(t *testing.T) {
	d := NewDocumentFromFormat(PageFormatA4)
	p := mustPage(t, d)
	f := NewTextFragment("Hello")
	f.TextState = TextState{Font: FontHelvetica, FontSize: 10}
	if err := NewTextBuilder(p).AppendText(f); err != nil {
		t.Fatal(err)
	}
	crop, _ := p.CropBox()
	asc, _, _ := textBox(FontHelvetica, 10)
	if f.Position.XIndent != crop.LLX+72 || math.Abs(f.Position.YIndent-(crop.URY-72-asc)) > 1e-9 {
		t.Fatalf("position %+v", f.Position)
	}
	fr := findFrag(t, mustPage(t, reopenDoc(t, d)), "Hello")
	if math.Abs(fr.X-(crop.LLX+72)) > 0.5 || math.Abs(fr.Y-f.Position.YIndent) > 0.5 {
		t.Fatalf("drawn at %v,%v", fr.X, fr.Y)
	}
}

func TestAppendTextHyperlink(t *testing.T) {
	d := NewDocumentFromFormat(PageFormatA4)
	p := mustPage(t, d)
	f := NewTextFragment("plain ")
	f.Position = Position{XIndent: 50, YIndent: 700}
	s := NewTextSegment("click")
	s.Hyperlink = "https://example.com"
	f.Segments = append(f.Segments, s)
	if err := NewTextBuilder(p).AppendText(f); err != nil {
		t.Fatal(err)
	}
	p2 := mustPage(t, reopenDoc(t, d))
	all := p2.Annotations().All()
	if len(all) != 1 {
		t.Fatalf("%d annotations", len(all))
	}
	l, ok := all[0].(*LinkAnnotation)
	if !ok {
		t.Fatalf("type %T", all[0])
	}
	if a, ok := l.Action().(*GoToURIAction); !ok || a.URI() != "https://example.com" {
		t.Fatalf("action %v", l.Action())
	}
	m, _ := p2.SearchText("click")
	r, lr := m[0].Rect, l.Rect()
	if lr.LLX > r.LLX+0.6 || lr.URX < r.URX-0.6 || lr.LLY > r.LLY+0.6 || lr.URY < r.URY-0.6 {
		t.Fatalf("link %v does not cover %v", lr, r)
	}
}

func TestNewTextFragmentText(t *testing.T) {
	f := NewTextFragment("x")
	if len(f.Segments) != 1 || f.Segments[0].Text != "x" || f.Text != "x" {
		t.Fatalf("%+v", f)
	}
}

func TestSegmentsToDrawInherit(t *testing.T) {
	f := &TextFragment{Text: "plain", TextState: TextState{FontSize: 7, Underline: true}}
	got := f.segmentsToDraw()
	if len(got) != 1 || got[0].Text != "plain" || got[0].TextState.FontSize != 7 || !got[0].TextState.Underline {
		t.Fatalf("%+v", got)
	}
	f.Segments = []*TextSegment{{Text: "a", TextState: TextState{FontSize: 9}}, {Text: "b"}}
	f.TextState.FontSize = 8
	got = f.segmentsToDraw()
	if got[0].TextState.FontSize != 9 || got[1].TextState.FontSize != 8 {
		t.Fatalf("%+v %+v", got[0], got[1])
	}
	if f.Segments[1].TextState.FontSize != 0 {
		t.Fatal("source segment mutated")
	}
}

func TestAppendTextRotation(t *testing.T) {
	d := NewDocumentFromFormat(PageFormatA4)
	p := mustPage(t, d)
	f := NewTextFragment("a")
	f.TextState.Rotation = 90
	f.Segments = append(f.Segments, NewTextSegment("b"))
	if err := NewTextBuilder(p).AppendText(f); err == nil {
		t.Fatal("rotated multi-segment accepted")
	}
	g := NewTextFragment("rot")
	g.TextState.Rotation = 90
	g.TextState.BackgroundColor = &Color{R: 1, A: 1}
	if err := NewTextBuilder(p).AppendText(g); err != nil {
		t.Fatalf("single rotated: %v", err)
	}
	// a rotated segment's background is not painted as an upright box
	img, err := p.RenderImage(RenderOptions{DPI: 72})
	if err != nil {
		t.Fatal(err)
	}
	r := g.Segments[0].Rectangle
	cx, cy := int((r.LLX+r.URX)/2), int(841.89-(r.LLY+r.URY)/2)
	if cr, cg, _, _ := img.At(cx, cy).RGBA(); cr > 0xC000 && cg < 0x4000 {
		t.Fatal("rotated segment painted an upright background box")
	}
	if err := NewTextBuilder(p).AppendText(nil); err == nil {
		t.Fatal("nil fragment accepted")
	}
	if err := NewTextBuilder(nil).AppendText(g); err == nil {
		t.Fatal("nil builder accepted")
	}
}

func TestFragmentSnapshot(t *testing.T) {
	f := NewTextFragment("abc")
	c := &Color{R: 1, A: 1}
	f.TextState = TextState{Font: FontTimesRoman, FontSize: 9, ForegroundColor: c}
	a := snapshotOf(f)
	if !a.equal(snapshotOf(f)) {
		t.Fatal("same fragment differs")
	}
	c.R = 0.5 // colour compared by value: the snapshot kept a copy
	if a.equal(snapshotOf(f)) {
		t.Fatal("colour change missed")
	}
	mut := func(name string, fn func(*TextFragment)) {
		g := NewTextFragment("abc")
		g.TextState = TextState{Font: FontTimesRoman, FontSize: 9, ForegroundColor: &Color{R: 1, A: 1}}
		base := snapshotOf(g)
		fn(g)
		if base.equal(snapshotOf(g)) {
			t.Errorf("%s change missed", name)
		}
	}
	mut("text", func(g *TextFragment) { g.Text = "x" })
	mut("position", func(g *TextFragment) { g.Position.XIndent = 3 })
	mut("font", func(g *TextFragment) { g.TextState.Font = FontCourier })
	mut("size", func(g *TextFragment) { g.TextState.FontSize = 10 })
	mut("style", func(g *TextFragment) { g.TextState.FontStyle = FontStyleBold })
	mut("bg", func(g *TextFragment) { g.TextState.BackgroundColor = &Color{G: 1, A: 1} })
	mut("stroke", func(g *TextFragment) { g.TextState.StrokingColor = &Color{G: 1, A: 1} })
	mut("mode", func(g *TextFragment) { g.TextState.RenderingMode = RenderingStroke })
	mut("underline", func(g *TextFragment) { g.TextState.Underline = true })
	mut("strike", func(g *TextFragment) { g.TextState.StrikeOut = true })
	mut("invisible", func(g *TextFragment) { g.TextState.Invisible = true })
	mut("tc", func(g *TextFragment) { g.TextState.CharacterSpacing = 1 })
	mut("tw", func(g *TextFragment) { g.TextState.WordSpacing = 1 })
	mut("tz", func(g *TextFragment) { g.TextState.HorizontalScaling = 50 })
	mut("leading", func(g *TextFragment) { g.TextState.LineSpacing = 4 })
	mut("rot", func(g *TextFragment) { g.TextState.Rotation = 5 })
	mut("link", func(g *TextFragment) { g.Segments[0].Hyperlink = "u" })
	mut("segtext", func(g *TextFragment) { g.Segments[0].Text = "q" })
	mut("segcount", func(g *TextFragment) { g.Segments = append(g.Segments, NewTextSegment("z")) })
}
