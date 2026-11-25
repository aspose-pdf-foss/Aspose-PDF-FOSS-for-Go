// SPDX-License-Identifier: MIT

package asposepdf

import "testing"

// ruledTwoCellDoc is a ruled table of one row and two cells (x 50-300 and
// 300-550) with left drawn from the left cell's left edge and right ending at
// the right cell's right edge, both on baseline 600 in DejaVu Sans 14: the
// layout pipeline extracts them as one line.
func ruledTwoCellDoc(t *testing.T, left, right string) *Document {
	t.Helper()
	d := NewDocumentFromFormat(PageFormatA4)
	p := mustPage(t, d)
	font, err := d.LoadFont("testdata/DejaVuSans.ttf")
	if err != nil {
		t.Fatal(err)
	}
	st := TextStyle{Font: font, Size: 14}
	if err := p.AddTextAt(left, st, 60, 600); err != nil {
		t.Fatal(err)
	}
	st.HAlign = HAlignRight
	if err := p.AddTextAt(right, st, 540, 600); err != nil {
		t.Fatal(err)
	}
	ls := LineStyle{Width: 1}
	for _, x := range []float64{50, 300, 550} {
		if err := p.DrawLine(Point{X: x, Y: 580}, Point{X: x, Y: 630}, ls); err != nil {
			t.Fatal(err)
		}
	}
	for _, y := range []float64{580, 630} {
		if err := p.DrawLine(Point{X: 50, Y: y}, Point{X: 550, Y: y}, ls); err != nil {
			t.Fatal(err)
		}
	}
	return reopenDoc(t, d)
}

// rectOf is the box of word's only occurrence on page 1.
func rectOf(t *testing.T, d *Document, word string) Rectangle {
	t.Helper()
	ms, err := mustPage(t, d).SearchText(word)
	if err != nil || len(ms) != 1 {
		t.Fatalf("%q: %d matches, %v", word, len(ms), err)
	}
	return ms[0].Rect
}

// editInCell replaces old with repl in d, through ReplaceText or a found
// fragment, with ShiftRestOfLine.
func editInCell(t *testing.T, d *Document, viaFragment bool, old, repl string) {
	t.Helper()
	font, err := d.LoadFont("testdata/DejaVuSans.ttf")
	if err != nil {
		t.Fatal(err)
	}
	opts := ReplaceOptions{Adjustment: ReplaceAdjustmentShiftRestOfLine, Font: font}
	if !viaFragment {
		if n, err := d.ReplaceText(old, repl, opts); err != nil || n != 1 {
			t.Fatalf("replace: %d, %v", n, err)
		}
		return
	}
	f := absorbOne(t, d, old)
	f.Text = repl
	f.ReplaceOptions = opts
	if err := d.ApplyTextChanges(); err != nil {
		t.Fatal(err)
	}
}

// pdf-go-p5m0: in a ruled table the extracted line spans both cells, but
// ShiftRestOfLine moves only what follows the replacement in its own cell,
// and the cell's text decides which way that is.
func TestShiftRestOfLineStaysInCell(t *testing.T) {
	for _, via := range []struct {
		name string
		frag bool
	}{{"ReplaceText", false}, {"Fragment", true}} {
		t.Run(via.name, func(t *testing.T) {
			// A left-to-right cell beside a right-to-left one with more
			// letters: what follows moves right, the other cell stays.
			t.Run("LTRCellInRTLLine", func(t *testing.T) {
				const right = "שלום עולם יפה מאוד גדול"
				d0 := ruledTwoCellDoc(t, "Name Bob end", right)
				bob, end, he := rectOf(t, d0, "Bob"), rectOf(t, d0, "end"), rectOf(t, d0, "שלום")
				d := ruledTwoCellDoc(t, "Name Bob end", right)
				editInCell(t, d, via.frag, "Bob", "Roberta")
				out := reopenDoc(t, d)
				nr := rectOf(t, out, "Roberta")
				grow := (nr.URX - nr.LLX) - (bob.URX - bob.LLX)
				if !nearPt(nr.LLX, bob.LLX) {
					t.Errorf("new text starts at %.2f, want %.2f", nr.LLX, bob.LLX)
				}
				if r := rectOf(t, out, "end"); !nearPt(r.LLX, end.LLX+grow) {
					t.Errorf("\"end\" at %.2f, want %.2f (moved right by %.2f)", r.LLX, end.LLX+grow, grow)
				}
				if r := rectOf(t, out, "Name"); !nearPt(r.LLX, 60) {
					t.Errorf("\"Name\" moved to %.2f", r.LLX)
				}
				if r := rectOf(t, out, "שלום"); !nearPt(r.LLX, he.LLX) {
					t.Errorf("the other cell moved: %.2f, was %.2f", r.LLX, he.LLX)
				}
			})
			// A right-to-left cell beside a left-to-right one with more
			// letters: what follows lies left and moves left, within the
			// cell.
			t.Run("RTLCellInLTRLine", func(t *testing.T) {
				const left = "Employee and other words"
				const right = "שלום עולם יפה"
				d0 := ruledTwoCellDoc(t, left, right)
				first, mid, last := rectOf(t, d0, "שלום"), rectOf(t, d0, "עולם"), rectOf(t, d0, "יפה")
				words := rectOf(t, d0, "words")
				d := ruledTwoCellDoc(t, left, right)
				editInCell(t, d, via.frag, "עולם", "עולםעולם")
				out := reopenDoc(t, d)
				nr := rectOf(t, out, "עולםעולם")
				grow := (nr.URX - nr.LLX) - (mid.URX - mid.LLX)
				if !nearPt(nr.URX, mid.URX) {
					t.Errorf("new text ends at %.2f, want the old right edge %.2f", nr.URX, mid.URX)
				}
				if r := rectOf(t, out, "יפה"); !nearPt(r.URX, last.URX-grow) {
					t.Errorf("the text to the left ends at %.2f, want %.2f", r.URX, last.URX-grow)
				}
				if r := rectOf(t, out, "שלום"); !nearPt(r.LLX, first.LLX) {
					t.Errorf("the text to the right moved: %.2f, was %.2f", r.LLX, first.LLX)
				}
				if r := rectOf(t, out, "words"); !nearPt(r.LLX, words.LLX) {
					t.Errorf("the other cell moved: %.2f, was %.2f", r.LLX, words.LLX)
				}
			})
			// Two left-to-right cells: the shift stops at the rule.
			t.Run("LTRCells", func(t *testing.T) {
				d0 := ruledTwoCellDoc(t, "Name Bob end", "Right side")
				rs := rectOf(t, d0, "Right")
				d := ruledTwoCellDoc(t, "Name Bob end", "Right side")
				editInCell(t, d, via.frag, "Bob", "Roberta")
				out := reopenDoc(t, d)
				if r := rectOf(t, out, "Right"); !nearPt(r.LLX, rs.LLX) {
					t.Errorf("the other cell moved: %.2f, was %.2f", r.LLX, rs.LLX)
				}
			})
		})
	}
}

