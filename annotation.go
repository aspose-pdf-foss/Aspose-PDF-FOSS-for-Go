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

	// internal accessor — implementers embed annotationBase which exposes
	// this. Not part of the public surface.
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

// All returns the page's annotations as a slice. The returned slice
// shares pointer identity with At() / Field-by-name lookups so mutating
// a value through one accessor is visible through the others.
func (c *AnnotationCollection) All() []Annotation {
	c.ensureBuilt()
	return c.items
}

// ensureBuilt populates c.items lazily on first access. For now this is
// a no-op; Task 2 fills it in.
func (c *AnnotationCollection) ensureBuilt() {
	if c.built {
		return
	}
	c.built = true
	// Task 2 walks page /Annots here.
}
