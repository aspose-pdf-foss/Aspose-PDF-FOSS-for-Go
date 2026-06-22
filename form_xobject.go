// SPDX-License-Identifier: MIT

package asposepdf

import (
	"fmt"
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
		return fmt.Errorf("AddForm: form belongs to a different document")
	}
	if rect.URX <= rect.LLX || rect.URY <= rect.LLY {
		return fmt.Errorf("AddForm: rect must be non-empty")
	}

	if form.formID == 0 {
		stream, err := form.build()
		if err != nil {
			return err
		}
		form.formID = p.doc.nextID
		p.doc.nextID++
		p.doc.objects[form.formID] = &pdfObject{Num: form.formID, Value: stream}
	}

	name := p.registerFormXObject(pdfRef{Num: form.formID})

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
