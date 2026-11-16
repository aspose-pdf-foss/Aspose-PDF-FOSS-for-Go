// SPDX-License-Identifier: MIT

package asposepdf

import (
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
