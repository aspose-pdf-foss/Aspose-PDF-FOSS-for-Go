// SPDX-License-Identifier: MIT

package asposepdf

import (
	"strings"
	"testing"
)

func absorberDoc(t *testing.T) (*Document, *Page) {
	t.Helper()
	d := NewDocumentFromFormat(PageFormatA4)
	p := mustPage(t, d)
	f := NewTextFragment("plain ")
	f.Position = Position{XIndent: 100, YIndent: 600}
	f.TextState = TextState{Font: FontHelvetica, FontSize: 12}
	b := NewTextSegment("bold")
	b.TextState = TextState{Font: FontHelveticaBold}
	c := NewTextSegment(" tail")
	c.TextState = TextState{Font: FontTimesRoman}
	f.Segments = []*TextSegment{{Text: "plain "}, b, c}
	if err := NewTextBuilder(p).AppendText(f); err != nil {
		t.Fatal(err)
	}
	return reopenDoc(t, d), nil
}

func TestAbsorberSegments(t *testing.T) {
	d, _ := absorberDoc(t)
	p := mustPage(t, d)
	a := NewTextFragmentAbsorber("plain bold tail")
	if err := a.Visit(p); err != nil {
		t.Fatal(err)
	}
	fs := a.TextFragments()
	if len(fs) != 1 || len(fs[0].Segments) != 3 {
		t.Fatalf("fragments %d", len(fs))
	}
	f := fs[0]
	if f.Text != "plain bold tail" || f.Segments[0].Text != "plain " || f.Segments[1].Text != "bold" || f.Segments[2].Text != " tail" {
		t.Fatalf("texts %q %q", f.Text, f.Segments[1].Text)
	}
	if got := f.Segments[1].TextState.Font.BaseFont(); got != "Helvetica-Bold" {
		t.Fatalf("font %q", got)
	}
	if f.Segments[1].TextState.FontStyle&FontStyleBold == 0 {
		t.Fatal("bold bit")
	}
	if f.TextState.Font != f.Segments[0].TextState.Font || f.Position != f.Segments[0].Position {
		t.Fatal("fragment state is not the first segment's")
	}
	if f.Position.XIndent < 99.9 || f.Position.XIndent > 100.1 || f.Position.YIndent < 599.9 || f.Position.YIndent > 600.1 {
		t.Fatalf("position %+v", f.Position)
	}
	if f.X != f.Position.XIndent || f.Y != f.Position.YIndent || f.FontName == "" || f.Width <= 0 {
		t.Fatalf("value fields %+v", f)
	}
	if f.Segments[2].Rectangle.LLX < f.Segments[1].Rectangle.URX-0.01 {
		t.Fatal("segment boxes overlap")
	}
	if !f.bind.snap.equal(snapshotOf(f)) {
		t.Fatal("untouched fragment differs from snapshot")
	}
	if len(d.boundFragments) != 1 || len(f.bind.drawn) != 3 || f.bind.noEdit != "" {
		t.Fatal("binding")
	}
}

func TestAbsorberAllText(t *testing.T) {
	d := NewDocumentFromFormat(PageFormatA4)
	p := mustPage(t, d)
	for i, s := range []string{"first line", "second line"} {
		f := NewTextFragment(s)
		f.Position = Position{XIndent: 100, YIndent: 700 - float64(i)*40}
		if err := NewTextBuilder(p).AppendText(f); err != nil {
			t.Fatal(err)
		}
	}
	a := NewTextFragmentAbsorber("")
	if err := a.VisitDocument(reopenDoc(t, d)); err != nil {
		t.Fatal(err)
	}
	fs := a.TextFragments()
	if len(fs) != 2 || fs[0].Text != "first line" || fs[1].Text != "second line" {
		t.Fatalf("got %d", len(fs))
	}
}

func TestAbsorberRegexAcrossLines(t *testing.T) {
	d := NewDocumentFromFormat(PageFormatA4)
	p := mustPage(t, d)
	for i, s := range []string{"alpha beta", "gamma delta"} {
		f := NewTextFragment(s)
		f.Position = Position{XIndent: 100, YIndent: 700 - float64(i)*40}
		if err := NewTextBuilder(p).AppendText(f); err != nil {
			t.Fatal(err)
		}
	}
	a := NewTextFragmentAbsorber(`beta\s+gamma`, SearchOptions{Regex: true})
	if err := a.Visit(mustPage(t, reopenDoc(t, d))); err != nil {
		t.Fatal(err)
	}
	fs := a.TextFragments()
	if len(fs) != 1 || len(fs[0].Segments) != 2 || fs[0].Segments[0].Text != "beta" || fs[0].Segments[1].Text != "gamma" {
		t.Fatalf("got %d fragments", len(fs))
	}
	if fs[0].Segments[0].Position.YIndent <= fs[0].Segments[1].Position.YIndent {
		t.Fatal("lines out of order")
	}
	// literal search never matches across lines
	b := NewTextFragmentAbsorber("beta gamma")
	if err := b.Visit(mustPage(t, reopenDoc(t, d))); err != nil || len(b.TextFragments()) != 0 {
		t.Fatal("literal matched across lines")
	}
}

