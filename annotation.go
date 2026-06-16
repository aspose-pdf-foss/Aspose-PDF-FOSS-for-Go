// SPDX-License-Identifier: MIT

package asposepdf

import "fmt"

// AnnotationType identifies the kind of annotation. Returned by
// Annotation.AnnotationType() so callers can switch on type without a
// type-assertion ladder.
type AnnotationType int

const (
	AnnotationTypeUnknown AnnotationType = iota
	AnnotationTypeLink
	AnnotationTypeHighlight
	AnnotationTypeUnderline
	AnnotationTypeStrikeOut
	AnnotationTypeSquiggly
	AnnotationTypeWidget
	AnnotationTypeSquare
	AnnotationTypeCircle
	AnnotationTypeLine
	AnnotationTypeInk
	AnnotationTypeText
	AnnotationTypeFreeText
	AnnotationTypeStamp
	AnnotationTypeFileAttachment
	AnnotationTypeRedact
)

// Annotation is the common interface implemented by every concrete
// annotation type. Page-scoped — annotations belong to a specific page
// and are managed through that page's AnnotationCollection.
type Annotation interface {
	AnnotationType() AnnotationType
	Rect() Rectangle
	SetRect(r Rectangle)
	Color() *Color
	SetColor(c *Color)
	Title() string
	SetTitle(s string)
	Contents() string
	SetContents(s string)
	PageIndex() int

	// Flatten bakes this annotation's normal appearance into the page
	// content and removes the annotation, making it non-interactive. See
	// (*annotationBase).Flatten in flatten.go.
	Flatten() error

	// seals the interface — external packages cannot implement Annotation directly.
	annotationBaseRef() *annotationBase
}

// annotationBase is embedded into every concrete annotation type. It
// owns the underlying pdfDict and tracks attachment state.
type annotationBase struct {
	dict         pdfDict
	doc          *Document
	page         *Page // construction-time page reference
	attachedPage *pdfObject
	objID        int // 0 until Add() runs
}

// annotationBaseRef satisfies the unexported part of the Annotation
// interface — see the interface declaration above.
func (b *annotationBase) annotationBaseRef() *annotationBase { return b }

// AnnotationCollection is the live, ordered set of annotations attached
// to a single page. Mutations through Add / Delete propagate to the
// page dict's /Annots array and to the document's object table; the
// next Save writes them out.
type AnnotationCollection struct {
	page *Page
	// items is rebuilt on every public access from page.pageObj()'s
	// /Annots so that handles obtained via different *Page references
	// to the same logical page (Document.Page(n) returns a fresh *Page
	// each call) all observe the same live state.
	items []Annotation
}

// Count reports how many annotations live on this page.
func (c *AnnotationCollection) Count() int {
	c.rebuild()
	return len(c.items)
}

// All returns the page's annotations as a slice. Each Annotation in the
// slice is a live handle: mutations write through to the underlying
// pdfDict and are visible to callers holding the same handle.
func (c *AnnotationCollection) All() []Annotation {
	c.rebuild()
	return c.items
}

// Add attaches an annotation to this page. Errors if the annotation is
// already attached to a different page; idempotent same-page Add returns
// nil. Panics on nil annotation (programmer error).
func (c *AnnotationCollection) Add(a Annotation) error {
	if a == nil {
		panic("Annotations.Add: nil annotation")
	}
	c.rebuild()
	base := a.annotationBaseRef()
	if base.objID != 0 {
		if base.attachedPage == c.page.pageObj() {
			return nil // idempotent same-page
		}
		return fmt.Errorf("annotation already attached to page %d; Delete from that page first", c.attachedPageIndex(base))
	}
	// First-time attach.
	base.dict["/P"] = pdfRef{Num: c.page.pageObj().Num}
	objID := c.page.doc.nextID
	c.page.doc.nextID++
	c.page.doc.objects[objID] = &pdfObject{Num: objID, Value: base.dict}
	base.objID = objID
	base.attachedPage = c.page.pageObj()
	base.doc = c.page.doc

	// Append to page's /Annots array (preserves indirect-ref form if used).
	// No need to update c.items here — the next public access calls rebuild().
	appendAnnotToPage(c.page.doc.objects, c.page.pageObj(), pdfRef{Num: objID})
	return nil
}

