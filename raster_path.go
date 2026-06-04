// SPDX-License-Identifier: MIT

package asposepdf

import "math"

// point is a device-space coordinate (pixels, Y-down).
type point struct{ x, y float64 }

// subpath is a flattened polyline. For filling it is treated as implicitly
// closed (the rasterizer connects the last point back to the first); the
// closed flag records whether the source path explicitly closed it (used by
// stroking).
type subpath struct {
	pts    []point
	closed bool
}

// devPath is a path flattened to straight segments in device space.
type devPath struct{ subs []subpath }

// flattener builds a devPath from move/line/curve calls, adaptively subdividing
// Bézier curves until they are within tol device units of straight segments.
type flattener struct {
	tol            float64
	subs           []subpath
	cur            []point
	startX, startY float64
	curX, curY     float64
	open           bool
}

func newFlattener(tol float64) *flattener {
	if tol <= 0 {
		tol = 0.2
	}
	return &flattener{tol: tol}
}

func (f *flattener) flush() {
	if f.open && len(f.cur) > 0 {
		f.subs = append(f.subs, subpath{pts: f.cur})
	}
	f.cur = nil
	f.open = false
}

func (f *flattener) moveTo(x, y float64) {
	f.flush()
	f.cur = []point{{x, y}}
	f.startX, f.startY = x, y
	f.curX, f.curY = x, y
	f.open = true
}

func (f *flattener) lineTo(x, y float64) {
	if !f.open {
		f.moveTo(x, y)
		return
	}
	f.cur = append(f.cur, point{x, y})
	f.curX, f.curY = x, y
}

func (f *flattener) cubicTo(x1, y1, x2, y2, x3, y3 float64) {
	if !f.open {
		f.moveTo(x1, y1)
	}
	flattenCubic(f.curX, f.curY, x1, y1, x2, y2, x3, y3, f.tol, 0, &f.cur)
	f.curX, f.curY = x3, y3
}

func (f *flattener) quadTo(x1, y1, x2, y2 float64) {
	if !f.open {
		f.moveTo(x1, y1)
	}
	// Elevate the quadratic to an equivalent cubic.
	c1x := f.curX + 2.0/3.0*(x1-f.curX)
	c1y := f.curY + 2.0/3.0*(y1-f.curY)
	c2x := x2 + 2.0/3.0*(x1-x2)
	c2y := y2 + 2.0/3.0*(y1-y2)
	f.cubicTo(c1x, c1y, c2x, c2y, x2, y2)
}

func (f *flattener) close() {
	if f.open && len(f.cur) > 0 {
		if f.curX != f.startX || f.curY != f.startY {
			f.cur = append(f.cur, point{f.startX, f.startY})
		}
		f.subs = append(f.subs, subpath{pts: f.cur, closed: true})
		f.cur = nil
		f.open = false
		f.curX, f.curY = f.startX, f.startY
	}
}

func (f *flattener) path() *devPath {
	f.flush()
	return &devPath{subs: f.subs}
}

// flattenCubic recursively subdivides a cubic Bézier, appending the end point
// of each sufficiently-flat segment to out. The start point is assumed already
// emitted by the caller.
func flattenCubic(x0, y0, x1, y1, x2, y2, x3, y3, tol float64, depth int, out *[]point) {
	if depth >= 24 {
		*out = append(*out, point{x3, y3})
		return
	}
	// Flat enough when both control points are within tol of the chord.
	if distPointLine(x1, y1, x0, y0, x3, y3) <= tol &&
		distPointLine(x2, y2, x0, y0, x3, y3) <= tol {
		*out = append(*out, point{x3, y3})
		return
	}
	x01, y01 := midpoint(x0, y0, x1, y1)
	x12, y12 := midpoint(x1, y1, x2, y2)
	x23, y23 := midpoint(x2, y2, x3, y3)
	x012, y012 := midpoint(x01, y01, x12, y12)
	x123, y123 := midpoint(x12, y12, x23, y23)
	xm, ym := midpoint(x012, y012, x123, y123)
	flattenCubic(x0, y0, x01, y01, x012, y012, xm, ym, tol, depth+1, out)
	flattenCubic(xm, ym, x123, y123, x23, y23, x3, y3, tol, depth+1, out)
}

func midpoint(ax, ay, bx, by float64) (float64, float64) {
	return (ax + bx) / 2, (ay + by) / 2
}

// distPointLine returns the perpendicular distance from (px,py) to the line
// through (ax,ay)-(bx,by). If the line is degenerate it returns the point
// distance.
func distPointLine(px, py, ax, ay, bx, by float64) float64 {
	dx, dy := bx-ax, by-ay
	l := math.Hypot(dx, dy)
	if l < 1e-12 {
		return math.Hypot(px-ax, py-ay)
	}
	return math.Abs((px-ax)*dy-(py-ay)*dx) / l
}
