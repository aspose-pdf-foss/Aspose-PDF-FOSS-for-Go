// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"os"
	"reflect"
	"strings"
	"testing"
)

// GitHub issue #24 (shadowhunter1470-svg): line breaking at no-break
// spaces, invisible characters, gaps in TextStyle.MissingGlyph, non-finite
// LineSpacing/colours, and hyphen break opportunities.

func issue24Font(t *testing.T, d *Document, path string) Font {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Skip(err)
	}
	f, err := d.LoadFontFromStream(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	return f
}

// issue24Wrap wraps text exactly as AddText does for font at size.
func issue24Wrap(t *testing.T, font Font, size float64, text string, width float64) []string {
	t.Helper()
	w, _, err := fontWidthAndAscent(font, size)
	if err != nil {
		t.Fatal(err)
	}
	return wrapText(text, lineMeasurerForFont(font, size, text, false, w), width)
}

// issue24Content draws text with AddText into a fresh page and returns the
// page's content stream.
func issue24Content(t *testing.T, d *Document, text string, style TextStyle, width float64) string {
	t.Helper()
	p, err := d.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.AddText(text, style, Rectangle{LLX: 50, LLY: 600, URX: 50 + width, URY: 800}); err != nil {
		t.Fatalf("AddText(%q): %v", text, err)
	}
	c, err := p.contentStreams()
	if err != nil {
		t.Fatal(err)
	}
	return string(c)
}

func TestIssue24NoBreakSpacesDoNotBreak(t *testing.T) {
	for _, nb := range []string{"\u00a0", "\u202f", "\u2007"} {
		got := issue24Wrap(t, FontHelvetica, 10, "Weight: 10"+nb+"kg", 55)
		want := []string{"Weight:", "10" + nb + "kg"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("Helvetica %U at 55pt: lines %q, want %q", []rune(nb)[0], got, want)
		}
	}
	// Helvetica: U+00A0 is in WinAnsi, so it is drawn as itself (0xA0).
	d := NewDocument(595, 842)
	c := issue24Content(t, d, "Weight: 10\u00a0kg", TextStyle{Size: 10}, 55)
	if !strings.Contains(c, "(10\xa0kg) Tj") {
		t.Errorf("Helvetica U+00A0 not drawn as itself on one line with its unit:\n%s", c)
	}
}

func TestIssue24NarrowNoBreakSpaceDrawnAsItself(t *testing.T) {
	for _, nb := range []rune{'\u202f', '\u2007'} {
		d := NewDocument(595, 842)
		f := issue24Font(t, d, "testdata/DejaVuSans.ttf")
		ef := f.(*embeddedFont)
		text := "Weight: 10" + string(nb) + "kg"
		if got := issue24Wrap(t, f, 10, text, 500); !reflect.DeepEqual(got, []string{text}) {
			t.Errorf("%U at 500pt: lines %q, want one line", nb, got)
		}
		gid := ef.ttf.glyphID(nb)
		space := ef.ttf.glyphID(' ')
		c := issue24Content(t, d, text, TextStyle{Font: f, Size: 10}, 500)
		if !strings.Contains(c, glyphHex(gid)) {
			t.Errorf("%U: glyph %04X not drawn:\n%s", nb, gid, c)
		}
		// The only space glyph is the one after "Weight:".
		if n := strings.Count(c, glyphHex(space)); n != 1 {
			t.Errorf("%U: %d space glyphs drawn, want 1:\n%s", nb, n, c)
		}
	}
}

func glyphHex(gid uint16) string {
	const hexd = "0123456789ABCDEF"
	return string([]byte{hexd[gid>>12&15], hexd[gid>>8&15], hexd[gid>>4&15], hexd[gid&15]})
}

func TestIssue24LineSeparatorsBreak(t *testing.T) {
	for _, sep := range []string{"\r", "\u0085", "\u2028", "\u2029", "\r\n", "\n"} {
		got := issue24Wrap(t, FontHelvetica, 10, "first"+sep+"second", 500)
		if want := []string{"first", "second"}; !reflect.DeepEqual(got, want) {
			t.Errorf("separator %q: lines %q, want %q", sep, got, want)
		}
		d := NewDocument(595, 842)
		c := issue24Content(t, d, "first"+sep+"second", TextStyle{Size: 10}, 500)
		if !strings.Contains(c, "(first) Tj") || !strings.Contains(c, "(second) Tj") {
			t.Errorf("separator %q not drawn as two lines:\n%s", sep, c)
		}
	}
	// CRLF is one break, CR CR two.
	if got := issue24Wrap(t, FontHelvetica, 10, "a\r\rb", 500); len(got) != 3 {
		t.Errorf("CR CR: lines %q, want 3", got)
	}
}

