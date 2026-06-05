// SPDX-License-Identifier: MIT

package asposepdf

import "math"

// tilingPattern is a parsed PatternType 1 pattern: a content stream (the cell)
// tiled across an area by /XStep and /YStep, mapped through /Matrix. PaintType 1
// patterns carry their own colour; PaintType 2 (uncolored) inherit the current
// fill colour, captured in the graphics state when the pattern was selected.
type tilingPattern struct {
	content      []byte
	resources    pdfDict
	matrix       [6]float64 // pattern space → page default (raw /Matrix)
	xstep, ystep float64
}

// maxTiles bounds the tiling loop so a fine pattern over a large area cannot
// blow up render time; beyond it the fill is left unpainted.
const maxTiles = 20000

// setFillColor sets the fill colour from 1/3/4 numeric colour operands.
func (rd *renderer) setFillColor(o []pdfValue) {
	switch len(o) {
	case 1:
		rd.gs.fillR, rd.gs.fillG, rd.gs.fillB = gray8(f(o[0]))
	case 3:
		rd.gs.fillR, rd.gs.fillG, rd.gs.fillB = clamp8(f(o[0])), clamp8(f(o[1])), clamp8(f(o[2]))
	case 4:
		rd.gs.fillR, rd.gs.fillG, rd.gs.fillB = cmykToRGB8(f(o[0]), f(o[1]), f(o[2]), f(o[3]))
	}
}

// setFillPattern resolves a /Pattern resource for filling: PatternType 1 →
// fillTiling, PatternType 2 → fillShading. An unresolved pattern leaves both nil
// (the fill is skipped).
func (rd *renderer) setFillPattern(name string) {
	rd.gs.fillShading, rd.gs.fillTiling = nil, nil
	objects := rd.page.doc.objects
	pats, ok := resolveRefToDict(objects, rd.res["/Pattern"])
	if !ok {
		return
	}
	pd, stream := asDictStream(resolveRef(objects, pats[name]))
	if pd == nil {
		return
	}
	switch int(operandFloat(resolveRef(objects, pd["/PatternType"]))) {
	case 1:
		if stream != nil {
			rd.gs.fillTiling = rd.parseTilingPattern(stream, pd)
		}
	case 2:
		rd.gs.fillShading, rd.gs.fillShadingM = rd.resolveShadingPattern(name)
	}
}

func (rd *renderer) parseTilingPattern(stream *pdfStream, pd pdfDict) *tilingPattern {
	objects := rd.page.doc.objects
	xs := operandFloat(resolveRef(objects, pd["/XStep"]))
	ys := operandFloat(resolveRef(objects, pd["/YStep"]))
	if xs == 0 || ys == 0 {
		return nil
	}
	m := identityMatrix()
	if pm := shFloats(objects, pd["/Matrix"]); len(pm) == 6 {
		m = [6]float64{pm[0], pm[1], pm[2], pm[3], pm[4], pm[5]}
	}
	res, _ := resolveRefToDict(objects, pd["/Resources"])
	return &tilingPattern{
		content:   decodedStreamData(stream),
		resources: res,
		matrix:    m,
		xstep:     math.Abs(xs),
		ystep:     math.Abs(ys),
	}
}

// fillTilingPattern paints the current path with a tiling pattern: the cell
// content stream is executed once per tile position covering the path's device
// bounding box, clipped to the path ∩ current clip.
func (rd *renderer) fillTilingPattern(tp *tilingPattern, rule fillRule) {
	if rd.depth >= 8 {
		return
	}
	dp := rd.fl.path()
	bx0, by0, bx1, by1, ok := pathDeviceBounds(dp)
	if !ok {
		return
	}
	// Clip every tile to the fill region (path ∩ existing clip).
	clip := intersectClip(rd.effectiveClip(), rd.ras.coverage(dp, rule))

	// pattern space → device, and its inverse to find which tiles are visible.
	m := matMul(tp.matrix, rd.base)
	inv, ok := invertMatrix(m)
	if !ok {
		return
	}
	pu0, pv0 := math.Inf(1), math.Inf(1)
	pu1, pv1 := math.Inf(-1), math.Inf(-1)
	for _, c := range [4][2]float64{{bx0, by0}, {bx1, by0}, {bx1, by1}, {bx0, by1}} {
		u, v := applyPt(inv, c[0], c[1])
		pu0, pu1 = math.Min(pu0, u), math.Max(pu1, u)
		pv0, pv1 = math.Min(pv0, v), math.Max(pv1, v)
	}
	i0 := int(math.Floor(pu0/tp.xstep)) - 1
	i1 := int(math.Ceil(pu1/tp.xstep)) + 1
	j0 := int(math.Floor(pv0/tp.ystep)) - 1
	j1 := int(math.Ceil(pv1/tp.ystep)) + 1
	if (i1-i0+1)*(j1-j0+1) > maxTiles {
		return // pattern too fine for the area — skip rather than stall
	}

	ops, err := parseContentStream(tp.content)
	if err != nil {
		return
	}
	savedGS, savedRes, savedStack, savedFl := rd.gs, rd.res, len(rd.stack), rd.fl
	rd.depth++
	for j := j0; j <= j1; j++ {
		for i := i0; i <= i1; i++ {
			rd.gs = savedGS // each tile starts from the pattern-selection state
			rd.gs.fillPattern, rd.gs.fillShading, rd.gs.fillTiling = false, nil, nil
			rd.gs.clip = clip
			rd.gs.ctm = matMul(translateMatrix(float64(i)*tp.xstep, float64(j)*tp.ystep), tp.matrix)
			if tp.resources != nil {
				rd.res = tp.resources
			}
			rd.fl = newFlattener(0.2)
			rd.exec(ops)
		}
	}
	rd.depth--
	rd.gs, rd.res, rd.fl = savedGS, savedRes, savedFl
	if len(rd.stack) > savedStack {
		rd.stack = rd.stack[:savedStack]
	}
}

// pathDeviceBounds returns the device-space bounding box of a flattened path.
func pathDeviceBounds(dp *devPath) (x0, y0, x1, y1 float64, ok bool) {
	x0, y0 = math.Inf(1), math.Inf(1)
	x1, y1 = math.Inf(-1), math.Inf(-1)
	for _, sp := range dp.subs {
		for _, p := range sp.pts {
			x0, y0 = math.Min(x0, p.x), math.Min(y0, p.y)
			x1, y1 = math.Max(x1, p.x), math.Max(y1, p.y)
		}
	}
	return x0, y0, x1, y1, x1 >= x0 && y1 >= y0
}
