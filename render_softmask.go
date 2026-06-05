// SPDX-License-Identifier: MIT

package asposepdf

import "image"

// A soft mask (gs /SMask, ISO 32000-1 §11.6.5.2) modulates the alpha of every
// subsequent paint by a per-pixel value derived from a transparency group: the
// group's luminosity (/S /Luminosity) or its coverage alpha (/S /Alpha). We
// render the group /G into an off-screen buffer once, reduce it to a w×h mask,
// and store it on the graphics state where effectiveClip folds it into painting.
// q/Q restore it; /SMask /None clears it.

// applySoftMask sets or clears the graphics-state soft mask from a gs /SMask
// value (the name /None, or a soft-mask dictionary).
func (rd *renderer) applySoftMask(sm pdfValue) {
	if n, ok := sm.(pdfName); ok && n == "/None" {
		rd.gs.softMask = nil
		return
	}
	smd, ok := sm.(pdfDict)
	if !ok {
		return
	}
	objects := rd.page.doc.objects
	group, ok := resolveRef(objects, smd["/G"]).(*pdfStream)
	if !ok {
		return
	}
	alpha := dictGetName(smd, "/S") == "/Alpha"
	rd.gs.softMask = rd.renderSoftMaskGroup(group, alpha)
}

// renderSoftMaskGroup renders the transparency group off-screen (in the current
// user space) and reduces it to a per-pixel mask: coverage alpha for an alpha
// mask, luminosity over a black backdrop for a luminosity mask.
func (rd *renderer) renderSoftMaskGroup(group *pdfStream, alpha bool) []float32 {
	if rd.depth >= 8 || rd.w <= 0 || rd.h <= 0 {
		return nil
	}
	mimg := image.NewRGBA(image.Rect(0, 0, rd.w, rd.h))
	if !alpha {
		// Luminosity: the backdrop defaults to black and opaque; areas the group
		// leaves untouched stay black → luminosity 0 → fully masked.
		for i := 3; i < len(mimg.Pix); i += 4 {
			mimg.Pix[i] = 255
		}
	}

	sub := newRenderer(rd.page, mimg, rd.w, rd.h, rd.base)
	sub.gs.ctm = rd.gs.ctm // the mask is defined in the current user space
	sub.depth = rd.depth + 1
	sub.drawFormXObject(group)

	mask := make([]float32, rd.w*rd.h)
	for i := range mask {
		o := i * 4
		if alpha {
			mask[i] = float32(mimg.Pix[o+3]) / 255
		} else {
			mask[i] = float32(lum(
				float64(mimg.Pix[o+0])/255,
				float64(mimg.Pix[o+1])/255,
				float64(mimg.Pix[o+2])/255))
		}
	}
	return mask
}
