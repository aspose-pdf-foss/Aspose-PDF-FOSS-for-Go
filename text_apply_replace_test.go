// SPDX-License-Identifier: MIT

package asposepdf

import (
	"strings"
	"testing"
)

// oneLineDoc is a page with text drawn as one line at (100, 600) in
// Helvetica 12, saved and reopened so its words are found.
func oneLineDoc(t *testing.T, text string) *Document {
	t.Helper()
	d := NewDocumentFromFormat(PageFormatA4)
	if err := mustPage(t, d).AddTextAt(text, TextStyle{Font: FontHelvetica, Size: 12}, 100, 600); err != nil {
		t.Fatal(err)
	}
	return reopenDoc(t, d)
}

// startOf is where word starts on page 1.
func startOf(t *testing.T, d *Document, word string) float64 {
	t.Helper()
	ms, err := mustPage(t, d).SearchText(word)
	if err != nil || len(ms) != 1 {
		t.Fatalf("%q: %d matches, %v", word, len(ms), err)
	}
	return ms[0].Rect.LLX
}

// helvWidth is text's width in Helvetica 12.
func helv12Width(t *testing.T, text string) float64 {
	t.Helper()
	w, err := MeasureText(text, TextStyle{Font: FontHelvetica, Size: 12})
	if err != nil {
		t.Fatal(err)
	}
	return w
}

// pdf-go-xiru: ShiftRestOfLine on a found fragment moves the rest of its
// line by the change in width; without it the line stays.
func TestFragmentReplaceOptionsShiftRestOfLine(t *testing.T) {
	const line = "Hello Bob, how are you"
	before := startOf(t, oneLineDoc(t, line), "how")
	grow := helv12Width(t, "Roberta") - helv12Width(t, "Bob")
	for _, c := range []struct {
		name  string
		adj   ReplaceAdjustment
		moved float64
	}{
		{"ShiftRestOfLine", ReplaceAdjustmentShiftRestOfLine, grow},
		{"None", ReplaceAdjustmentNone, 0},
	} {
		t.Run(c.name, func(t *testing.T) {
			d := oneLineDoc(t, line)
			f := absorbOne(t, d, "Bob")
			f.Text = "Roberta"
			f.ReplaceOptions.Adjustment = c.adj
			out := reopenDoc(t, d)
			if got := startOf(t, out, "how") - before; !nearPt(got, c.moved) {
				t.Errorf("\"how\" moved by %.2f, want %.2f", got, c.moved)
			}
			if !nearPt(startOf(t, out, "Hello"), 100) {
				t.Error("text before the fragment moved")
			}
			if n := absorbCount(t, out, "Hello Roberta, how are you"); c.adj == ReplaceAdjustmentShiftRestOfLine && n != 1 {
				t.Errorf("the shifted line reads differently (%d matches of the whole line)", n)
			}
		})
	}
	// The absorber's TextReplaceOptions reach every fragment it finds, and
	// a shorter text pulls the rest of the line in.
	d := oneLineDoc(t, line)
	a := NewTextFragmentAbsorber("Bob")
	a.TextReplaceOptions.Adjustment = ReplaceAdjustmentShiftRestOfLine
	if err := a.VisitDocument(d); err != nil {
		t.Fatal(err)
	}
	f := a.TextFragments()[0]
	if f.ReplaceOptions.Adjustment != ReplaceAdjustmentShiftRestOfLine {
		t.Fatalf("fragment ReplaceOptions %+v", f.ReplaceOptions)
	}
	f.Text = "B"
	shrink := helv12Width(t, "B") - helv12Width(t, "Bob")
	if got := startOf(t, reopenDoc(t, d), "how") - before; !nearPt(got, shrink) {
		t.Errorf("\"how\" moved by %.2f, want %.2f", got, shrink)
	}
}

// Two fragments on one line edited together: the second's new text moves
// with the first's shift; a fragment of the line not edited cannot be
// changed afterwards (its text has moved).
func TestFragmentReplaceOptionsSameLine(t *testing.T) {
	const line = "aa bb cc dd"
	d0 := oneLineDoc(t, line)
	bbBefore, ddBefore := startOf(t, d0, "bb"), startOf(t, d0, "dd")
	d := oneLineDoc(t, line)
	a := NewTextFragmentAbsorber(`aa|bb|dd`, SearchOptions{Regex: true})
	a.TextReplaceOptions.Adjustment = ReplaceAdjustmentShiftRestOfLine
	if err := a.VisitDocument(d); err != nil || len(a.TextFragments()) != 3 {
		t.Fatalf("visit: %d fragments, %v", len(a.TextFragments()), err)
	}
	frs := a.TextFragments()
	frs[0].Text = "aaaa"
	frs[1].Text = "BBBB"
	if err := d.ApplyTextChanges(); err != nil {
		t.Fatal(err)
	}
	growA := helv12Width(t, "aaaa") - helv12Width(t, "aa")
	growB := helv12Width(t, "BBBB") - helv12Width(t, "bb")
	if got := startOf(t, d, "BBBB") - bbBefore; !nearPt(got, growA) {
		t.Errorf("BBBB starts %.2f after bb did, want %.2f", got, growA)
	}
	if got := startOf(t, d, "dd") - ddBefore; !nearPt(got, growA+growB) {
		t.Errorf("dd moved by %.2f, want %.2f", got, growA+growB)
	}
	frs[2].Text = "DD"
	if err := d.ApplyTextChanges(); err == nil || !strings.Contains(err.Error(), "rewritten") {
		t.Errorf("editing a fragment of the shifted line: %v, want a refusal", err)
	}
}

