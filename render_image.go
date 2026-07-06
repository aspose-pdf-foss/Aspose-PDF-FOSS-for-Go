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
	if oc, ok := stream.Dict["/OC"]; ok && !rd.ocVisible(oc) {
		return // XObject hidden by Optional Content
	}
	switch dictGetName(stream.Dict, "/Subtype") {
	case "/Image":
		rd.drawImageXObject(name, stream)
	case "/Form":
		if rd.groupNeedsComposite(stream) {
			rd.drawFormGroup(stream)
		} else {
			rd.drawFormXObject(stream)
		}
	}
}

// drawImageXObject decodes the named image and blits it into the unit square
// transformed by the current matrix. Stencil image masks are skipped for now.
func (rd *renderer) drawImageXObject(name string, stream *pdfStream) {
	if b, _ := stream.Dict["/ImageMask"].(bool); b {
		objects := rd.page.doc.objects
		w := int(operandFloat(resolveRef(objects, stream.Dict["/Width"])))
		h := int(operandFloat(resolveRef(objects, stream.Dict["/Height"])))
		data := decodedStreamData(stream)
		// JBIG2 can't be decoded by decodeStream (its globals live in
		// /DecodeParms), so decodedStreamData returns raw compressed bytes here.
		// Decode it so the mask samples are the real 1-bpp bitmap (jbig2Decode
		// packs foreground as sample 0, which a default /Decode paints).
		if f := primaryFilter(stream.Dict); f == "/JBIG2Decode" || f == "/JBIG2" {
			if dec, err := jbig2Decode(stream.Data, jbig2GlobalsData(objects, stream.Dict), w, h); err == nil {
				data = dec
			}
		}
		rd.drawImageMask(data, w, h, maskPaintWhenOne(objects, stream.Dict))
		return
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
			rd.drawInlineMask(dict, operands[1])
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
	savedTS, savedTSStack := rd.ts, len(rd.tsStack)
	savedMC, savedHidden := len(rd.mcStack), rd.ocHidden

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

	// Restore (and drop any q's / unbalanced marked content the form left).
	rd.gs = savedGS
	rd.res = savedRes
	rd.ts = savedTS
	if len(rd.stack) > savedStack {
		rd.stack = rd.stack[:savedStack]
	}
	if len(rd.tsStack) > savedTSStack {
		rd.tsStack = rd.tsStack[:savedTSStack]
	}
	rd.mcStack = rd.mcStack[:savedMC]
	rd.ocHidden = savedHidden
	rd.resetPath()
}

// blitImage paints m into the unit square (0,0)-(1,1) transformed by the
// current matrix, inverse-mapping each device pixel to a source sample
// (nearest neighbour) and compositing with the source alpha.
func (rd *renderer) blitImage(m image.Image) {
	if rd.ocHidden > 0 {
		return
	}
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

	// Source-pixel footprint of one device pixel along each axis. When it
	// exceeds one source pixel the image is minified, and nearest sampling
	// would alias (and drop the gray edge a masked /Matte image expects); box-
	// average the footprint instead, matching Acrobat/MuPDF. Straight RGBA
	// averaging reproduces the /Matte border naturally — the transparent frame
	// stores the matte colour, so averaging it darkens the edge.
	eu := (math.Abs(inv[0]) + math.Abs(inv[2])) * float64(iw)
	ev := (math.Abs(inv[1]) + math.Abs(inv[3])) * float64(ih)
	minify := eu > 1.05 || ev > 1.05

	for py := y0; py < y1; py++ {
		for px := x0; px < x1; px++ {
			u, v := applyPt(inv, float64(px)+0.5, float64(py)+0.5)
			if u < 0 || u >= 1 || v < 0 || v >= 1 {
				continue
			}
			var sr, sg, sb uint8
			var a float64
			if minify {
				sr, sg, sb, a = sampleImageBox(src, b, u*float64(iw), (1-v)*float64(ih), eu, ev)
			} else {
				col := clampInt(int(u*float64(iw)), 0, iw-1)
				row := clampInt(int((1-v)*float64(ih)), 0, ih-1)
				off := src.PixOffset(b.Min.X+col, b.Min.Y+row)
				sr, sg, sb, a = src.Pix[off], src.Pix[off+1], src.Pix[off+2], float64(src.Pix[off+3])/255
			}
			if a == 0 {
				continue
			}
			a *= rd.gs.fillA // constant alpha (/ca) from the ExtGState
			if clip != nil {
				a *= float64(clip[py*rd.w+px])
			}
			compositePixel(rd.img, (py*rd.w+px)*4, sr, sg, sb, a, rd.gs.blend)
		}
	}
}

// sampleImageBox straight-averages the RGBA of the source pixels in the
// footprint centred at (sx, sy) with extents (eu, ev) source pixels. The grid
// is capped so a heavy downscale stays bounded. Returns the averaged colour
// and alpha (0..1).
func sampleImageBox(src *image.NRGBA, b image.Rectangle, sx, sy, eu, ev float64) (uint8, uint8, uint8, float64) {
	iw, ih := b.Dx(), b.Dy()
	c0 := clampInt(int(sx-eu/2), 0, iw-1)
	c1 := clampInt(int(sx+eu/2), 0, iw-1)
	r0 := clampInt(int(sy-ev/2), 0, ih-1)
	r1 := clampInt(int(sy+ev/2), 0, ih-1)
	cStep := (c1-c0)/16 + 1
	rStep := (r1-r0)/16 + 1
	var sumR, sumG, sumB, sumA, n float64
	for r := r0; r <= r1; r += rStep {
		for c := c0; c <= c1; c += cStep {
			off := src.PixOffset(b.Min.X+c, b.Min.Y+r)
			sumR += float64(src.Pix[off])
			sumG += float64(src.Pix[off+1])
			sumB += float64(src.Pix[off+2])
			sumA += float64(src.Pix[off+3])
			n++
		}
	}
	if n == 0 {
		return 0, 0, 0, 0
	}
	return uint8(sumR/n + 0.5), uint8(sumG/n + 0.5), uint8(sumB/n + 0.5), sumA / n / 255
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
