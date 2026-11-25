// SPDX-License-Identifier: MIT

package asposepdf

import "math"

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
// majority of strong characters. A fragment belongs to the cell it starts
// in (text running on under the next cell is the previous cell's). ok is
// false when the cell has no bound or
// no strong character, and the line's own direction applies.
func cellRTL(line *TextLine, lo, hi float64) (rtl, ok bool) {
	if line == nil || (lo == 0 && hi == 0) {
		return false, false
	}
	var nr, nl int
	var left, right int // the strong class at each end: 1 L, 2 R
	minX, maxX := math.Inf(1), math.Inf(-1)
	for i := range line.Fragments {
		f := &line.Fragments[i]
		if (lo != 0 && f.X < lo-cellTol) || (hi != 0 && f.X >= hi-cellTol) {
			continue
		}
		for _, r := range f.Text {
			c := 0
			switch bidiClass(r) {
			case clsR, clsAL:
				nr, c = nr+1, 2
			case clsL:
				nl, c = nl+1, 1
			}
			if c == 0 {
				continue
			}
			if f.X < minX {
				minX, left = f.X, c
			}
			if f.X+f.Width >= maxX {
				maxX, right = f.X+f.Width, c
			}
		}
	}
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
