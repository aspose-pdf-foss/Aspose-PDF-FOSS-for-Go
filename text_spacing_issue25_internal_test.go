// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"encoding/binary"
	"math"
	"os"
	"strings"
	"testing"
)

// GitHub issue #25 point 1: non-finite spacing values are refused, as a
// non-finite LineSpacing is, by every text call that writes or measures them.
func TestNonFiniteSpacingRefused(t *testing.T) {
	for _, c := range []struct {
		name  string
		style TextStyle
	}{
		{"char spacing NaN", TextStyle{Size: 12, CharSpacing: math.NaN()}},
		{"word spacing +Inf", TextStyle{Size: 12, WordSpacing: math.Inf(1)}},
		{"horizontal scaling NaN", TextStyle{Size: 12, HorizontalScaling: math.NaN()}},
	} {
		t.Run(c.name, func(t *testing.T) {
			d := NewDocument(300, 200)
			p := mustPage(t, d)
			if err := p.AddTextAt("a b", c.style, 50, 100); err == nil {
				t.Error("AddTextAt accepted it")
			}
			if w, err := MeasureText("a b", c.style); err == nil {
				t.Errorf("MeasureText returned %v", w)
			}
			if err := p.AddText("a b", c.style, Rectangle{10, 10, 290, 190}); err == nil {
				t.Error("AddText accepted it")
			}
			st := TextState{FontSize: 12, CharacterSpacing: c.style.CharSpacing, WordSpacing: c.style.WordSpacing,
				HorizontalScaling: c.style.HorizontalScaling}
			f := NewTextFragment("a b")
			f.TextState = st
			if err := NewTextBuilder(p).AppendText(f); err == nil {
				t.Error("TextBuilder.AppendText accepted it")
			}
			tp := NewTextParagraph()
			tp.Rectangle = Rectangle{10, 10, 290, 190}
			tp.AppendLine(f)
			if err := NewTextBuilder(p).AppendParagraph(tp); err == nil {
				t.Error("TextBuilder.AppendParagraph accepted it")
			}
			var buf bytes.Buffer
			if _, err := d.WriteTo(&buf); err != nil {
				t.Fatal(err)
			}
			for _, bad := range []string{"NaN", "Inf"} {
				if bytes.Contains(buf.Bytes(), []byte(bad)) {
					t.Errorf("%s written to the file", bad)
				}
			}
		})
	}
}

// GitHub issue #25 point 2: Tc applies once per glyph shown, so a ligature
// counts once in MeasureText, and a right-aligned line ends where asked.
func TestCharSpacingCountsShownGlyphs(t *testing.T) {
	d := NewDocument(300, 200)
	f, err := d.LoadFont("testdata/DejaVuSans.ttf")
	if err != nil {
		t.Fatal(err)
	}
	ef := f.(*embeddedFont)
	shaped := shapeLine(ef.ttf, "ffiX", 0, false)
	glyphs := len(shaped.glyphs)
	if glyphs >= 4 {
		t.Skipf("no ffi ligature in the test font (%d glyphs)", glyphs)
	}
	plain, err := MeasureText("ffiX", TextStyle{Font: f, Size: 12})
	if err != nil {
		t.Fatal(err)
	}
	spaced, err := MeasureText("ffiX", TextStyle{Font: f, Size: 12, CharSpacing: 3})
	if err != nil {
		t.Fatal(err)
	}
	if want := plain + float64(glyphs)*3; math.Abs(spaced-want) > 1e-6 {
		t.Errorf("spaced width %v, want %v (%d glyphs shown)", spaced, want, glyphs)
	}
	// A space glyph still takes the word spacing.
	ws, err := MeasureText("a b", TextStyle{Font: f, Size: 12, WordSpacing: 5})
	if err != nil {
		t.Fatal(err)
	}
	wp, _ := MeasureText("a b", TextStyle{Font: f, Size: 12})
	if math.Abs(ws-wp-5) > 1e-6 {
		t.Errorf("word spacing added %v, want 5", ws-wp)
	}

	// Right-aligned at x = 250: the shown text ends there.
	p := mustPage(t, d)
	if err := p.AddTextAt("ffiX", TextStyle{Font: f, Size: 12, CharSpacing: 3, HAlign: HAlignRight}, 250, 100); err != nil {
		t.Fatal(err)
	}
	ms, err := mustPage(t, reopenDoc(t, d)).SearchText("ffiX")
	if err != nil || len(ms) != 1 {
		t.Fatalf("search: %v %+v", err, ms)
	}
	if end := ms[0].Rect.URX; math.Abs(end-250) > 3.5 {
		// URX is the last glyph's advance plus Tc; allow that one Tc.
		t.Errorf("right-aligned line ends at %v, want about 250", end)
	}
}

// kernOnlyFont is DejaVu Sans with its GSUB and GPOS directory entries
// renamed, so the font keeps only its legacy 'kern' table.
func kernOnlyFont(t *testing.T) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/DejaVuSans.ttf")
	if err != nil {
		t.Fatal(err)
	}
	b = append([]byte(nil), b...)
	n := int(binary.BigEndian.Uint16(b[4:]))
	renamed := 0
	for i := 0; i < n; i++ {
		rec := b[12+16*i:]
		switch string(rec[:4]) {
		case "GSUB":
			copy(rec, "XSUB")
			renamed++
		case "GPOS":
			copy(rec, "XPOS")
			renamed++
		}
	}
	if renamed != 2 {
		t.Skip("test font lacks GSUB/GPOS")
	}
	return b
}

// GitHub issue #25 point 3: a font with only a 'kern' table is kerned.
func TestKernOnlyFontIsKerned(t *testing.T) {
	d := NewDocument(300, 200)
	f, err := d.LoadFontFromStream(bytes.NewReader(kernOnlyFont(t)))
	if err != nil {
		t.Fatal(err)
	}
	st := TextStyle{Font: f, Size: 12}
	av, _ := MeasureText("AV", st)
	a, _ := MeasureText("A", st)
	v, _ := MeasureText("V", st)
	if !(av < a+v-0.5) {
		t.Fatalf("AV %v, A+V %v: not kerned", av, a+v)
	}
	p := mustPage(t, d)
	if err := p.AddTextAt("AV", st, 50, 100); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if _, err := d.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	d2, err := OpenStream(&buf)
	if err != nil {
		t.Fatal(err)
	}
	txt, err := mustPage(t, d2).ExtractText()
	if err != nil || strings.TrimSpace(txt) != "AV" {
		t.Errorf("text %q, %v", txt, err)
	}
	ms, _ := mustPage(t, d2).SearchText("AV")
	if len(ms) != 1 || math.Abs((ms[0].Rect.URX-ms[0].Rect.LLX)-av) > 0.5 {
		t.Errorf("drawn width %+v, measured %v", ms, av)
	}
}

// A paragraph whose second line has a non-finite spacing is refused before
// its first line is drawn.
func TestParagraphNonFiniteSpacingDrawsNothing(t *testing.T) {
	d := NewDocument(300, 200)
	p := mustPage(t, d)
	tp := NewTextParagraph()
	tp.Rectangle = Rectangle{10, 10, 290, 190}
	tp.AppendLine(NewTextFragment("first line"))
	bad := NewTextFragment("second line")
	bad.TextState.CharacterSpacing = math.NaN()
	tp.AppendLine(bad)
	if err := NewTextBuilder(p).AppendParagraph(tp); err == nil {
		t.Fatal("AppendParagraph accepted it")
	}
	txt, err := mustPage(t, reopenDoc(t, d)).ExtractText()
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(txt) != "" {
		t.Errorf("drawn before the refusal: %q", txt)
	}
}
