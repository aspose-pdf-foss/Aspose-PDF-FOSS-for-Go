// SPDX-License-Identifier: MIT

package asposepdf

import (
	"math"
	"sort"
	"strings"
	"testing"
)

const paraWords = "alpha bravo charlie delta echo foxtrot golf hotel india juliet kilo lima mike november oscar papa quebec romeo sierra tango"

func paraRect() Rectangle { return Rectangle{LLX: 100, LLY: 300, URX: 200, URY: 700} }

func buildPara(t *testing.T, tp *TextParagraph) (*Document, *Page) {
	t.Helper()
	d := NewDocumentFromFormat(PageFormatA4)
	p := mustPage(t, d)
	if tp.Rectangle == (Rectangle{}) {
		tp.Rectangle = paraRect()
	}
	if err := NewTextBuilder(p).AppendParagraph(tp); err != nil {
		t.Fatal(err)
	}
	return d, p
}

// paraLines groups the page's text into lines: baseline, left and right edge.
type pLine struct{ y, x0, x1 float64 }

func paraLines(t *testing.T, p *Page) []pLine {
	t.Helper()
	tl, err := p.ExtractTextWithLayout()
	if err != nil {
		t.Fatal(err)
	}
	var out []pLine
	for _, l := range tl {
		pl := pLine{y: l.Fragments[0].Y, x0: math.Inf(1), x1: math.Inf(-1)}
		for _, f := range l.Fragments {
			pl.x0 = math.Min(pl.x0, f.X)
			pl.x1 = math.Max(pl.x1, f.X+f.Width)
		}
		out = append(out, pl)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].y > out[j].y })
	return out
}

func pnear(a, b, tol float64) bool { return math.Abs(a-b) <= tol }

func oneLine(text string) *TextParagraph {
	tp := NewTextParagraph()
	tp.AppendLine(NewTextFragment(text))
	return tp
}

func TestAppendParagraphWrap(t *testing.T) {
	_, p := buildPara(t, oneLine(paraWords))
	lines := paraLines(t, p)
	if len(lines) < 4 {
		t.Fatalf("want several lines, got %d", len(lines))
	}
	m, err := GetFontMetrics(FontHelvetica, 12)
	if err != nil {
		t.Fatal(err)
	}
	if want := 700 - m.Ascent; !pnear(lines[0].y, want, 0.01) {
		t.Fatalf("first baseline %v, want %v", lines[0].y, want)
	}
	for i, l := range lines {
		if l.x1 > 200.01 || l.x0 < 99.99 {
			t.Fatalf("line %d spans %v..%v", i, l.x0, l.x1)
		}
		if i > 0 && !pnear(lines[i-1].y-l.y, 14.4, 0.01) {
			t.Fatalf("line pitch %v", lines[i-1].y-l.y)
		}
	}
	text, _ := p.ExtractText()
	if strings.Join(strings.Fields(text), " ") != paraWords {
		t.Fatalf("text lost or reordered: %q", text)
	}
}

func TestAppendParagraphMixedRuns(t *testing.T) {
	f := NewTextFragment("the")
	f.Segments[0].TextState = TextState{Font: FontTimesRoman, FontSize: 20}
	f.Segments = append(f.Segments, NewTextSegment(" text is justified 1 2 3"))
	f.Segments[1].TextState = TextState{FontSize: 12}
	tp := NewTextParagraph()
	tp.AppendLine(f)
	_, p := buildPara(t, tp)
	text, _ := p.ExtractText()
	text = strings.Join(strings.Fields(text), " ")
	last := -1
	for _, w := range []string{"the", "text", "is", "justified", "1", "2", "3"} {
		i := strings.Index(text[last+1:], w)
		if i < 0 {
			t.Fatalf("%q not found in order in %q", w, text)
		}
		last += 1 + i
	}
	if len(paraLines(t, p)) < 2 {
		t.Fatal("expected wrapping")
	}
	// the 20pt word is bigger than the 12pt ones on its line
	if the := findFrag(t, p, "the"); the.FontSize != 20 {
		t.Fatalf("size of the first run %v", the.FontSize)
	}
	if f.Segments[0].Rectangle == (Rectangle{}) || f.Segments[1].Rectangle == (Rectangle{}) {
		t.Fatal("segment rectangles not recorded")
	}
}

