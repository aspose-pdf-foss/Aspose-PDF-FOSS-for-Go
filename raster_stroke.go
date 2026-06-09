// SPDX-License-Identifier: MIT

package asposepdf

import "math"

// strokeStyle carries the stroke parameters the rasterizing stroker honors:
// half line width, line cap, line join, and miter limit (ISO 32000-1 §8.4.3).
type strokeStyle struct {
	hw         float64
	cap        LineCap
	join       LineJoin
	miterLimit float64
}

// strokeToFill converts a flattened device-space path into a fill outline that
// reproduces a stroke: each segment becomes a rectangle, each interior vertex a
// join (miter / round / bevel), and each open end a cap (butt / round / square).
// Pieces overlap and are forced to one winding so a nonzero fill yields their
// union. Dash patterns are applied beforehand (see applyDash); a closed subpath
// is stroked with joins all round and no caps.
func strokeToFill(dp *devPath, st strokeStyle) *devPath {
	if st.hw <= 0 {
		st.hw = 0.5
	}
	if st.miterLimit < 1 {
		st.miterLimit = 10
	}
	out := &devPath{}
	for _, sp := range dp.subs {
		pts := dedupePoints(sp.pts)
		// A closed subpath whose path explicitly draws back to the start leaves
		// the last point coincident with the first. Drop it so the seam join is
		// computed between two real (non-zero-length) segments; otherwise the
		// start vertex's join degenerates and the corner is truncated — e.g. a
		// star's tip rendered flat instead of pointed.
		if sp.closed && len(pts) > 1 {
			f, l := pts[0], pts[len(pts)-1]
			if math.Abs(f.x-l.x) <= 1e-9 && math.Abs(f.y-l.y) <= 1e-9 {
				pts = pts[:len(pts)-1]
			}
		}
		strokeSubpath(out, pts, sp.closed, st)
	}
	return out
}

func strokeSubpath(out *devPath, pts []point, closed bool, st strokeStyle) {
	hw := st.hw
	if len(pts) == 0 {
		return
	}
	if len(pts) == 1 { // a lone point only marks under round/square caps
		switch st.cap {
		case LineCapRound:
			out.subs = append(out.subs, discPolygon(pts[0], hw))
		case LineCapSquare:
			out.subs = append(out.subs, squareDot(pts[0], hw))
		}
		return
	}

	segEnd := len(pts) - 1
	if closed {
		segEnd = len(pts) // also stroke the closing edge pts[n-1]→pts[0]
	}
	for i := 0; i < segEnd; i++ {
		if q, ok := segmentQuad(pts[i], pts[(i+1)%len(pts)], hw); ok {
			out.subs = append(out.subs, q)
		}
	}

	if closed {
		for i := 0; i < len(pts); i++ {
			addJoin(out, pts[(i-1+len(pts))%len(pts)], pts[i], pts[(i+1)%len(pts)], st)
		}
		return
	}
	for i := 1; i < len(pts)-1; i++ {
		addJoin(out, pts[i-1], pts[i], pts[i+1], st)
	}
	addCap(out, pts[0], pts[1], st)                 // start cap (dir points outward)
	addCap(out, pts[len(pts)-1], pts[len(pts)-2], st) // end cap
}

// addCap adds the cap shape at end, whose outward direction is end−neighbor.
func addCap(out *devPath, end, neighbor point, st strokeStyle) {
	dx, dy, ok := unit(end.x-neighbor.x, end.y-neighbor.y)
	if !ok {
		return
	}
	switch st.cap {
	case LineCapRound:
		out.subs = append(out.subs, discPolygon(end, st.hw))
	case LineCapSquare:
		nx, ny := -dy*st.hw, dx*st.hw // left normal · hw
		ex, ey := dx*st.hw, dy*st.hw  // extension · hw
		out.subs = append(out.subs, closedPoly(
			point{end.x + nx, end.y + ny},
			point{end.x + nx + ex, end.y + ny + ey},
			point{end.x - nx + ex, end.y - ny + ey},
			point{end.x - nx, end.y - ny},
		))
	}
	// LineCapButt: the segment rectangle already ends flush — nothing to add.
}