func TestAbsorberColors(t *testing.T) {
	d := NewDocumentFromFormat(PageFormatA4)
	p := mustPage(t, d)
	if err := p.DrawRectangle(Rectangle{LLX: 90, LLY: 590, URX: 400, URY: 620},
		ShapeStyle{FillColor: &Color{R: 1, G: 1, A: 1}}); err != nil {
		t.Fatal(err)
	}
	f := NewTextFragment("red ")
	f.Position = Position{XIndent: 100, YIndent: 600}
	f.TextState = TextState{ForegroundColor: &Color{R: 1, A: 1}}
	g := NewTextSegment("blue")
	g.TextState = TextState{ForegroundColor: &Color{B: 1, A: 1}}
	f.Segments = []*TextSegment{{Text: "red "}, g}
	if err := NewTextBuilder(p).AppendText(f); err != nil {
		t.Fatal(err)
	}
	r := reopenDoc(t, d)
	a := NewTextFragmentAbsorber("red blue", SearchOptions{SearchForTextRelatedGraphics: true})
	if err := a.Visit(mustPage(t, r)); err != nil {
		t.Fatal(err)
	}
	fs := a.TextFragments()
	if len(fs) != 1 || len(fs[0].Segments) != 2 || fs[0].Text != "red blue" {
		t.Fatalf("got %d / %v", len(fs), fs)
	}
	c0, c1 := fs[0].Segments[0].TextState.ForegroundColor, fs[0].Segments[1].TextState.ForegroundColor
	if c0 == nil || c1 == nil || c0.R != 1 || c0.B != 0 || c1.B != 1 || c1.R != 0 {
		t.Fatalf("colours %v %v", c0, c1)
	}
	bg := fs[0].Segments[0].TextState.BackgroundColor
	if bg == nil || bg.R != 1 || bg.G != 1 || bg.B != 0 {
		t.Fatalf("background %v", bg)
	}
	// without the option no background
	b := NewTextFragmentAbsorber("red blue")
	_ = b.Visit(mustPage(t, reopenDoc(t, d)))
	if b.TextFragments()[0].Segments[0].TextState.BackgroundColor != nil {
		t.Fatal("background without option")
	}
}

func TestAbsorberRotatedNotEditable(t *testing.T) {
	d := NewDocumentFromFormat(PageFormatA4)
	p := mustPage(t, d)
	f := NewTextFragment("tilted")
	f.Position = Position{XIndent: 200, YIndent: 400}
	f.TextState = TextState{Rotation: 45}
	if err := NewTextBuilder(p).AppendText(f); err != nil {
		t.Fatal(err)
	}
	a := NewTextFragmentAbsorber("tilted")
	if err := a.Visit(mustPage(t, reopenDoc(t, d))); err != nil {
		t.Fatal(err)
	}
	if len(a.TextFragments()) != 1 || a.TextFragments()[0].bind.noEdit == "" {
		t.Fatal("rotated match not flagged")
	}
}

func TestAbsorberRectangleAndInvalid(t *testing.T) {
	d, _ := absorberDoc(t)
	far := Rectangle{LLX: 0, LLY: 0, URX: 50, URY: 50}
	a := NewTextFragmentAbsorber("plain", SearchOptions{Rectangle: &far})
	_ = a.Visit(mustPage(t, d))
	if len(a.TextFragments()) != 0 {
		t.Fatal("rectangle ignored")
	}
	if err := NewTextFragmentAbsorber("(", SearchOptions{Regex: true}).Visit(mustPage(t, d)); err == nil {
		t.Fatal("expected error")
	}
}

func TestExtractionSplitsOnColour(t *testing.T) {
	d := NewDocumentFromFormat(PageFormatA4)
	p := mustPage(t, d)
	f := NewTextFragment("ab")
	f.Position = Position{XIndent: 100, YIndent: 600}
	g := NewTextSegment("cd")
	g.TextState = TextState{ForegroundColor: &Color{R: 1, A: 1}}
	f.Segments = []*TextSegment{{Text: "ab"}, g}
	if err := NewTextBuilder(p).AppendText(f); err != nil {
		t.Fatal(err)
	}
	lines, _ := mustPage(t, reopenDoc(t, d)).ExtractTextWithLayout()
	if len(lines) != 1 || lines[0].Text != "abcd" || len(lines[0].Fragments) != 2 {
		t.Fatalf("%q %d", lines[0].Text, len(lines[0].Fragments))
	}
}

