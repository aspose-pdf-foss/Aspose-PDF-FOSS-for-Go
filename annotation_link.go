package asposepdf

// LinkAnnotation is a clickable region. Its visual is rendered by the
// viewer (no /AP needed). The associated /A action determines what
// happens on click — see Action and the various NewXxxAction factories.
type LinkAnnotation struct {
	annotationBase
}

func (a *LinkAnnotation) AnnotationType() AnnotationType { return AnnotationTypeLink }

// Action returns the action attached to this link, or nil if no /A is
// present or the action type is unsupported.
func (a *LinkAnnotation) Action() Action {
	v, ok := a.dict["/A"]
	if !ok {
		return nil
	}
	d, ok := resolveRefToDict(a.doc.objects, v)
	if !ok {
		return nil
	}
	act := parseAction(d)
	// Post-process: resolve GoTo destinations that reference a page object.
	if gt, ok := act.(*GoToAction); ok {
		gt.doc = a.doc
		if gt.pageNum == 0 {
			if dest, ok := d["/D"].(pdfArray); ok && len(dest) > 0 {
				if ref, ok := dest[0].(pdfRef); ok {
					for i, p := range a.doc.pages {
						if p.Num == ref.Num {
							gt.pageNum = i + 1
							break
						}
					}
				}
			}
		}
	}
	return act
}

// SetAction writes the /A entry. nil clears /A.
func (a *LinkAnnotation) SetAction(act Action) {
	if act == nil {
		delete(a.dict, "/A")
		return
	}
	a.dict["/A"] = act.encode()
}

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
