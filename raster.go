// SPDX-License-Identifier: MIT

package asposepdf

import (
	"math"
	"sort"
)

// fillRule selects the winding rule used to decide which regions of a path are
// inside.
type fillRule int

const (
	fillNonZero fillRule = iota // nonzero winding (PDF f/F, B)
	fillEvenOdd                 // even-odd (PDF f*, B*)
)

// rasterizer turns flattened device-space paths into per-pixel coverage in
// [0,1]. It uses analytic coverage along X and supersampling along Y, which
// gives smooth anti-aliasing with a small, predictable amount of work and no
// dependencies.
type rasterizer struct {
	w, h int
}

func newRasterizer(w, h int) *rasterizer { return &rasterizer{w: w, h: h} }

// ssY is the number of sub-scanlines per pixel row (vertical supersampling).
// X coverage is analytic, so 4 already yields good quality.
const ssY = 4

type rasterEdge struct {
	ytop, ybot float64 // ytop < ybot
	xAtTop     float64
	slope      float64 // dx/dy
	dir        int     // +1 if original edge went downward (y increasing), else -1
}

type xCrossing struct {
	x   float64
	dir int
}

// coverage rasterizes dp under the given fill rule, returning a w*h coverage
// buffer (row-major) with values in [0,1].
func (r *rasterizer) coverage(dp *devPath, rule fillRule) []float32 {
	cov := make([]float32, r.w*r.h)
	if r.w <= 0 || r.h <= 0 {
		return cov
	}

	var edges []rasterEdge
	ymin, ymax := math.Inf(1), math.Inf(-1)
	for _, sp := range dp.subs {
		n := len(sp.pts)
		if n < 2 {
			continue
		}
		for i := 0; i < n; i++ {
			a := sp.pts[i]
			b := sp.pts[(i+1)%n] // implicitly close every subpath for filling
			if a.y == b.y {
				continue // horizontal edges never cross a scanline
			}
			dir := 1
			if a.y > b.y {
				a, b = b, a
				dir = -1
			}
			edges = append(edges, rasterEdge{
				ytop:   a.y,
				ybot:   b.y,
				xAtTop: a.x,
				slope:  (b.x - a.x) / (b.y - a.y),
				dir:    dir,
			})
			if a.y < ymin {
				ymin = a.y
			}
			if b.y > ymax {
				ymax = b.y
			}
		}
	}
	if len(edges) == 0 {
		return cov
	}

	// Only scan rows the path actually touches.
	py0 := int(math.Floor(ymin))
	py1 := int(math.Ceil(ymax))
	if py0 < 0 {
		py0 = 0
	}
	if py1 > r.h {
		py1 = r.h
	}

	const inv = 1.0 / float64(ssY)
	xs := make([]xCrossing, 0, len(edges))
	for py := py0; py < py1; py++ {
		for s := 0; s < ssY; s++ {
			sy := float64(py) + (float64(s)+0.5)*inv
			xs = xs[:0]
			for i := range edges {
				e := &edges[i]
				if sy < e.ytop || sy >= e.ybot {
					continue
				}
				xs = append(xs, xCrossing{x: e.xAtTop + (sy-e.ytop)*e.slope, dir: e.dir})
			}
			if len(xs) < 2 {
				continue
			}
			sort.Slice(xs, func(i, j int) bool { return xs[i].x < xs[j].x })

			wind := 0
			for i := 0; i+1 < len(xs); i++ {
				wind += xs[i].dir
				if insideRule(wind, rule) {
					addSpanRow(cov, r.w, py, xs[i].x, xs[i+1].x, inv)
				}
			}
		}
	}
	return cov
}

func insideRule(wind int, rule fillRule) bool {
	if rule == fillEvenOdd {
		return wind&1 != 0
	}
	return wind != 0
}

// addSpanRow adds weight coverage over the horizontal span [xa,xb) of row py,
// with fractional coverage at the partially-covered end cells.
func addSpanRow(cov []float32, w, py int, xa, xb, weight float64) {
	if xb <= xa {
		return
	}
	if xa < 0 {
		xa = 0
	}
	if xb > float64(w) {
		xb = float64(w)
	}
	if xb <= xa {
		return
	}
	ix0 := int(math.Floor(xa))
	ix1 := int(math.Floor(xb))
	base := py * w
	if ix0 == ix1 {
		cov[base+ix0] += float32(weight * (xb - xa))
		return
	}
	cov[base+ix0] += float32(weight * (float64(ix0+1) - xa)) // first partial cell
	for ix := ix0 + 1; ix < ix1; ix++ {
		cov[base+ix] += float32(weight) // full cells
	}
	if ix1 < w {
		cov[base+ix1] += float32(weight * (xb - float64(ix1))) // last partial cell
	}
}

// fillPolygon is a small test/utility helper: it flattens a closed polygon and
// returns its coverage.
func (r *rasterizer) fillPolygon(pts []point, rule fillRule) []float32 {
	f := newFlattener(0.2)
	if len(pts) > 0 {
		f.moveTo(pts[0].x, pts[0].y)
		for _, p := range pts[1:] {
			f.lineTo(p.x, p.y)
		}
		f.close()
	}
	return r.coverage(f.path(), rule)
}
