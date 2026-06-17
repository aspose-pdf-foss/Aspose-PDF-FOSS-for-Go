// SPDX-License-Identifier: MIT

package asposepdf

// CaretSymbol is the /Sy entry of a Caret annotation per ISO 32000-1
// §12.5.6.11 Table 180 — an optional symbol drawn together with the
// caret to associate it with an editing action.
type CaretSymbol int

const (
	// CaretSymbolNone draws the caret alone (/Sy /None or absent).
	CaretSymbolNone CaretSymbol = iota
	// CaretSymbolParagraph draws a paragraph symbol with the caret (/Sy /P).
	CaretSymbolParagraph
)

// CaretAnnotation marks a point of text insertion or deletion, drawn as
// an upward caret ("^") filled with the annotation colour. Mirrors
// Aspose.PDF for .NET's CaretAnnotation.
type CaretAnnotation struct {
	drawingAnnotationBase
}

func (a *CaretAnnotation) AnnotationType() AnnotationType { return AnnotationTypeCaret }

// NewCaretAnnotation builds an unbound caret annotation occupying rect.
// Page must be non-nil. The caret is drawn to fill the rectangle; set a
// colour with SetColor (default black).
func NewCaretAnnotation(page *Page, rect Rectangle) *CaretAnnotation {
	if page == nil {
		panic("NewCaretAnnotation: nil page")
	}
	a := &CaretAnnotation{drawingAnnotationBase: drawingAnnotationBase{
		annotationBase: annotationBase{
			dict: pdfDict{
				"/Type":    pdfName("/Annot"),
				"/Subtype": pdfName("/Caret"),
				"/Rect":    pdfArray{rect.LLX, rect.LLY, rect.URX, rect.URY},
			},
			doc:  page.doc,
			page: page,
		},
	}}
	a.regenerate = a.regenerateAP
	a.regenerateAP()
	return a
}

// Symbol returns the /Sy symbol drawn with the caret.
func (a *CaretAnnotation) Symbol() CaretSymbol {
	if n, _ := a.dict["/Sy"].(pdfName); n == "/P" {
		return CaretSymbolParagraph
	}
	return CaretSymbolNone
}

// SetSymbol writes /Sy. CaretSymbolNone removes the entry.
func (a *CaretAnnotation) SetSymbol(s CaretSymbol) {
	switch s {
	case CaretSymbolParagraph:
		a.dict["/Sy"] = pdfName("/P")
	default:
		delete(a.dict, "/Sy")
	}
	a.regenerateAP()
}

func (a *CaretAnnotation) regenerateAP() {
	setAppearanceN(&a.annotationBase, generateCaretAppearance(a))
}

// RegenerateAppearance forces /AP/N to be rebuilt from current properties.
func (a *CaretAnnotation) RegenerateAppearance() { a.regenerateAP() }
