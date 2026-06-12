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
		if filter == "/CCITTFaxDecode" || filter == "/CCF" {
			// CCITT needs its /DecodeParms (K, Columns, BlackIs1, …) —
			// applyFilter has no params, so route through ccittFilter the way
			// decodeStream does. Scanned-fax Type3 glyph masks use this.
			params, _ := dict["/DecodeParms"].(pdfDict)
			if d, err := ccittFilter(data, params, dict); err == nil {
				data = d
			}
		} else if d, err := applyFilter(filter, data); err == nil {
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

	sampleBit := func(col, row int) (on, ok bool) {
		idx := row*rowBytes + col/8
		if idx >= len(data) {
			return false, false
		}
		bit := (data[idx] >> uint(7-col%8)) & 1
		return (bit == 1) == paintWhenOne, true
	}

	// Source-pixel footprint of one device pixel (per-axis extents of the
	// inverse mapping). When it exceeds one source pixel the mask is being
	// minified: nearest sampling would drop thin strokes (a 1-px glyph stroke
	// vanishes at 4× downscale — scanned-fax Type3 fonts, 43336.pdf), so
	// box-filter the footprint into a coverage alpha instead.
	eu := (math.Abs(inv[0]) + math.Abs(inv[2])) * float64(w)
	ev := (math.Abs(inv[1]) + math.Abs(inv[3])) * float64(h)
	minify := eu > 1 || ev > 1

	for py := y0; py < y1; py++ {
		for px := x0; px < x1; px++ {
			u, v := applyPt(inv, float64(px)+0.5, float64(py)+0.5)
			if u < 0 || u >= 1 || v < 0 || v >= 1 {
				continue
			}
			coverage := 1.0
			if minify {
				sx, sy := u*float64(w), (1-v)*float64(h)
				c0 := clampInt(int(sx-eu/2), 0, w-1)
				c1 := clampInt(int(sx+eu/2), 0, w-1)
				r0 := clampInt(int(sy-ev/2), 0, h-1)
				r1 := clampInt(int(sy+ev/2), 0, h-1)
				cStep := (c1-c0)/16 + 1 // cap the grid at ~16×16 samples
				rStep := (r1-r0)/16 + 1
				on, total := 0, 0
				for r := r0; r <= r1; r += rStep {
					for c := c0; c <= c1; c += cStep {
						if hit, ok := sampleBit(c, r); ok {
							total++
							if hit {
								on++
							}
						}
					}
				}
				if on == 0 {
					continue
				}
				coverage = float64(on) / float64(total)
			} else {
				col := clampInt(int(u*float64(w)), 0, w-1)
				row := clampInt(int((1-v)*float64(h)), 0, h-1)
				hit, ok := sampleBit(col, row)
				if !ok || !hit {
					continue
				}
			}
			a := rd.gs.fillA * coverage
			if clip != nil {
				a *= float64(clip[py*rd.w+px])
			}
			compositePixel(rd.img, (py*rd.w+px)*4, rd.gs.fillR, rd.gs.fillG, rd.gs.fillB, a, rd.gs.blend)
		}
	}
}