// Table row heights (measureText) agree with what AddText draws.
func TestIssue24MeasureTextAgrees(t *testing.T) {
	n, _, err := measureText("Weight: 10\u00a0kg", TextStyle{Size: 10}, 55)
	if err != nil || n != 2 {
		t.Errorf("measureText NBSP: %d lines, err %v; want 2", n, err)
	}
	n, _, _ = measureText("first\u2028second", TextStyle{Size: 10}, 500)
	if n != 2 {
		t.Errorf("measureText U+2028: %d lines, want 2", n)
	}
}

// issue24PDFAHasNotdef converts d to PDF/A-2B and reports whether the
// report names a .notdef or missing glyph.
func issue24PDFAHasNotdef(t *testing.T, d *Document) bool {
	t.Helper()
	rep, err := d.ConvertToPDFA(PDFA2B)
	if err != nil {
		t.Fatal(err)
	}
	for _, is := range rep.Issues {
		if is.Rule == "NOTDEF_GLYPH_USED" || is.Rule == "GLYPH_MISSING" {
			return true
		}
	}
	return false
}

var issue24Ignorables = []string{"\u200b", "\u200d", "\u2060", "\ufeff", "\u00ad"}

func TestIssue24IgnorablesHelvetica(t *testing.T) {
	for _, ig := range issue24Ignorables {
		d := NewDocument(595, 842)
		c := issue24Content(t, d, "a"+ig+"b", TextStyle{Size: 10, MissingGlyph: MissingGlyphError}, 500)
		if !strings.Contains(c, "(ab) Tj") {
			t.Errorf("%U: Helvetica drew something for it:\n%s", []rune(ig)[0], c)
		}
		p, _ := d.Page(1)
		if err := p.AddTextAt("c"+ig+"d", TextStyle{Size: 10}, 50, 100); err != nil {
			t.Fatal(err)
		}
		cs, _ := p.contentStreams()
		if !strings.Contains(string(cs), "(cd) Tj") {
			t.Errorf("%U: AddTextAt drew something for it:\n%s", []rune(ig)[0], cs)
		}
		w1, _ := MeasureText("a"+ig+"b", TextStyle{Size: 10, CharSpacing: 1})
		w2, _ := MeasureText("ab", TextStyle{Size: 10, CharSpacing: 1})
		if w1 != w2 {
			t.Errorf("%U: MeasureText %v, want %v", []rune(ig)[0], w1, w2)
		}
	}
}

// Carlito (bundled) has no GSUB/GPOS and no glyph for these characters, so
// they used to be written as glyph 0.
func TestIssue24IgnorablesUnshapedEmbedded(t *testing.T) {
	for _, ig := range issue24Ignorables {
		d := NewDocument(595, 842)
		f := issue24Font(t, d, "fonts/Carlito-Regular.ttf")
		if fontHasShaping(f.(*embeddedFont).ttf) {
			t.Fatal("fixture has layout tables")
		}
		c := issue24Content(t, d, "a"+ig+"b", TextStyle{Font: f, Size: 10, MissingGlyph: MissingGlyphError}, 500)
		if strings.Contains(c, "0000") {
			t.Errorf("%U: glyph 0 drawn:\n%s", []rune(ig)[0], c)
		}
		p, _ := d.Page(1)
		if err := p.AddTextAt("a"+ig+"b", TextStyle{Font: f, Size: 10, MissingGlyph: MissingGlyphError}, 50, 100); err != nil {
			t.Fatal(err)
		}
		cs, _ := p.contentStreams()
		if strings.Contains(string(cs), "0000") {
			t.Errorf("%U: AddTextAt drew glyph 0:\n%s", []rune(ig)[0], cs)
		}
		w1, _ := MeasureText("a"+ig+"b", TextStyle{Font: f, Size: 10})
		w2, _ := MeasureText("ab", TextStyle{Font: f, Size: 10})
		if w1 != w2 {
			t.Errorf("%U: MeasureText %v, want %v", []rune(ig)[0], w1, w2)
		}
		if issue24PDFAHasNotdef(t, d) {
			t.Errorf("%U: ConvertToPDFA reports a .notdef glyph", []rune(ig)[0])
		}
	}
}

func issue24Extract(t *testing.T, d *Document) string {
	t.Helper()
	texts, err := d.ExtractText()
	if err != nil {
		t.Fatal(err)
	}
	return strings.Join(texts, "|")
}

