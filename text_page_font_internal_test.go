// SPDX-License-Identifier: MIT

package asposepdf

import (
	"math"
	"strings"
	"testing"
)

// pageFontDocBytes builds a page showing "Hello brave world" in a simple
// font the library has no metrics for (every glyph 700/1000 em wide, unlike
// any Standard-14 face), set with 4pt character spacing, and
// " and untouched words" after it on the same line ("br" and "ave" kerned
// apart), and returns it saved.
// With embedded, the font descriptor names a (stand-in) program.
func pageFontDocBytes(t *testing.T, embedded bool) []byte {
	t.Helper()
	d := NewDocumentFromFormat(PageFormatA4)
	p := mustPage(t, d)
	widths := make(pdfArray, 95)
	for i := range widths {
		widths[i] = 700
	}
	font := pdfDict{
		"/Type": pdfName("/Font"), "/Subtype": pdfName("/TrueType"), "/BaseFont": pdfName("/Harrington"),
		"/FirstChar": 32, "/LastChar": 126, "/Widths": widths, "/Encoding": pdfName("/WinAnsiEncoding"),
	}
	if embedded {
		prog := d.addObject(&pdfStream{Dict: pdfDict{}, Data: []byte("stand-in"), Decoded: true})
		font["/FontDescriptor"] = pdfDict{"/Type": pdfName("/FontDescriptor"), "/FontName": pdfName("/Harrington"),
			"/Flags": 32, "/FontFile2": pdfRef{Num: prog}}
	}
	id := d.addObject(font)
	res := p.pageResources()
	if res == nil {
		res = pdfDict{}
		p.pageDict()["/Resources"] = res
	}
	res["/Font"] = pdfDict{"/F9": pdfRef{Num: id}}
	content := "BT /F9 12 Tf 4 Tc 72 600 Td [(Hello br) -150 (ave world)] TJ ( and untouched words) Tj ET\n"
	if err := p.appendToContentStream([]byte(content)); err != nil {
		t.Fatal(err)
	}
	return writeBytes(t, d)
}

// glyphStarts returns where each character of the line holding phrase
// starts, and the font names of its fragments.
func glyphStarts(t *testing.T, d *Document, phrase string) ([]float64, []TextFragment) {
	t.Helper()
	p, err := d.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	lines, err := p.ExtractTextWithLayout()
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range lines {
		if !strings.Contains(l.Text, phrase) {
			continue
		}
		var xs []float64
		for _, f := range l.Fragments {
			xs = append(xs, f.runeX...)
		}
		return xs, l.Fragments
	}
	t.Fatalf("no line with %q", phrase)
	return nil, nil
}

// An edit of found text that needs no other glyphs — a background, an
// underline, a colour, a style — redraws it in the page's own font with
// every glyph where it was: the line keeps its width and the text after it
// does not move.
func TestApplyDecorationKeepsPageFontAndPlacement(t *testing.T) {
	src := pageFontDocBytes(t, false)
	before, _ := glyphStarts(t, openBytes(t, src), "Hello brave world")
	for _, c := range []struct {
		name string
		edit func(f *TextFragment)
	}{
		{"background", func(f *TextFragment) { f.TextState.BackgroundColor = &Color{0, 0.5, 0, 1} }},
		{"underline", func(f *TextFragment) { f.TextState.Underline = true }},
		{"colour", func(f *TextFragment) { f.TextState.ForegroundColor = &Color{1, 0, 0, 1} }},
		{"bold", func(f *TextFragment) { f.TextState.FontStyle = FontStyleBold }},
		{"italic", func(f *TextFragment) { f.TextState.FontStyle = FontStyleItalic }},
	} {
		t.Run(c.name, func(t *testing.T) {
			d := openBytes(t, src)
			c.edit(absorbOne(t, d, "brave"))
			got := openBytes(t, writeBytes(t, d))
			after, frags := glyphStarts(t, got, "Hello brave world")
			if len(after) != len(before) {
				t.Fatalf("%d glyphs, was %d", len(after), len(before))
			}
			for i := range before {
				if math.Abs(after[i]-before[i]) > 0.01 {
					t.Fatalf("glyph %d moved from %.3f to %.3f", i, before[i], after[i])
				}
			}
			for _, f := range frags {
				if f.FontName != "Harrington" {
					t.Errorf("%q drawn in %q, want the page's font", f.Text, f.FontName)
				}
			}
			if n := absorbCount(t, got, "brave"); n != 1 {
				t.Errorf("brave found %d times", n)
			}
			g := absorbOne(t, got, "brave")
			switch c.name {
			case "bold":
				if !g.Bold {
					t.Error("bold not read back")
				}
			case "italic":
				if !g.Italic {
					t.Error("italic not read back")
				}
			case "colour":
				if g.Color.R < 0.9 || g.Color.G > 0.1 {
					t.Errorf("colour %+v", g.Color)
				}
			}
		})
	}
}

// A background on found text keeps its glyphs and shows behind them.
func TestApplyBackgroundOnFoundText(t *testing.T) {
	d := openBytes(t, pageFontDocBytes(t, false))
	f := absorbOne(t, d, "brave")
	f.TextState.BackgroundColor = &Color{0, 0.5, 0, 1}
	got := openBytes(t, writeBytes(t, d))
	p, _ := got.Page(1)
	m, err := p.SearchText("brave", SearchOptions{SearchForTextRelatedGraphics: true})
	if err != nil || len(m) != 1 {
		t.Fatalf("%d matches, %v", len(m), err)
	}
	if bg := m[0].BackgroundColor; bg == nil || bg.G < 0.4 || bg.R > 0.1 {
		t.Errorf("background %+v", bg)
	}
}

// New text is drawn in the page's font when the font shows it: any character
// of its encoding for a font the viewer supplies, only the characters the
// fragment showed for an embedded program (a subset need not hold others);
// otherwise in a Standard-14 face, as before.
func TestApplyTextChangePageFontCoverage(t *testing.T) {
	for _, c := range []struct {
		embedded bool
		text     string
		page     bool
	}{
		{false, "bravo", true},
		{true, "rave", true},
		{true, "bravo", false},
	} {
		d := openBytes(t, pageFontDocBytes(t, c.embedded))
		absorbOne(t, d, "brave").Text = c.text
		got := openBytes(t, writeBytes(t, d))
		g := absorbOne(t, got, c.text)
		if (g.FontName == "Harrington") != c.page {
			t.Errorf("embedded %v, %q: drawn in %q", c.embedded, c.text, g.FontName)
		}
	}
}

// Glyphs slanted by the text matrix read as italic; upright ones do not.
func TestExtractSlantedTextIsItalic(t *testing.T) {
	d := NewDocumentFromFormat(PageFormatA4)
	p := mustPage(t, d)
	if err := p.AddTextAt("slanted", TextStyle{Skew: 12}, 72, 600); err != nil {
		t.Fatal(err)
	}
	if err := p.AddTextAt("upright", TextStyle{}, 72, 500); err != nil {
		t.Fatal(err)
	}
	if !findFrag(t, p, "slanted").Italic {
		t.Error("slanted text not italic")
	}
	if findFrag(t, p, "upright").Italic {
		t.Error("upright text italic")
	}
}
