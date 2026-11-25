// SPDX-License-Identifier: MIT

package asposepdf

import "math"

// clipTracker follows the rectangular part of the clipping path through a
// content stream, so text the clip hides can be told from text it shows
// (pdf-go-64zt): a table cell's text that runs on under the next cell is
// drawn there but clipped at the rule, and a region over the next cell must
// not mistake it for that cell's own. Only clips built from rectangles under
// an axis-aligned CTM narrow it (several rectangles count as their bounding
// box); any other clip path leaves it as it was, so nothing is ever taken for
// hidden that may show.
type clipTracker struct {
	// clip is the page-space bound of the clip, nil when none is known.
	clip *Rectangle

	path     Rectangle // bounding box of the path's rectangles so far
	havePath bool
	complex  bool // the path has something other than axis-aligned rectangles
	pending  bool // W or W* seen: the path clips once it is painted
}

// op updates the tracker for one content-stream operator drawn under ctm.
// q and Q are the caller's: it saves and restores clip with the rest of its
// graphics state.
func (c *clipTracker) op(op contentOp, ctm [6]float64) {
	switch op.Operator {
	case "re":
		if len(op.Operands) < 4 || ctm[1] != 0 || ctm[2] != 0 {
			c.complex = true
			return
		}
		x, y := operandFloat(op.Operands[0]), operandFloat(op.Operands[1])
		w, h := operandFloat(op.Operands[2]), operandFloat(op.Operands[3])
		x0, y0 := matApplyPoint(ctm, x, y)
		x1, y1 := matApplyPoint(ctm, x+w, y+h)
		r := Rectangle{LLX: math.Min(x0, x1), LLY: math.Min(y0, y1), URX: math.Max(x0, x1), URY: math.Max(y0, y1)}
		if c.havePath {
			r = unionRect(c.path, r)
		}
		c.path, c.havePath = r, true
	case "m", "l", "c", "v", "y", "h":
		c.complex = true
	case "W", "W*":
		c.pending = true
	case "n", "f", "F", "f*", "S", "s", "B", "B*", "b", "b*":
		if c.pending && c.havePath && !c.complex {
			r := c.path
			if c.clip != nil {
				r = Rectangle{
					LLX: math.Max(r.LLX, c.clip.LLX), LLY: math.Max(r.LLY, c.clip.LLY),
					URX: math.Min(r.URX, c.clip.URX), URY: math.Min(r.URY, c.clip.URY),
				}
			}
			c.clip = &r
		}
		c.path, c.havePath, c.complex, c.pending = Rectangle{}, false, false, false
	}
}

// hides reports whether the clip hides the point (x, y) in page space.
func (c *clipTracker) hides(x, y float64) bool {
	const tol = 0.01
	return c.clip != nil && (x < c.clip.LLX-tol || x > c.clip.URX+tol || y < c.clip.LLY-tol || y > c.clip.URY+tol)
}
