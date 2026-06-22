// SPDX-License-Identifier: MIT

package asposepdf

import (
	"fmt"
	"sort"
	"strconv"
)

// XForm is a reusable Form XObject (ISO 32000-1 §8.10) — a self-contained
// content stream (with its own resources) that can be placed on any number of
// pages and positions with a single Do invocation. Draw into it once via its
// Canvas (the full *Page drawing API: AddText, Draw*, AddImage, AddTable, …),
// then place it with Page.AddForm. Ideal for page templates, shared
// headers/footers and watermarks. Mirrors the intent of Aspose.PDF for .NET's
// XForm.
type XForm struct {
	doc    *Document
	canvas *Page     // detached drawing surface (not part of doc.pages)
	bbox   Rectangle // form coordinate box, [0 0 width height]
	formID int       // object ID of the built /Form XObject; 0 until first placed
}

// CreateForm creates an empty Form XObject of the given size (points). Draw its
// content through Canvas(), then place it on pages with Page.AddForm.
func (d *Document) CreateForm(width, height float64) *XForm {
	canvasDict := pdfDict{
		"/Type":      pdfName("/Page"),
		"/MediaBox":  pdfArray{0.0, 0.0, width, height},
		"/Resources": pdfDict{},
	}
	return &XForm{
		doc:    d,
		canvas: &Page{doc: d, obj: &pdfObject{Value: canvasDict}},
		bbox:   Rectangle{URX: width, URY: height},
	}
}

// Canvas returns the drawing surface for the form. Use the full *Page drawing
// API on it (AddText, DrawLine, AddImage, AddTable, …). Draw everything before
// the first Page.AddForm — the form's content is frozen when it is first placed.
func (x *XForm) Canvas() *Page { return x.canvas }

// Size returns the form's width and height in points.
func (x *XForm) Size() (width, height float64) { return x.bbox.URX, x.bbox.URY }

// ensureBuilt materialises the /Form XObject object in x.doc (once) from the
// canvas content, returning its object number. A form obtained from Page.Forms
// already has its object, so it is returned unchanged.
func (x *XForm) ensureBuilt() (int, error) {
	if x.formID != 0 {
		return x.formID, nil
	}
	stream, err := x.build()
	if err != nil {
		return 0, err
	}
	x.formID = x.doc.nextID
	x.doc.nextID++
	x.doc.objects[x.formID] = &pdfObject{Num: x.formID, Value: stream}
	return x.formID, nil
}

// Forms returns the Form XObjects referenced by the page's resources — the
// reusable content groups already present (e.g. parsed from a loaded PDF or
// placed earlier). Each can be re-placed on any page of the same document with
// AddForm, or imported into another document with Document.ImportForm.
func (p *Page) Forms() []*XForm {
	resources := p.pageResources()
	if resources == nil {
		return nil
	}
	xobj, ok := resolveRefToDict(p.doc.objects, resources["/XObject"])
	if !ok {
		return nil
	}
	names := make([]string, 0, len(xobj))
	for k := range xobj {
		names = append(names, k)
	}
	sort.Strings(names)

	var out []*XForm
	seen := map[int]bool{}
	for _, name := range names {
		ref, ok := xobj[name].(pdfRef)
		if !ok || seen[ref.Num] {
			continue
		}
		obj, ok := p.doc.objects[ref.Num]
		if !ok {
			continue
		}
		st, ok := obj.Value.(*pdfStream)
		if !ok || dictGetName(st.Dict, "/Subtype") != "/Form" {
			continue
		}
		seen[ref.Num] = true
		out = append(out, &XForm{doc: p.doc, formID: ref.Num, bbox: bboxFromForm(p.doc, st)})
	}
	return out
}

// ImportForm copies a Form XObject (and its whole resource graph: fonts,
// images, nested forms, …) from another document into this one, returning a new
// XForm ready to place with Page.AddForm. A form already belonging to this
// document is returned unchanged.
func (d *Document) ImportForm(form *XForm) (*XForm, error) {
	if form == nil {
		return nil, fmt.Errorf("ImportForm: nil form")
	}
	if form.doc == d {
		return form, nil
	}
	srcID, err := form.ensureBuilt()
	if err != nil {
		return nil, err
	}
	idMap := map[int]int{}
	remapped := d.importGraph(form.doc.objects, pdfRef{Num: srcID}, idMap)
	ref, ok := remapped.(pdfRef)
	if !ok {
		return nil, fmt.Errorf("ImportForm: failed to import the form object")
	}
	return &XForm{doc: d, formID: ref.Num, bbox: form.bbox}, nil
}

// bboxFromForm reads a Form XObject's /BBox as a Rectangle (zero if missing).
func bboxFromForm(d *Document, st *pdfStream) Rectangle {
	if bb := shFloats(d.objects, st.Dict["/BBox"]); len(bb) >= 4 {
		return Rectangle{LLX: bb[0], LLY: bb[1], URX: bb[2], URY: bb[3]}
	}
	return Rectangle{}
}

// build harvests the canvas content + resources into a /Form XObject stream.
func (x *XForm) build() (*pdfStream, error) {
	content, err := x.canvas.contentStreams()
	if err != nil {
		return nil, err
	}
	resources := x.canvas.pageResources()
	if resources == nil {
		resources = pdfDict{}
	}
	return makeFormXObjectWithResources(content, x.bbox, resources), nil
}

// AddForm places the form on the page so its box maps onto rect, preserving
// the form's own scale per axis. The form is built (frozen) on the first call
// and reused for every subsequent placement — one /Form XObject, many Do
// invocations across pages and positions. A zero-size rect is rejected.
func (p *Page) AddForm(form *XForm, rect Rectangle) error {
	if form == nil {
		return fmt.Errorf("AddForm: nil form")
	}
	if form.doc != p.doc {
		return fmt.Errorf("AddForm: form belongs to a different document (use Document.ImportForm first)")
	}
	if rect.URX <= rect.LLX || rect.URY <= rect.LLY {
		return fmt.Errorf("AddForm: rect must be non-empty")
	}

	id, err := form.ensureBuilt()
	if err != nil {
		return err
	}
	name := p.registerFormXObject(pdfRef{Num: id})

	// Map the form bbox onto rect (scale per axis, then translate).
	bw, bh := form.bbox.URX-form.bbox.LLX, form.bbox.URY-form.bbox.LLY
	sx, sy := (rect.URX-rect.LLX)/bw, (rect.URY-rect.LLY)/bh
	tx := rect.LLX - sx*form.bbox.LLX
	ty := rect.LLY - sy*form.bbox.LLY
	ops := fmt.Sprintf("q\n%s 0 0 %s %s %s cm\n%s Do\nQ\n",
		formNum(sx), formNum(sy), formNum(tx), formNum(ty), name)
	return p.appendToContentStream([]byte(ops))
}

// formNum formats a coordinate compactly (no exponent, trimmed) for the cm
// operator.
func formNum(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}
