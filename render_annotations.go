// SPDX-License-Identifier: MIT

package asposepdf

// renderAnnotations paints each annotation's normal appearance stream (/AP/N)
// onto the page, the way a viewer does. Form-field widgets, stamps, highlights,
// free text and any other annotation carrying an appearance are drawn here —
// none of them are part of the page content stream. Each appearance is a Form
// XObject mapped into the annotation /Rect per ISO 32000-1 §12.5.5; hidden /
// no-view / popup annotations are skipped.
func (rd *renderer) renderAnnotations() {
	objects := rd.page.doc.objects
	pd := rd.page.pageDict()
	if pd == nil {
		return
	}
	annots, ok := resolveRefToArray(objects, pd["/Annots"])
	if !ok {
		return
	}
	for _, a := range annots {
		ad, ok := resolveRefToDict(objects, a)
		if !ok {
			continue
		}
		if dictGetName(ad, "/Subtype") == "/Popup" || annotHidden(objects, ad) {
			continue
		}
		ap := appearanceStream(objects, ad)
		if ap == nil {
			continue
		}
		rect, ok := normRect(shFloats(objects, ad["/Rect"]))
		if !ok {
			continue
		}
		m, ok := annotAppearanceMatrix(objects, ap, rect)
		if !ok {
			continue
		}
		rd.drawAnnotationAppearance(ap, m)
	}
}

// drawAnnotationAppearance renders one appearance Form XObject with a fresh
// graphics state whose CTM (m) maps the transformed appearance box onto the
// annotation rect; drawFormXObject then concatenates the form's own /Matrix.
func (rd *renderer) drawAnnotationAppearance(ap *pdfStream, m [6]float64) {
	savedGS, savedRes, savedStack := rd.gs, rd.res, len(rd.stack)
	rd.gs = gstate{ctm: m, fillA: 1, strokeA: 1, lineWidth: 1}
	rd.drawFormXObject(ap)
	rd.gs, rd.res = savedGS, savedRes
	if len(rd.stack) > savedStack {
		rd.stack = rd.stack[:savedStack]
	}
	rd.resetPath()
}

// annotHidden reports whether the annotation's /F flags request Hidden (bit 2)
// or NoView (bit 6) per ISO 32000-1 Table 165.
func annotHidden(objects map[int]*pdfObject, ad pdfDict) bool {
	f := int(operandFloat(resolveRef(objects, ad["/F"])))
	return f&0x02 != 0 || f&0x20 != 0
}

// appearanceStream resolves /AP/N to a single appearance stream, selecting by
// /AS when /N is a subdictionary of appearance states (e.g. checkbox on/off).
func appearanceStream(objects map[int]*pdfObject, ad pdfDict) *pdfStream {
	ap, ok := resolveRefToDict(objects, ad["/AP"])
	if !ok {
		return nil
	}
	n := resolveRef(objects, ap["/N"])
	if s, ok := n.(*pdfStream); ok {
		return s
	}
	d, ok := n.(pdfDict)
	if !ok {
		return nil
	}
	// When /AS names the active state, it is authoritative: render only that
	// state's stream. An off checkbox/radio commonly has /AS /Off with no /Off
	// appearance in /N (only the on-states are present) — in that case there is
	// nothing to draw, so return nil. Falling back to an arbitrary state here
	// would paint the "on" look on an unchecked widget.
	if as := dictGetName(ad, "/AS"); as != "" {
		if s, ok := resolveRef(objects, d[as]).(*pdfStream); ok {
			return s
		}
		return nil
	}
	for _, v := range d { // no /AS: fall back to any state with a stream
		if s, ok := resolveRef(objects, v).(*pdfStream); ok {
			return s
		}
	}
	return nil
}

// annotAppearanceMatrix computes the matrix that maps the appearance's /BBox
// (after its /Matrix) onto the annotation /Rect (ISO 32000-1 §12.5.5): transform
// the four BBox corners by /Matrix, take their bounding box, then scale and
// translate that box onto rect. The returned matrix maps transformed-appearance
// space to page user space; the form's /Matrix is applied on top by the caller.
func annotAppearanceMatrix(objects map[int]*pdfObject, ap *pdfStream, rect [4]float64) ([6]float64, bool) {
	bb := shFloats(objects, ap.Dict["/BBox"])
	if len(bb) < 4 {
		return identityMatrix(), false
	}
	mtx := identityMatrix()
	if pm := shFloats(objects, ap.Dict["/Matrix"]); len(pm) == 6 {
		mtx = [6]float64{pm[0], pm[1], pm[2], pm[3], pm[4], pm[5]}
	}
	// Transformed bounding box of the BBox corners.
	minx, miny := 1e308, 1e308
	maxx, maxy := -1e308, -1e308
	for _, c := range [4][2]float64{{bb[0], bb[1]}, {bb[2], bb[1]}, {bb[2], bb[3]}, {bb[0], bb[3]}} {
		x, y := applyPt(mtx, c[0], c[1])
		minx, maxx = min(minx, x), max(maxx, x)
		miny, maxy = min(miny, y), max(maxy, y)
	}
	tw, th := maxx-minx, maxy-miny
	if tw <= 0 || th <= 0 {
		return identityMatrix(), false
	}
	sx := (rect[2] - rect[0]) / tw
	sy := (rect[3] - rect[1]) / th
	return [6]float64{sx, 0, 0, sy, rect[0] - sx*minx, rect[1] - sy*miny}, true
}

// normRect normalizes a /Rect array to [llx, lly, urx, ury] with llx<urx,
// lly<ury, returning false if it is missing or degenerate.
func normRect(r []float64) ([4]float64, bool) {
	if len(r) < 4 {
		return [4]float64{}, false
	}
	x0, x1 := r[0], r[2]
	y0, y1 := r[1], r[3]
	if x1 < x0 {
		x0, x1 = x1, x0
	}
	if y1 < y0 {
		y0, y1 = y1, y0
	}
	if x1-x0 <= 0 || y1-y0 <= 0 {
		return [4]float64{}, false
	}
	return [4]float64{x0, y0, x1, y1}, true
}
