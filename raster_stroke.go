// SPDX-License-Identifier: MIT

package asposepdf

import "math"

// strokeToFill converts a flattened device-space path into a fill outline that
// approximates a stroke of half-width hw. Each segment becomes a rectangle and
// each vertex a disc, giving round joins and round caps. The pieces overlap;
// every piece is forced to a consistent winding so that filling the whole set
// with the nonzero rule yields their union (no cancellation at overlaps).
//
// This is the simple, correct P1 stroker. Proper miter/bevel joins, butt/square
// caps, and dash patterns arrive in P6.
func strokeToFill(dp *devPath, hw float64) *devPath {
	if hw <= 0 {
		hw = 0.5
	}
	out := &devPath{}
	for _, sp := range dp.subs {
		pts := sp.pts
		switch {
		case len(pts) == 0:
			continue
		case len(pts) == 1:
			out.subs = append(out.subs, discPolygon(pts[0], hw))
			continue
		}
		for i := 0; i+1 < len(pts); i++ {
			if q, ok := segmentQuad(pts[i], pts[i+1], hw); ok {
				out.subs = append(out.subs, q)
			}
		}
		// Discs at every vertex cover round joins (interior) and round caps
		// (ends). Degenerate duplicate vertices are harmless.
		for _, p := range pts {
			out.subs = append(out.subs, discPolygon(p, hw))
		}
	}
	return out
}

// segmentQuad returns the rectangle covering segment a→b expanded by hw on each
// side, with positive winding. ok is false for a zero-length segment.
func segmentQuad(a, b point, hw float64) (subpath, bool) {
	dx, dy := b.x-a.x, b.y-a.y
	l := math.Hypot(dx, dy)
	if l < 1e-9 {
		return subpath{}, false
	}
	nx, ny := -dy/l*hw, dx/l*hw // left normal scaled to hw
	sp := subpath{closed: true, pts: []point{
		{a.x + nx, a.y + ny},
		{b.x + nx, b.y + ny},
		{b.x - nx, b.y - ny},
		{a.x - nx, a.y - ny},
	}}
	ensurePositiveWinding(&sp)
	return sp, true
}

// discPolygon approximates a filled circle of radius r centred at c.
func discPolygon(c point, r float64) subpath {
	const n = 16
	pts := make([]point, n)
	for i := 0; i < n; i++ {
		a := 2 * math.Pi * float64(i) / float64(n)
		pts[i] = point{c.x + r*math.Cos(a), c.y + r*math.Sin(a)}
	}
	sp := subpath{pts: pts, closed: true}
	ensurePositiveWinding(&sp)
	return sp
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
