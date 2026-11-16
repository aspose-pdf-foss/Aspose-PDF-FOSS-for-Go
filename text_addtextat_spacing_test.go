// SPDX-License-Identifier: MIT

package asposepdf

import (
	"math"
	"strings"
	"testing"
)

func spacingContent(t *testing.T, style TextStyle, text string, font func(*Document) Font) (string, *Document, *Page) {
	t.Helper()
	doc := NewDocumentFromFormat(PageFormatA4)
	if font != nil {
		style.Font = font(doc)
	}
	p, err := doc.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.AddTextAt(text, style, 100, 200); err != nil {
		t.Fatal(err)
	}
	c, err := p.contentStreams()
	if err != nil {
		t.Fatal(err)
	}
	return string(c), doc, p
}

func TestAddTextAtSpacing(t *testing.T) {
	style := TextStyle{Size: 12, CharSpacing: 2, WordSpacing: 5, HorizontalScaling: 80}
	s, doc, _ := spacingContent(t, style, "a b c", nil)
	for _, op := range []string{"2 Tc", "5 Tw", "80 Tz"} {
		if !strings.Contains(s, op) {
			t.Errorf("content missing %q:\n%s", op, s)
		}
	}
	w, err := MeasureText("a b c", style)
	if err != nil {
		t.Fatal(err)
	}
	// 5 runes * Tc 2 + 2 spaces * Tw 5 = 20, all scaled by 0.8.
	base, _ := MeasureText("a b c", TextStyle{Size: 12})
	if math.Abs(w-(base+20)*0.8) > 1e-9 {
		t.Fatalf("MeasureText = %v, want %v", w, (base+20)*0.8)
	}
	var buf strings.Builder
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	re, err := OpenStream(strings.NewReader(buf.String()))
	if err != nil {
		t.Fatal(err)
	}
	p, _ := re.Page(1)
	lines, err := p.ExtractTextWithLayout()
	if err != nil || len(lines) == 0 || len(lines[0].Fragments) == 0 {
		t.Fatalf("no text: %v", err)
	}
	if got := lines[0].Fragments[0].CharSpacing; got != 2 {
		t.Errorf("fragment CharSpacing = %v, want 2", got)
	}
	m, err := p.SearchText("a b c")
	if err != nil || len(m) != 1 {
		t.Fatalf("search: %v %v", m, err)
	}
	// The last character's Tc/Tw trails the visible glyph, so the box ends
	// within one scaled Tc of the measured advance.
	if d := (100 + w) - m[0].Rect.URX; d < -0.1 || d > 2*0.8+0.1 {
		t.Errorf("right edge %v vs x+MeasureText %v", m[0].Rect.URX, 100+w)
	}
}

func TestAddTextAtSpacingWordSpacingEmbedded(t *testing.T) {
	load := func(d *Document) Font {
		f, err := d.LoadFont("testdata/DejaVuSans.ttf")
		if err != nil {
			t.Skipf("LoadFont: %v", err)
		}
		return f
	}
	plain, _, _ := spacingContent(t, TextStyle{Size: 10}, "a b", load)
	s, _, _ := spacingContent(t, TextStyle{Size: 10, WordSpacing: 5}, "a b", load)
	if strings.Contains(s, " Tw") {
		t.Error("embedded font must not use the Tw operator")
	}
	if !strings.Contains(s, "TJ") || !strings.Contains(s, "-499.") {
		t.Errorf("want TJ with -500 for 5pt at 10pt:\n%s", s)
	}
	if strings.Contains(plain, "TJ") {
		t.Error("control stream has no word spacing")
	}
}

func TestSpacedTJ(t *testing.T) {
	enc := func(s string) string {
		var b strings.Builder
		b.WriteByte('<')
		for _, r := range s {
			b.WriteString(map[rune]string{'a': "0001", ' ': "0002", 'b': "0003"}[r])
		}
		b.WriteByte('>')
		return b.String()
	}
	got := spacedTJ("a b", enc, 10, 5)
	if got != "[<0001 0002> -500 <0003>] TJ\n" && got != "[<00010002> -500 <0003>] TJ\n" {
		t.Errorf("got %q", got)
	}
	if got != "[<00010002> -500 <0003>] TJ\n" {
		t.Errorf("got %q", got)
	}
}