func TestBindFragmentOnce(t *testing.T) {
	d := NewDocumentFromFormat(PageFormatA4)
	f := &TextFragment{}
	d.bindFragment(f)
	d.bindFragment(f)
	if len(d.boundFragments) != 1 {
		t.Fatal("bound twice")
	}
}

func TestAbsorberWidthIsFirstLine(t *testing.T) {
	d, _ := absorberDoc(t)
	a := NewTextFragmentAbsorber("plain bold tail")
	if err := a.Visit(mustPage(t, d)); err != nil {
		t.Fatal(err)
	}
	f := a.TextFragments()[0]
	want := f.Segments[2].Rectangle.URX - f.Segments[0].Rectangle.LLX
	if f.Width < want-0.5 || f.Width > want+0.5 {
		t.Fatalf("width %v want %v", f.Width, want)
	}
	if f.Width < f.Segments[0].Rectangle.URX-f.Segments[0].Rectangle.LLX+10 {
		t.Fatal("width is the first segment's")
	}
}

func TestAbsorberAcrossPages(t *testing.T) {
	d := NewDocumentFromFormat(PageFormatA4)
	if err := d.AddBlankPageFromFormat(PageFormatA4); err != nil {
		t.Fatal(err)
	}
	for n, s := range []string{"alpha", "beta"} {
		p, _ := d.Page(n + 1)
		if n == 1 {
			if err := p.DrawRectangle(Rectangle{LLX: 90, LLY: 690, URX: 300, URY: 720},
				ShapeStyle{FillColor: &Color{G: 1, A: 1}}); err != nil {
				t.Fatal(err)
			}
		}
		f := NewTextFragment(s)
		f.Position = Position{XIndent: 100, YIndent: 700}
		if err := NewTextBuilder(p).AppendText(f); err != nil {
			t.Fatal(err)
		}
	}
	a := NewTextFragmentAbsorber(`alpha\s+beta`, SearchOptions{Regex: true, SearchForTextRelatedGraphics: true})
	if err := a.VisitDocument(reopenDoc(t, d)); err != nil {
		t.Fatal(err)
	}
	fs := a.TextFragments()
	if len(fs) != 1 || len(fs[0].Segments) != 2 {
		t.Fatalf("got %d", len(fs))
	}
	if fs[0].bind.noEdit == "" {
		t.Fatal("cross-page match editable")
	}
	if fs[0].Segments[0].TextState.BackgroundColor != nil {
		t.Fatal("page 1 has no fill")
	}
	if bg := fs[0].Segments[1].TextState.BackgroundColor; bg == nil || bg.G != 1 {
		t.Fatalf("page 2 background %v", bg)
	}
}

func TestAbsorberEmbeddedFlag(t *testing.T) {
	d := NewDocumentFromFormat(PageFormatA4)
	p := mustPage(t, d)
	dj, err := d.LoadFont("testdata/DejaVuSans.ttf")
	if err != nil {
		t.Fatal(err)
	}
	for i, fnt := range []Font{FontHelvetica, dj} {
		f := NewTextFragment("word")
		f.Position = Position{XIndent: 100, YIndent: 700 - float64(i)*40}
		f.TextState = TextState{Font: fnt}
		if err := NewTextBuilder(p).AppendText(f); err != nil {
			t.Fatal(err)
		}
	}
	a := NewTextFragmentAbsorber("word")
	if err := a.Visit(mustPage(t, reopenDoc(t, d))); err != nil {
		t.Fatal(err)
	}
	fs := a.TextFragments()
	if len(fs) != 2 || fs[0].TextState.Font.IsEmbedded() || !fs[1].TextState.Font.IsEmbedded() {
		t.Fatalf("embedded flags wrong (%d fragments)", len(fs))
	}
}

func TestAbsorberRTLPosition(t *testing.T) {
	d := NewDocumentFromFormat(PageFormatA4)
	p := mustPage(t, d)
	dj, err := d.LoadFont("testdata/DejaVuSans.ttf")
	if err != nil {
		t.Fatal(err)
	}
	f := NewTextFragment("שלום עולם")
	f.Position = Position{XIndent: 100, YIndent: 600}
	f.TextState = TextState{Font: dj, FontSize: 20}
	if err := NewTextBuilder(p).AppendText(f); err != nil {
		t.Fatal(err)
	}
	a := NewTextFragmentAbsorber("שלום")
	if err := a.Visit(mustPage(t, reopenDoc(t, d))); err != nil {
		t.Fatal(err)
	}
	fs := a.TextFragments()
	if len(fs) != 1 {
		t.Fatalf("got %d", len(fs))
	}
	r := fs[0].Segments[0].Rectangle
	if r.URX-r.LLX < 20 {
		t.Fatalf("rect %+v", r)
	}
	if fs[0].Position.XIndent != r.LLX || fs[0].Segments[0].Position.XIndent != r.LLX || fs[0].X != r.LLX {
		t.Fatalf("position %+v vs left edge %v", fs[0].Position, r.LLX)
	}
}

