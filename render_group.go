// SPDX-License-Identifier: MIT

package asposepdf

import "image"

// Transparency-group compositing (ISO 32000-1 §11.4.7, §11.6.6) — the
// remaining piece of the transparency model (epic pdf-go-rom) on top of the
// existing constant alpha, blend modes and soft masks.
//
// A Form XObject whose /Group has /S /Transparency is a transparency group:
// its contents composite with each other first, and the *result* is then
// composited onto the backdrop as a single object. Executing such a form
// inline (the default path) is correct when the group is drawn opaquely in
// Normal mode — but when a group-level alpha, blend mode or soft mask is in
// effect at the Do, inline execution applies it per element (the classic
// symptom: overlaps inside a 50%-opacity group double-darken). For those
// cases the group is rendered into an off-screen buffer over a transparent
// backdrop (isolated-group semantics — the standard approximation for
// non-isolated groups too) and the flattened result is composited once with
// the group-level alpha/blend/mask.
//
// A knockout group (/K true) additionally makes every element composite
// against the group's *initial* backdrop instead of the accumulated result:
// later elements replace earlier ones where they overlap. This is implemented
// for the vector paint path (fills, strokes, glyphs — the overprint idiom
// knockout groups exist for); images inside knockout groups still composite
// normally (documented limitation).

// transparencyGroup returns the stream's /Group dictionary when it declares a
// transparency group.
func transparencyGroup(objects map[int]*pdfObject, stream *pdfStream) (pdfDict, bool) {
	g, ok := resolveRefToDict(objects, stream.Dict["/Group"])
	if !ok || dictGetName(g, "/S") != "/Transparency" {
		return nil, false
	}
	return g, true
}

// groupNeedsComposite reports whether drawing this form requires off-screen
// group compositing: a transparency group with a non-trivial compositing state
// at the Do (group alpha, non-Normal blend, soft mask) or knockout semantics.
// An isolated group drawn opaquely in Normal mode renders identically inline,
// so it takes the cheap path.
func (rd *renderer) groupNeedsComposite(stream *pdfStream) bool {
	g, ok := transparencyGroup(rd.page.doc.objects, stream)
	if !ok {
		return false
	}
	if rd.gs.fillA < 1 || rd.gs.blend.sep != nil || rd.gs.blend.ns != nil || rd.gs.softMask != nil {
		return true
	}
	return dictGetBool(g, "/K")
}

// drawFormGroup renders a transparency-group form into an off-screen buffer
// over a transparent backdrop and composites the flattened result onto the
// page with the group-level alpha, blend mode, soft mask and clip.
func (rd *renderer) drawFormGroup(stream *pdfStream) {
	if rd.vec != nil {
		// Transparency-group compositing has no SVG equivalent (per-group
		// alpha/blend/soft-mask over the flattened group) — raster patch.
		if rd.ocHidden == 0 {
			rd.vecPatch(func(sub *renderer) { sub.drawFormGroup(stream) })
		}
		return
	}
	if rd.depth >= 8 || rd.w <= 0 || rd.h <= 0 {
		// Too deep for an off-screen pass: fall back to inline execution.
		rd.drawFormXObject(stream)
		return
	}
	buf := image.NewRGBA(image.Rect(0, 0, rd.w, rd.h))

	sub := newRenderer(rd.page, buf, rd.w, rd.h, rd.base)
	sub.gs.ctm = rd.gs.ctm // the group is defined in the current user space
	sub.depth = rd.depth + 1
	sub.suppressText = rd.suppressText // group content is page content
	if g, ok := transparencyGroup(rd.page.doc.objects, stream); ok {
		sub.knockout = dictGetBool(g, "/K")
	}
	sub.drawFormXObject(stream)

	rd.compositeGroup(buf)
}

// compositeGroup composites an off-screen group buffer (premultiplied RGBA,
// same dimensions as the page) onto the page image, applying the current
// group-level alpha, blend mode, and clip/soft-mask.
func (rd *renderer) compositeGroup(src *image.RGBA) {
	alpha := rd.gs.fillA // /ca applies to XObjects (non-stroking context)
	clip := rd.effectiveClip()
	bm := rd.gs.blend
	dst := rd.img

	for i := 0; i < rd.w*rd.h; i++ {
		o := i * 4
		sa := float64(src.Pix[o+3]) / 255
		if sa <= 0 {
			continue
		}
		m := alpha
		if clip != nil {
			m *= float64(clip[i])
			if m <= 0 {
				continue
			}
		}
		if bm.sep != nil || bm.ns != nil {
			// Un-premultiply the group pixel and route through the blend
			// compositor (it implements C = (1−a)·Cb + a·B(Cb,Cs)).
			ur := uint8(float64(src.Pix[o+0]) / sa)
			ug := uint8(float64(src.Pix[o+1]) / sa)
			ub := uint8(float64(src.Pix[o+2]) / sa)
			blendApply(dst, o, ur, ug, ub, sa*m, bm)
			continue
		}
		// Normal: src-over with premultiplied source scaled by m.
		a := sa * m
		dst.Pix[o+0] = uint8(float64(src.Pix[o+0])*m + float64(dst.Pix[o+0])*(1-a) + 0.5)
		dst.Pix[o+1] = uint8(float64(src.Pix[o+1])*m + float64(dst.Pix[o+1])*(1-a) + 0.5)
		dst.Pix[o+2] = uint8(float64(src.Pix[o+2])*m + float64(dst.Pix[o+2])*(1-a) + 0.5)
		dst.Pix[o+3] = uint8(float64(src.Pix[o+3])*m + float64(dst.Pix[o+3])*(1-a) + 0.5)
	}
}

// compositeCoverageKnockout paints coverage with knockout semantics: within the
// covered area the source replaces the accumulated backdrop (composites against
// the group's initial — transparent — backdrop) instead of blending over it:
//
//	C' = Cs·a·cov + Cb·(1 − cov)   (vs src-over's (1 − a·cov))
//
// so a semi-transparent element erases what earlier elements left underneath.
func compositeCoverageKnockout(dst *image.RGBA, w int, cov []float32, bx0, by0, bx1, by1 int, sr, sg, sb uint8, srcA float64, clip []float32) {
	bw := bx1 - bx0
	for y := by0; y < by1; y++ {
		row := (y*w + bx0)
		for x := 0; x < bw; x++ {
			i := row + x
			c := float64(cov[(y-by0)*bw+x])
			if c <= 0 {
				continue
			}
			if clip != nil {
				c *= float64(clip[i])
				if c <= 0 {
					continue
				}
			}
			a := srcA * c
			o := i * 4
			dst.Pix[o+0] = uint8(float64(sr)*a + float64(dst.Pix[o+0])*(1-c) + 0.5)
			dst.Pix[o+1] = uint8(float64(sg)*a + float64(dst.Pix[o+1])*(1-c) + 0.5)
			dst.Pix[o+2] = uint8(float64(sb)*a + float64(dst.Pix[o+2])*(1-c) + 0.5)
			dst.Pix[o+3] = uint8(255*a + float64(dst.Pix[o+3])*(1-c) + 0.5)
		}
	}
}

// dictGetBool reads a boolean entry (missing or non-bool → false).
func dictGetBool(d pdfDict, key string) bool {
	b, _ := d[key].(bool)
	return b
}
