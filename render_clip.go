// SPDX-License-Identifier: MIT

package asposepdf

// applyPendingClip intersects the current path into the graphics-state clip if a
// W / W* was seen since the last paint. It is called by every painting operator
// (and by n) before the path is cleared, because the clip update takes effect
// only after the path-painting operation that follows the clip operator
// (ISO 32000-1 §8.5.4). The clip is a per-pixel coverage mask in [0,1]; nil
// means unclipped. q/Q save and restore it with the rest of the graphics state,
// and because intersectClip allocates a fresh mask we never mutate a saved one.
func (rd *renderer) applyPendingClip() {
	if rd.pendingClip == 0 {
		return
	}
	rule := fillNonZero
	if rd.pendingClip == 2 {
		rule = fillEvenOdd
	}
	cov := rd.ras.coverage(rd.fl.path(), rule)
	rd.gs.clip = intersectClip(rd.gs.clip, cov)
	rd.pendingClip = 0
}
