package asposepdf

// RedactAnnotation marks regions for redaction. Mark mode (this type)
// renders a semi-transparent fill of /QuadPoints regions. The
// destructive content removal happens when (*Document).ApplyRedactions
// is called — this annotation is then removed and the underlying page
// content is irreversibly rewritten. Per ISO 32000-1 §12.5.6.20.
type RedactAnnotation struct {
	drawingAnnotationBase
}

func (a *RedactAnnotation) AnnotationType() AnnotationType { return AnnotationTypeRedact }

// NewRedactAnnotation builds an unbound redact annotation. Page must
// be non-nil. By default, /QuadPoints is empty (rendering uses /Rect
// as a single quad). Callers typically call SetQuadPoints to specify
// multiple disjoint regions.
func NewRedactAnnotation(page *Page, rect Rectangle) *RedactAnnotation {
	if page == nil {
		panic("NewRedactAnnotation: nil page")
	}
	dict := pdfDict{
		"/Type":    pdfName("/Annot"),
		"/Subtype": pdfName("/Redact"),
		"/Rect":    pdfArray{rect.LLX, rect.LLY, rect.URX, rect.URY},
	}
	a := &RedactAnnotation{drawingAnnotationBase: drawingAnnotationBase{
		annotationBase: annotationBase{
			dict: dict,
			doc:  page.doc,
			page: page,
		},
	}}
	a.regenerate = a.regenerateAP
	a.regenerateAP()
	return a
}

// QuadPoints returns the regions to redact in page space. Returns nil
// if /QuadPoints is absent (Apply uses /Rect as the single region).
func (a *RedactAnnotation) QuadPoints() []QuadPoint {
	return readQuadPoints(a.dict["/QuadPoints"])
}

// SetQuadPoints writes /QuadPoints. nil/empty slice removes the entry
// (Apply will then use /Rect as single region).
func (a *RedactAnnotation) SetQuadPoints(qp []QuadPoint) {
	if len(qp) == 0 {
		delete(a.dict, "/QuadPoints")
	} else {
		a.dict["/QuadPoints"] = quadPointsToPDFArray(qp)
	}
	a.regenerateAP()
}

// regenerateAP rebuilds /AP/N for mark-mode visual.
func (a *RedactAnnotation) regenerateAP() {
	setAppearanceN(&a.annotationBase, generateRedactAppearance(a))
}

// RegenerateAppearance forces /AP/N to be rebuilt.
func (a *RedactAnnotation) RegenerateAppearance() {
	a.regenerateAP()
}

// parseRedactAnnotation builds a RedactAnnotation from a parsed dict.
func parseRedactAnnotation(base annotationBase) *RedactAnnotation {
	a := &RedactAnnotation{drawingAnnotationBase: drawingAnnotationBase{annotationBase: base}}
	a.regenerate = a.regenerateAP
	return a
}
