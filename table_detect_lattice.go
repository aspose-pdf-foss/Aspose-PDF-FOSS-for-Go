// SPDX-License-Identifier: MIT

package asposepdf

import (
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
	var find func(int) int
	find = func(x int) int {
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
