// SPDX-License-Identifier: MIT

package asposepdf

import "fmt"

// Flatten bakes the document's interactive form fields into static page
// content and removes the AcroForm, so the result renders identically but is
// no longer fillable. Equivalent to (*Document).Form().Flatten().
//
// Mirrors Aspose.PDF for .NET's Document.Flatten (which flattens form fields).
// Annotations are not affected here — flatten those via a page's
// AnnotationCollection.Flatten or an individual Annotation.Flatten.
func (d *Document) Flatten() error {
	return d.Form().Flatten()
}

// Flatten bakes every form field's current appearance into its page's content
// stream and then removes all fields, their widgets, and the /AcroForm dict,
// producing a non-interactive document that renders the same. A field whose
// widget has no normal appearance is removed without drawing.
//
// Mirrors Aspose.PDF for .NET's Form.Flatten. After Flatten the Form is empty;
// previously returned Field handles are dangling and must not be used.
func (f *Form) Flatten() error {
	if f.root == nil {
		return nil // no AcroForm — nothing to flatten
	}

	// Object IDs of every field dict and widget dict to remove afterwards.
	dictToID := buildDictToID(f.doc.objects)
	targetIDs := map[int]bool{}
	for _, node := range f.leaves {
		if id, ok := dictToID[dictIdentity(node.dict)]; ok {
			targetIDs[id] = true
		}
		for _, w := range node.widgets {
			if id, ok := dictToID[dictIdentity(w)]; ok {
				targetIDs[id] = true
			}
		}
	}

	// Bake widget appearances. Walking each page's /Annots gives the owning
	// page directly (no reliance on the widget's /P back-reference).
	for _, pageObj := range f.doc.pages {
		pageDict, ok := pageObj.Value.(pdfDict)
		if !ok {
			continue
		}
		annots, _ := pageDict["/Annots"].(pdfArray)
		for _, a := range annots {
			ref, ok := a.(pdfRef)
			if !ok || !targetIDs[ref.Num] {
				continue
			}
			wObj := f.doc.objects[ref.Num]
			if wObj == nil {
				continue
			}
			wDict, ok := wObj.Value.(pdfDict)
			if !ok {
				continue
			}
			if _, err := f.doc.bakeAppearanceOntoPage(pageObj, wDict); err != nil {
				return err
			}
		}
	}

	// Remove field + widget objects from /AcroForm/Fields and every /Annots,
	// then delete the objects and drop the AcroForm entirely.
	if arr, ok := f.root["/Fields"].(pdfArray); ok {
		f.root["/Fields"] = spliceRefs(arr, targetIDs)
	}
	for _, pageObj := range f.doc.pages {
		pageDict, ok := pageObj.Value.(pdfDict)
		if !ok {
			continue
		}
		if arr, ok := pageDict["/Annots"].(pdfArray); ok {
			pageDict["/Annots"] = spliceRefs(arr, targetIDs)
		}
	}
	for id := range targetIDs {
		delete(f.doc.objects, id)
	}
	delete(f.doc.catalog, "/AcroForm")
	f.root = nil
	f.leaves = nil
	f.cache = nil
	f.fieldsList = nil
	return nil
}

// Flatten bakes this single field's appearance (each of its widgets) into the
// owning page content and removes the field, leaving every other field and the
// /AcroForm dict intact. Mirrors Aspose.PDF for .NET's Field.Flatten.
//
// After Flatten the field handle is dangling and must not be used.
func (b *fieldBase) Flatten() error {
	if b.node == nil || b.node.form == nil {
		return fmt.Errorf("flatten: field is not attached to a document")
	}
	d := b.node.form.doc

	dictToID := buildDictToID(d.objects)
	ids := map[int]bool{}
	if id, ok := dictToID[dictIdentity(b.node.dict)]; ok {
		ids[id] = true
	}
	for _, w := range b.node.widgets {
		if id, ok := dictToID[dictIdentity(w)]; ok {
			ids[id] = true
		}
	}

	// Bake each widget onto its owning page (found by walking /Annots).
	for _, pageObj := range d.pages {
		pageDict, ok := pageObj.Value.(pdfDict)
		if !ok {
			continue
		}
		annots, _ := pageDict["/Annots"].(pdfArray)
		for _, a := range annots {
			ref, ok := a.(pdfRef)
			if !ok || !ids[ref.Num] {
				continue
			}
			wObj := d.objects[ref.Num]
			if wObj == nil {
				continue
			}
			wDict, ok := wObj.Value.(pdfDict)
			if !ok {
				continue
			}
			if _, err := d.bakeAppearanceOntoPage(pageObj, wDict); err != nil {
				return err
			}
		}
	}

	// Remove just this field from /AcroForm/Fields and from page /Annots.
	if root, ok := resolveRefToDict(d.objects, d.catalog["/AcroForm"]); ok {
		if arr, ok := root["/Fields"].(pdfArray); ok {
			root["/Fields"] = spliceRefs(arr, ids)
		}
	}
	for _, pageObj := range d.pages {
		pageDict, ok := pageObj.Value.(pdfDict)
		if !ok {
			continue
		}
		if arr, ok := pageDict["/Annots"].(pdfArray); ok {
			pageDict["/Annots"] = spliceRefs(arr, ids)
		}
	}
	for id := range ids {
		delete(d.objects, id)
	}
	b.node = nil
	return nil
}

