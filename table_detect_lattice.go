// SPDX-License-Identifier: MIT

package asposepdf

import (
	"math"
	"sort"
)

// Lattice (ruled-table) detection (epic pdf-go-w4ht): the vector-native
// pipeline of tabula-java's SpreadsheetExtractionAlgorithm over the ruling
// lines pageRules collects. Intersections of horizontal and vertical rules
// form a point grid; each point tried as a cell's top-left corner searches
// right and down for the smallest closable rectangle whose edges are
// continuously ruled — a missing inner ruling simply yields a bigger cell,
// which is how rowspan/colspan fall out naturally. Contiguous cells group
// into tables, and the physical cells map onto a logical row/column grid.

const (
	xTolPt           = 2.5 // intersection tolerance
	cellCornerJoinPt = 6.0 // cells sharing corners within this group into one table
	minTableCells    = 4
)

// latticeCell is one physical (possibly spanning) ruled cell.
type latticeCell struct {
	Rectangle
}

// latticeIntersections returns the grid points where an H and a V rule
// cross (within tolerance), plus fast lookup structures.
type latticeGrid struct {
	hRules []rule // sorted by pos (Y)
	vRules []rule // sorted by pos (X)
	// point[i][j] = true when hRules[i] and vRules[j] intersect.
	point [][]bool
}

func buildLatticeGrid(hRules, vRules []rule) *latticeGrid {
	g := &latticeGrid{hRules: hRules, vRules: vRules}
	g.point = make([][]bool, len(hRules))
	for i, h := range hRules {
		g.point[i] = make([]bool, len(vRules))
		for j, v := range vRules {
			if v.pos >= h.lo-xTolPt && v.pos <= h.hi+xTolPt &&
				h.pos >= v.lo-xTolPt && h.pos <= v.hi+xTolPt {
				g.point[i][j] = true
			}
		}
	}
	return g
}

// hCovers reports whether hRules[i] is continuously ruled between the X
// positions of vRules[j1] and vRules[j2].
func (g *latticeGrid) hCovers(i, j1, j2 int) bool {
	h := g.hRules[i]
	return h.lo <= g.vRules[j1].pos+xTolPt && h.hi >= g.vRules[j2].pos-xTolPt
}

// vCovers reports whether vRules[j] spans between hRules[i1] and hRules[i2].
func (g *latticeGrid) vCovers(j, i1, i2 int) bool {
	v := g.vRules[j]
	lo, hi := g.hRules[i1].pos, g.hRules[i2].pos
	if lo > hi {
		lo, hi = hi, lo
	}
	return v.lo <= lo+xTolPt && v.hi >= hi-xTolPt
}

// latticeCells runs the corner search: for each intersection taken as a
// top-left corner (PDF Y-up: top = larger Y), find the smallest rectangle
// closable with continuous rules.
func latticeCells(g *latticeGrid) []latticeCell {
	// Order rows top-to-bottom.
	rowIdx := make([]int, len(g.hRules))
	for i := range rowIdx {
		rowIdx[i] = i
	}
	sort.Slice(rowIdx, func(a, b int) bool { return g.hRules[rowIdx[a]].pos > g.hRules[rowIdx[b]].pos })

	var cells []latticeCell
	for ri, i := range rowIdx {
		for j := range g.vRules {
			if !g.point[i][j] {
				continue
			}
			found := false
			// Nearest right neighbour first.
			for j2 := j + 1; j2 < len(g.vRules) && !found; j2++ {
				if !g.point[i][j2] || !g.hCovers(i, j, j2) {
					continue
				}
				// Nearest lower row first.
				for ri2 := ri + 1; ri2 < len(rowIdx); ri2++ {
					i2 := rowIdx[ri2]
					if !g.point[i2][j] || !g.point[i2][j2] {
						continue
					}
					if !g.vCovers(j, i2, i) || !g.vCovers(j2, i2, i) || !g.hCovers(i2, j, j2) {
						continue
					}
					cells = append(cells, latticeCell{Rectangle{
						LLX: g.vRules[j].pos, LLY: g.hRules[i2].pos,
						URX: g.vRules[j2].pos, URY: g.hRules[i].pos,
					}})
					found = true
					break
				}
			}
		}
	}
	return cells
}

// groupCells clusters cells sharing edges/corners (within cellCornerJoinPt)
// into connected components — one component per table candidate.
func groupCells(cells []latticeCell) [][]latticeCell {
	n := len(cells)
	parent := make([]int, n)
	for i := range parent {
		parent[i] = i
	}
	find := func(x int) int {
		for parent[x] != x {
			parent[x] = parent[parent[x]]
			x = parent[x]
		}
		return x
	}
	union := func(a, b int) { parent[find(a)] = find(b) }

	near := func(a, b float64) bool { return a-b <= cellCornerJoinPt && b-a <= cellCornerJoinPt }
	touch := func(a, b Rectangle) bool {
		// Share an edge segment (same boundary line and overlapping extent).
		if near(a.URX, b.LLX) || near(b.URX, a.LLX) {
			return a.LLY < b.URY+xTolPt && b.LLY < a.URY+xTolPt
		}
		if near(a.URY, b.LLY) || near(b.URY, a.LLY) {
			return a.LLX < b.URX+xTolPt && b.LLX < a.URX+xTolPt
		}
		return false
	}
	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			if touch(cells[i].Rectangle, cells[j].Rectangle) {
				union(i, j)
			}
		}
	}
	byRoot := map[int][]latticeCell{}
	for i, c := range cells {
		r := find(i)
		byRoot[r] = append(byRoot[r], c)
	}
	var out [][]latticeCell
	roots := make([]int, 0, len(byRoot))
	for r := range byRoot {
		roots = append(roots, r)
	}
	sort.Ints(roots)
	for _, r := range roots {
		out = append(out, byRoot[r])
	}
	return out
}

