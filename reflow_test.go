// SPDX-License-Identifier: MIT

package asposepdf

import (
	"math"
	"strings"
	"testing"
)

// reflowSource is a paragraph AddText wraps in a 300pt column at x 100,
// with a short placeholder in its first line.
const reflowSource = "Dear {{who}}, the terms of this agreement are set out below and apply from the day it is signed by both of the parties named."

const reflowName = "Alexandra Montgomery-Fitzwilliam of the Westminster Gardens Trust"

// reflowDoc is a page with reflowSource set in Helvetica 12 in the
// rectangle 100..400, from y 720 down, saved and reopened.
func reflowDoc(t *testing.T, text string) *Document {
	t.Helper()
	d := NewDocumentFromFormat(PageFormatA4)
	st := TextStyle{Font: FontHelvetica, Size: 12, LineSpacing: 1.5}
	if err := mustPage(t, d).AddText(text, st, Rectangle{LLX: 100, LLY: 500, URX: 400, URY: 720}); err != nil {
		t.Fatal(err)
	}
	return reopenDoc(t, d)
}

// textLines are the lines of page 1 whose text starts at or right of x0 and
// left of x1, top to bottom.
func textLines(t *testing.T, d *Document, x0, x1 float64) []TextLine {
	t.Helper()
	lines, err := mustPage(t, d).ExtractTextWithLayout()
	if err != nil {
		t.Fatal(err)
	}
	var out []TextLine
	for _, l := range lines {
		var keep TextLine
		keep.Y = l.Y
		for _, f := range l.Fragments {
			if f.X >= x0-1 && f.X < x1 {
				keep.Fragments = append(keep.Fragments, f)
			}
		}
		if len(keep.Fragments) > 0 {
			keep.Text = lineJoinedText(keep)
			out = append(out, keep)
		}
	}
	return out
}

// joined is the lines' text with white space collapsed.
func joined(lines []TextLine) string {
	var parts []string
	for _, l := range lines {
		parts = append(parts, l.Text)
	}
	return strings.Join(strings.Fields(strings.Join(parts, " ")), " ")
}

// pdf-go-1vv8: WholeWordsHyphenation re-wraps the paragraph the
// replacement lands in, so its words flow on after the new text within the
// paragraph's own width, starting where it started.
func TestReflowParagraph(t *testing.T) {
	want := strings.Replace(reflowSource, "{{who}}", reflowName, 1)
	for _, via := range []string{"ReplaceText", "Fragment"} {
		t.Run(via, func(t *testing.T) {
			d0 := reflowDoc(t, reflowSource)
			before := textLines(t, d0, 0, 600)
			d := reflowDoc(t, reflowSource)
			opts := ReplaceOptions{Adjustment: ReplaceAdjustmentWholeWordsHyphenation}
			if via == "ReplaceText" {
				if n, err := d.ReplaceText("{{who}}", reflowName, opts); err != nil || n != 1 {
					t.Fatalf("replace: %d, %v", n, err)
				}
			} else {
				f := absorbOne(t, d, "{{who}}")
				f.Text = reflowName
				f.ReplaceOptions = opts
				if err := d.ApplyTextChanges(); err != nil {
					t.Fatal(err)
				}
			}
			out := reopenDoc(t, d)
			after := textLines(t, out, 0, 600)
			if got := joined(after); got != want {
				t.Fatalf("paragraph reads\n%q\nwant\n%q", got, want)
			}
			if len(after) <= len(before) {
				t.Errorf("%d lines after, %d before: the longer text did not flow on", len(after), len(before))
			}
			maxR := 0.0
			for _, l := range before {
				_, r := reflowLineExtent(l)
				maxR = math.Max(maxR, r)
			}
			for i, l := range after {
				left, right := reflowLineExtent(l)
				if !nearPt(left, 100) {
					t.Errorf("line %d starts at %.2f, want 100", i, left)
				}
				if right > maxR+0.5 {
					t.Errorf("line %d ends at %.2f, past the paragraph's %.2f", i, right, maxR)
				}
				if i < len(before) && !nearPt(l.Y, before[i].Y) {
					t.Errorf("line %d on baseline %.2f, want the old pitch's %.2f", i, l.Y, before[i].Y)
				}
			}
		})
	}
}

