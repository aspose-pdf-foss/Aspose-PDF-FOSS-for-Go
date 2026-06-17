// SPDX-License-Identifier: MIT

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
	resolveGoToPage(a.doc, act, d)
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

// LinkHighlightMode controls the visual feedback when the link is
// activated by the user (the /H entry per ISO 32000-1 §12.5.6.5).
type LinkHighlightMode int

const (
	// LinkHighlightInvert (default) inverts the contents of the link
	// rectangle when activated.
	LinkHighlightInvert LinkHighlightMode = iota
	// LinkHighlightNone means no visible feedback on activation.
	LinkHighlightNone
	// LinkHighlightOutline inverts the annotation's border.
	LinkHighlightOutline
	// LinkHighlightPush visually depresses the rectangle on activation.
	LinkHighlightPush
)

// Highlight returns the click-feedback mode (/H entry). Returns
// LinkHighlightInvert if /H is absent (the PDF default).
func (a *LinkAnnotation) Highlight() LinkHighlightMode {
	n, ok := a.dict["/H"].(pdfName)
	if !ok {
		return LinkHighlightInvert
	}
	switch n {
	case "/N":
		return LinkHighlightNone
	case "/I":
		return LinkHighlightInvert
	case "/O":
		return LinkHighlightOutline
	case "/P":
		return LinkHighlightPush
	}
	return LinkHighlightInvert
}

// SetHighlight writes the /H entry. LinkHighlightInvert (the default)
// removes /H entirely so that the dict matches a freshly-created link.
func (a *LinkAnnotation) SetHighlight(h LinkHighlightMode) {
	switch h {
	case LinkHighlightNone:
		a.dict["/H"] = pdfName("/N")
	case LinkHighlightOutline:
		a.dict["/H"] = pdfName("/O")
	case LinkHighlightPush:
		a.dict["/H"] = pdfName("/P")
	default: // LinkHighlightInvert is the implicit default — no /H needed
		delete(a.dict, "/H")
	}
}

// BorderWidth returns the width (in points) of the rectangular border
// drawn around the link annotation. Returns the PDF default of 1pt when
// /Border is absent, matching the behaviour of viewers like Acrobat.
func (a *LinkAnnotation) BorderWidth() float64 {
	arr, ok := a.dict["/Border"].(pdfArray)
	if !ok || len(arr) < 3 {
		return 1
	}
	w, err := toFloat(arr[2])
	if err != nil {
		return 1
	}
	return w
}

// SetBorderWidth writes the /Border entry — [horiz-corner-radius
// vert-corner-radius border-width]. Pass 0 to suppress the rectangle
// PDF viewers otherwise draw around the link by default.
func (a *LinkAnnotation) SetBorderWidth(width float64) {
	if width < 0 {
		width = 0
	}
	a.dict["/Border"] = pdfArray{0.0, 0.0, width}
}