func TestAppendParagraphJustify(t *testing.T) {
	for _, mode := range []string{"flag", "alignment"} {
		tp := oneLine(paraWords)
		if mode == "flag" {
			tp.Justify = true
		} else {
			tp.HorizontalAlignment = HorizontalAlignmentJustify
		}
		_, p := buildPara(t, tp)
		lines := paraLines(t, p)
		for i, l := range lines[:len(lines)-1] {
			if !pnear(l.x0, 100, 0.5) || !pnear(l.x1, 200, 0.5) {
				t.Fatalf("%s: line %d spans %v..%v", mode, i, l.x0, l.x1)
			}
		}
		if lastL := lines[len(lines)-1]; !pnear(lastL.x0, 100, 0.01) || lastL.x1 > 199 {
			t.Fatalf("%s: last line must stay ragged left: %v..%v", mode, lastL.x0, lastL.x1)
		}
	}
}

func TestAppendParagraphFullJustify(t *testing.T) {
	tp := oneLine(paraWords + " uniform victor whiskey")
	tp.HorizontalAlignment = HorizontalAlignmentFullJustify
	_, p := buildPara(t, tp)
	lines := paraLines(t, p)
	last := lines[len(lines)-1]
	if !pnear(last.x0, 100, 0.5) || !pnear(last.x1, 200, 0.5) {
		t.Fatalf("last line spans %v..%v", last.x0, last.x1)
	}
}

func TestAppendParagraphJustifyEachAppendedLine(t *testing.T) {
	tp := NewTextParagraph()
	tp.Justify = true
	tp.AppendLine(NewTextFragment("one two three four five six seven eight nine ten eleven"))
	tp.AppendLine(NewTextFragment("short end"))
	_, p := buildPara(t, tp)
	lines := paraLines(t, p)
	// the last line of each appended line is not stretched
	ragged := 0
	for _, l := range lines {
		if l.x1 < 199 {
			ragged++
		}
	}
	if ragged != 2 {
		t.Fatalf("%d ragged lines, want 2: %+v", ragged, lines)
	}
}

func TestAppendParagraphAlign(t *testing.T) {
	for _, tc := range []struct {
		a    HorizontalAlignment
		edge func(l pLine) float64
		want float64
	}{
		{HorizontalAlignmentRight, func(l pLine) float64 { return l.x1 }, 200},
		{HorizontalAlignmentLeft, func(l pLine) float64 { return l.x0 }, 100},
		{HorizontalAlignmentNone, func(l pLine) float64 { return l.x0 }, 100},
		{HorizontalAlignmentCenter, func(l pLine) float64 { return (l.x0 + l.x1) / 2 }, 150},
	} {
		tp := oneLine(paraWords)
		tp.HorizontalAlignment = tc.a
		_, p := buildPara(t, tp)
		for i, l := range paraLines(t, p) {
			if !pnear(tc.edge(l), tc.want, 0.5) {
				t.Fatalf("align %v line %d edge %v want %v", tc.a, i, tc.edge(l), tc.want)
			}
		}
	}
}