// A list item is a paragraph of its own: only the item holding the
// replacement is re-wrapped, keeping its hanging indent.
func TestReflowListItem(t *testing.T) {
	build := func() *Document {
		d := NewDocumentFromFormat(PageFormatA4)
		p := mustPage(t, d)
		st := TextStyle{Font: FontHelvetica, Size: 12}
		items := []string{
			"(a) The first item of the list carries {{x}} and runs on over a second line here.",
			"(b) The second item of the list is left exactly where it was by the replacement.",
		}
		y := 720.0
		for _, it := range items {
			marker, body := it[:3], it[4:]
			if err := p.AddTextAt(marker, st, 100, y); err != nil {
				t.Fatal(err)
			}
			lines, _, err := wrapReplacement(FontHelvetica, 12, body, false, 250)
			if err != nil {
				t.Fatal(err)
			}
			for i, l := range lines {
				if err := p.AddTextAt(l.text, st, 125, y-float64(i)*14); err != nil {
					t.Fatal(err)
				}
			}
			y -= float64(len(lines)) * 14
		}
		return reopenDoc(t, d)
	}
	d0 := build()
	second := rectOf(t, d0, "(b)")
	d := build()
	if n, err := d.ReplaceText("{{x}}", "a much longer replacement than before", ReplaceOptions{Adjustment: ReplaceAdjustmentWholeWordsHyphenation}); err != nil || n != 1 {
		t.Fatalf("replace: %d, %v", n, err)
	}
	out := reopenDoc(t, d)
	if r := rectOf(t, out, "(b)"); r != second {
		t.Errorf("the next item moved: %+v, was %+v", r, second)
	}
	ms, _ := mustPage(t, out).SearchText("runs on over")
	if len(ms) != 1 {
		t.Fatal("the item's text is gone")
	}
	lines := textLines(t, out, 0, 600)
	if len(lines) < 2 || !nearPt(lines[1].Fragments[0].X, 125) {
		t.Errorf("the item's second line starts at %.2f, want the hanging indent 125", lines[1].Fragments[0].X)
	}
}