func TestAddTextAtRenderMode(t *testing.T) {
	red := &Color{1, 0, 0, 1}
	s, _, _ := spacingContent(t, TextStyle{Size: 12, RenderMode: RenderingStroke, StrokeColor: red}, "x", nil)
	if !strings.Contains(s, "1 Tr") || !strings.Contains(s, "1 0 0 RG") {
		t.Errorf("stroke mode:\n%s", s)
	}
	s, _, _ = spacingContent(t, TextStyle{Size: 12, RenderMode: RenderingFillStroke}, "x", nil)
	if !strings.Contains(s, "2 Tr") || strings.Contains(s, " RG") {
		t.Errorf("fill+stroke without colour:\n%s", s)
	}
	a, _, _ := spacingContent(t, TextStyle{Size: 12, RenderMode: RenderingInvisible, Underline: true}, "x", nil)
	b, _, _ := spacingContent(t, TextStyle{Size: 12, Invisible: true, Underline: true}, "x", nil)
	if a != b || !strings.Contains(a, "3 Tr") {
		t.Errorf("invisible mode differs:\n%s\n---\n%s", a, b)
	}
	z, _, _ := spacingContent(t, TextStyle{Size: 12}, "x", nil)
	for _, op := range []string{" Tr", " Tc", " Tw", " Tz", " RG"} {
		if strings.Contains(z, op) {
			t.Errorf("zero style emitted %q", op)
		}
	}
	h, _, _ := spacingContent(t, TextStyle{Size: 12, HorizontalScaling: 100}, "x", nil)
	if h != z {
		t.Error("HorizontalScaling 100 must equal default output")
	}
}

func TestTextStateStyleSpacingFields(t *testing.T) {
	red := &Color{1, 0, 0, 1}
	st := TextState{CharacterSpacing: 1, WordSpacing: 2, HorizontalScaling: 70, StrokingColor: red, RenderingMode: RenderingStroke}.style()
	if st.CharSpacing != 1 || st.WordSpacing != 2 || st.HorizontalScaling != 70 || st.StrokeColor != red || st.RenderMode != RenderingStroke {
		t.Errorf("style() = %+v", st)
	}
	s := TextState{Font: FontHelvetica, FontSize: 10, CharacterSpacing: 1, WordSpacing: 2, HorizontalScaling: 50}
	w, _ := s.MeasureString("a b")
	m, _ := MeasureText("a b", s.resolved().style())
	if w != m {
		t.Errorf("MeasureString %v != MeasureText %v", w, m)
	}
}

func TestAddTextAtSpacingUnshapedEmbeddedAndAlignment(t *testing.T) {
	doc := NewDocumentFromFormat(PageFormatA4)
	f, err := doc.LoadFont("testdata/DejaVuSans.ttf")
	if err != nil {
		t.Skipf("LoadFont: %v", err)
	}
	ef := f.(*embeddedFont)
	ef.ttf.otOnce.Do(func() {})
	ef.ttf.ot = nil // no GSUB/GPOS: the unshaped embedded path
	p, _ := doc.Page(1)
	if err := p.AddTextAt("a b", TextStyle{Font: f, Size: 10, WordSpacing: 5}, 100, 200); err != nil {
		t.Fatal(err)
	}
	c, _ := p.contentStreams()
	if !strings.Contains(string(c), "-500 <") || !strings.Contains(string(c), "] TJ") {
		t.Errorf("unshaped embedded font needs TJ word spacing:\n%s", c)
	}

	// Right alignment must anchor with the spaced width.
	style := TextStyle{Size: 12, CharSpacing: 3, HAlign: HAlignRight}
	s, _, _ := spacingContent(t, style, "abc", nil)
	w, _ := MeasureText("abc", style)
	want := formatFloat(100-w) + " 200 Td"
	if !strings.Contains(s, want) {
		t.Errorf("want %q in:\n%s", want, s)
	}
}
