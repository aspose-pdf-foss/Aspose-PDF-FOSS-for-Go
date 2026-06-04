// SPDX-License-Identifier: MIT

package asposepdf

import (
	"image"
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

func TestFillSolidRect(t *testing.T) {
	r := newRasterizer(10, 10)
	cov := r.fillPolygon([]point{{2, 2}, {8, 2}, {8, 8}, {2, 8}}, fillNonZero)
	if c := cov[5*10+5]; c < 0.99 {
		t.Errorf("interior (5,5) coverage = %.3f, want ~1", c)
	}
	if c := cov[0]; c > 0.01 {
		t.Errorf("exterior (0,0) coverage = %.3f, want ~0", c)
	}
	// Total coverage equals the 6x6 area within AA tolerance.
	var sum float64
	for _, c := range cov {
		sum += float64(c)
	}
	if math.Abs(sum-36) > 0.5 {
		t.Errorf("total coverage = %.2f, want ~36 (6x6)", sum)
	}
}

func TestFillCircleCoverageArea(t *testing.T) {
	const n = 128
	cx, cy, rad := 25.0, 25.0, 15.0
	pts := make([]point, n)
	for i := 0; i < n; i++ {
		a := 2 * math.Pi * float64(i) / float64(n)
		pts[i] = point{cx + rad*math.Cos(a), cy + rad*math.Sin(a)}
	}
	r := newRasterizer(50, 50)
	cov := r.fillPolygon(pts, fillNonZero)

	if c := cov[25*50+25]; c < 0.99 {
		t.Errorf("disc centre coverage = %.3f, want ~1", c)
	}
	if c := cov[2*50+2]; c > 0.01 {
		t.Errorf("far-outside coverage = %.3f, want ~0", c)
	}
	var sum float64
	for _, c := range cov {
		sum += float64(c)
	}
	want := math.Pi * rad * rad
	if rel := math.Abs(sum-want) / want; rel > 0.02 {
		t.Errorf("total coverage = %.1f, want ~%.1f (rel err %.3f)", sum, want, rel)
	}
}

// pentagram returns the 5 outer vertices of a self-intersecting star; drawing
// them in order 0,1,2,3,4 with implicit close winds the centre twice.
func pentagram(cx, cy, rad float64) []point {
	idx := []int{0, 2, 4, 1, 3}
	pts := make([]point, 5)
	for i, k := range idx {
		a := -math.Pi/2 + 2*math.Pi*float64(k)/5
		pts[i] = point{cx + rad*math.Cos(a), cy + rad*math.Sin(a)}
	}
	return pts
}

func TestEvenOddVsNonZeroStar(t *testing.T) {
	pts := pentagram(25, 25, 20)
	r := newRasterizer(50, 50)
	center := 25*50 + 25

	nz := r.fillPolygon(pts, fillNonZero)
	if nz[center] < 0.99 {
		t.Errorf("nonzero star centre = %.3f, want ~1 (doubly wound)", nz[center])
	}
	eo := r.fillPolygon(pts, fillEvenOdd)
	if eo[center] > 0.01 {
		t.Errorf("even-odd star centre = %.3f, want ~0 (hole)", eo[center])
	}
}

func TestCompositeOverWhite(t *testing.T) {
	const w, h = 2, 2
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := range dst.Pix {
		dst.Pix[i] = 255 // opaque white
	}
	cov := []float32{0.5, 0.5, 0.5, 0.5}
	compositeCoverage(dst, w, cov, 255, 0, 0, 1.0, nil) // red at 50% coverage

	// red over white at 50% → (255,127,127)
	r, g, b, a := dst.Pix[0], dst.Pix[1], dst.Pix[2], dst.Pix[3]
	if r != 255 || abs8(g, 127) > 1 || abs8(b, 127) > 1 || a != 255 {
		t.Errorf("composited pixel = (%d,%d,%d,%d), want ~(255,127,127,255)", r, g, b, a)
	}
}

func TestClipMaskIntersect(t *testing.T) {
	a := []float32{1, 0.5, 0, 1}
	b := []float32{0.5, 0.5, 1, 0}
	got := intersectClip(a, b)
	want := []float32{0.5, 0.25, 0, 0}
	for i := range want {
		if math.Abs(float64(got[i]-want[i])) > 1e-6 {
			t.Errorf("intersectClip[%d] = %v, want %v", i, got[i], want[i])
		}
	}
	if intersectClip(nil, b)[2] != 1 {
		t.Error("intersectClip(nil, b) should return b")
	}
}

func abs8(a, b uint8) int {
	d := int(a) - int(b)
	if d < 0 {
		return -d
	}
	return d
}