// A line with no rules is shifted as a whole, as before.
func TestColumnRulesCell(t *testing.T) {
	c := columnRules{{pos: 50, lo: 500, hi: 700}, {pos: 300, lo: 500, hi: 700}, {pos: 550, lo: 500, hi: 600}}
	for _, k := range []struct {
		x, y, lo, hi float64
	}{
		{60, 600, 50, 300},
		{310, 600, 300, 550},
		{299.5, 600, 300, 550}, // a hair before the rule
		{298, 600, 50, 300},
		{310, 650, 300, 0}, // the right rule ends below
		{60, 800, 0, 0},    // above every rule
		{10, 600, 0, 50},
	} {
		if lo, hi := c.cell(k.x, k.y); lo != k.lo || hi != k.hi {
			t.Errorf("cell(%v, %v) = %v, %v; want %v, %v", k.x, k.y, lo, hi, k.lo, k.hi)
		}
	}
	if s := (lineShift{fromX: 100, dx: 5}).bound(50, 300); s.toX != 300 || s.loX != 0 {
		t.Errorf("LTR bound: %+v", s)
	}
	if s := (lineShift{fromX: 100, dx: 5, rtl: true}).bound(50, 300); s.loX != 50 || s.toX != 0 {
		t.Errorf("RTL bound: %+v", s)
	}
	shifts := []lineShift{{y: 600, tol: 3, fromX: 100, dx: 5, rtl: true, loX: 50}}
	if d := lineShiftAt(shifts, 40, 600); d != 0 {
		t.Errorf("left of the cell moved by %v", d)
	}
	if d := lineShiftAt(shifts, 60, 600); d != 5 {
		t.Errorf("inside the cell moved by %v, want 5", d)
	}
}

// cellRTL counts the strong letters of the fragments inside the cell.
func TestCellRTL(t *testing.T) {
	line := &TextLine{Fragments: []TextFragment{
		{Text: "ab", X: 60, Width: 20},
		{Text: "שלום", X: 320, Width: 30},
		{Text: "cd", X: 360, Width: 20},
	}}
	for _, k := range []struct {
		lo, hi  float64
		rtl, ok bool
	}{
		{50, 300, false, true},
		{300, 340, true, true},
		{300, 550, false, true}, // the ends disagree, the text keeps left
		{300, 0, true, true},    // bounded on one side: 4 to 2
		{0, 0, false, false},    // no cell
		{400, 550, false, false},
	} {
		if rtl, ok := cellRTL(line, k.lo, k.hi); rtl != k.rtl || ok != k.ok {
			t.Errorf("cellRTL(%v, %v) = %v, %v; want %v, %v", k.lo, k.hi, rtl, ok, k.rtl, k.ok)
		}
	}
	// Latin at the left end of a right-aligned right-to-left line.
	ph := &TextLine{Fragments: []TextFragment{{Text: "ab", X: 480, Width: 20}, {Text: "שלום", X: 510, Width: 35}}}
	if rtl, ok := cellRTL(ph, 300, 550); !rtl || !ok {
		t.Errorf("a right-aligned line ending in Hebrew reads %v, %v; want right to left", rtl, ok)
	}
	// Both ends Hebrew, more Latin between: right to left.
	mid := &TextLine{Fragments: []TextFragment{{Text: "של", X: 310, Width: 10}, {Text: "abcdefgh", X: 330, Width: 40}, {Text: "של", X: 380, Width: 10}}}
	if rtl, ok := cellRTL(mid, 300, 550); !rtl || !ok {
		t.Errorf("Hebrew at both ends reads %v, %v; want right to left", rtl, ok)
	}
	// Text of the previous cell running on under this one does not count.
	under := &TextLine{Fragments: []TextFragment{{Text: "abcdefghij", X: 290, Width: 60}, {Text: "שלום", X: 510, Width: 35}}}
	if rtl, ok := cellRTL(under, 300, 550); !rtl || !ok {
		t.Errorf("a cell with the previous cell's text under it reads %v, %v; want right to left", rtl, ok)
	}
	tie := &TextLine{Fragments: []TextFragment{{Text: "ab", X: 310, Width: 10}, {Text: "של", X: 330, Width: 10}}}
	if rtl, ok := cellRTL(tie, 300, 550); rtl || !ok {
		t.Errorf("a tie reads %v, %v; want left to right", rtl, ok)
	}
}
