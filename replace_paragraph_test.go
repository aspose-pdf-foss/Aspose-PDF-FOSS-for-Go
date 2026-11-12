// SPDX-License-Identifier: MIT

package asposepdf_test

import (
	"bytes"
	"math"
	"regexp"
	"strings"
	"testing"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

const replParaText = "The new text of the paragraph is longer than the old one and has to be wrapped over several lines of the rectangle it is laid out in, word by word."

// replParaDoc draws a three-line paragraph at 10pt Times, 14pt apart, from
// x=72 at baselines 700, 686, 672.
func replParaDoc(t *testing.T) (*pdf.Document, *pdf.Page) {
	t.Helper()
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	p, _ := doc.Page(1)
	style := pdf.TextStyle{Font: pdf.FontTimesRoman, Size: 10}
	for i, l := range []string{"Old paragraph opening line that runs", "on over a second line of about the", "same width and ends here."} {
		if err := p.AddTextAt(l, style, 72, 700-14*float64(i)); err != nil {
			t.Fatal(err)
		}
	}
	return doc, p
}

func spaced(text string) string {
	w := strings.Fields(text)
	for i := range w {
		w[i] = regexp.QuoteMeta(w[i])
	}
	return strings.Join(w, `\s+`)
}

// replacePara replaces the old paragraph with replParaText (or text), saves
// and reopens, returning the old paragraph's rectangle and the reopened page.
func replacePara(t *testing.T, text string, opts pdf.ReplaceOptions) (pdf.Rectangle, *pdf.Page) {
	t.Helper()
	doc, p := replParaDoc(t)
	old := `Old\s+paragraph(?s:.*)ends\s+here\.`
	ms, err := p.SearchText(old, pdf.SearchOptions{Regex: true})
	if err != nil || len(ms) != 1 {
		t.Fatalf("old paragraph found %d times, %v", len(ms), err)
	}
	opts.Regex = true
	if n, err := p.ReplaceText(old, text, opts); err != nil || n != 1 {
		t.Fatalf("ReplaceText = %d, %v", n, err)
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
	if s, _ := rp.ExtractText(); strings.Contains(s, "Old paragraph") {
		t.Errorf("old text left: %q", s)
	}
	return ms[0].Rect, rp
}

func findPara(t *testing.T, p *pdf.Page, text string) pdf.TextMatch {
	t.Helper()
	ms, err := p.SearchText(spaced(text), pdf.SearchOptions{Regex: true})
	if err != nil || len(ms) != 1 {
		t.Fatalf("replacement found %d times, %v", len(ms), err)
	}
	return ms[0]
}

// paraLines returns the replacement's lines: baseline, left, right and size.
func paraLines(t *testing.T, p *pdf.Page, m pdf.TextMatch) [][4]float64 {
	t.Helper()
	var out [][4]float64
	for _, s := range m.Segments {
		lines, _ := p.ExtractTextWithLayout()
		for _, l := range lines {
			if l.Y >= s.Rect.LLY && l.Y <= s.Rect.URY {
				f, g := l.Fragments[0], l.Fragments[len(l.Fragments)-1]
				out = append(out, [4]float64{l.Y, f.X, g.X + g.Width, f.FontSize})
				break
			}
		}
	}
	return out
}

func within(a, b, tol float64) bool { return math.Abs(a-b) <= tol }

// TestReplaceIntoRectangle: with ReplaceOptions.Rectangle the replacement is
// wrapped to the rectangle's width from its top-left corner, at the match's
// size and line pitch, and runs on below a rectangle too short for it.
func TestReplaceIntoRectangle(t *testing.T) {
	r := pdf.Rectangle{LLX: 200, LLY: 400, URX: 400, URY: 500}
	_, p := replacePara(t, replParaText, pdf.ReplaceOptions{Rectangle: &r})
	m := findPara(t, p, replParaText)
	if !within(m.Rect.URY, r.URY, 0.3) || !within(m.Rect.LLX, r.LLX, 0.3) || m.Rect.URX > r.URX+0.01 {
		t.Errorf("paragraph %+v in %+v", m.Rect, r)
	}
	lines := paraLines(t, p, m)
	if len(lines) < 3 {
		t.Fatalf("%d lines", len(lines))
	}
	for i, l := range lines {
		if l[3] != 10 {
			t.Errorf("line %d size %v, want 10", i, l[3])
		}
		if i > 0 && !within(lines[i-1][0]-l[0], 14, 0.01) {
			t.Errorf("line %d %.2f below the previous, want 14", i, lines[i-1][0]-l[0])
		}
	}

	small := pdf.Rectangle{LLX: 200, LLY: 480, URX: 400, URY: 500}
	_, p = replacePara(t, replParaText, pdf.ReplaceOptions{Rectangle: &small})
	if m := findPara(t, p, replParaText); m.Rect.LLY > small.LLY-5 {
		t.Errorf("text %+v kept to a rectangle %+v without FontSizeAdjustment", m.Rect, small)
	}
}

// TestReplaceShrinkAndScaleToFit: ShrinkToFit lowers the size until the text
// fits the rectangle, ScaleToFill raises it to the largest that fits.
func TestReplaceShrinkAndScaleToFit(t *testing.T) {
	small := pdf.Rectangle{LLX: 72, LLY: 650, URX: 272, URY: 680}
	_, p := replacePara(t, replParaText, pdf.ReplaceOptions{Rectangle: &small, FontSizeAdjustment: pdf.FontSizeAdjustmentShrinkToFit})
	m := findPara(t, p, replParaText)
	lines := paraLines(t, p, m)
	if m.Rect.LLY < small.LLY-0.01 || m.Rect.URY > small.URY+0.3 || lines[0][3] >= 10 {
		t.Errorf("shrunk paragraph %+v at %vpt in %+v", m.Rect, lines[0][3], small)
	}

	big := pdf.Rectangle{LLX: 72, LLY: 300, URX: 472, URY: 700}
	_, p = replacePara(t, replParaText, pdf.ReplaceOptions{Rectangle: &big, FontSizeAdjustment: pdf.FontSizeAdjustmentScaleToFill})
	m = findPara(t, p, replParaText)
	lines = paraLines(t, p, m)
	if lines[0][3] <= 10 || m.Rect.LLY < big.LLY-0.01 || m.Rect.LLY > big.LLY+lines[0][3]*1.2*1.05 || m.Rect.URX > big.URX+0.01 {
		t.Errorf("scaled paragraph %+v at %vpt in %+v", m.Rect, lines[0][3], big)
	}

	// Already fitting: ShrinkToFit leaves the size.
	_, p = replacePara(t, "Short.", pdf.ReplaceOptions{Rectangle: &big, FontSizeAdjustment: pdf.FontSizeAdjustmentShrinkToFit})
	if l := paraLines(t, p, findPara(t, p, "Short.")); l[0][3] != 10 {
		t.Errorf("fitting text resized to %v", l[0][3])
	}
}

// TestReplaceJustified: AdjustSpaceWidth spreads every line but the last to
// the rectangle's width (here the old paragraph's, widened by the edge
// adjustments), and the words still read as one text.
func TestReplaceJustified(t *testing.T) {
	old, p := replacePara(t, replParaText, pdf.ReplaceOptions{
		Adjustment:     pdf.ReplaceAdjustmentAdjustSpaceWidth | pdf.ReplaceAdjustmentWholeWordsHyphenation,
		LeftAdjustment: -10, RightAdjustment: 40,
	})
	m := findPara(t, p, replParaText)
	lines := paraLines(t, p, m)
	if len(lines) < 3 {
		t.Fatalf("%d lines", len(lines))
	}
	left, right := old.LLX-10, old.URX+40
	for i, l := range lines {
		if !within(l[1], left, 0.01) {
			t.Errorf("line %d starts at %.2f, want %.2f", i, l[1], left)
		}
		last := i == len(lines)-1
		if !last && !within(l[2], right, 0.05) {
			t.Errorf("line %d ends at %.2f, want %.2f", i, l[2], right)
		}
		if last && l[2] > right-1 {
			t.Errorf("last line justified: ends at %.2f", l[2])
		}
	}
	if !within(m.Rect.URY, old.URY, 0.3) {
		t.Errorf("paragraph top %.2f, old %.2f", m.Rect.URY, old.URY)
	}
}

// TestReplaceIntoFreeSpace: a paragraph adjustment on a match within one line
// lays the replacement out from the match to the page's right edge less the
// margin on the match's left.
func TestReplaceIntoFreeSpace(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4) // 595 wide
	p, _ := doc.Page(1)
	if err := p.AddTextAt("{{placeholder}}", pdf.TextStyle{Font: pdf.FontHelvetica, Size: 12}, 100, 700); err != nil {
		t.Fatal(err)
	}
	if n, err := p.ReplaceText("{{placeholder}}", replParaText, pdf.ReplaceOptions{Adjustment: pdf.ReplaceAdjustmentWholeWordsHyphenation}); err != nil || n != 1 {
		t.Fatalf("ReplaceText = %d, %v", n, err)
	}
	m := findPara(t, p, replParaText)
	if len(m.Segments) < 2 || !within(m.Rect.LLX, 100, 0.01) || m.Rect.URX > 595-100+0.01 || m.Rect.URX < 595-100-60 {
		t.Errorf("paragraph %+v in %d lines, want x 100..495", m.Rect, len(m.Segments))
	}
}
