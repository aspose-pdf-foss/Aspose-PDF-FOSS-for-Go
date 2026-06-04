// SPDX-License-Identifier: MIT

package asposepdf

import (
	"math"
	"testing"
)

// TestCoverageBBoxMatchesFullFrame guards the P6 performance optimization: the
// bbox-scoped coverage must agree pixel-for-pixel with the whole-frame coverage
// inside the bbox, and the whole-frame buffer must be empty outside it.
func TestCoverageBBoxMatchesFullFrame(t *testing.T) {
	r := newRasterizer(120, 90)
	pts := []point{{20, 15}, {95, 30}, {55, 78}} // an off-centre triangle
	full := r.fillPolygon(pts, fillNonZero)

	f := newFlattener(0.2)
	f.moveTo(pts[0].x, pts[0].y)
	f.lineTo(pts[1].x, pts[1].y)
	f.lineTo(pts[2].x, pts[2].y)
	f.close()
	cov, bx0, by0, bx1, by1 := r.coverageBBox(f.path(), fillNonZero)
	if cov == nil {
		t.Fatal("coverageBBox returned nil for a non-empty path")
	}
	bw := bx1 - bx0

	var maxDiff, outside float32
	for y := 0; y < r.h; y++ {
		for x := 0; x < r.w; x++ {
			fv := full[y*r.w+x]
			if x >= bx0 && x < bx1 && y >= by0 && y < by1 {
				bv := cov[(y-by0)*bw+(x-bx0)]
				if d := float32(math.Abs(float64(fv - bv))); d > maxDiff {
					maxDiff = d
				}
			} else if fv > outside {
				outside = fv // full-frame coverage outside the reported bbox
			}
		}
	}
	if maxDiff > 1e-6 {
		t.Errorf("bbox vs full coverage differ by %g inside bbox", maxDiff)
	}
	if outside > 1e-6 {
		t.Errorf("full-frame coverage %g leaked outside the reported bbox", outside)
	}
}
