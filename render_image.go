// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"image"
	"image/draw"
	"math"
)

// doXObject handles the Do operator: an Image XObject is sampled onto the page,
// a Form XObject is recursively interpreted. Anything else is ignored.
func (rd *renderer) doXObject(name string) {
	if name == "" || rd.res == nil {
		return
	}
	xobjDict, ok := resolveRefToDict(rd.page.doc.objects, rd.res["/XObject"])
	if !ok {
		return
	}
	stream, ok := resolveRef(rd.page.doc.objects, xobjDict[name]).(*pdfStream)
	if !ok {
		return
	}
	switch dictGetName(stream.Dict, "/Subtype") {
	case "/Image":
		rd.drawImageXObject(name, stream)
	case "/Form":
		rd.drawFormXObject(stream)
	}
}

// drawImageXObject decodes the named image and blits it into the unit square
// transformed by the current matrix. Stencil image masks are skipped for now.
func (rd *renderer) drawImageXObject(name string, stream *pdfStream) {
	if b, _ := stream.Dict["/ImageMask"].(bool); b {
		return // stencil masks (painted with fill colour) — later phase
	}
	info, ok := xobjectImageInfo(rd.page.doc.objects, rd.res, name, identityMatrix())
	if !ok {
		return
	}
	img, err := info.Extract()
	if err != nil {
		return
	}
	m, _, err := image.Decode(bytes.NewReader(img.Data))
	if err != nil {
		return
	}
	rd.blitImage(m)
}

// drawInlineImage decodes an inline image (BI…ID…EI) and blits it into the
// unit square transformed by the current matrix, reusing the same decode
// pipeline as extraction. operands are [normalized dict, raw data] from the BI
// content op. Stencil image masks are skipped (painted with fill colour — later).
func (rd *renderer) drawInlineImage(operands []pdfValue) {
	if len(operands) < 2 {
		return
	}
	if dict, ok := operands[0].(pdfDict); ok {
		if b, _ := dict["/ImageMask"].(bool); b {
			return
		}
	}
	info, ok := inlineImageInfo(operands[0], operands[1], identityMatrix())
	if !ok {
		return
	}
	img, err := info.Extract()
	if err != nil {
		return
	}
	m, _, err := image.Decode(bytes.NewReader(img.Data))
	if err != nil {
		return
	}
	rd.blitImage(m)
}

// drawFormXObject interprets a form XObject's content with its own resources
// and /Matrix, sharing the page's image buffer and graphics state.
func (rd *renderer) drawFormXObject(stream *pdfStream) {
	if rd.depth >= 12 {
		return
	}
	ops, err := parseContentStream(decodedStreamData(stream))
	if err != nil {
		return
	}
	savedGS := rd.gs
	savedRes := rd.res
	savedStack := len(rd.stack)

	if matVal, ok := stream.Dict["/Matrix"].(pdfArray); ok && len(matVal) == 6 {
		var fm [6]float64
		for i := 0; i < 6; i++ {
			fm[i] = operandFloat(matVal[i])
		}
		rd.gs.ctm = matMul(fm, rd.gs.ctm)
	}
	if r, ok := resolveRefToDict(rd.page.doc.objects, stream.Dict["/Resources"]); ok {
		rd.res = r
	}

	rd.depth++
	rd.exec(ops)
	rd.depth--

	// Restore (and drop any q's the form left unbalanced).
	rd.gs = savedGS
	rd.res = savedRes
	if len(rd.stack) > savedStack {
		rd.stack = rd.stack[:savedStack]
	}
	rd.resetPath()
}

// blitImage paints m into the unit square (0,0)-(1,1) transformed by the
// current matrix, inverse-mapping each device pixel to a source sample
// (nearest neighbour) and compositing with the source alpha.
func (rd *renderer) blitImage(m image.Image) {
	src := toNRGBA(m)
	b := src.Bounds()
	iw, ih := b.Dx(), b.Dy()
	if iw == 0 || ih == 0 {
		return
	}

	mt := rd.dmat()
	inv, ok := invertMatrix(mt)
	if !ok {
		return
	}

	// Device bounding box of the transformed unit square.
	minx, miny := math.Inf(1), math.Inf(1)
	maxx, maxy := math.Inf(-1), math.Inf(-1)
	for _, c := range [4][2]float64{{0, 0}, {1, 0}, {1, 1}, {0, 1}} {
		x, y := applyPt(mt, c[0], c[1])
		minx, maxx = math.Min(minx, x), math.Max(maxx, x)
		miny, maxy = math.Min(miny, y), math.Max(maxy, y)
	}
	x0, y0 := clampInt(int(math.Floor(minx)), 0, rd.w), clampInt(int(math.Floor(miny)), 0, rd.h)
	x1, y1 := clampInt(int(math.Ceil(maxx)), 0, rd.w), clampInt(int(math.Ceil(maxy)), 0, rd.h)
	clip := rd.effectiveClip()

	for py := y0; py < y1; py++ {
		for px := x0; px < x1; px++ {
			u, v := applyPt(inv, float64(px)+0.5, float64(py)+0.5)
			if u < 0 || u >= 1 || v < 0 || v >= 1 {
				continue
			}
			col := clampInt(int(u*float64(iw)), 0, iw-1)
			row := clampInt(int((1-v)*float64(ih)), 0, ih-1)
			off := src.PixOffset(b.Min.X+col, b.Min.Y+row)
			a := float64(src.Pix[off+3]) / 255
			if clip != nil {
				a *= float64(clip[py*rd.w+px])
			}
			compositePixel(rd.img, (py*rd.w+px)*4, src.Pix[off], src.Pix[off+1], src.Pix[off+2], a, rd.gs.blend)
		}
	}
}

// compositePixel composites (sr,sg,sb) with alpha a at the given
// premultiplied-RGBA byte offset, applying the blend mode (zero value → plain
// source-over).
func compositePixel(dst *image.RGBA, off int, sr, sg, sb uint8, a float64, bm blendMode) {
	if a <= 0 {
		return
	}
	if a > 1 {
		a = 1
	}
	blendApply(dst, off, sr, sg, sb, a, bm)
}

func toNRGBA(m image.Image) *image.NRGBA {
	if n, ok := m.(*image.NRGBA); ok {
		return n
	}
	b := m.Bounds()
	n := image.NewNRGBA(b)
	draw.Draw(n, b, m, b.Min, draw.Src)
	return n
}

// invertMatrix inverts a PDF [a b c d e f] affine matrix.
func invertMatrix(m [6]float64) ([6]float64, bool) {
	det := m[0]*m[3] - m[1]*m[2]
	if math.Abs(det) < 1e-12 {
		return [6]float64{}, false
	}
	id := 1 / det
	a := m[3] * id
	b := -m[1] * id
	c := -m[2] * id
	d := m[0] * id
	e := -(m[4]*a + m[5]*c)
	f := -(m[4]*b + m[5]*d)
	return [6]float64{a, b, c, d, e, f}, true
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