// The paragraph adjustments lay the new text out in the free space on the
// right, wrapped at whole words, as ReplaceText does; the fragment is not
// edited again after that.
func TestFragmentReplaceOptionsParagraph(t *testing.T) {
	d := oneLineDoc(t, "Name: PLACEHOLDER")
	f := absorbOne(t, d, "PLACEHOLDER")
	long := strings.Repeat("several words of replacement text ", 12)
	f.Text = strings.TrimSpace(long)
	f.ReplaceOptions.Adjustment = ReplaceAdjustmentWholeWordsHyphenation
	if err := d.ApplyTextChanges(); err != nil {
		t.Fatal(err)
	}
	lines, err := mustPage(t, reopenDoc(t, d)).ExtractTextWithLayout()
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, l := range lines {
		if strings.Contains(l.Text, "replacement") {
			n++
			for _, fr := range l.Fragments {
				if fr.X+fr.Width > 595-100+1 {
					t.Errorf("line %q reaches %.1f, past the free space", l.Text, fr.X+fr.Width)
				}
			}
		}
	}
	if n < 3 {
		t.Errorf("the replacement takes %d lines, want it wrapped over several", n)
	}
	f.Text = "again"
	if err := d.ApplyTextChanges(); err == nil || !strings.Contains(err.Error(), "paragraph") {
		t.Errorf("a second edit: %v, want a refusal", err)
	}
	// An adjustment ReplaceText refuses is refused here too, before the
	// page changes.
	d = oneLineDoc(t, "Name: PLACEHOLDER")
	f = absorbOne(t, d, "PLACEHOLDER")
	f.Text = "x"
	f.ReplaceOptions.Adjustment = ReplaceAdjustment(64)
	if err := d.ApplyTextChanges(); err == nil {
		t.Error("an unsupported adjustment was accepted")
	}
}

// On a right-to-left line what follows the fragment lies to its left: it
// moves left as the new text grows, which keeps the old right edge.
func TestFragmentReplaceOptionsShiftRTL(t *testing.T) {
	const line = "שלום עולם יפה"
	rect := func(d *Document, w string) Rectangle {
		ms, err := mustPage(t, d).SearchText(w)
		if err != nil || len(ms) != 1 {
			t.Fatalf("%q: %d matches, %v", w, len(ms), err)
		}
		return ms[0].Rect
	}
	d0 := openBytes(t, dejaVuDoc(t, line))
	first, mid, last := rect(d0, "שלום"), rect(d0, "עולם"), rect(d0, "יפה")
	d := openBytes(t, dejaVuDoc(t, line))
	f := absorbOne(t, d, "עולם")
	f.Text = "עולםעולם" // letters the subset has
	f.ReplaceOptions.Adjustment = ReplaceAdjustmentShiftRestOfLine
	out := reopenDoc(t, d)
	grow := (rect(out, "עולםעולם").URX - rect(out, "עולםעולם").LLX) - (mid.URX - mid.LLX)
	if grow < 10 {
		t.Fatalf("the new text grew by %.2f", grow)
	}
	if r := rect(out, "עולםעולם"); !nearPt(r.URX, mid.URX) {
		t.Errorf("new text ends at %.2f, want the old right edge %.2f", r.URX, mid.URX)
	}
	if r := rect(out, "יפה"); !nearPt(r.URX, last.URX-grow) {
		t.Errorf("the text to the left ends at %.2f, want %.2f (moved left by %.2f)", r.URX, last.URX-grow, grow)
	}
	if r := rect(out, "שלום"); !nearPt(r.LLX, first.LLX) {
		t.Errorf("the text to the right moved: %.2f, was %.2f", r.LLX, first.LLX)
	}
}

