// SPDX-License-Identifier: MIT

package asposepdf

import (
	"math"
	"strings"
	"testing"
)

func replaceAdjFixture(t *testing.T, lines ...string) (*Document, *Page) {
	t.Helper()
	doc := NewDocumentFromFormat(PageFormatA4)
	p, _ := doc.Page(1)
	for i, l := range lines {
		y := 700 - float64(i)*30
		if err := p.AddText(l, TextStyle{Font: FontHelvetica, Size: 12}, Rectangle{LLX: 72, LLY: y, URX: 560, URY: y + 20}); err != nil {
			t.Fatal(err)
		}
	}
	return doc, p
}

func findOne(t *testing.T, p *Page, s string) Rectangle {
	t.Helper()
	ms, err := p.SearchText(s)
	if err != nil || len(ms) != 1 {
		t.Fatalf("SearchText(%q): %v, %d matches", s, err, len(ms))
	}
	return ms[0].Rect
}

func helvWidth(t *testing.T, s string) float64 {
	t.Helper()
	w, _, err := fontWidthAndAscent(FontHelvetica, 12)
	if err != nil {
		t.Fatal(err)
	}
	return measureString(s, w)
}

func TestReplaceShiftRestOfLineLonger(t *testing.T) {
	_, p := replaceAdjFixture(t, "Hello World and more text", "Second line stays put")
	andBefore := findOne(t, p, "and")
	// "put" sits right of the replaced word, on the next line: it must stay.
	secondBefore := findOne(t, p, "put")

	n, err := p.ReplaceText("World", "Wonderful Universe", ReplaceOptions{Adjustment: ReplaceAdjustmentShiftRestOfLine})
	if err != nil || n != 1 {
		t.Fatalf("ReplaceText: %d, %v", n, err)
	}
	text, _ := p.ExtractText()
	if !strings.Contains(text, "Hello Wonderful Universe and more text") {
		t.Errorf("text = %q", text)
	}
	delta := helvWidth(t, "Wonderful Universe") - helvWidth(t, "World")
	andAfter := findOne(t, p, "and")
	if math.Abs(andAfter.LLX-(andBefore.LLX+delta)) > 0.5 {
		t.Errorf("'and' at x=%.2f, want %.2f (shifted by %.2f)", andAfter.LLX, andBefore.LLX+delta, delta)
	}
	if u := findOne(t, p, "Universe"); u.URX > andAfter.LLX {
		t.Errorf("replacement (ends %.2f) overlaps 'and' (starts %.2f)", u.URX, andAfter.LLX)
	}
	if got := findOne(t, p, "put"); math.Abs(got.LLX-secondBefore.LLX) > 1e-6 || math.Abs(got.LLY-secondBefore.LLY) > 1e-6 {
		t.Errorf("other line moved: %+v → %+v", secondBefore, got)
	}
}

func TestReplaceShiftRestOfLineShorterAndRepeated(t *testing.T) {
	_, p := replaceAdjFixture(t, "aaa XXXXXXXX bbb XXXXXXXX ccc")
	cBefore := findOne(t, p, "ccc")
	n, err := p.ReplaceText("XXXXXXXX", "y", ReplaceOptions{Adjustment: ReplaceAdjustmentShiftRestOfLine})
	if err != nil || n != 2 {
		t.Fatalf("ReplaceText: %d, %v", n, err)
	}
	text, _ := p.ExtractText()
	if !strings.Contains(text, "aaa y bbb y ccc") {
		t.Errorf("text = %q", text)
	}
	delta := 2 * (helvWidth(t, "y") - helvWidth(t, "XXXXXXXX"))
	if c := findOne(t, p, "ccc"); math.Abs(c.LLX-(cBefore.LLX+delta)) > 0.5 {
		t.Errorf("'ccc' at %.2f, want %.2f — both shifts must accumulate", c.LLX, cBefore.LLX+delta)
	}
	// The second replacement moved with the first shift.
	ys, _ := p.SearchText("y")
	b := findOne(t, p, "bbb")
	var second *Rectangle
	for i := range ys {
		if ys[i].Rect.LLX > b.LLX {
			second = &ys[i].Rect
		}
	}
	if second == nil || second.LLX < b.URX || second.LLX-b.URX > helvWidth(t, " ")+0.5 {
		t.Errorf("second replacement at %+v, want right after 'bbb ' (ends %.2f)", second, b.URX)
	}
}

// A line drawn by several operators, each positioned on its own: every
// piece after the match moves.
func TestReplaceShiftRestOfLineAcrossOperators(t *testing.T) {
	doc := NewDocumentFromFormat(PageFormatA4)
	p, _ := doc.Page(1)
	f, err := p.ensureStandardFontResource("/Helvetica")
	if err != nil {
		t.Fatal(err)
	}
	content := "BT " + f + " 12 Tf 72 700 Td (Price: ) Tj (OLD) Tj ( EUR) Tj ET " +
		"BT " + f + " 12 Tf 200 700 Td (tail) Tj 60 0 Td (more) Tj ET " +
		"BT " + f + " 12 Tf 1 0 0 1 300 700 Tm [(ke) -200 (rn)] TJ ET"
	if err := replacePageContents(p, []byte(content)); err != nil {
		t.Fatal(err)
	}
	eur, tail, more, kern := findOne(t, p, "EUR"), findOne(t, p, "tail"), findOne(t, p, "more"), findOne(t, p, "rn")
	if _, err := p.ReplaceText("OLD", "NEWPRICE", ReplaceOptions{Adjustment: ReplaceAdjustmentShiftRestOfLine}); err != nil {
		t.Fatal(err)
	}
	delta := helvWidth(t, "NEWPRICE") - helvWidth(t, "OLD")
	// "more" follows a Td inside the same text object as the shifted
	// "tail": the Td restarts from the unshifted line position, so it
	// needs the shift again.
	for name, before := range map[string]Rectangle{"EUR": eur, "tail": tail, "more": more, "rn": kern} {
		if got := findOne(t, p, name); math.Abs(got.LLX-(before.LLX+delta)) > 0.5 {
			t.Errorf("%s at %.2f, want %.2f", name, got.LLX, before.LLX+delta)
		}
	}
}

// The default leaves the rest of the line where it was.
func TestReplaceDefaultDoesNotShift(t *testing.T) {
	_, p := replaceAdjFixture(t, "Hello World and more text")
	before := findOne(t, p, "and")
	if _, err := p.ReplaceText("World", "Wonderful Universe"); err != nil {
		t.Fatal(err)
	}
	if got := findOne(t, p, "and"); math.Abs(got.LLX-before.LLX) > 1e-6 {
		t.Errorf("'and' moved without an adjustment: %+v → %+v", before, got)
	}
}

func TestReplaceUnsupportedAdjustment(t *testing.T) {
	doc, _ := replaceAdjFixture(t, "Hello World")
	for _, a := range []ReplaceAdjustment{ReplaceAdjustmentAdjustSpaceWidth, ReplaceAdjustmentWholeWordsHyphenation} {
		if _, err := doc.ReplaceText("World", "X", ReplaceOptions{Adjustment: a}); err == nil {
			t.Errorf("adjustment %d: no error", a)
		}
	}
}
