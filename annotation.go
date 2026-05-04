package asposepdf

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
	page  *Page
	items []Annotation
	built bool // false until first Annotations() call walks /Annots
}

// Count reports how many annotations live on this page.
func (c *AnnotationCollection) Count() int {
	c.ensureBuilt()
	return len(c.items)
}

// All returns the page's annotations as a slice. Each Annotation in the
// slice is a live handle: mutations write through to the underlying
// pdfDict and are visible to callers holding the same handle.
func (c *AnnotationCollection) All() []Annotation {
	c.ensureBuilt()
	return c.items
}

// ensureBuilt populates c.items lazily on first access.
func (c *AnnotationCollection) ensureBuilt() {
	if c.built {
		return
	}
	c.built = true
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
	arr, ok := b.dict["/C"].(pdfArray)
	if !ok {
		return nil
	}
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
		// CMYK — convert to a rough RGB approximation. Most annotation
		// software writes RGB; CMYK is rare for /C.
		c, _ := toFloat(arr[0])
		m, _ := toFloat(arr[1])
		y, _ := toFloat(arr[2])
		k, _ := toFloat(arr[3])
		return &Color{
			R: (1 - c) * (1 - k),
			G: (1 - m) * (1 - k),
			B: (1 - y) * (1 - k),
			A: 1,
		}
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
	}
	return &GenericAnnotation{annotationBase: base}
}
