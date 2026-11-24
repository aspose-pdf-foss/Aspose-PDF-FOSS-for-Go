// SPDX-License-Identifier: MIT

package asposepdf

import (
	"math"
	"sort"
)

// fragmentIndex answers which fragments may have glyphs inside a rectangle,
// so that clipping one page's text to many rectangles does not test every
// fragment for each (GHSA-gc9q-3vx8-phx2). A fragment's box spans the points
// clipFragments tests — its start, end and glyph starts — so a fragment whose
// box misses a rectangle has nothing in it.
type fragmentIndex struct {
	frags []textFragment
	box   []Rectangle // per fragment, by original index
	order []int       // fragment indexes by box.LLY
	lly   []float64   // box.LLY in order
	tree  []float64   // max box.URY over each node's range of order
	size  int         // leaves in tree
}

func newFragmentIndex(frags []textFragment) *fragmentIndex {
	ix := &fragmentIndex{frags: frags, box: make([]Rectangle, len(frags)), order: make([]int, len(frags))}
	for i := range frags {
		f := &frags[i]
		b := Rectangle{LLX: minf(f.x, f.endX), LLY: minf(f.y, f.endY), URX: maxf(f.x, f.endX), URY: maxf(f.y, f.endY)}
		for _, x := range f.runeX {
			b.LLX, b.URX = minf(b.LLX, x), maxf(b.URX, x)
		}
		for _, y := range f.runeY {
			b.LLY, b.URY = minf(b.LLY, y), maxf(b.URY, y)
		}
		if math.IsNaN(b.LLX + b.LLY + b.URX + b.URY) {
			// Unordered: always a candidate, clipFragments decides.
			b = Rectangle{LLX: math.Inf(-1), LLY: math.Inf(-1), URX: math.Inf(1), URY: math.Inf(1)}
		}
		ix.box[i] = b
		ix.order[i] = i
	}
	sort.SliceStable(ix.order, func(a, b int) bool { return ix.box[ix.order[a]].LLY < ix.box[ix.order[b]].LLY })
	ix.lly = make([]float64, len(frags))
	for k, i := range ix.order {
		ix.lly[k] = ix.box[i].LLY
	}
	ix.size = 1
	for ix.size < len(frags) {
		ix.size *= 2
	}
	ix.tree = make([]float64, 2*ix.size)
	for k := range ix.tree {
		ix.tree[k] = math.Inf(-1)
	}
	for k, i := range ix.order {
		ix.tree[ix.size+k] = ix.box[i].URY
	}
	for k := ix.size - 1; k >= 1; k-- {
		ix.tree[k] = math.Max(ix.tree[2*k], ix.tree[2*k+1])
	}
	return ix
}

// clip is clipFragments(frags, r) over the fragments that can contribute,
// in their original order.
func (ix *fragmentIndex) clip(r Rectangle) []textFragment {
	hits := ix.candidates(r)
	sub := make([]textFragment, len(hits))
	for k, i := range hits {
		sub[k] = ix.frags[i]
	}
	return clipFragments(sub, r)
}

// candidates returns, in increasing order, the indexes of the fragments
// whose box meets r.
func (ix *fragmentIndex) candidates(r Rectangle) []int {
	const tol = 0.01 // as clipFragments
	lo, hi := r.LLY-tol, r.URY+tol
	cut := sort.SearchFloat64s(ix.lly, math.Nextafter(hi, math.Inf(1))) // boxes with LLY <= hi
	var hits []int
	var walk func(node, from, to int)
	walk = func(node, from, to int) {
		if from >= cut || ix.tree[node] < lo {
			return
		}
		if to-from == 1 {
			i := ix.order[from]
			if b := ix.box[i]; b.URX >= r.LLX-tol && b.LLX <= r.URX+tol {
				hits = append(hits, i)
			}
			return
		}
		mid := (from + to) / 2
		walk(2*node, from, mid)
		walk(2*node+1, mid, to)
	}
	if len(ix.frags) > 0 {
		walk(1, 0, ix.size)
	}
	sort.Ints(hits)
	return hits
}