// addJoin fills the wedge between the two segments meeting at cur.
func addJoin(out *devPath, prev, cur, next point, st strokeStyle) {
	d0x, d0y, ok0 := unit(cur.x-prev.x, cur.y-prev.y)
	d1x, d1y, ok1 := unit(next.x-cur.x, next.y-cur.y)
	if !ok0 || !ok1 {
		return
	}
	cross := d0x*d1y - d0y*d1x
	if math.Abs(cross) < 1e-9 {
		return // collinear: segments already abut, no wedge
	}
	hw := st.hw
	n0x, n0y := -d0y*hw, d0x*hw // left normals · hw
	n1x, n1y := -d1y*hw, d1x*hw
	// Outer side is opposite the turn: left turn (cross>0) → right normals.
	var a, b point
	if cross > 0 {
		a = point{cur.x - n0x, cur.y - n0y}
		b = point{cur.x - n1x, cur.y - n1y}
	} else {
		a = point{cur.x + n0x, cur.y + n0y}
		b = point{cur.x + n1x, cur.y + n1y}
	}

	switch st.join {
	case LineJoinRound:
		out.subs = append(out.subs, discPolygon(cur, hw))
	case LineJoinBevel:
		out.subs = append(out.subs, closedPoly(cur, a, b))
	default: // LineJoinMiter, falling back to bevel past the miter limit
		dot := d0x*d1x + d0y*d1y
		if 1+dot <= 1e-12 || 1/math.Sqrt((1+dot)/2) > st.miterLimit {
			out.subs = append(out.subs, closedPoly(cur, a, b))
			return
		}
		if m, ok := lineIntersect(a, d0x, d0y, b, d1x, d1y); ok {
			out.subs = append(out.subs, closedPoly(cur, a, m, b))
		} else {
			out.subs = append(out.subs, closedPoly(cur, a, b))
		}
	}
}

// segmentQuad returns the rectangle covering segment a→b expanded by hw on each
// side, with positive winding. ok is false for a zero-length segment.
func segmentQuad(a, b point, hw float64) (subpath, bool) {
	dx, dy, ok := unit(b.x-a.x, b.y-a.y)
	if !ok {
		return subpath{}, false
	}
	nx, ny := -dy*hw, dx*hw // left normal scaled to hw
	return closedPoly(
		point{a.x + nx, a.y + ny},
		point{b.x + nx, b.y + ny},
		point{b.x - nx, b.y - ny},
		point{a.x - nx, a.y - ny},
	), true
}

// discPolygon approximates a filled circle of radius r centred at c.
func discPolygon(c point, r float64) subpath {
	const n = 16
	pts := make([]point, n)
	for i := 0; i < n; i++ {
		a := 2 * math.Pi * float64(i) / float64(n)
		pts[i] = point{c.x + r*math.Cos(a), c.y + r*math.Sin(a)}
	}
	return closedPolyPts(pts)
}

// squareDot is the axis-aligned square of half-side r centred at c (for a square
// cap on a zero-length subpath, which has no direction).
func squareDot(c point, r float64) subpath {
	return closedPoly(
		point{c.x - r, c.y - r}, point{c.x + r, c.y - r},
		point{c.x + r, c.y + r}, point{c.x - r, c.y + r},
	)
}

// closedPoly builds a closed, positively-wound subpath from the given vertices.
func closedPoly(pts ...point) subpath { return closedPolyPts(pts) }

func closedPolyPts(pts []point) subpath {
	sp := subpath{pts: pts, closed: true}
	ensurePositiveWinding(&sp)
	return sp
}

// unit returns the normalized (dx,dy) and false for a near-zero vector.
func unit(dx, dy float64) (float64, float64, bool) {
	l := math.Hypot(dx, dy)
	if l < 1e-9 {
		return 0, 0, false
	}
	return dx / l, dy / l, true
}

// lineIntersect returns the intersection of line a+t·d0 and line b+u·d1.
func lineIntersect(a point, d0x, d0y float64, b point, d1x, d1y float64) (point, bool) {
	denom := d0x*d1y - d0y*d1x
	if math.Abs(denom) < 1e-12 {
		return point{}, false
	}
	t := ((b.x-a.x)*d1y - (b.y-a.y)*d1x) / denom
	return point{a.x + d0x*t, a.y + d0y*t}, true
}