// In a ruled table the paragraph is the cell's: the next cell's text on the
// same baselines stays, and the paragraph wraps within its cell.
func TestReflowInCell(t *testing.T) {
	build := func() *Document {
		d := NewDocumentFromFormat(PageFormatA4)
		p := mustPage(t, d)
		st := TextStyle{Font: FontHelvetica, Size: 11}
		for i, l := range []string{"The cell holds {{x}} and a", "few more words that", "wrap."} {
			if err := p.AddTextAt(l, st, 55, 610-13*float64(i)); err != nil {
				t.Fatal(err)
			}
		}
		// The next row, a line's pitch below the paragraph, past a rule.
		if err := p.AddTextAt("Next row", st, 55, 571); err != nil {
			t.Fatal(err)
		}
		if err := p.AddText("Right cell text stays put.", st, Rectangle{LLX: 305, LLY: 560, URX: 545, URY: 625}); err != nil {
			t.Fatal(err)
		}
		ls := LineStyle{Width: 1}
		for _, x := range []float64{50, 300, 550} {
			if err := p.DrawLine(Point{X: x, Y: 520}, Point{X: x, Y: 630}, ls); err != nil {
				t.Fatal(err)
			}
		}
		for _, y := range []float64{520, 578, 630} {
			if err := p.DrawLine(Point{X: 50, Y: y}, Point{X: 550, Y: y}, ls); err != nil {
				t.Fatal(err)
			}
		}
		return reopenDoc(t, d)
	}
	d0 := build()
	right := rectOf(t, d0, "Right cell text stays put.")
	next := rectOf(t, d0, "Next row")
	srcR := 0.0
	for _, l := range textLines(t, d0, 0, 300) {
		if l.Y > 578 {
			_, r := reflowLineExtent(l)
			srcR = math.Max(srcR, r)
		}
	}
	d := build()
	if n, err := d.ReplaceText("{{x}}", "a replacement long enough to need another line", ReplaceOptions{Adjustment: ReplaceAdjustmentWholeWordsHyphenation}); err != nil || n != 1 {
		t.Fatalf("replace: %d, %v", n, err)
	}
	out := reopenDoc(t, d)
	if r := rectOf(t, out, "Right cell text stays put."); r != right {
		t.Errorf("the other cell moved: %+v, was %+v", r, right)
	}
	if r := rectOf(t, out, "Next row"); r != next {
		t.Errorf("the next row moved: %+v, was %+v", r, next)
	}
	var cell []TextLine
	for _, l := range textLines(t, out, 0, 300) {
		if l.Y > 578 {
			cell = append(cell, l)
		}
	}
	want := "The cell holds a replacement long enough to need another line and a few more words that wrap."
	if got := joined(cell); got != want {
		t.Errorf("cell reads %q, want %q", got, want)
	}
	widest := 0.0
	for _, l := range cell {
		_, r := reflowLineExtent(l)
		if r > 300 {
			t.Errorf("line %q runs to %.2f, past the rule at 300", l.Text, r)
		}
		widest = math.Max(widest, r)
	}
	if widest < srcR+20 {
		t.Errorf("the paragraph is wrapped at %.2f, its old width; want the cell's", widest)
	}
}

// A right-to-left paragraph is re-wrapped right to left: its lines keep
// its right edge and its words their order.
func TestReflowRTL(t *testing.T) {
	const src = "שלום {{x}} עולם יפה מאוד גדול וארוך שממשיך הלאה בשורה נוספת"
	build := func() *Document {
		d := NewDocumentFromFormat(PageFormatA4)
		font, err := d.LoadFont("testdata/DejaVuSans.ttf")
		if err != nil {
			t.Fatal(err)
		}
		st := TextStyle{Font: font, Size: 12, RTL: true}
		if err := mustPage(t, d).AddText(src, st, Rectangle{LLX: 300, LLY: 500, URX: 500, URY: 720}); err != nil {
			t.Fatal(err)
		}
		return reopenDoc(t, d)
	}
	d0 := build()
	before := textLines(t, d0, 0, 600)
	d := build()
	font, err := d.LoadFont("testdata/DejaVuSans.ttf")
	if err != nil {
		t.Fatal(err)
	}
	if n, err := d.ReplaceText("{{x}}", "עולםעולםעולם", ReplaceOptions{Adjustment: ReplaceAdjustmentWholeWordsHyphenation, Font: font}); err != nil || n != 1 {
		t.Fatalf("replace: %d, %v", n, err)
	}
	out := reopenDoc(t, d)
	after := textLines(t, out, 0, 600)
	want := strings.Replace(src, "{{x}}", "עולםעולםעולם", 1)
	var logical []string
	for _, l := range after {
		m := rtlFragmentRuneMap(&l, out.ownFont)
		logical = append(logical, string(m.text))
	}
	if got := strings.Join(strings.Fields(strings.Join(logical, " ")), " "); got != want {
		t.Errorf("paragraph reads\n%q\nwant\n%q", got, want)
	}
	_, r0 := reflowLineExtent(before[0])
	for i, l := range after {
		if _, r := reflowLineExtent(l); math.Abs(r-r0) > 1 {
			t.Errorf("line %d ends at %.2f, want the right edge %.2f", i, r, r0)
		}
	}
}