// A fragment found on a line that a shift moved, far from the edited text,
// cannot be changed afterwards: its text has moved.
func TestFragmentReplaceOptionsShiftedLineIsStale(t *testing.T) {
	d := oneLineDoc(t, "aa bb cc dd ee ff gg")
	a := NewTextFragmentAbsorber(`aa|gg`, SearchOptions{Regex: true})
	a.TextReplaceOptions.Adjustment = ReplaceAdjustmentShiftRestOfLine
	if err := a.VisitDocument(d); err != nil || len(a.TextFragments()) != 2 {
		t.Fatalf("visit: %d fragments, %v", len(a.TextFragments()), err)
	}
	frs := a.TextFragments()
	frs[0].Text = "aaa"
	if err := d.ApplyTextChanges(); err != nil {
		t.Fatal(err)
	}
	frs[1].Text = "GG"
	if err := d.ApplyTextChanges(); err == nil || !strings.Contains(err.Error(), "rewritten") {
		t.Errorf("editing a fragment of the shifted line: %v, want a refusal", err)
	}
	// A fragment on another line is not affected.
	d = NewDocumentFromFormat(PageFormatA4)
	p := mustPage(t, d)
	for i, s := range []string{"aa bb", "other line"} {
		if err := p.AddTextAt(s, TextStyle{Font: FontHelvetica, Size: 12}, 100, 600-float64(40*i)); err != nil {
			t.Fatal(err)
		}
	}
	d = reopenDoc(t, d)
	b := NewTextFragmentAbsorber(`aa|other`, SearchOptions{Regex: true})
	b.TextReplaceOptions.Adjustment = ReplaceAdjustmentShiftRestOfLine
	if err := b.VisitDocument(d); err != nil || len(b.TextFragments()) != 2 {
		t.Fatalf("visit: %d fragments, %v", len(b.TextFragments()), err)
	}
	b.TextFragments()[0].Text = "aaaa"
	if err := d.ApplyTextChanges(); err != nil {
		t.Fatal(err)
	}
	b.TextFragments()[1].Text = "another"
	if err := d.ApplyTextChanges(); err != nil {
		t.Errorf("editing a fragment on another line: %v", err)
	}
}

// On a page without rules, text set apart by a gap wider than three ems is
// another column: a fragment's shift moves the rest of its own text, and
// stops before the next column, as Aspose's does.
func TestFragmentShiftStopsAtColumnGap(t *testing.T) {
	d0 := NewDocumentFromFormat(PageFormatA4)
	p := mustPage(t, d0)
	st := TextStyle{Font: FontHelvetica, Size: 12}
	if err := p.AddTextAt("Name: Bob here", st, 100, 600); err != nil {
		t.Fatal(err)
	}
	if err := p.AddTextAt("Label", st, 300, 600); err != nil {
		t.Fatal(err)
	}
	d := reopenDoc(t, d0)
	here, label := startOf(t, d, "here"), startOf(t, d, "Label")
	f := absorbOne(t, d, "Bob")
	f.Text = "Roberta Longname"
	f.ReplaceOptions.Adjustment = ReplaceAdjustmentShiftRestOfLine
	out := reopenDoc(t, d)
	grow := helv12Width(t, "Roberta Longname") - helv12Width(t, "Bob")
	if got := startOf(t, out, "here") - here; !nearPt(got, grow) {
		t.Errorf("\"here\" moved by %.2f, want %.2f", got, grow)
	}
	if got := startOf(t, out, "Label") - label; !nearPt(got, 0) {
		t.Errorf("the next column moved by %.2f", got)
	}
}

// Right to left the shift moves what lies left of the fragment, and stops
// before a column further left.
func TestFragmentShiftRTLStopsAtColumnGap(t *testing.T) {
	d0 := NewDocumentFromFormat(PageFormatA4)
	p := mustPage(t, d0)
	font, err := d0.LoadFont("testdata/DejaVuSans.ttf")
	if err != nil {
		t.Fatal(err)
	}
	f0 := NewTextFragment("שלום עולם יפה")
	f0.Position = Position{XIndent: 250, YIndent: 600}
	f0.TextState = TextState{Font: font, FontSize: 14}
	if err := NewTextBuilder(p).AppendText(f0); err != nil {
		t.Fatal(err)
	}
	if err := p.AddTextAt("Label", TextStyle{Font: FontHelvetica, Size: 14}, 60, 600); err != nil {
		t.Fatal(err)
	}
	if _, err := d0.SubsetFonts(); err != nil {
		t.Fatal(err)
	}
	d := openBytes(t, writeBytes(t, d0))
	last, label := startOf(t, d, "יפה"), startOf(t, d, "Label")
	f := absorbOne(t, d, "עולם")
	f.Text = "עולםעולם"
	f.ReplaceOptions.Adjustment = ReplaceAdjustmentShiftRestOfLine
	out := reopenDoc(t, d)
	if got := startOf(t, out, "יפה") - last; got > -10 {
		t.Errorf("the text to the left moved by %.2f, want it moved left", got)
	}
	if got := startOf(t, out, "Label") - label; !nearPt(got, 0) {
		t.Errorf("the column on the left moved by %.2f", got)
	}
}