func TestAppendParagraphVertical(t *testing.T) {
	m, _ := GetFontMetrics(FontHelvetica, 12)
	for _, tc := range []struct {
		v      VAlign
		topGap bool
	}{{VAlignTop, true}, {VAlignMiddle, false}, {VAlignBottom, false}} {
		tp := oneLine("alpha bravo charlie delta echo foxtrot golf hotel")
		tp.VerticalAlignment = tc.v
		_, p := buildPara(t, tp)
		lines := paraLines(t, p)
		first, last := lines[0], lines[len(lines)-1]
		total := 14.4 * float64(len(lines))
		switch tc.v {
		case VAlignTop:
			if !pnear(first.y, 700-m.Ascent, 0.01) {
				t.Fatalf("top: first baseline %v", first.y)
			}
		case VAlignBottom:
			if bottom := last.y - 0.2*12; bottom < 300 || bottom > 300+14.4 {
				t.Fatalf("bottom: last baseline %v", last.y)
			}
		case VAlignMiddle:
			mid := (first.y + m.Ascent + last.y - (14.4 - m.Ascent)) / 2
			if !pnear(mid, 500, 0.01) || total >= 400 {
				t.Fatalf("middle: block centre %v", mid)
			}
		}
	}
}

func TestAppendParagraphIndents(t *testing.T) {
	tp := oneLine(paraWords)
	tp.FirstLineIndent = 20
	tp.SubsequentLinesIndent = 10
	_, p := buildPara(t, tp)
	lines := paraLines(t, p)
	for i, l := range lines {
		want := 110.0
		if i == 0 {
			want = 120
		}
		if !pnear(l.x0, want, 0.01) || l.x1 > 200.01 {
			t.Fatalf("line %d spans %v..%v, want start %v", i, l.x0, l.x1, want)
		}
	}
	// first line is narrower: its text must fit 80pt
	if lines[0].x1-lines[0].x0 > 80.01 {
		t.Fatalf("first line too wide: %v", lines[0].x1-lines[0].x0)
	}
}

func TestAppendParagraphIndentJustifyRight(t *testing.T) {
	tp := oneLine(paraWords)
	tp.FirstLineIndent = 20
	tp.Justify = true
	_, p := buildPara(t, tp)
	lines := paraLines(t, p)
	if !pnear(lines[0].x0, 120, 0.5) || !pnear(lines[0].x1, 200, 0.5) {
		t.Fatalf("first justified line %v..%v", lines[0].x0, lines[0].x1)
	}
	tp = oneLine(paraWords)
	tp.SubsequentLinesIndent = 10
	tp.HorizontalAlignment = HorizontalAlignmentRight
	_, p = buildPara(t, tp)
	for _, l := range paraLines(t, p) {
		if !pnear(l.x1, 200, 0.5) {
			t.Fatalf("right edge %v", l.x1)
		}
	}
}

func TestAppendParagraphNoWrap(t *testing.T) {
	tp := oneLine(paraWords)
	tp.FormattingOptions.WrapMode = WrapNoWrap
	_, p := buildPara(t, tp)
	if n := len(paraLines(t, p)); n != 1 {
		t.Fatalf("%d lines, want 1", n)
	}
	for _, w := range []string{"alpha", "bravo"} {
		if m, _ := p.SearchText(w); len(m) != 1 {
			t.Fatalf("%q should be drawn", w)
		}
	}
	for _, w := range []string{"november", "tango", "sierra"} {
		if m, _ := p.SearchText(w); len(m) != 0 {
			t.Fatalf("%q lies past the right edge and must not be drawn", w)
		}
	}
	// the clip keeps what is drawn partly outside from showing
	data, err := p.contentStreams()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "100 300 100 400 re W n") {
		t.Fatalf("no clip in content: %s", data)
	}
}

func TestAppendParagraphBreakLong(t *testing.T) {
	long := "abcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyz"
	for _, mode := range []WordWrapMode{WrapByWords, WrapByWordsBreakLong} {
		tp := oneLine("a " + long + " b")
		tp.FormattingOptions.WrapMode = mode
		_, p := buildPara(t, tp)
		lines := paraLines(t, p)
		if mode == WrapByWords {
			if len(lines) != 3 {
				t.Fatalf("by words: %d lines", len(lines))
			}
			if lines[1].x1 <= 200 {
				t.Fatal("the long word should overflow its line")
			}
			continue
		}
		if len(lines) < 4 {
			t.Fatalf("break long: %d lines", len(lines))
		}
		for _, l := range lines {
			if l.x1 > 200.01 {
				t.Fatalf("line past the edge: %v", l.x1)
			}
		}
		text, _ := p.ExtractText()
		if got := strings.ReplaceAll(strings.Join(strings.Fields(text), ""), "\n", ""); got != "a"+long+"b" {
			t.Fatalf("text lost: %q", got)
		}
	}
}