// At returns the annotation at the given index. Panics if out of range.
func (c *AnnotationCollection) At(index int) Annotation {
	c.rebuild()
	return c.items[index]
}

// Delete removes the annotation from this page. Returns true if found,
// false otherwise. The annotation handle becomes dangling after Delete:
// mutations through it write to an unlinked dict that is no longer
// reachable from the document and are silently dropped on next Save.
func (c *AnnotationCollection) Delete(a Annotation) bool {
	if a == nil {
		return false
	}
	c.rebuild()
	base := a.annotationBaseRef()
	if base.objID == 0 || base.attachedPage != c.page.pageObj() {
		return false
	}
	// Splice out of /Annots (preserves indirect-ref form if used).
	removeAnnotFromPage(c.page.doc.objects, c.page.pageObj(), base.objID)
	delete(c.page.doc.objects, base.objID)
	// No need to update c.items — the next public access calls rebuild().
	base.objID = 0
	base.attachedPage = nil
	return true
}

// DeleteAt removes the annotation at the given index. Errors on
// out-of-range index. The annotation handle becomes dangling after
// DeleteAt — see Delete for the dangling-handle semantics.
func (c *AnnotationCollection) DeleteAt(index int) error {
	c.rebuild()
	if index < 0 || index >= len(c.items) {
		return fmt.Errorf("AnnotationCollection.DeleteAt(%d): out of range [0,%d)", index, len(c.items))
	}
	a := c.items[index]
	// items[index] is guaranteed attached to this page by Add; Delete only
	// returns false for nil, unattached, or wrong-page handles — none apply.
	// The branch below is a defensive invariant assertion.
	if !c.Delete(a) {
		return fmt.Errorf("AnnotationCollection.DeleteAt(%d): invariant violated (Delete returned false on a known-attached handle)", index)
	}
	return nil
}

// attachedPageIndex returns the 1-based index of the page an annotation
// is currently attached to (used in error messages).
func (c *AnnotationCollection) attachedPageIndex(base *annotationBase) int {
	if base.attachedPage == nil {
		return 0
	}
	for i, p := range c.page.doc.pages {
		if p.Num == base.attachedPage.Num {
			return i + 1
		}
	}
	return 0
}

// rebuild rebuilds c.items from the page's /Annots. Called from every
// public method so that AnnotationCollection is a thin view, not a
// stale snapshot. See the AnnotationCollection field comment.
func (c *AnnotationCollection) rebuild() {
	c.items = nil
	c.walkAnnotations()
}

// WidgetAnnotation is the read-only view of a form widget annotation
// surfaced through AnnotationCollection. Form fields continue to be
// mutated via the Form API — a WidgetAnnotation only exposes the base
// Annotation surface (Rect, Color, Title, Contents, PageIndex).
type WidgetAnnotation struct {
	annotationBase
}

func (a *WidgetAnnotation) AnnotationType() AnnotationType { return AnnotationTypeWidget }

// GenericAnnotation is the catch-all surface for /Subtype values this
// release does not yet model (Stamp, FreeText, Ink, etc.). It exposes
// only the base Annotation accessors — callers can detect it via
// AnnotationType() == AnnotationTypeUnknown.
type GenericAnnotation struct {
	annotationBase
}

func (a *GenericAnnotation) AnnotationType() AnnotationType { return AnnotationTypeUnknown }

// Rect returns the annotation rectangle. Empty Rectangle if /Rect is
// missing or malformed.
func (b *annotationBase) Rect() Rectangle {
	arr, ok := b.dict["/Rect"].(pdfArray)
	if !ok || len(arr) != 4 {
		return Rectangle{}
	}
	llx, _ := toFloat(arr[0])
	lly, _ := toFloat(arr[1])
	urx, _ := toFloat(arr[2])
	ury, _ := toFloat(arr[3])
	return Rectangle{LLX: llx, LLY: lly, URX: urx, URY: ury}
}

