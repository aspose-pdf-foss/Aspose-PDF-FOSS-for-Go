// SPDX-License-Identifier: MIT

package asposepdf

import "testing"

// mcPara builds a paragraph of lines at size 10: each line is (left, right)
// with text standing in for the words; text is the paragraph's text.
func mcPara(text string, size float64, lines ...[2]float64) MarkupParagraph {
	p := MarkupParagraph{Text: text}
	for i, l := range lines {
		p.Lines = append(p.Lines, TextLine{Y: 700 - 12*float64(i), Fragments: []TextFragment{{Text: "w", X: l[0], Width: l[1] - l[0], FontSize: size}}})
	}
	return p
}

// TestContinuesInto exercises each condition of the continuation rule on its
// own (pdf-go-6u85).
func TestContinuesInto(t *testing.T) {
	full := [2]float64{72, 272}
	for _, c := range []struct {
		name string
		a, b MarkupParagraph
		want bool
	}{
		{"cut mid-sentence", mcPara("runs on to the", 10, full, full), mcPara("Next part.", 10, full, full), true},
		{"lower case after a full stop", mcPara("ends here.", 10, full, full), mcPara("and goes on", 10, full, full), true},
		{"finished sentence, capital", mcPara("ends here.", 10, full, full), mcPara("New one", 10, full, full), false},
		{"short last line", mcPara("runs on to", 10, full, [2]float64{72, 200}), mcPara("and goes on", 10, full, full), false},
		{"indented first line", mcPara("runs on to", 10, full, full), mcPara("and goes on", 10, [2]float64{90, 272}, full), false},
		{"larger type", mcPara("runs on to", 10, full, full), mcPara("and goes on", 12, full, full), false},
		{"smaller type", mcPara("runs on to", 12, full, full), mcPara("and goes on", 10, full, full), false},
	} {
		a, b := c.a, c.b
		if got := continuesInto(&a, &b); got != c.want {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
}

// TestJoinContinuationsChain: a paragraph running through three columns keeps
// both continuations, and the test for the third uses the second part's
// lines.
func TestJoinContinuationsChain(t *testing.T) {
	full := [2]float64{72, 272}
	sec := func(p MarkupParagraph) MarkupSection {
		return MarkupSection{Paragraphs: []MarkupParagraph{p}}
	}
	pages := []PageMarkup{{PageNumber: 1, Sections: []MarkupSection{
		sec(mcPara("one runs", 10, full, full)),
		sec(mcPara("two runs", 10, full, full)),
		sec(mcPara("three ends.", 10, full, [2]float64{72, 120})),
		sec(mcPara("four", 10, full, full)),
	}}}
	out := joinContinuations(pages)
	if len(out[0].Sections) != 2 {
		t.Fatalf("%d sections, want 2", len(out[0].Sections))
	}
	p := out[0].Sections[0].Paragraphs[0]
	if len(p.Continuations) != 2 || p.Text != "one runs two runs three ends." || len(p.Lines) != 6 {
		t.Errorf("joined %q, %d continuations, %d lines", p.Text, len(p.Continuations), len(p.Lines))
	}
}
