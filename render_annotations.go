// SPDX-License-Identifier: MIT

package asposepdf

import "math"

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
	// Two passes: non-widget annotations first, then form-field widgets on top.
	// Acrobat (and pdf.js) paint interactive form fields as a separate top
	// layer rather than in raw /Annots order, so a markup annotation drawn over
	// a field does not hide its value. 39103.pdf relies on this: opaque white
	// /Square "white-out" boxes appear after the widgets in /Annots and would
	// otherwise erase every field's text. The /Annots order is preserved within
	// each pass.
	for pass := 0; pass < 2; pass++ {
		for _, a := range annots {
			ad, ok := resolveRefToDict(objects, a)
			if !ok {
				continue
			}
			isWidget := dictGetName(ad, "/Subtype") == "/Widget"
			if (pass == 0) == isWidget {
				continue // pass 0 = non-widgets, pass 1 = widgets
			}
			// Interactive-forms HTML export: fields exported as real HTML
			// controls must not also appear in the background raster/SVG.
			if isWidget && rd.hideFormWidgets && convertibleWidget(objects, ad) {
				continue
			}
			rd.drawAnnotation(objects, ad)
		}
	}
}

// drawAnnotation paints one annotation's appearance (its /AP/N, or a
// synthesized one for markup shapes that carry none), mapped into its /Rect.
// Popup and hidden / no-view annotations are skipped.
func (rd *renderer) drawAnnotation(objects map[int]*pdfObject, ad pdfDict) {
	if dictGetName(ad, "/Subtype") == "/Popup" || annotHidden(objects, ad) {
		return
	}
	ap := appearanceStream(objects, ad)
	if ap == nil {
		// No /AP: a viewer synthesizes the appearance from the annotation's
		// properties (ISO 32000-1 §12.5.5). Draw the markup shapes the library
		// can build (Square/Circle/Line/Ink); others are skipped as before.
		ap = rd.synthesizeAnnotationAppearance(ad)
		if ap == nil {
			return
		}
	}
	rect, ok := normRect(shFloats(objects, ad["/Rect"]))
	if !ok {
		return
	}
	m, ok := annotAppearanceMatrix(objects, ap, rect)
	if !ok {
		return
	}
	rd.widgetFieldBackground(objects, ad, rect)
	rd.drawAnnotationAppearance(ap, m)
}

// synthesizeAnnotationAppearance builds an appearance stream for a drawing
// annotation that carries no /AP, the way a viewer does (ISO 32000-1 §12.5.5:
// absent an appearance, one is generated from the annotation's properties).
// Covers the markup shapes the library can render from /Rect, /C, /IC, /BS and
// geometry (Square/Circle/Line/Ink); returns nil for other subtypes, which are
// skipped. The stream is drawn directly and not stored on the document.
func (rd *renderer) synthesizeAnnotationAppearance(ad pdfDict) *pdfStream {
	db := drawingAnnotationBase{annotationBase: annotationBase{
		dict: ad, doc: rd.page.doc, page: rd.page,
	}}
	switch dictGetName(ad, "/Subtype") {
	case "/Square":
		return generateSquareAppearance(&SquareAnnotation{drawingAnnotationBase: db})
	case "/Circle":
		return generateCircleAppearance(&CircleAnnotation{drawingAnnotationBase: db})
	case "/Line":
		return generateLineAppearance(&LineAnnotation{drawingAnnotationBase: db})
	case "/Ink":
		return generateInkAppearance(&InkAnnotation{drawingAnnotationBase: db})
	}
	return nil
}

// widgetFieldBackground paints an opaque background behind a text or choice
// field widget before its appearance is drawn. Acrobat and MuPDF render
// interactive form fields as opaque boxes, so underlying page content does not
// show through; without this a document that bakes real text into the content
// stream and layers placeholder fields on top (39103.pdf) renders both layers
// overlapping. The colour is /MK/BG when present, else white. Button,
// checkbox and radio widgets keep their transparent default (their own
// appearances supply the visible chrome).
func (rd *renderer) widgetFieldBackground(objects map[int]*pdfObject, ad pdfDict, rect [4]float64) {
	if dictGetName(ad, "/Subtype") != "/Widget" {
		return
	}
	switch inheritedFieldType(objects, ad) {
	case "/Tx", "/Ch":
	default:
		return
	}
	r, g, b := uint8(255), uint8(255), uint8(255)
	if mk, ok := resolveRefToDict(objects, ad["/MK"]); ok {
		if bg := shFloats(objects, mk["/BG"]); len(bg) > 0 {
			r, g, b = bgColorComponents(bg)
		}
	}
	rd.fillUserRect(rect, r, g, b)
}

// inheritedFieldType returns the field type (/FT) of a widget, walking the
// /Parent chain since a widget often inherits /FT from its field dictionary.
func inheritedFieldType(objects map[int]*pdfObject, ad pdfDict) string {
	for i := 0; i < 32 && ad != nil; i++ {
		if ft := dictGetName(ad, "/FT"); ft != "" {
			return ft
		}
		parent, ok := resolveRefToDict(objects, ad["/Parent"])
		if !ok {
			return ""
		}
		ad = parent
	}
	return ""
}

// bgColorComponents converts a /MK/BG array (1 gray, 3 RGB, 4 CMYK) to RGB.
func bgColorComponents(bg []float64) (uint8, uint8, uint8) {
	switch len(bg) {
	case 1:
		return gray8(bg[0])
	case 4:
		return clamp8(1 - math.Min(1, bg[0]+bg[3])), clamp8(1 - math.Min(1, bg[1]+bg[3])), clamp8(1 - math.Min(1, bg[2]+bg[3]))
	default:
		return clamp8(bg[0]), clamp8(bg[1]), clamp8(bg[2])
	}
}

// fillUserRect fills a page-user-space rectangle with an opaque colour, mapped
// to device space through the page base matrix, ignoring any leftover clip or
// blend state from content rendering.
func (rd *renderer) fillUserRect(rect [4]float64, r, g, b uint8) {
	m := rd.base
	fl := newFlattener(0.2)
	ax, ay := applyPt(m, rect[0], rect[1])
	bx, by := applyPt(m, rect[2], rect[1])
	cx, cy := applyPt(m, rect[2], rect[3])
	dx, dy := applyPt(m, rect[0], rect[3])
	fl.moveTo(ax, ay)
	fl.lineTo(bx, by)
	fl.lineTo(cx, cy)
	fl.lineTo(dx, dy)
	fl.close()
	savedGS := rd.gs
	rd.gs = gstate{fillA: 1, strokeA: 1, lineWidth: 1}
	rd.compositePath(fl.path(), fillNonZero, r, g, b, 1)
	rd.gs = savedGS
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