// Point 3a: white space AddText consumes is not a missing glyph.
func TestIssue24MissingGlyphSkipsSeparators(t *testing.T) {
	texts := []string{"first\nsecond", "a\tb", "a\r\nb", "a\u3000b"}
	fonts := map[string]func(*testing.T, *Document) Font{
		"Helvetica": func(*testing.T, *Document) Font { return FontHelvetica },
		"DejaVu":    func(t *testing.T, d *Document) Font { return issue24Font(t, d, "testdata/DejaVuSans.ttf") },
	}
	for fname, mk := range fonts {
		for _, text := range texts {
			var want string
			for i, pol := range []MissingGlyphPolicy{MissingGlyphAllow, MissingGlyphError, MissingGlyphReplace} {
				d := NewDocument(595, 842)
				p, _ := d.Page(1)
				err := p.AddText(text, TextStyle{Font: mk(t, d), Size: 10, MissingGlyph: pol}, Rectangle{LLX: 50, LLY: 600, URX: 550, URY: 800})
				if err != nil {
					t.Errorf("%s %q policy %d: %v", fname, text, pol, err)
					continue
				}
				got := issue24Extract(t, d)
				if i == 0 {
					want = got
				} else if got != want {
					t.Errorf("%s %q policy %d: extracted %q, want %q", fname, text, pol, got, want)
				}
			}
		}
	}
	// AddTextAt and MeasureText draw white space as given and keep flagging it.
	d := NewDocument(595, 842)
	p, _ := d.Page(1)
	st := TextStyle{Size: 10, MissingGlyph: MissingGlyphError}
	if err := p.AddTextAt("a\tb", st, 50, 50); err == nil {
		t.Error("AddTextAt with a tab and MissingGlyphError: want error")
	}
	if _, err := MeasureText("a\tb", st); err == nil {
		t.Error("MeasureText with a tab and MissingGlyphError: want error")
	}
}

// Point 3b: a replacement the font lacks is an error, never .notdef.
func TestIssue24ReplacementMustBeInFont(t *testing.T) {
	cases := []struct {
		path string
		rep  rune
	}{
		{"fonts/Carlito-Regular.ttf", 0},
		{"fonts/Carlito-Regular.ttf", '\u25a1'},
		{"testdata/SourceSerif4-Regular.otf", 0},
		{"", 0}, // Helvetica: U+FFFD is not in WinAnsi
	}
	for _, c := range cases {
		d := NewDocument(595, 842)
		var f Font = FontHelvetica
		name := "Helvetica"
		if c.path != "" {
			f = issue24Font(t, d, c.path)
			name = f.BaseFont()
		}
		p, _ := d.Page(1)
		st := TextStyle{Font: f, Size: 10, MissingGlyph: MissingGlyphReplace, ReplacementRune: c.rep}
		err := p.AddText("a\u4e2db", st, Rectangle{LLX: 50, LLY: 600, URX: 550, URY: 800})
		if err == nil {
			cs, _ := p.contentStreams()
			t.Errorf("%s rep %U: want error, content:\n%s", c.path, c.rep, cs)
			continue
		}
		want := "U+FFFD"
		if c.rep != 0 {
			want = "U+25A1"
		}
		if !strings.Contains(err.Error(), want) || !strings.Contains(err.Error(), name) {
			t.Errorf("%s: error %q does not name the replacement %s and the font %s", c.path, err, want, name)
		}
		if _, err := MeasureText("a\u4e2db", st); err == nil {
			t.Errorf("%s: MeasureText: want error", c.path)
		}
		if err := p.AddTextAt("a\u4e2db", st, 50, 50); err == nil {
			t.Errorf("%s: AddTextAt: want error", c.path)
		}
	}
	// A replacement the font has still works.
	d := NewDocument(595, 842)
	p, _ := d.Page(1)
	if err := p.AddText("a\u4e2db", TextStyle{Size: 10, MissingGlyph: MissingGlyphReplace, ReplacementRune: '?'}, Rectangle{LLX: 50, LLY: 600, URX: 550, URY: 800}); err != nil {
		t.Errorf("Helvetica with '?': %v", err)
	}
}

// Point 3c: a character a shaping font draws from its decomposition is
// not missing.
func TestIssue24MissingGlyphDecomposition(t *testing.T) {
	for _, text := range []string{"a\u01f5b", "a\u1e7fb"} {
		d := NewDocument(595, 842)
		f := issue24Font(t, d, "testdata/SourceSerif4-Regular.otf")
		ef := f.(*embeddedFont)
		if ef.ttf.glyphID([]rune(text)[1]) != 0 {
			t.Fatalf("fixture has a precomposed glyph for %q", text)
		}
		st := TextStyle{Font: f, Size: 10, MissingGlyph: MissingGlyphError}
		p, _ := d.Page(1)
		if err := p.AddText(text, st, Rectangle{LLX: 50, LLY: 600, URX: 550, URY: 800}); err != nil {
			t.Errorf("%q: AddText: %v", text, err)
		}
		if _, err := MeasureText(text, st); err != nil {
			t.Errorf("%q: MeasureText: %v", text, err)
		}
		if err := p.AddTextAt(text, st, 50, 50); err != nil {
			t.Errorf("%q: AddTextAt: %v", text, err)
		}
		if issue24PDFAHasNotdef(t, d) {
			t.Errorf("%q: ConvertToPDFA reports a .notdef glyph", text)
		}
	}
	// A character with no decomposition stays missing.
	d := NewDocument(595, 842)
	f := issue24Font(t, d, "testdata/SourceSerif4-Regular.otf")
	if fontHasGlyph(f, '\u4e2d') {
		t.Error("U+4E2D reported present in Source Serif 4")
	}
}
