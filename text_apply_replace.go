// SPDX-License-Identifier: MIT

package asposepdf

import (
	"fmt"
	"math"
	"strings"
)

// A fragment's ReplaceOptions applied when its text changes (pdf-go-xiru):
// ShiftRestOfLine moves the rest of its line by the change in width, and the
// paragraph adjustments (WholeWordsHyphenation, AdjustSpaceWidth, a
// Rectangle or a FontSizeAdjustment) lay the new text out as ReplaceText
// would. Mirrors Aspose.PDF for .NET's TextFragment.ReplaceOptions.

// oldExtent is the plan's old text on its first line: baseline start,
// baseline, left and right edges, from the snapshot and the drawn quads.
func (pl *textEditPlan) oldExtent() (x, y, left, right float64, ok bool) {
	x, y = pl.snap.position.XIndent, pl.snap.position.YIndent
	for i, q := range pl.regions {
		if i < len(pl.snap.segments) && pl.snap.segments[i].pos.YIndent != y && pl.snap.segments[i].pos.YIndent != 0 {
			continue // a segment on another line
		}
		r := quadBounds(q)
		if !ok {
			left, right, ok = r.LLX, r.URX, true
			continue
		}
		left, right = math.Min(left, r.LLX), math.Max(right, r.URX)
	}
	return x, y, left, right, ok
}

// newText is the plan's new text and the state of its first piece, resolved.
func (pl *textEditPlan) newText() (string, TextState) {
	var b strings.Builder
	var st TextState
	for i, s := range pl.tmp.segmentsToDraw() {
		if i == 0 {
			st = s.TextState.resolved()
		}
		b.WriteString(s.Text)
	}
	return b.String(), st
}

// rtl reports whether the plan's line reads right to left: the line's own
// direction for a found fragment (a Latin placeholder in an Arabic line),
// else its old text's. Then what follows the text lies to its left, and the
// new text keeps the old right edge, as ReplaceText does.
func (pl *textEditPlan) rtl() bool {
	if b := pl.f.bind; b != nil && b.found {
		return b.rtlLine || bidiBaseLevel(pl.snap.text, false) == 1
	}
	return bidiBaseLevel(pl.snap.text, false) == 1
}

// lineShift is the shift ShiftRestOfLine asks of the plan's line: what
// follows the old text moves by the change in width (on a right-to-left
// line, what lies left of it, the new text keeping the old right edge).
func (pl *textEditPlan) lineShift() (lineShift, bool, error) {
	if pl.adj != ReplaceAdjustmentShiftRestOfLine || !pl.whole {
		return lineShift{}, false, nil
	}
	x, y, left, right, ok := pl.oldExtent()
	if !ok {
		return lineShift{}, false, nil
	}
	newW := 0.0
	for _, s := range pl.tmp.segmentsToDraw() {
		w, err := s.TextState.resolved().MeasureString(s.Text)
		if err != nil {
			return lineShift{}, false, err
		}
		newW += w
	}
	_, st := pl.newText()
	tol := 0.3 * math.Abs(st.FontSize)
	var lo, hi float64
	if b := pl.f.bind; b != nil && b.found {
		lo, hi = b.colLo, b.colHi
	}
	if pl.rtl() {
		return lineShift{y: y, tol: tol, fromX: left, dx: (right - left) - newW, rtl: true}.bound(lo, hi), true, nil
	}
	return lineShift{y: y, tol: tol, fromX: right, dx: newW - (right - x)}.bound(lo, hi), true, nil
}

// applyLineShifts moves the rest of each line a plan on the page shifts
// (the plans' old text already stripped from data), and the plans' own new
// text with the shifts of the others on its line. It returns the content
// and the bands of the lines moved, whose other fragments no longer
// describe the page.
func applyLineShifts(p *Page, data []byte, pls []*textEditPlan) ([]byte, []QuadPoint, error) {
	var shifts []lineShift
	owner := map[int]*textEditPlan{}
	for _, pl := range pls {
		s, ok, err := pl.lineShift()
		if err != nil {
			return nil, nil, pl.errorf("shift rest of line: %w", err)
		}
		if ok {
			owner[len(shifts)] = pl
			shifts = append(shifts, s)
		}
	}
	if len(shifts) == 0 {
		return data, nil, nil
	}
	out, err := shiftLineText(data, p.doc.objects, p.pageResources(), shifts)
	if err != nil {
		return nil, nil, fmt.Errorf("shift rest of line: %w", err)
	}
	// A plan's new text moves with the shifts of the others on its line,
	// never its own.
	for _, pl := range pls {
		if !pl.whole || pl.tmp == nil {
			continue
		}
		var others []lineShift
		for i, s := range shifts {
			if owner[i] != pl {
				others = append(others, s)
			}
		}
		y := pl.tmp.Position.YIndent
		pl.tmp.Position.XIndent += lineShiftAt(others, pl.tmp.Position.XIndent, y)
		for i := range pl.place {
			if pl.place[i] != (Position{}) {
				pl.place[i].XIndent += lineShiftAt(others, pl.place[i].XIndent, pl.place[i].YIndent)
			}
		}
	}
	page, err := p.MediaBox()
	if err != nil {
		return nil, nil, err
	}
	var bands []QuadPoint
	for _, s := range shifts {
		r := Rectangle{LLX: s.fromX, LLY: s.y - s.tol*4, URX: page.URX, URY: s.y + s.tol*4}
		if s.toX != 0 {
			r.URX = s.toX
		}
		if s.rtl {
			r.LLX, r.URX = page.LLX, s.fromX
			if s.loX != 0 {
				r.LLX = s.loX
			}
		}
		bands = append(bands, rectAsQuadPoint(r))
	}
	return out, bands, nil
}

// drawParagraph draws the plan's new text as a paragraph in the layout its
// ReplaceOptions ask for, as ReplaceText does for a match.
func (pl *textEditPlan) drawParagraph(p *Page) error {
	text, st := pl.newText()
	if text == "" {
		return nil
	}
	x, y, left, right, ok := pl.oldExtent()
	if !ok {
		return fmt.Errorf("the fragment's old text has no box")
	}
	asc, desc := textBoxEm(st.Font)
	size := math.Abs(st.FontSize)
	m := replaceMatch{
		rect: Rectangle{LLX: left, LLY: y + desc*size, URX: right, URY: y + asc*size},
		x:    x, y: y, size: size, font: st.style().Font,
		rtl: pl.rtl(),
	}
	if c := st.ForegroundColor; c != nil {
		m.color = *c
	}
	return drawReplacementParagraph(p, m, text, pl.lay)
}

// cellClip is the clip for the plan's new text when a found fragment's text
// runs past its table cell (pdf-go-64zt): the document clips the cell's text
// at the rule, so the redraw is clipped there too.
func (pl *textEditPlan) cellClip(p *Page) (*Rectangle, error) {
	b := pl.f.bind
	if b == nil || !b.found || (b.colLo == 0 && b.colHi == 0) {
		return nil, nil
	}
	w := 0.0
	for _, s := range pl.tmp.segmentsToDraw() {
		sw, err := s.TextState.resolved().MeasureString(s.Text)
		if err != nil {
			return nil, err
		}
		w += sw
	}
	return cellClip(p, pl.tmp.Position.XIndent, w, b.colLo, b.colHi), nil
}
