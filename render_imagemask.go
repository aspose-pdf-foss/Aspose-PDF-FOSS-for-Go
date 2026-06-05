// SPDX-License-Identifier: MIT

package asposepdf

import "math"

// A stencil image mask (/ImageMask true, ISO 32000-1 §8.9.6.2) is a 1-bit image
// that paints the current fill colour through its "on" samples instead of
// supplying its own colour — used for one-colour stamps, logos and glyph-like
// marks. The renderer blits it like any image (inverse-mapping the unit square
// through the CTM) but writes the fill colour where the sample marks paint.

// maskPaintWhenOne reports whether a sample value of 1 marks a painted point.
// The default /Decode [0 1] paints sample 0; /Decode [1 0] reverses it.
func maskPaintWhenOne(objects map[int]*pdfObject, dict pdfDict) bool {
	if dec := shFloats(objects, dict["/Decode"]); len(dec) >= 2 {
		return dec[0] == 1
	}
	return false
}

// drawInlineMask paints an inline image mask (BI … /IM true … EI). The data may
// be filtered; apply its filter before reading the 1-bit samples.
func (rd *renderer) drawInlineMask(dict pdfDict, dataVal pdfValue) {
	raw, ok := dataVal.(string)
	if !ok {
		return
	}
	data := []byte(raw)
	if filter := primaryFilter(dict); filter != "" {
		if d, err := applyFilter(filter, data); err == nil {
			data = d
		}
	}
	objects := rd.page.doc.objects
	w := int(operandFloat(resolveRef(objects, dict["/Width"])))
	h := int(operandFloat(resolveRef(objects, dict["/Height"])))
	rd.drawImageMask(data, w, h, maskPaintWhenOne(objects, dict))
}

// drawImageMask blits a 1-bit mask of w×h packed samples (rows byte-aligned)
// into the unit square transformed by the current matrix, painting the fill
// colour where a sample marks paint (per paintWhenOne), modulated by the current
// alpha, clip and blend mode.
func (rd *renderer) drawImageMask(data []byte, w, h int, paintWhenOne bool) {
	if rd.ocHidden > 0 || w <= 0 || h <= 0 || len(data) == 0 {
		return
	}
	mt := rd.dmat()
	inv, ok := invertMatrix(mt)
	if !ok {
		return
	}
	minx, miny := math.Inf(1), math.Inf(1)
	maxx, maxy := math.Inf(-1), math.Inf(-1)
	for _, c := range [4][2]float64{{0, 0}, {1, 0}, {1, 1}, {0, 1}} {
		x, y := applyPt(mt, c[0], c[1])
		minx, maxx = math.Min(minx, x), math.Max(maxx, x)
		miny, maxy = math.Min(miny, y), math.Max(maxy, y)
	}
	x0, y0 := clampInt(int(math.Floor(minx)), 0, rd.w), clampInt(int(math.Floor(miny)), 0, rd.h)
	x1, y1 := clampInt(int(math.Ceil(maxx)), 0, rd.w), clampInt(int(math.Ceil(maxy)), 0, rd.h)
	rowBytes := (w + 7) / 8
	clip := rd.effectiveClip()

	for py := y0; py < y1; py++ {
		for px := x0; px < x1; px++ {
			u, v := applyPt(inv, float64(px)+0.5, float64(py)+0.5)
			if u < 0 || u >= 1 || v < 0 || v >= 1 {
				continue
			}
			col := clampInt(int(u*float64(w)), 0, w-1)
			row := clampInt(int((1-v)*float64(h)), 0, h-1)
			idx := row*rowBytes + col/8
			if idx >= len(data) {
				continue
			}
			bit := (data[idx] >> uint(7-col%8)) & 1
			if (bit == 1) != paintWhenOne {
				continue // this sample is masked out
			}
			a := rd.gs.fillA
			if clip != nil {
				a *= float64(clip[py*rd.w+px])
			}
			compositePixel(rd.img, (py*rd.w+px)*4, rd.gs.fillR, rd.gs.fillG, rd.gs.fillB, a, rd.gs.blend)
		}
	}
}