// Two font objects of one name in one extracted fragment (two subsets of a
// face): the absorber's segments each name their own object, so an edit
// draws in the font the text was shown in.
func TestSegmentsSplitByFontObject(t *testing.T) {
	d := NewDocumentFromFormat(PageFormatA4)
	p := mustPage(t, d)
	font := func() pdfRef {
		return pdfRef{Num: d.addObject(pdfDict{"/Type": pdfName("/Font"), "/Subtype": pdfName("/Type1"), "/BaseFont": pdfName("/Helvetica")})}
	}
	p.pageDict()["/Resources"] = pdfDict{"/Font": pdfDict{"/F1": font(), "/F2": font()}}
	if err := p.appendToContentStream([]byte("BT /F1 12 Tf 100 700 Td (abc) Tj /F2 12 Tf (def) Tj ET\n")); err != nil {
		t.Fatal(err)
	}
	d = reopenDoc(t, d)
	f := absorbOne(t, d, "abcdef")
	if len(f.Segments) != 2 {
		t.Fatalf("%d segments, want one per font object", len(f.Segments))
	}
	a, _ := f.Segments[0].TextState.Font.(documentFont)
	b, _ := f.Segments[1].TextState.Font.(documentFont)
	if a.ref == 0 || b.ref == 0 || a.ref == b.ref {
		t.Errorf("segment fonts %d and %d, want two distinct objects", a.ref, b.ref)
	}
}

// Text shaped into a document is read back before a save: the glyphs that
// shaping substituted map to their text at once.
func TestShapedTextReadBeforeSave(t *testing.T) {
	d := NewDocumentFromFormat(PageFormatA4)
	font, err := d.LoadFont("testdata/DejaVuSans.ttf")
	if err != nil {
		t.Fatal(err)
	}
	// DejaVu Sans draws "ffi" as one glyph through its liga feature.
	if err := mustPage(t, d).AddTextAt("office affine", TextStyle{Font: font, Size: 14}, 100, 700); err != nil {
		t.Fatal(err)
	}
	// Search (the layout pipeline) first: a read refreshes the fonts.
	if ms, err := mustPage(t, d).SearchText("office"); err != nil || len(ms) != 1 {
		t.Errorf("search before saving: %d matches, %v", len(ms), err)
	}
	txt, err := mustPage(t, d).ExtractText()
	if err != nil {
		t.Fatal(err)
	}
	if strings.ContainsRune(txt, 0xFFFD) || !strings.Contains(txt, "office affine") {
		t.Errorf("text read before saving: %q", txt)
	}
}

// A right-to-left word is measured as it is drawn, right to left.
func TestRunTokenWidthRTL(t *testing.T) {
	d := NewDocumentFromFormat(PageFormatA4)
	font, err := d.LoadFont("testdata/DejaVuSans.ttf")
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{"في", "عليكم", "سلام"} {
		st := TextStyle{Font: font, Size: 11}
		w, err := MeasureText(s, st)
		if err != nil {
			t.Fatal(err)
		}
		m, err := metricsFor(st, s)
		if err != nil {
			t.Fatal(err)
		}
		if got := tokenWidth([]runMetrics{m}, 0, s); math.Abs(got-w) > 0.01 {
			t.Errorf("%q: token width %.2f, drawn %.2f", s, got, w)
		}
	}
}