// Flatten bakes every annotation on the page into the page content and removes
// it. Widget annotations (form-field widgets) are skipped — flatten those via
// (*Document).Flatten or (*Form).Flatten so the /AcroForm is cleaned up too.
func (c *AnnotationCollection) Flatten() error {
	c.rebuild()
	for _, a := range c.items {
		if a.AnnotationType() == AnnotationTypeWidget {
			continue
		}
		if err := a.Flatten(); err != nil {
			return err
		}
	}
	return nil
}

// Flatten bakes this annotation's normal appearance (/AP/N, honoring the /AS
// state) into its page's content stream at the annotation's /Rect, then
// removes the annotation. The annotation handle is dangling afterwards.
//
// An annotation with no normal appearance (e.g. a sticky-note Text annotation,
// which viewers render as an icon) is simply removed — nothing is drawn.
//
// Mirrors Aspose.PDF for .NET's Annotation.Flatten. For form-field widgets
// prefer (*Form).Flatten, which also clears the /AcroForm entry.
func (b *annotationBase) Flatten() error {
	if b.objID == 0 || b.attachedPage == nil {
		return fmt.Errorf("flatten: annotation is not attached to a page")
	}
	if _, err := b.doc.bakeAppearanceOntoPage(b.attachedPage, b.dict); err != nil {
		return err
	}
	removeAnnotFromPage(b.doc.objects, b.attachedPage, b.objID)
	delete(b.doc.objects, b.objID)
	b.objID = 0
	b.attachedPage = nil
	return nil
}

// bakeAppearanceOntoPage draws annotDict's normal appearance into pageObj's
// content stream at the annotation's /Rect, registering the appearance Form
// XObject in the page resources. It returns (false, nil) when there is no
// usable normal appearance or rectangle (nothing baked) and (true, nil) on a
// successful draw. The annotation itself is not removed.
func (d *Document) bakeAppearanceOntoPage(pageObj *pdfObject, annotDict pdfDict) (bool, error) {
	apRef, ok := normalAppearanceRef(d.objects, annotDict)
	if !ok {
		return false, nil
	}
	apObj, ok := d.objects[apRef.Num]
	if !ok {
		return false, nil
	}
	apStream, ok := apObj.Value.(*pdfStream)
	if !ok {
		return false, nil
	}
	rect, ok := readRectArray(annotDict["/Rect"])
	if !ok {
		return false, nil
	}

	page := d.pageForObject(pageObj)
	if page == nil {
		return false, fmt.Errorf("flatten: annotation page not found")
	}

	matrix := appearanceMatrix(apStream.Dict, rect)
	name := page.registerFormXObject(apRef)
	ops := fmt.Sprintf("\nq\n%s cm\n%s Do\nQ\n", matrix, name)
	if err := page.appendToContentStream([]byte(ops)); err != nil {
		return false, err
	}
	return true, nil
}

// normalAppearanceRef resolves an annotation's normal appearance to the
// indirect reference of a single Form XObject. /AP/N may be a direct stream
// reference or, for button widgets, a subdictionary keyed by appearance state
// — in which case the /AS entry selects the active state.
func normalAppearanceRef(objects map[int]*pdfObject, annotDict pdfDict) (pdfRef, bool) {
	ap, ok := resolveRefToDict(objects, annotDict["/AP"])
	if !ok {
		return pdfRef{}, false
	}
	switch n := ap["/N"].(type) {
	case pdfRef:
		return n, true
	case pdfDict:
		state := dictGetName(annotDict, "/AS")
		if state != "" {
			if ref, ok := n[state].(pdfRef); ok {
				return ref, true
			}
		}
		// Fall back to any state, preferring a non-/Off appearance.
		var off pdfRef
		var haveOff bool
		for k, v := range n {
			ref, ok := v.(pdfRef)
			if !ok {
				continue
			}
			if k == "/Off" {
				off, haveOff = ref, true
				continue
			}
			return ref, true
		}
		if haveOff {
			return off, true
		}
	}
	return pdfRef{}, false
}