func TestAppendParagraphLineSpacing(t *testing.T) {
	f := NewTextFragment("alpha bravo charlie delta echo foxtrot golf hotel")
	f.Segments[0].TextState = TextState{LineSpacing: 30}
	tp := NewTextParagraph()
	tp.AppendLine(f)
	_, p := buildPara(t, tp)
	lines := paraLines(t, p)
	if len(lines) < 3 {
		t.Fatal("want several lines")
	}
	for i := 1; i < len(lines); i++ {
		if !pnear(lines[i-1].y-lines[i].y, 30, 0.01) {
			t.Fatalf("pitch %v", lines[i-1].y-lines[i].y)
		}
	}
}

func TestAppendParagraphMargin(t *testing.T) {
	tp := oneLine(paraWords)
	tp.Margin = MarginInfo{Top: 10, Right: 15, Bottom: 5, Left: 20}
	_, p := buildPara(t, tp)
	m, _ := GetFontMetrics(FontHelvetica, 12)
	lines := paraLines(t, p)
	if !pnear(lines[0].y, 690-m.Ascent, 0.01) {
		t.Fatalf("first baseline %v", lines[0].y)
	}
	for _, l := range lines {
		if l.x0 < 119.99 || l.x1 > 185.01 {
			t.Fatalf("line %v..%v outside the margins", l.x0, l.x1)
		}
	}
}

func TestAppendParagraphSpacing(t *testing.T) {
	f := NewTextFragment(paraWords)
	f.TextState = TextState{CharacterSpacing: 1, HorizontalScaling: 90}
	tp := NewTextParagraph()
	tp.AppendLine(f)
	_, p := buildPara(t, tp)
	for _, l := range paraLines(t, p) {
		if l.x1 > 200.5 {
			t.Fatalf("spaced line overflows: %v", l.x1)
		}
	}
}

func TestAppendParagraphBackgroundAndLink(t *testing.T) {
	f := NewTextFragment("alpha bravo charlie delta echo foxtrot golf")
	f.Segments[0].Hyperlink = "https://example.com"
	f.Segments[0].TextState = TextState{BackgroundColor: &Color{R: 1, G: 1, A: 1}}
	tp := NewTextParagraph()
	tp.AppendLine(f)
	_, p := buildPara(t, tp)
	if n := p.Annotations().Count(); n < 2 {
		t.Fatalf("%d link annotations, want one per drawn piece", n)
	}
	data, _ := p.contentStreams()
	if !strings.Contains(string(data), "re\nf") && !strings.Contains(string(data), " re") {
		t.Fatal("no background rectangle")
	}
	img, err := p.RenderImage(RenderOptions{DPI: 72})
	if err != nil {
		t.Fatal(err)
	}
	// 842pt page: a point inside the first word's box shows the yellow fill
	r, g, b, _ := img.At(103, 143).RGBA()
	if r>>8 < 200 || g>>8 < 200 || b>>8 > 100 {
		t.Fatalf("background not painted: %d %d %d", r>>8, g>>8, b>>8)
	}
}

func TestAppendParagraphBinding(t *testing.T) {
	f := NewTextFragment(paraWords)
	g := NewTextFragment("second line")
	tp := NewTextParagraph()
	tp.AppendLine(f)
	tp.AppendLine(g)
	d, p := buildPara(t, tp)
	if len(d.boundFragments) != 2 || f.bind == nil || g.bind == nil || f.bind.page != p {
		t.Fatalf("bindings: %d", len(d.boundFragments))
	}
	if len(f.bind.drawn) < 4 {
		t.Fatalf("wrapped pieces not all covered: %d", len(f.bind.drawn))
	}
	if f.Text != paraWords || g.Text != "second line" {
		t.Fatalf("text %q %q", f.Text, g.Text)
	}
	if !f.bind.snap.equal(snapshotOf(f)) {
		t.Fatal("snapshot out of date")
	}
	if g.Position.YIndent >= f.Position.YIndent {
		t.Fatal("second line must lie below the first")
	}
}

