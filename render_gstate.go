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
}
