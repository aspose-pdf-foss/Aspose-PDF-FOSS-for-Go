// SPDX-License-Identifier: MIT

package asposepdf

import (
	"image"
	"testing"
)

// TestStrokeClosedRedundantStartPoint guards the star-tip bug: a closed path that
// explicitly draws back to its start point (leaving the last point coincident
// with the first) must stroke identically to the same path closed with `h`
// alone. Before the fix the coincident seam made the start vertex's miter join
// degenerate, truncating that corner (the star's tip rendered flat).
func TestStrokeClosedRedundantStartPoint(t *testing.T) {
	// Up-pointing triangle, thick green miter stroke. apex (50,80) at the top.
	const style = "10 w 1 j 0 1 0 RG "
	clean := renderContent(t, style+"50 80 m 90 20 l 10 20 l h S\n").(*image.RGBA)
	redundant := renderContent(t, style+"50 80 m 50 80 l 90 20 l 10 20 l 50 80 l h S\n").(*image.RGBA)

	if clean.Bounds() != redundant.Bounds() {
		t.Fatalf("bounds differ: %v vs %v", clean.Bounds(), redundant.Bounds())
	}
	diff := 0
	for i := range clean.Pix {
		if clean.Pix[i] != redundant.Pix[i] {
			diff++
		}
	}
	if diff != 0 {
		t.Errorf("closed stroke with a redundant return-to-start point differs from the clean close in %d byte(s) — the seam join is degenerate", diff)
	}
}