func TestAppendParagraphBindingCoversEveryWrappedLine(t *testing.T) {
	f := NewTextFragment(paraWords)
	tp := NewTextParagraph()
	tp.AppendLine(f)
	_, p := buildPara(t, tp)
	n := len(paraLines(t, p))
	if len(f.bind.drawn) != n {
		t.Fatalf("%d quads for %d lines", len(f.bind.drawn), n)
	}
	for _, q := range f.bind.drawn {
		if q.X1 < 99.99 || q.X2 > 200.01 {
			t.Fatalf("quad outside the rectangle: %+v", q)
		}
	}
}

func TestAppendParagraphErrors(t *testing.T) {
	d := NewDocumentFromFormat(PageFormatA4)
	p := mustPage(t, d)
	b := NewTextBuilder(p)
	if err := b.AppendParagraph(nil); err == nil {
		t.Fatal("nil paragraph")
	}
	if err := (*TextBuilder)(nil).AppendParagraph(NewTextParagraph()); err == nil {
		t.Fatal("nil builder")
	}
	if err := b.AppendParagraph(NewTextParagraph()); err != nil {
		t.Fatalf("empty paragraph: %v", err)
	}
	bad := func(mod func(*TextParagraph)) {
		t.Helper()
		tp := oneLine("x")
		tp.Rectangle = paraRect()
		mod(tp)
		if err := b.AppendParagraph(tp); err == nil {
			t.Fatal("expected an error")
		}
	}
	bad(func(tp *TextParagraph) { tp.Rectangle = Rectangle{LLX: 10, LLY: 10, URX: 10, URY: 50} })
	bad(func(tp *TextParagraph) { tp.Margin = MarginInfo{Left: 60, Right: 60} })
	bad(func(tp *TextParagraph) { tp.Margin = MarginInfo{Top: 300, Bottom: 100} })
	bad(func(tp *TextParagraph) { tp.FirstLineIndent = 100 })
	bad(func(tp *TextParagraph) { tp.SubsequentLinesIndent = 150 })
	bad(func(tp *TextParagraph) { tp.FirstLineIndent = -1 })
	bad(func(tp *TextParagraph) { tp.SubsequentLinesIndent = -1 })
	bad(func(tp *TextParagraph) { tp.FormattingOptions.WrapMode = 9 })
	bad(func(tp *TextParagraph) { tp.FormattingOptions.WrapMode = -1 })
	bad(func(tp *TextParagraph) { tp.lines[0].TextState.Rotation = 30 })
	bad(func(tp *TextParagraph) { tp.lines[0].TextState.FontSize = math.NaN() })
	bad(func(tp *TextParagraph) { tp.lines = append(tp.lines, nil) })
	if len(d.boundFragments) != 0 {
		t.Fatal("a failed paragraph must not bind anything")
	}
	if data, _ := p.contentStreams(); len(data) != 0 {
		t.Fatalf("a failed paragraph must not draw: %q", data)
	}
}

func TestAppendParagraphBlankLine(t *testing.T) {
	tp := NewTextParagraph()
	tp.AppendLine(NewTextFragment("first"))
	tp.AppendLine(NewTextFragment(""))
	tp.AppendLine(NewTextFragment("third"))
	_, p := buildPara(t, tp)
	lines := paraLines(t, p)
	if len(lines) != 2 || !pnear(lines[0].y-lines[1].y, 28.8, 0.01) {
		t.Fatalf("blank line must keep its height: %+v", lines)
	}
}