// appearanceMatrix returns the "A B C D E F cm" placement matrix that maps a
// Form XObject's transformed bounding box onto rect, per ISO 32000-1 §12.5.5
// (Algorithm: Appearance streams). The form's own /Matrix is applied by the Do
// operator, so this matrix maps the /Matrix-transformed /BBox to /Rect.
func appearanceMatrix(apDict pdfDict, rect Rectangle) string {
	bx0, by0, bx1, by1 := 0.0, 0.0, rect.URX-rect.LLX, rect.URY-rect.LLY
	if bb, ok := apDict["/BBox"].(pdfArray); ok && len(bb) == 4 {
		bx0, _ = toFloat(bb[0])
		by0, _ = toFloat(bb[1])
		bx1, _ = toFloat(bb[2])
		by1, _ = toFloat(bb[3])
	}
	m := [6]float64{1, 0, 0, 1, 0, 0}
	if mm, ok := apDict["/Matrix"].(pdfArray); ok && len(mm) == 6 {
		for i := 0; i < 6; i++ {
			m[i], _ = toFloat(mm[i])
		}
	}

	// Transform the four BBox corners by the form matrix and take their
	// axis-aligned bounding box.
	xs := [4]float64{}
	ys := [4]float64{}
	corners := [4][2]float64{{bx0, by0}, {bx1, by0}, {bx1, by1}, {bx0, by1}}
	for i, c := range corners {
		xs[i] = m[0]*c[0] + m[2]*c[1] + m[4]
		ys[i] = m[1]*c[0] + m[3]*c[1] + m[5]
	}
	tx0, ty0, tx1, ty1 := xs[0], ys[0], xs[0], ys[0]
	for i := 1; i < 4; i++ {
		tx0 = minF(tx0, xs[i])
		tx1 = maxF(tx1, xs[i])
		ty0 = minF(ty0, ys[i])
		ty1 = maxF(ty1, ys[i])
	}

	sx, sy := 1.0, 1.0
	if tx1-tx0 != 0 {
		sx = (rect.URX - rect.LLX) / (tx1 - tx0)
	}
	if ty1-ty0 != 0 {
		sy = (rect.URY - rect.LLY) / (ty1 - ty0)
	}
	e := rect.LLX - sx*tx0
	fv := rect.LLY - sy*ty0
	return fmt.Sprintf("%s 0 0 %s %s %s",
		formatFloat(sx), formatFloat(sy), formatFloat(e), formatFloat(fv))
}

// registerFormXObject adds the appearance Form XObject to the page's
// /Resources/XObject under a fresh /Fm* name and returns that name.
func (p *Page) registerFormXObject(ref pdfRef) string {
	pageDict := p.pageDict()
	resources := p.pageResources()
	if resources == nil {
		resources = pdfDict{}
		pageDict["/Resources"] = resources
	}
	xobjDict, _ := resolveRef(p.doc.objects, resources["/XObject"]).(pdfDict)
	if xobjDict == nil {
		xobjDict = pdfDict{}
		resources["/XObject"] = xobjDict
	}
	for i := 0; ; i++ {
		name := fmt.Sprintf("/Fm%d", i)
		if _, exists := xobjDict[name]; !exists {
			xobjDict[name] = ref
			return name
		}
	}
}

// pageForObject returns a *Page view for the given page object, or nil if the
// object is not one of the document's pages.
func (d *Document) pageForObject(obj *pdfObject) *Page {
	for i, p := range d.pages {
		if p == obj || p.Num == obj.Num {
			return &Page{doc: d, index: i}
		}
	}
	return nil
}

// readRectArray reads a [llx lly urx ury] rectangle from a PDF array value.
func readRectArray(v pdfValue) (Rectangle, bool) {
	arr, ok := v.(pdfArray)
	if !ok || len(arr) != 4 {
		return Rectangle{}, false
	}
	llx, _ := toFloat(arr[0])
	lly, _ := toFloat(arr[1])
	urx, _ := toFloat(arr[2])
	ury, _ := toFloat(arr[3])
	// Normalize so LL is the lower-left corner.
	return Rectangle{
		LLX: minF(llx, urx), LLY: minF(lly, ury),
		URX: maxF(llx, urx), URY: maxF(lly, ury),
	}, true
}

// buildDictToID maps each dict's identity to its object ID. pdfDict is a map
// (reference type); its identity is stable for the lifetime of the document.
func buildDictToID(objects map[int]*pdfObject) map[string]int {
	m := make(map[string]int, len(objects))
	for id, obj := range objects {
		if d, ok := obj.Value.(pdfDict); ok {
			m[dictIdentity(d)] = id
		}
	}
	return m
}

func dictIdentity(d pdfDict) string { return fmt.Sprintf("%p", d) }

// spliceRefs returns arr with every indirect reference whose object number is
// in remove dropped.
func spliceRefs(arr pdfArray, remove map[int]bool) pdfArray {
	out := make(pdfArray, 0, len(arr))
	for _, item := range arr {
		if ref, ok := item.(pdfRef); ok && remove[ref.Num] {
			continue
		}
		out = append(out, item)
	}
	return out
}
