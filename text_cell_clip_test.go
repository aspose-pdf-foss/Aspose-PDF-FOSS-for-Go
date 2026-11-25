// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"strings"
	"testing"
)

// clippedCellsDoc is a ruled row of two cells (x 50-300, 300-550) whose
// left cell's text is clipped at the rule, as producers write table cells,
// and runs on under the right cell: "KEYWORD" starts left of the rule and
// ends under the right cell's visible "Right side".
func clippedCellsDoc(t *testing.T) *Document {
	t.Helper()
	d := NewDocumentFromFormat(PageFormatA4)
	p := mustPage(t, d)
	st := TextStyle{Font: FontHelvetica, Size: 12}
	left := strings.Repeat("a", 34) + " KEYWORD tail"
	if err := p.clipped(&Rectangle{LLX: 50, LLY: 580, URX: 300, URY: 630}, func() error {
		return p.AddTextAt(left, st, 60, 600)
	}); err != nil {
		t.Fatal(err)
	}
	if err := p.AddTextAt("Right side", st, 312, 600); err != nil {
		t.Fatal(err)
	}
	ls := LineStyle{Width: 1}
	for _, x := range []float64{50, 300, 550} {
		if err := p.DrawLine(Point{X: x, Y: 580}, Point{X: x, Y: 630}, ls); err != nil {
			t.Fatal(err)
		}
	}
	d = reopenDoc(t, d)
	kw := rectOf(t, d, "KEYWORD")
	if kw.LLX >= 300 || kw.URX <= 312 {
		t.Fatalf("fixture: KEYWORD at %.1f-%.1f does not cross the rule into the right cell's text", kw.LLX, kw.URX)
	}
	return d
}

// pdf-go-64zt: the hidden part of a match under the next cell is removed,
// the next cell's own text there stays, and the replacement is clipped at
// the rule as the document's text is.
func TestReplaceInClippedCell(t *testing.T) {
	for _, via := range []struct {
		name string
		frag bool
	}{{"ReplaceText", false}, {"Fragment", true}} {
		t.Run(via.name, func(t *testing.T) {
			d := clippedCellsDoc(t)
			repl := "LONGERREPLACEMENT"
			if via.frag {
				f := absorbOne(t, d, "KEYWORD")
				f.Text = repl
				if err := d.ApplyTextChanges(); err != nil {
					t.Fatal(err)
				}
			} else if n, err := d.ReplaceText("KEYWORD", repl); err != nil || n != 1 {
				t.Fatalf("replace: %d, %v", n, err)
			}
			out := reopenDoc(t, d)
			if ms, _ := mustPage(t, out).SearchText("Right side"); len(ms) != 1 {
				t.Errorf("the right cell's text was damaged: %d matches", len(ms))
			}
			for _, w := range []string{"KEYWORD", "WORD", "EYW"} {
				if ms, _ := mustPage(t, out).SearchText(w); len(ms) != 0 {
					t.Errorf("%q of the old text is still on the page", w)
				}
			}
			// The replacement's glyphs past the rule are hidden.
			lines, err := mustPage(t, out).ExtractTextWithLayout()
			if err != nil {
				t.Fatal(err)
			}
			hidden, shown := 0, 0
			for _, l := range lines {
				for _, f := range l.Fragments {
					k := 0
					for range f.Text {
						if k < len(f.runeX) && f.runeX[k] > 301 && strings.ContainsRune(repl, []rune(f.Text)[k]) && f.Text != "Right side" {
							if k < len(f.runeHidden) && f.runeHidden[k] {
								hidden++
							} else {
								shown++
							}
						}
						k++
					}
				}
			}
			if hidden == 0 || shown != 0 {
				t.Errorf("replacement past the rule: %d hidden, %d shown; want all hidden", hidden, shown)
			}
		})
	}
}

// The extractor marks the glyphs a rectangular clip hides, through q/Q, cm
// and forms, and leaves alone a clip it cannot bound.
func TestExtractMarksClippedGlyphs(t *testing.T) {
	content := func(ops string) *Document {
		d := NewDocumentFromFormat(PageFormatA4)
		p := mustPage(t, d)
		if err := p.AddTextAt("x", TextStyle{Font: FontHelvetica, Size: 12}, 10, 10); err != nil {
			t.Fatal(err)
		}
		if err := p.appendToContentStream([]byte(ops)); err != nil {
			t.Fatal(err)
		}
		return reopenDoc(t, d)
	}
	hiddenOf := func(d *Document, word string) []bool {
		lines, err := mustPage(t, d).ExtractTextWithLayout()
		if err != nil {
			t.Fatal(err)
		}
		for _, l := range lines {
			for _, f := range l.Fragments {
				if strings.Contains(f.Text, word) {
					out := make([]bool, len([]rune(f.Text)))
					for i := range out {
						out[i] = i < len(f.runeHidden) && f.runeHidden[i]
					}
					return out
				}
			}
		}
		t.Fatalf("%q not found", word)
		return nil
	}
	text := "BT /Helv 20 Tf 100 500 Td (ABCDEFGH) Tj ET"
	font := "" // AddTextAt registered Helvetica as a resource; find its name
	d0 := content("")
	for name := range mustPage(t, d0).pageResources()["/Font"].(pdfDict) {
		font = name
	}
	text = strings.Replace(text, "/Helv", font, 1)
	for _, c := range []struct {
		name, ops string
		want     string // per rune: h hidden, . shown
	}{
		{"none", text, "........"},
		{"clip", "q 0 0 140 800 re W n " + text + " Q", "...hhhhh"},
		{"restored", "q 0 0 140 800 re W n Q " + text, "........"},
		{"scaled", "q 2 0 0 2 0 0 cm 0 0 70 400 re W n 0.5 0 0 0.5 0 0 cm " + text + " Q", "...hhhhh"},
		{"curve", "q 0 0 m 140 0 l 140 800 l h W n " + text + " Q", "........"},
		{"nested", "q 0 0 140 800 re W n 0 0 300 800 re W n " + text + " Q", "...hhhhh"},
		{"mixed", "q 0 0 140 800 re 500 500 m 600 600 l h W n " + text + " Q", "........"},
		{"rotated", "q 0.7071 0.7071 -0.7071 0.7071 0 0 cm 0 0 200 200 re W n 1.4142 0 0 1.4142 0 0 cm " + text + " Q", "........"},
	} {
		t.Run(c.name, func(t *testing.T) {
			got := hiddenOf(content(c.ops), "ABCD")
			var b bytes.Buffer
			for _, h := range got {
				if h {
					b.WriteByte('h')
				} else {
					b.WriteByte('.')
				}
			}
			if b.String() != c.want {
				t.Errorf("hidden %s, want %s", b.String(), c.want)
			}
		})
	}
}
