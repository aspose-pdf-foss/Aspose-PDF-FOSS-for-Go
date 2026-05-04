package asposepdf

// LinkAnnotation is a clickable region. Its visual is rendered by the
// viewer (no /AP needed). The associated /A action determines what
// happens on click — see Action and the various NewXxxAction factories.
type LinkAnnotation struct {
	annotationBase
}

func (a *LinkAnnotation) AnnotationType() AnnotationType { return AnnotationTypeLink }

// NewLinkAnnotation builds an unbound link annotation. Page must be
// non-nil. The annotation is not added to the document until
// page.Annotations().Add(link) succeeds.
func NewLinkAnnotation(page *Page, rect Rectangle) *LinkAnnotation {
	if page == nil {
		panic("NewLinkAnnotation: nil page")
	}
	dict := pdfDict{
		"/Type":    pdfName("/Annot"),
		"/Subtype": pdfName("/Link"),
		"/Rect":    pdfArray{rect.LLX, rect.LLY, rect.URX, rect.URY},
	}
	return &LinkAnnotation{annotationBase: annotationBase{
		dict: dict,
		doc:  page.doc,
		page: page,
	}}
}
