// SPDX-License-Identifier: MIT

package asposepdf

// A sticky note or a file attachment normally has no appearance of its own: the
// viewer draws its icon. PDF/A-2 and later require every visible annotation to have
// one (ISO 19005-2 §6.3.3), so conversion draws a plain icon in the annotation's
// colour. Annotations built with this library stay without one until then, as a
// viewer's own icon looks better wherever it is allowed.

// generateIconAppearance draws a sticky-note annotation as a page with lines of
// text on it, and a file attachment as a page with a folded corner.
func generateIconAppearance(subtype string, fill Color, width, height float64) *pdfStream {
	b := newAppearanceBuilder()
	if width > 0 && height > 0 {
		x0, y0, x1, y1 := width*0.1, height*0.1, width*0.9, height*0.9
		fold := width * 0.25
		b.PushState()
		b.SetFillColorRGB(fill)
		b.SetStrokeColorRGB(Color{A: 1})
		b.SetLineWidth(0.5)
		b.MoveTo(x0, y0)
		b.LineTo(x1, y0)
		if subtype == "/FileAttachment" {
			b.LineTo(x1, y1-fold)
			b.LineTo(x1-fold, y1)
		} else {
			b.LineTo(x1, y1)
		}
		b.LineTo(x0, y1)
		b.ClosePath()
		b.FillStroke()
		if subtype == "/FileAttachment" {
			b.MoveTo(x1, y1-fold)
			b.LineTo(x1-fold, y1-fold)
			b.LineTo(x1-fold, y1)
			b.Stroke()
		} else {
			for _, f := range []float64{0.7, 0.5, 0.3} {
				b.MoveTo(width*0.25, height*f)
				b.LineTo(width*0.75, height*f)
				b.Stroke()
			}
		}
		b.PopState()
	}
	return makeFormXObject(b.Bytes(), Rectangle{URX: width, URY: height})
}

// generateIconAppearances gives each visible sticky note and file attachment with
// no appearance one of its own.
func (d *Document) generateIconAppearances() {
	for _, page := range d.Pages() {
		dict, ok := page.pageObj().Value.(pdfDict)
		if !ok {
			continue
		}
		annots, _ := resolveRefToArray(d.objects, dict["/Annots"])
		for _, v := range annots {
			ad, ok := resolveRefToDict(d.objects, v)
			if !ok {
				continue
			}
			subtype := dictGetName(ad, "/Subtype")
			if subtype != "/Text" && subtype != "/FileAttachment" {
				continue
			}
			if _, has := resolveRefToDict(d.objects, ad["/AP"]); has {
				continue
			}
			rect, ok := pdfaAnnotRect(d.objects, ad)
			if !ok || (rect.URX <= rect.LLX && rect.URY <= rect.LLY) {
				continue
			}
			fill := Color{R: 1, G: 0.92, B: 0.4, A: 1}
			if c, ok := resolveRefToArray(d.objects, ad["/C"]); ok && len(c) == 3 {
				fill = Color{R: operandFloat(resolveRef(d.objects, c[0])), G: operandFloat(resolveRef(d.objects, c[1])), B: operandFloat(resolveRef(d.objects, c[2])), A: 1}
			}
			form := generateIconAppearance(subtype, fill, rect.URX-rect.LLX, rect.URY-rect.LLY)
			ad["/AP"] = pdfDict{"/N": pdfRef{Num: d.addObject(form)}}
		}
	}
}

// pdfaAnnotRect reads an annotation's /Rect, normalized.
func pdfaAnnotRect(objects map[int]*pdfObject, ad pdfDict) (Rectangle, bool) {
	arr, ok := resolveRefToArray(objects, ad["/Rect"])
	if !ok || len(arr) != 4 {
		return Rectangle{}, false
	}
	x0, y0 := operandFloat(resolveRef(objects, arr[0])), operandFloat(resolveRef(objects, arr[1]))
	x1, y1 := operandFloat(resolveRef(objects, arr[2])), operandFloat(resolveRef(objects, arr[3]))
	return Rectangle{LLX: min(x0, x1), LLY: min(y0, y1), URX: max(x0, x1), URY: max(y0, y1)}, true
}