// SetRect writes the annotation rectangle.
func (b *annotationBase) SetRect(r Rectangle) {
	b.dict["/Rect"] = pdfArray{r.LLX, r.LLY, r.URX, r.URY}
}

// Color returns the /C array as an RGB Color. Returns nil if /C is
// absent.
func (b *annotationBase) Color() *Color {
	arr, _ := b.dict["/C"].(pdfArray)
	return annotColorFromComponents(arr)
}

// annotColorFromComponents converts a PDF annotation colour array (/C, /IC) to a
// Color per ISO 32000-1 §8.6.3 / Table 164: 1 number = DeviceGray, 3 = DeviceRGB,
// 4 = DeviceCMYK, 0 (or anything else) = no colour (nil = transparent). CMYK is
// converted through the shared Adobe LUT so it matches the renderer's other CMYK
// paths (and Acrobat) rather than the bluish naive formula.
func annotColorFromComponents(arr pdfArray) *Color {
	switch len(arr) {
	case 1:
		g, _ := toFloat(arr[0])
		return &Color{R: g, G: g, B: g, A: 1}
	case 3:
		r, _ := toFloat(arr[0])
		g, _ := toFloat(arr[1])
		bl, _ := toFloat(arr[2])
		return &Color{R: r, G: g, B: bl, A: 1}
	case 4:
		c, _ := toFloat(arr[0])
		m, _ := toFloat(arr[1])
		y, _ := toFloat(arr[2])
		k, _ := toFloat(arr[3])
		r, g, bl := adobeCMYKToRGB(c, m, y, k)
		return &Color{R: float64(r) / 255, G: float64(g) / 255, B: float64(bl) / 255, A: 1}
	}
	return nil
}

// SetColor writes /C as an RGB array; nil removes the entry.
func (b *annotationBase) SetColor(c *Color) {
	if c == nil {
		delete(b.dict, "/C")
		return
	}
	b.dict["/C"] = pdfArray{c.R, c.G, c.B}
}

// Title returns /T (the annotation author / reviewer name).
func (b *annotationBase) Title() string {
	return decodeFormString(b.dict["/T"])
}

// SetTitle writes /T (the annotation author / reviewer name); empty
// string removes the entry.
func (b *annotationBase) SetTitle(s string) {
	if s == "" {
		delete(b.dict, "/T")
		return
	}
	b.dict["/T"] = encodeFormString(s)
}

// Contents returns /Contents (the annotation body text).
func (b *annotationBase) Contents() string {
	return decodeFormString(b.dict["/Contents"])
}

// SetContents writes /Contents (the annotation body text); empty string
// removes the entry.
func (b *annotationBase) SetContents(s string) {
	if s == "" {
		delete(b.dict, "/Contents")
		return
	}
	b.dict["/Contents"] = encodeFormString(s)
}

// PageIndex returns the 1-based index of the page this annotation lives
// on. 0 if the annotation is not yet attached or its /P doesn't resolve.
func (b *annotationBase) PageIndex() int {
	if b.attachedPage == nil {
		return 0
	}
	for i, p := range b.doc.pages {
		if p.Num == b.attachedPage.Num {
			return i + 1
		}
	}
	return 0
}

// walkAnnotations builds the AnnotationCollection.items slice from the
// page's /Annots array. Each ref is dispatched by /Subtype to the right
// concrete type.
func (c *AnnotationCollection) walkAnnotations() {
	pageDict, _ := c.page.pageObj().Value.(pdfDict)
	if pageDict == nil {
		return
	}
	arr, ok := resolveRefToArray(c.page.doc.objects, pageDict["/Annots"])
	if !ok || len(arr) == 0 {
		return
	}
	for _, item := range arr {
		ref, ok := item.(pdfRef)
		if !ok {
			continue
		}
		obj, ok := c.page.doc.objects[ref.Num]
		if !ok {
			continue
		}
		dict, ok := obj.Value.(pdfDict)
		if !ok {
			continue
		}
		base := annotationBase{
			dict:         dict,
			doc:          c.page.doc,
			page:         c.page,
			attachedPage: c.page.pageObj(),
			objID:        ref.Num,
		}
		annot := parseAnnotation(base)
		if annot != nil {
			c.items = append(c.items, annot)
		}
	}
}

