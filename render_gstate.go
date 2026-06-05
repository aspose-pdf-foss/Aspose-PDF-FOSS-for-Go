// SPDX-License-Identifier: MIT

package asposepdf

// applyExtGState handles the gs operator: it looks up the named graphics-state
// parameter dictionary in the current /ExtGState resources and applies the
// subset the renderer honors — constant fill alpha (/ca), constant stroke alpha
// (/CA), and line width (/LW). Soft-mask groups (/SMask as a dictionary) are not
// rasterized; an explicit /SMask /None is a no-op since we track no soft mask.
func (rd *renderer) applyExtGState(name string) {
	objects := rd.page.doc.objects
	egs, ok := resolveRefToDict(objects, rd.res["/ExtGState"])
	if !ok {
		return
	}
	gd, ok := resolveRefToDict(objects, egs[name])
	if !ok {
		return
	}
	if raw, present := gd["/ca"]; present {
		rd.gs.fillA = clamp01(operandFloat(resolveRef(objects, raw)))
	}
	if raw, present := gd["/CA"]; present {
		rd.gs.strokeA = clamp01(operandFloat(resolveRef(objects, raw)))
	}
	if raw, present := gd["/LW"]; present {
		if lw := operandFloat(resolveRef(objects, raw)); lw > 0 {
			rd.gs.lineWidth = lw
		}
	}
	if raw, present := gd["/LC"]; present {
		rd.gs.lineCap = LineCap(int(operandFloat(resolveRef(objects, raw))))
	}
	if raw, present := gd["/LJ"]; present {
		rd.gs.lineJoin = LineJoin(int(operandFloat(resolveRef(objects, raw))))
	}
	if raw, present := gd["/ML"]; present {
		if ml := operandFloat(resolveRef(objects, raw)); ml >= 1 {
			rd.gs.miterLimit = ml
		}
	}
	// /BM blend mode (a name, or an array of names — first supported wins).
	if raw, present := gd["/BM"]; present {
		rd.gs.blend = nil
		switch v := resolveRef(objects, raw).(type) {
		case pdfName:
			rd.gs.blend = blendFor(string(v))
		case pdfArray:
			for _, e := range v {
				if n, ok := resolveRef(objects, e).(pdfName); ok {
					if b := blendFor(string(n)); b != nil {
						rd.gs.blend = b
						break
					}
				}
			}
		}
	}
	// /D is [dashArray phase].
	if arr, ok := resolveRefToArray(objects, gd["/D"]); ok && len(arr) == 2 {
		da, _ := resolveRefToArray(objects, arr[0])
		rd.gs.dash = make([]float64, 0, len(da))
		for _, e := range da {
			rd.gs.dash = append(rd.gs.dash, operandFloat(resolveRef(objects, e)))
		}
		rd.gs.dashPhase = operandFloat(resolveRef(objects, arr[1]))
	}
}