// closeOpenEdges adds the border a grid implies without drawing it: when every
// rule crossing a table's last border runs on to one common end, that end is
// the table's edge. A table whose last column (or row) has no closing rule
// otherwise loses it, as its cells never close (pdf-go-c19b). Both ends of both
// directions are checked.
func closeOpenEdges(hRules, vRules []rule) ([]rule, []rule) {
	return closeEnds(hRules, vRules), closeEnds(vRules, hRules)
}

// closeEnds returns cross with a rule added at each implied edge of rules: a
// rule c of cross (a border) such that the rules meeting it number at least
// three, span c's length, all run on past it to one end at least minSpan away,
// and meet no other rule of cross beyond it.
func closeEnds(cross, rules []rule) []rule {
	const endTol, minSpan = 1.0, 8.0
	out := append([]rule(nil), cross...)
	meets := func(c, r rule) bool {
		return c.pos >= r.lo-xTolPt && c.pos <= r.hi+xTolPt && r.pos >= c.lo-xTolPt && r.pos <= c.hi+xTolPt
	}
	for _, c := range cross {
		var members []rule
		for _, r := range rules {
			if meets(c, r) {
				members = append(members, r)
			}
		}
		if len(members) < 3 {
			continue
		}
		lo, hi := members[0].pos, members[0].pos
		for _, m := range members {
			lo, hi = minf(lo, m.pos), maxf(hi, m.pos)
		}
		if math.Abs(lo-c.lo) > 2*xTolPt || math.Abs(hi-c.hi) > 2*xTolPt {
			continue // c is not the whole border of these rules
		}
		for _, high := range []bool{true, false} {
			end, ok := members[0].hi, true
			if !high {
				end = members[0].lo
			}
			for _, m := range members {
				e := m.hi
				if !high {
					e = m.lo
				}
				if math.Abs(e-end) > endTol {
					ok = false
					break
				}
			}
			beyond := end - c.pos
			if !high {
				beyond = c.pos - end
			}
			if !ok || beyond < minSpan {
				continue
			}
			// No other border between c and the end, nor at it.
			for _, o := range cross {
				between := (high && o.pos > c.pos+endTol && o.pos <= end+2*endTol) ||
					(!high && o.pos < c.pos-endTol && o.pos >= end-2*endTol)
				if between && o.hi > lo+xTolPt && o.lo < hi-xTolPt {
					ok = false
					break
				}
			}
			if ok {
				out = append(out, rule{pos: end, lo: lo, hi: hi})
			}
		}
	}
	if len(out) != len(cross) {
		sort.Slice(out, func(i, j int) bool { return out[i].pos < out[j].pos })
	}
	return out
}

// splitAtGaps splits a component at a band no cell covers: a row (or column)
// of the component's grid that lies between cells without being part of any.
// groupCells joins cells whose edges are within cellCornerJoinPt, which also
// joins two tables drawn a few points apart (pdf-go-t81f); inside one table
// every band is covered — an empty ruled row is still cells, and a spanning
// cell covers the bands it crosses — so an uncovered band is the gap between
// two tables.
func splitAtGaps(comp []latticeCell) [][]latticeCell {
	var xs, ys []float64
	for _, c := range comp {
		xs = append(xs, c.LLX, c.URX)
		ys = append(ys, c.LLY, c.URY)
	}
	uncovered := func(lo, hi float64, vertical bool) bool {
		mid := (lo + hi) / 2
		for _, c := range comp {
			a, b := c.LLY, c.URY
			if vertical {
				a, b = c.LLX, c.URX
			}
			if a < mid && mid < b {
				return false
			}
		}
		return true
	}
	split := func(at float64, vertical bool) [][]latticeCell {
		var low, high []latticeCell
		for _, c := range comp {
			edge := c.URY
			if vertical {
				edge = c.URX
			}
			if edge <= at {
				low = append(low, c)
			} else {
				high = append(high, c)
			}
		}
		return append(splitAtGaps(low), splitAtGaps(high)...)
	}
	for _, axis := range []struct {
		pos      []float64
		vertical bool
	}{{snapPositions(ys), false}, {snapPositions(xs), true}} {
		for k := 0; k+1 < len(axis.pos); k++ {
			if uncovered(axis.pos[k], axis.pos[k+1], axis.vertical) {
				return split((axis.pos[k]+axis.pos[k+1])/2, axis.vertical)
			}
		}
	}
	return [][]latticeCell{comp}
}

// snapPositions collects the distinct boundary positions of a component
// (cell tops/bottoms or lefts/rights), snapped within tolerance.
func snapPositions(vals []float64) []float64 {
	if len(vals) == 0 {
		return nil
	}
	sort.Float64s(vals)
	var out []float64
	sum, n := vals[0], 1
	prev := vals[0]
	for _, v := range vals[1:] {
		if v-prev <= xTolPt {
			sum += v
			n++
			prev = v
			continue
		}
		out = append(out, sum/float64(n))
		sum, n, prev = v, 1, v
	}
	out = append(out, sum/float64(n))
	return out
}

// nearestIdx returns the index of the closest position, or -1 when it is
// farther than tol.
func nearestIdx(positions []float64, v, tol float64) int {
	best, bestD := -1, tol
	for i, p := range positions {
		d := p - v
		if d < 0 {
			d = -d
		}
		if d <= bestD {
			best, bestD = i, d
		}
	}
	return best
}
