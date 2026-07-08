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
	if rd.vec != nil {
		// Mirror the clip as an SVG <clipPath> chained onto the current one.
		// The raster clip above stays live too — patches paint through it.
		rd.gs.vecClip = rd.vec.addClip(rd.vec.pathData(), rule, rd.gs.vecClip)
	}
}

// applyTextClip is called at ET: if any glyphs were drawn under a clipping text
// rendering mode (4-7) since BT, their combined outline (nonzero winding)
// intersects the graphics-state clip (ISO 32000-1 §9.4.3). The clip then
// constrains subsequent painting until the enclosing q/Q restores it — the
// mechanism behind "show glyphs as a clip, then paint an image through them".
func (rd *renderer) applyTextClip() {
	if len(rd.textClip) == 0 {
		return
	}
	cov := rd.ras.coverage(&devPath{subs: rd.textClip}, fillNonZero)
	rd.gs.clip = intersectClip(rd.gs.clip, cov)
	if rd.vec != nil {
		rd.gs.vecClip = rd.vec.addClip(devPathData(&devPath{subs: rd.textClip}), fillNonZero, rd.gs.vecClip)
	}
	rd.textClip = nil
}
