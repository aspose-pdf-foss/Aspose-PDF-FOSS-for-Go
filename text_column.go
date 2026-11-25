// SPDX-License-Identifier: MIT

package asposepdf

import (
	"fmt"
	"math"
	"unicode"
)

// columnRules are a page's vertical ruling lines, the column boundaries of a
// ruled table. An extracted line runs across them (the layout pipeline joins
// what lies on one baseline), so ShiftRestOfLine and a line's direction are
// bounded by the cell the text is in (pdf-go-p5m0): moving what follows a
// replacement must stop at the cell's edge, and an English cell beside an
// Arabic one reads left to right.
type columnRules []rule

// pageColumnRules returns the page's vertical rules (nil when it has none).
func pageColumnRules(p *Page) columnRules {
	_, v, _ := pageRules(p)
	return columnRules(normalizeRules(v))
}

// cellTol is how far a rule may lie inside the text it bounds, and beyond
// its own extent, and still count.
const cellTol = 1.0

// cell returns the edges of the cell holding x on baseline y: the nearest
// rules at or left of x and right of it whose extent covers y. A side with
// no such rule is 0. A match is placed by where it starts: a document's
// text may run under the next cell, hidden by a clip, and a match reaching
// over the rule still belongs to the cell it starts in.
func (c columnRules) cell(x, y float64) (lo, hi float64) {
	lo, hi = math.Inf(-1), math.Inf(1)
	for _, r := range c {
		if y < r.lo-cellTol || y > r.hi+cellTol {
			continue
		}
		if r.pos <= x+cellTol {
			lo = math.Max(lo, r.pos)
		} else {
			hi = math.Min(hi, r.pos)
		}
	}
	if math.IsInf(lo, -1) {
		lo = 0
	}
	if math.IsInf(hi, 1) {
		hi = 0
	}
	return lo, hi
}

// cellRTL reports whether the part of line between lo and hi (0 = no bound
// on that side) reads right to left. The glyphs are in visual order, so the
// paragraph's first strong character (rule P2) is the leftmost when it reads
// left to right and the rightmost when it reads right to left: when both
// ends agree, that is the direction. When they do not (Latin at the left of
// an Arabic line, a placeholder in it), the side the text keeps to inside
// the cell decides, and a cell bounded on one side only goes by the
// majority of strong characters. Characters the clip hides are not the
// cell's (another cell's text pushed under it). ok is false when the cell
// has no bound or no strong character, and the line's own direction
// applies.
func cellRTL(line *TextLine, lo, hi float64) (rtl, ok bool) {
	if line == nil || (lo == 0 && hi == 0) {
		return false, false
	}
	var nr, nl int
	var left, right int // the strong class at each end: 1 L, 2 R
	minX, maxX := math.Inf(1), math.Inf(-1)
	visibleRunes(line, func(x float64, r rune) {
		if (lo != 0 && x < lo-cellTol) || (hi != 0 && x >= hi-cellTol) {
			return
		}
		c := 0
		switch bidiClass(r) {
		case clsR, clsAL:
			nr, c = nr+1, 2
		case clsL:
			nl, c = nl+1, 1
		}
		if c == 0 {
			return
		}
		if x < minX {
			minX, left = x, c
		}
		if x >= maxX {
			maxX, right = x, c
		}
	})
	switch {
	case nr+nl == 0:
		return false, false
	case left == right:
		return left == 2, true
	case lo != 0 && hi != 0:
		return hi-maxX < minX-lo, true
	}
	return nr > nl, true
}

// visibleRunes calls fn with the start x of each rune of line the clip
// does not hide (pdf-go-64zt): a cell's text pushed under the next cell is
// not that cell's.
func visibleRunes(line *TextLine, fn func(x float64, r rune)) {
	eachRune(line, func(x float64, r rune, hidden bool) {
		if !hidden {
			fn(x, r)
		}
	})
}

// eachRune calls fn with the start x of each rune of line and whether the
// clip hides it.
func eachRune(line *TextLine, fn func(x float64, r rune, hidden bool)) {
	for i := range line.Fragments {
		f := &line.Fragments[i]
		k := 0
		for _, r := range f.Text {
			x := f.X
			if k < len(f.runeX) {
				x = f.runeX[k]
			}
			fn(x, r, k < len(f.runeHidden) && f.runeHidden[k])
			k++
		}
	}
}

// bound limits a shift to the cell [lo, hi]: what lies beyond the cell's
// edge on the moving side stays.
func (s lineShift) bound(lo, hi float64) lineShift {
	if s.rtl {
		s.loX = lo
	} else {
		s.toX = hi
	}
	return s
}

// stripInCell limits r, the box of a match on line, to its cell [lo, hi]
// on a side where r runs past the rule and the clip hides text there
// (pdf-go-64zt). A shift of the rest of a line can push one cell's text
// under the next, where the document's clip at the rule hides it; removing
// the match by its whole box would take the next cell's own glyphs drawn
// there too. The caller removes the hidden glyphs of the whole box
// separately (rewriteTextOperatorsHidden). Where nothing past the rule is
// hidden, the document does not clip there and r is kept.
func stripInCell(line *TextLine, r Rectangle, lo, hi float64) Rectangle {
	if line == nil {
		return r
	}
	hiddenIn := func(a, b float64) bool {
		found := false
		eachRune(line, func(x float64, r rune, hidden bool) {
			if hidden && x >= a && x < b && !unicode.IsSpace(r) {
				found = true
			}
		})
		return found
	}
	if hi != 0 && r.URX > hi+cellTol && hiddenIn(hi-cellTol, r.URX) {
		r.URX = hi
	}
	if lo != 0 && r.LLX < lo-cellTol && hiddenIn(r.LLX, lo+cellTol) {
		r.LLX = lo
	}
	return r
}

// cellClip is the rectangle to clip a redraw of width w starting at x to,
// when it runs past its cell [lo, hi] (0 = no bound), as the document's own
// clip does for the cell's text; nil when it stays inside.
func cellClip(p *Page, x, w, lo, hi float64) *Rectangle {
	if (hi == 0 || x+w <= hi+cellTol) && (lo == 0 || x >= lo-cellTol) {
		return nil
	}
	box, err := p.MediaBox()
	if err != nil {
		return nil
	}
	if lo != 0 {
		box.LLX = lo
	}
	if hi != 0 {
		box.URX = hi
	}
	return &box
}

// clipped runs draw with the page's clip narrowed to r (nil: no clip).
func (p *Page) clipped(r *Rectangle, draw func() error) error {
	if r == nil {
		return draw()
	}
	op := fmt.Sprintf("q %s %s %s %s re W n\n", formatFloat(r.LLX), formatFloat(r.LLY), formatFloat(r.URX-r.LLX), formatFloat(r.URY-r.LLY))
	if err := p.appendToContentStream([]byte(op)); err != nil {
		return err
	}
	if err := draw(); err != nil {
		return err
	}
	return p.appendToContentStream([]byte("Q\n"))
}