// dedupePoints drops consecutive duplicate points so direction math is stable.
func dedupePoints(pts []point) []point {
	if len(pts) == 0 {
		return pts
	}
	out := pts[:1]
	for _, p := range pts[1:] {
		last := out[len(out)-1]
		if math.Abs(p.x-last.x) > 1e-9 || math.Abs(p.y-last.y) > 1e-9 {
			out = append(out, p)
		}
	}
	return out
}

// ensurePositiveWinding reverses the subpath's points if its signed area is
// negative, so all stroke pieces share one winding direction.
func ensurePositiveWinding(sp *subpath) {
	if signedArea(sp.pts) < 0 {
		for i, j := 0, len(sp.pts)-1; i < j; i, j = i+1, j-1 {
			sp.pts[i], sp.pts[j] = sp.pts[j], sp.pts[i]
		}
	}
}

func signedArea(pts []point) float64 {
	var a float64
	for i := 0; i < len(pts); i++ {
		j := (i + 1) % len(pts)
		a += pts[i].x*pts[j].y - pts[j].x*pts[i].y
	}
	return a / 2
}

// applyDash splits each subpath of dp into the "on" intervals of a dash pattern
// (lengths in the same device units as dp), returning a path of open subpaths to
// be stroked with caps. A nil/empty/zero-sum pattern returns dp unchanged.
func applyDash(dp *devPath, dash []float64, phase float64) *devPath {
	pat := normalizeDash(dash)
	if pat == nil {
		return dp
	}
	out := &devPath{}
	for _, sp := range dp.subs {
		pts := sp.pts
		if sp.closed && len(pts) > 1 {
			pts = append(append([]point(nil), pts...), pts[0]) // include closing edge
		}
		dashPolyline(out, pts, pat, phase)
	}
	return out
}

// normalizeDash validates a dash array: it rejects negatives and zero-sum
// patterns (→ nil = solid) and doubles an odd-length array per ISO 32000-1.
func normalizeDash(dash []float64) []float64 {
	if len(dash) == 0 {
		return nil
	}
	sum := 0.0
	for _, d := range dash {
		if d < 0 {
			return nil
		}
		sum += d
	}
	if sum <= 0 {
		return nil
	}
	if len(dash)%2 == 1 {
		dash = append(append([]float64(nil), dash...), dash...)
	}
	return dash
}

// dashPolyline walks the polyline emitting the "on" spans of the dash pattern as
// open subpaths into out.
func dashPolyline(out *devPath, pts []point, pat []float64, phase float64) {
	if len(pts) < 2 {
		return
	}
	total := 0.0
	for _, d := range pat {
		total += d
	}
	pos := math.Mod(phase, total)
	if pos < 0 {
		pos += total
	}
	idx := 0
	for pos >= pat[idx] {
		pos -= pat[idx]
		idx = (idx + 1) % len(pat)
	}
	remaining := pat[idx] - pos
	on := idx%2 == 0

	var cur []point
	if on {
		cur = append(cur, pts[0])
	}
	flush := func() {
		if len(cur) >= 2 {
			out.subs = append(out.subs, subpath{pts: cur})
		}
		cur = nil
	}
	for i := 0; i+1 < len(pts); i++ {
		a, b := pts[i], pts[i+1]
		dx, dy, ok := unit(b.x-a.x, b.y-a.y)
		if !ok {
			continue
		}
		segLen := math.Hypot(b.x-a.x, b.y-a.y)
		traveled := 0.0
		for segLen-traveled > 1e-9 {
			step := math.Min(remaining, segLen-traveled)
			traveled += step
			remaining -= step
			p := point{a.x + dx*traveled, a.y + dy*traveled}
			if on {
				cur = append(cur, p)
			}
			if remaining <= 1e-9 { // dash boundary reached
				if on {
					flush()
				} else {
					cur = []point{p} // begin a new "on" run
				}
				on = !on
				idx = (idx + 1) % len(pat)
				remaining = pat[idx]
			}
		}
	}
	if on {
		flush()
	}
}