// A colour change inside a line must not change the line text or what
// SearchText reports, whether the boundary is a space, a hairline gap or none.
func TestColourBoundaryKeepsTextAndSearch(t *testing.T) {
	build := func(coloured bool) *Page {
		d := NewDocumentFromFormat(PageFormatA4)
		p := mustPage(t, d)
		col := func(c *Color) *Color {
			if coloured {
				return c
			}
			return nil
		}
		red, blue := col(&Color{R: 1, A: 1}), col(&Color{B: 1, A: 1})
		x := 100.0
		// word, space in the other colour, word
		for _, pc := range []struct {
			s string
			c *Color
		}{{"ab", red}, {" ", blue}, {"cd", red}} {
			if err := p.AddTextAt(pc.s, TextStyle{Size: 12, Color: pc.c}, x, 700); err != nil {
				t.Fatal(err)
			}
			wd, _ := MeasureText(pc.s, TextStyle{Size: 12})
			x += wd
		}
		// small positive gap between differently coloured pieces
		x = 100
		for _, pc := range []struct {
			s   string
			c   *Color
			gap float64
		}{{"ef", red, 0}, {"gh", blue, 0.6}} {
			x += pc.gap
			if err := p.AddTextAt(pc.s, TextStyle{Size: 12, Color: pc.c}, x, 650); err != nil {
				t.Fatal(err)
			}
			wd, _ := MeasureText(pc.s, TextStyle{Size: 12})
			x += wd
		}
		return mustPage(t, reopenDoc(t, d))
	}
	base, col := build(false), build(true)
	lb, _ := base.ExtractTextWithLayout()
	lc, _ := col.ExtractTextWithLayout()
	if len(lb) != len(lc) {
		t.Fatalf("lines %d vs %d", len(lb), len(lc))
	}
	split := false
	for i := range lb {
		if lb[i].Text != lc[i].Text {
			t.Fatalf("line %q vs %q", lb[i].Text, lc[i].Text)
		}
		if len(lc[i].Fragments) > len(lb[i].Fragments) {
			split = true
		}
	}
	if !split {
		t.Fatal("colour did not split any fragment")
	}
	for _, q := range []string{"ab cd", "b c", "cd", "efgh", "fg"} {
		mb, _ := base.SearchText(q)
		mc, _ := col.SearchText(q)
		if len(mb) != len(mc) {
			t.Fatalf("%q: %d vs %d matches", q, len(mb), len(mc))
		}
		for i := range mb {
			if mb[i].Rect != mc[i].Rect {
				t.Fatalf("%q rect %+v vs %+v", q, mb[i].Rect, mc[i].Rect)
			}
		}
	}
}

// Text set with a negative size and a negative horizontal scale reads upright and reports a negative effective size; the found
// state carries the size as a magnitude, so the fragment can be edited.
func TestAbsorberNegativeSizeEditable(t *testing.T) {
	d := NewDocumentFromFormat(PageFormatA4)
	p := mustPage(t, d)
	if _, err := p.ensureStandardFontResource("/Helvetica"); err != nil {
		t.Fatal(err)
	}
	if err := p.appendToContentStream([]byte("BT /F0 -12 Tf -100 Tz 1 0 0 1 100 200 Tm (Flipped) Tj ET\n")); err != nil {
		t.Fatal(err)
	}
	d = reopenDoc(t, d)
	p = mustPage(t, d)
	a := NewTextFragmentAbsorber("Flipped")
	if err := a.Visit(p); err != nil {
		t.Fatal(err)
	}
	fr := a.TextFragments()
	if len(fr) != 1 {
		t.Fatalf("found %d fragments", len(fr))
	}
	if got := fr[0].Segments[0].TextState.FontSize; got != 12 {
		t.Fatalf("segment size %v, want 12", got)
	}
	fr[0].Text = "Edited"
	if err := d.ApplyTextChanges(); err != nil {
		t.Fatal(err)
	}
	txt, _ := p.ExtractText()
	if !strings.Contains(txt, "Edited") || strings.Contains(txt, "Flipped") {
		t.Fatalf("extracted %q", txt)
	}
}
