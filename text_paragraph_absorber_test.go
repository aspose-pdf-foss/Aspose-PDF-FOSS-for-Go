// SPDX-License-Identifier: MIT

package asposepdf_test

import (
	"bytes"
	"math"
	"strings"
	"testing"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

// alignDoc draws a ragged left-aligned paragraph of four lines inside the
// box x 72..372, the third line ending the source's paragraph, and a label at
// x 450 on the first line's baseline, outside the box.
func alignDoc(t *testing.T) (*pdf.Document, *pdf.Page, pdf.Rectangle) {
	t.Helper()
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	p, _ := doc.Page(1)
	style := pdf.TextStyle{Font: pdf.FontHelvetica, Size: 10}
	for i, l := range []string{
		"The quick brown fox jumps over the lazy dog and",
		"keeps running across the wide green field until",
		"it stops.",
		"A new paragraph starts here and goes on.",
	} {
		if err := p.AddTextAt(l, style, 72, 700-12*float64(i)); err != nil {
			t.Fatal(err)
		}
	}
	if err := p.AddTextAt("Label", style, 450, 700); err != nil {
		t.Fatal(err)
	}
	// The box starts 0.3pt right of the text, as a clip rectangle drawn flush
	// with it may.
	return doc, p, pdf.Rectangle{LLX: 72.3, LLY: 650, URX: 372, URY: 712}
}

// alignedLines aligns the paragraph in the box, saves, reopens, and returns
// the left and right edge of each of the paragraph's lines plus the label.
func alignedLines(t *testing.T, a pdf.HorizontalAlignment) ([][2]float64, pdf.Rectangle, pdf.Rectangle) {
	t.Helper()
	doc, p, box := alignDoc(t)
	abs := pdf.NewTextParagraphAbsorber(box)
	if err := abs.Visit(p); err != nil {
		t.Fatal(err)
	}
	ps := abs.TextParagraphs()
	if len(ps) != 1 || len(ps[0].Lines()) != 4 || ps[0].Lines()[2] != "it stops." {
		t.Fatalf("paragraphs %d, lines %q", len(ps), ps[0].Lines())
	}
	if err := ps[0].SetHorizontalAlignment(a); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	re, err := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	rp, _ := re.Page(1)
	text, _ := rp.ExtractText()
	for _, w := range []string{"The quick brown fox jumps over the lazy dog and", "it stops.", "A new paragraph starts here and goes on."} {
		if !strings.Contains(strings.Join(strings.Fields(text), " "), w) {
			t.Errorf("text %q lost %q", text, w)
		}
	}
	var out [][2]float64
	for _, first := range []string{"The quick", "keeps running", "it stops", "A new paragraph"} {
		ms, _ := rp.SearchText(first)
		if len(ms) != 1 {
			t.Fatalf("%q found %d times", first, len(ms))
		}
		lines, _ := rp.ExtractTextWithLayout()
		for _, l := range lines {
			if math.Abs(l.Y-ms[0].Rect.LLY) < 12 && strings.Contains(l.Text, first) {
				lo, hi := math.Inf(1), math.Inf(-1)
				for _, f := range l.Fragments {
					if strings.TrimSpace(f.Text) == "" || f.X > 400 {
						continue
					}
					lo, hi = math.Min(lo, f.X), math.Max(hi, f.X+f.Width)
				}
				out = append(out, [2]float64{lo, hi})
				break
			}
		}
	}
	label, _ := rp.SearchText("Label")
	return out, label[0].Rect, box
}

func near(a, b float64) bool { return math.Abs(a-b) < 0.05 }

// TestTextParagraphAlignment: each alignment moves the lines within the box
// and nothing outside it; Justify leaves the last line and a line the source
// ended a paragraph on, FullJustify spreads them too (pdf-go-v0v4).
func TestTextParagraphAlignment(t *testing.T) {
	orig, _, _ := alignedLines(t, pdf.HorizontalAlignmentNone)
	for _, c := range []struct {
		a     pdf.HorizontalAlignment
		check func(i int, l, o [2]float64, box pdf.Rectangle) bool
	}{
		{pdf.HorizontalAlignmentLeft, func(i int, l, o [2]float64, box pdf.Rectangle) bool {
			return near(l[0], box.LLX) && near(l[1]-l[0], o[1]-o[0])
		}},
		{pdf.HorizontalAlignmentRight, func(i int, l, o [2]float64, box pdf.Rectangle) bool {
			return near(l[1], box.URX) && near(l[1]-l[0], o[1]-o[0])
		}},
		{pdf.HorizontalAlignmentCenter, func(i int, l, o [2]float64, box pdf.Rectangle) bool {
			return near((l[0]+l[1])/2, (box.LLX+box.URX)/2) && near(l[1]-l[0], o[1]-o[0])
		}},
		{pdf.HorizontalAlignmentJustify, func(i int, l, o [2]float64, box pdf.Rectangle) bool {
			if i == 2 || i == 3 { // a paragraph's end and the last line
				return near(l[0], box.LLX) && near(l[1]-l[0], o[1]-o[0])
			}
			return near(l[0], box.LLX) && near(l[1], box.URX)
		}},
		{pdf.HorizontalAlignmentFullJustify, func(i int, l, o [2]float64, box pdf.Rectangle) bool {
			return near(l[0], box.LLX) && near(l[1], box.URX)
		}},
	} {
		lines, label, box := alignedLines(t, c.a)
		for i, l := range lines {
			if !c.check(i, l, orig[i], box) {
				t.Errorf("alignment %d line %d: %.2f..%.2f (was %.2f..%.2f)", c.a, i, l[0], l[1], orig[i][0], orig[i][1])
			}
		}
		if !near(label.LLX, 450) {
			t.Errorf("alignment %d moved the label outside the box to %.2f", c.a, label.LLX)
		}
	}
}

// TestTextParagraphAbsorberParagraphs: without rectangles the absorber takes
// the paragraphs Page.Paragraphs finds; an unknown alignment is refused.
func TestTextParagraphAbsorberParagraphs(t *testing.T) {
	_, p, _ := alignDoc(t)
	abs := pdf.NewTextParagraphAbsorber()
	if err := abs.Visit(p); err != nil {
		t.Fatal(err)
	}
	if len(abs.TextParagraphs()) == 0 {
		t.Fatal("no paragraphs")
	}
	tp := abs.TextParagraphs()[0]
	if !strings.HasPrefix(tp.Text(), "The quick brown fox") || tp.Page() != p {
		t.Errorf("first paragraph %q", tp.Text())
	}
	if err := tp.SetHorizontalAlignment(pdf.HorizontalAlignment(9)); err == nil {
		t.Error("alignment 9 accepted")
	}
	if err := (&pdf.TextParagraph{}).SetHorizontalAlignment(pdf.HorizontalAlignmentLeft); err == nil {
		t.Error("a paragraph not found by an absorber was accepted")
	}
}