// A justified paragraph stays justified, stretched at its word spaces only:
// text drawn in another run right after a word (a comma after a bold name)
// keeps to it.
func TestReflowJustified(t *testing.T) {
	build := func() *Document {
		d := NewDocumentFromFormat(PageFormatA4)
		f := NewTextFragment("")
		f.Segments = []*TextSegment{
			{Text: "Dear "},
			{Text: "{{who}}", TextState: TextState{FontStyle: FontStyleBold}},
			{Text: ", the terms of this agreement are set out below and apply from the day it is signed by both of the parties named in it, and by no one else at all."},
		}
		f.TextState = TextState{Font: FontHelvetica, FontSize: 12}
		tp := NewTextParagraph()
		tp.Rectangle = Rectangle{LLX: 100, LLY: 500, URX: 400, URY: 720}
		tp.Justify = true
		tp.AppendLine(f)
		if err := NewTextBuilder(mustPage(t, d)).AppendParagraph(tp); err != nil {
			t.Fatal(err)
		}
		return reopenDoc(t, d)
	}
	d := build()
	if n, err := d.ReplaceText("{{who}}", "Alexandra Montgomery", ReplaceOptions{Adjustment: ReplaceAdjustmentWholeWordsHyphenation}); err != nil || n != 1 {
		t.Fatalf("replace: %d, %v", n, err)
	}
	out := reopenDoc(t, d)
	if ms, _ := mustPage(t, out).SearchText("Montgomery,"); len(ms) != 1 {
		t.Error("the comma after the name drifted away from it")
	}
	lines := textLines(t, out, 0, 600)
	if len(lines) < 3 {
		t.Fatalf("%d lines", len(lines))
	}
	for i, l := range lines[:len(lines)-1] {
		if _, r := reflowLineExtent(l); !nearPt(r, 400) && r < 398 {
			t.Errorf("line %d ends at %.2f: not justified to 400", i, r)
		}
	}
}

// Lines set far apart (one placeholder to a line, as on a form) are not one
// paragraph: replacing one leaves the next where it was.
func TestReflowWideSpacedLines(t *testing.T) {
	build := func() *Document {
		d := NewDocumentFromFormat(PageFormatA4)
		p := mustPage(t, d)
		st := TextStyle{Font: FontHelvetica, Size: 12}
		for i, s := range []string{"{{one}} and its tail", "{{two}}", "{{three}}"} {
			if err := p.AddTextAt(s, st, 100, 700-45*float64(i)); err != nil {
				t.Fatal(err)
			}
		}
		return reopenDoc(t, d)
	}
	d0 := build()
	two := rectOf(t, d0, "{{two}}")
	d := build()
	if n, err := d.ReplaceText("{{one}}", "a replacement long enough to wrap onto more than one line of the free space on this page, which is wide", ReplaceOptions{Adjustment: ReplaceAdjustmentWholeWordsHyphenation}); err != nil || n != 1 {
		t.Fatalf("replace: %d, %v", n, err)
	}
	out := reopenDoc(t, d)
	if r := rectOf(t, out, "{{two}}"); r != two {
		t.Errorf("the next line moved: %+v, was %+v", r, two)
	}
	if ms, _ := mustPage(t, out).SearchText("and its tail"); len(ms) != 1 {
		t.Error("the replaced line's own text is gone")
	}
}

// A replacement covering its whole paragraph is laid out as one alone is:
// its text box keeps the paragraph's top.
func TestReflowWholeParagraph(t *testing.T) {
	d := NewDocumentFromFormat(PageFormatA4)
	if err := mustPage(t, d).AddText(reflowSource, TextStyle{Font: FontCourier, Size: 12}, Rectangle{LLX: 100, LLY: 500, URX: 400, URY: 720}); err != nil {
		t.Fatal(err)
	}
	d = reopenDoc(t, d)
	top := 0.0
	for _, l := range textLines(t, d, 0, 600) {
		top = math.Max(top, l.Y)
	}
	ms, err := mustPage(t, d).SearchText(`Dear[\s\S]*named\.`, SearchOptions{Regex: true})
	if err != nil || len(ms) != 1 {
		t.Fatalf("paragraph: %d matches, %v", len(ms), err)
	}
	want := ms[0].Rect.URY
	if n, err := d.ReplaceText(`Dear[\s\S]*named\.`, "A new paragraph in place of the old one.", ReplaceOptions{Regex: true, Adjustment: ReplaceAdjustmentWholeWordsHyphenation, Font: FontHelvetica}); err != nil || n != 1 {
		t.Fatalf("replace: %d, %v", n, err)
	}
	r := rectOf(t, reopenDoc(t, d), "A new paragraph in place of the old one.")
	if math.Abs(r.URY-want) > 0.5 {
		t.Errorf("text box top %.2f, want the paragraph's %.2f", r.URY, want)
	}
}