// parseAnnotation builds the right concrete type for the given dict.
// Future subepics extend this dispatch.
func parseAnnotation(base annotationBase) Annotation {
	subtype, _ := base.dict["/Subtype"].(pdfName)
	switch subtype {
	case "/Widget":
		return &WidgetAnnotation{annotationBase: base}
	case "/Link":
		return &LinkAnnotation{annotationBase: base}
	case "/Highlight":
		return &HighlightAnnotation{annotationBase: base}
	case "/Underline":
		return &UnderlineAnnotation{annotationBase: base}
	case "/StrikeOut":
		return &StrikeOutAnnotation{annotationBase: base}
	case "/Squiggly":
		return &SquigglyAnnotation{annotationBase: base}
	case "/Square":
		sq := &SquareAnnotation{drawingAnnotationBase: drawingAnnotationBase{annotationBase: base}}
		sq.regenerate = sq.regenerateAP
		return sq
	case "/Circle":
		c := &CircleAnnotation{drawingAnnotationBase: drawingAnnotationBase{annotationBase: base}}
		c.regenerate = c.regenerateAP
		return c
	case "/Line":
		ln := &LineAnnotation{drawingAnnotationBase: drawingAnnotationBase{annotationBase: base}}
		ln.regenerate = ln.regenerateAP
		return ln
	case "/Ink":
		ink := &InkAnnotation{drawingAnnotationBase: drawingAnnotationBase{annotationBase: base}}
		ink.regenerate = ink.regenerateAP
		return ink
	case "/Text":
		return parseTextAnnotation(base)
	case "/Stamp":
		return parseStampAnnotation(base)
	case "/FreeText":
		return parseFreeTextAnnotation(base)
	case "/FileAttachment":
		return parseFileAttachmentAnnotation(base)
	case "/Redact":
		return parseRedactAnnotation(base)
	}
	return &GenericAnnotation{annotationBase: base}
}

// appendAnnotToPage appends annotRef to the page's /Annots array,
// preserving the original storage form: if /Annots is an indirect
// reference, the referenced array object is mutated in place; if it is
// an inline array, the inline array is replaced with the appended copy;
// if it is absent, an inline single-element array is created.
func appendAnnotToPage(objects map[int]*pdfObject, pageObj *pdfObject, annotRef pdfRef) {
	pageDict, ok := pageObj.Value.(pdfDict)
	if !ok {
		return
	}
	switch v := pageDict["/Annots"].(type) {
	case pdfRef:
		if obj, ok := objects[v.Num]; ok {
			if arr, ok := obj.Value.(pdfArray); ok {
				obj.Value = append(arr, annotRef)
				return
			}
		}
		pageDict["/Annots"] = pdfArray{annotRef}
	case pdfArray:
		pageDict["/Annots"] = append(v, annotRef)
	default:
		pageDict["/Annots"] = pdfArray{annotRef}
	}
}

// removeAnnotFromPage splices annotRef out of the page's /Annots array,
// preserving the original storage form. If the ref is not found the
// page state is unchanged.
func removeAnnotFromPage(objects map[int]*pdfObject, pageObj *pdfObject, annotObjID int) {
	pageDict, ok := pageObj.Value.(pdfDict)
	if !ok {
		return
	}
	splice := func(arr pdfArray) pdfArray {
		out := make(pdfArray, 0, len(arr))
		for _, item := range arr {
			if r, ok := item.(pdfRef); ok && r.Num == annotObjID {
				continue
			}
			out = append(out, item)
		}
		return out
	}
	switch v := pageDict["/Annots"].(type) {
	case pdfRef:
		if obj, ok := objects[v.Num]; ok {
			if arr, ok := obj.Value.(pdfArray); ok {
				obj.Value = splice(arr)
			}
		}
	case pdfArray:
		pageDict["/Annots"] = splice(v)
	}
}