func TestAppendParagraphKeepsOtherContent(t *testing.T) {
	d := NewDocumentFromFormat(PageFormatA4)
	p := mustPage(t, d)
	if err := p.AddTextAt("before", TextStyle{}, 300, 100); err != nil {
		t.Fatal(err)
	}
	tp := oneLine(paraWords)
	tp.Rectangle = paraRect()
	if err := NewTextBuilder(p).AppendParagraph(tp); err != nil {
		t.Fatal(err)
	}
	if err := p.AddTextAt("after", TextStyle{}, 300, 90); err != nil {
		t.Fatal(err)
	}
	// "after" is outside the clip rectangle: the clip must have been closed
	if m, _ := p.SearchText("after"); len(m) != 1 {
		t.Fatal("after missing")
	}
	img, err := p.RenderImage(RenderOptions{DPI: 72})
	if err != nil {
		t.Fatal(err)
	}
	dark := false
	for x := 300; x < 330; x++ {
		for y := 744; y < 752; y++ {
			if r, _, _, _ := img.At(x, y).RGBA(); r>>8 < 128 {
				dark = true
			}
		}
	}
	if !dark {
		t.Fatal("text after the paragraph was clipped away: clip not restored")
	}
}

func TestAppendParagraphFirstLineIndentOnlyOnce(t *testing.T) {
	tp := NewTextParagraph()
	tp.FirstLineIndent = 20
	tp.AppendLine(NewTextFragment("aaaaaa aaaaaa bbbbbb"))
	tp.AppendLine(NewTextFragment("cccccc"))
	_, p := buildPara(t, tp)
	lines := paraLines(t, p)
	if len(lines) != 3 {
		t.Fatalf("%d lines: %+v", len(lines), lines)
	}
	// the first line is 80pt wide: one word; the second appended line is not
	// the paragraph's first and starts at the edge
	if !pnear(lines[0].x0, 120, 0.01) || !pnear(lines[1].x0, 100, 0.01) || !pnear(lines[2].x0, 100, 0.01) {
		t.Fatalf("starts %v %v %v", lines[0].x0, lines[1].x0, lines[2].x0)
	}
	if lines[0].x1-lines[0].x0 > 41 {
		t.Fatalf("first line should hold one word, spans %v", lines[0].x1-lines[0].x0)
	}
}

func TestAppendParagraphNoWrapJustify(t *testing.T) {
	tp := oneLine(paraWords)
	tp.FormattingOptions.WrapMode = WrapNoWrap
	tp.Justify = true
	tp.HorizontalAlignment = HorizontalAlignmentFullJustify
	_, p := buildPara(t, tp)
	if m, _ := p.SearchText("tango"); len(m) != 0 {
		t.Fatal("an overflowing line must not be squeezed to fit")
	}
	if m, _ := p.SearchText("alpha"); len(m) != 1 || !pnear(m[0].Rect.LLX, 100, 0.5) {
		t.Fatalf("first word: %+v", m)
	}
}

func TestLayoutRunsOptFirstWidthAndTinyBreak(t *testing.T) {
	style := TextStyle{Font: FontHelvetica, Size: 12}
	runs := []textRun{{text: "aaaaaa aaaaaa", style: style}}
	m, err := metricsFor(style, runs[0].text)
	if err != nil {
		t.Fatal(err)
	}
	ms := []runMetrics{m}
	if got := len(layoutRunsOpt(runs, ms, 100, 100, false)); got != 1 {
		t.Fatalf("100/100: %d lines", got)
	}
	if got := len(layoutRunsOpt(runs, ms, 100, 50, false)); got != 2 {
		t.Fatalf("first line 50 wide: %d lines", got)
	}
	// a width no rune fits in still progresses one rune per line
	lines := layoutRunsOpt([]textRun{{text: "abcd", style: style}}, ms, 1, 1, true)
	if len(lines) != 4 {
		t.Fatalf("%d lines for 4 runes at 1pt", len(lines))
	}
}
