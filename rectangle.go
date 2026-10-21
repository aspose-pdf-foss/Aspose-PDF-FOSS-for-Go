// SPDX-License-Identifier: MIT

package asposepdf

import (
	"fmt"
	"math"
)

// Rectangle represents a PDF rectangle [llx, lly, urx, ury] in points (1/72 inch).
type Rectangle struct {
	LLX, LLY float64 // lower-left corner
	URX, URY float64 // upper-right corner
}

// validate checks that the rectangle has positive, finite width and
// height.
//
// The width/height comparisons are written as !(URX > LLX) rather than
// URX <= LLX so a NaN coordinate fails too: any comparison with NaN is
// false, so the direct form let NaN silently pass (GitHub issue #19,
// shadowhunter1470-svg) — negating a false comparison is true, so the
// negated form rejects it instead of letting it through to become
// unparseable PDF syntax downstream. That trick alone does not catch
// +Inf/-Inf, though: infinity compares as genuinely greater than any
// finite number, so e.g. URX=+Inf, LLX=0 passes "has positive width"
// under IEEE 754 — a separate explicit finiteness check is needed for
// that case.
func (r Rectangle) validate() error {
	for _, v := range [4]float64{r.LLX, r.LLY, r.URX, r.URY} {
		if math.IsInf(v, 0) {
			return fmt.Errorf("invalid rectangle: coordinates must be finite, got %v", v)
		}
	}
	if !(r.URX > r.LLX) {
		return fmt.Errorf("invalid rectangle: URX (%.2f) must be greater than LLX (%.2f)", r.URX, r.LLX)
	}
	if !(r.URY > r.LLY) {
		return fmt.Errorf("invalid rectangle: URY (%.2f) must be greater than LLY (%.2f)", r.URY, r.LLY)
	}
	return nil
}
