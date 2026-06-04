// SPDX-License-Identifier: MIT

package asposepdf

import (
	"math"
	"testing"
)

func minDist(pts []point, target point) float64 {
	best := math.Inf(1)
	for _, p := range pts {
		if d := math.Hypot(p.x-target.x, p.y-target.y); d < best {
			best = d
		}
	}
	return best
}

func TestFlattenLineAndCubic(t *testing.T) {
	f := newFlattener(0.2)
	f.moveTo(0, 0)
	f.lineTo(10, 0)
	dp := f.path()
	if len(dp.subs) != 1 {
		t.Fatalf("subs = %d, want 1", len(dp.subs))
	}
	if got := dp.subs[0].pts; len(got) != 2 || got[0] != (point{0, 0}) || got[1] != (point{10, 0}) {
		t.Fatalf("line pts = %v, want [(0,0) (10,0)]", got)
	}

	f2 := newFlattener(0.1)
	f2.moveTo(0, 0)
	f2.cubicTo(0, 10, 10, 10, 10, 0) // passes through (5, 7.5) at t=0.5
	pts := f2.path().subs[0].pts
	if pts[0] != (point{0, 0}) {
		t.Errorf("cubic start = %v, want (0,0)", pts[0])
	}
	if last := pts[len(pts)-1]; math.Abs(last.x-10) > 1e-9 || math.Abs(last.y) > 1e-9 {
		t.Errorf("cubic end = %v, want (10,0)", last)
	}
	if d := minDist(pts, point{5, 7.5}); d > 0.2 {
		t.Errorf("no flattened point within 0.2 of curve midpoint (5,7.5); min dist = %.3f", d)
	}
	// The polyline should subdivide a curved cubic into several segments.
	if len(pts) < 4 {
		t.Errorf("cubic flattened to only %d points; expected adaptive subdivision", len(pts))
	}
}

func TestFlattenClose(t *testing.T) {
	f := newFlattener(0.2)
	f.moveTo(0, 0)
	f.lineTo(10, 0)
	f.lineTo(10, 10)
	f.close()
	dp := f.path()
	if len(dp.subs) != 1 || !dp.subs[0].closed {
		t.Fatalf("expected one closed subpath, got %+v", dp.subs)
	}
	// close appends the start point back.
	pts := dp.subs[0].pts
	if pts[len(pts)-1] != (point{0, 0}) {
		t.Errorf("closed subpath should end at start; got %v", pts[len(pts)-1])
	}
}
