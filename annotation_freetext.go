package asposepdf

// FreeTextIntent per ISO 32000-1 §12.5.6.6 /IT entry. Defaults to
// FreeTextIntentFreeText (plain text in a rectangle).
type FreeTextIntent int

const (
	FreeTextIntentFreeText  FreeTextIntent = iota // /FreeText
	FreeTextIntentCallout                          // /FreeTextCallout
	FreeTextIntentTypewriter                       // /FreeTextTypeWriter
)

// BorderEffect controls the /BE/S entry per ISO 32000-1 §12.5.4 Table 167.
type BorderEffect int

const (
	BorderEffectNone   BorderEffect = iota // /S = /S (default)
	BorderEffectCloudy                      // /S = /C — wavy "cloud" border
)

// FreeTextAnnotation displays text directly on the page, rendered into
// /AP/N using an embedded font. Per ISO 32000-1 §12.5.6.6.
//
// Supports /BS border, /BG background, /Q alignment via TextStyle, plus
// /IT intent (FreeText / FreeTextCallout / FreeTextTypewriter), /CL
// callout points, /BE border effect (cloudy), and /RD inner rect for
// callouts. Full feature set added incrementally across Tasks 9-17.
type FreeTextAnnotation struct {
	drawingAnnotationBase
}

func (a *FreeTextAnnotation) AnnotationType() AnnotationType { return AnnotationTypeFreeText }

// NewFreeTextAnnotation builds an unbound FreeText annotation. Page
// must be non-nil. Contents is the text body. style configures font,
// size, color, alignment, and background — full TextStyle ↔ /DA/Q/BG
// mapping wired in Task 10.
func NewFreeTextAnnotation(page *Page, rect Rectangle, contents string, style TextStyle) *FreeTextAnnotation {
	if page == nil {
		panic("NewFreeTextAnnotation: nil page")
	}
	dict := pdfDict{
		"/Type":     pdfName("/Annot"),
		"/Subtype":  pdfName("/FreeText"),
		"/Rect":     pdfArray{rect.LLX, rect.LLY, rect.URX, rect.URY},
		"/Contents": encodeFormString(contents),
		// /DA is required by spec — minimal default for now (Task 10
		// populates it from TextStyle).
		"/DA": "/Helv 12 Tf 0 0 0 rg",
	}
	a := &FreeTextAnnotation{drawingAnnotationBase: drawingAnnotationBase{
		annotationBase: annotationBase{
			dict: dict,
			doc:  page.doc,
			page: page,
		},
	}}
	a.regenerate = a.regenerateAP
	a.regenerateAP()
	_ = style // TODO Task 10: serialize style to /DA + /Q + /BG
	return a
}

// Contents returns the /Contents text body. Overrides annotationBase to
// ensure the value is always read from the dict.
func (a *FreeTextAnnotation) Contents() string {
	return decodeFormString(a.dict["/Contents"])
}

// SetContents writes /Contents and regenerates /AP/N (the rendered
// text content depends on Contents).
func (a *FreeTextAnnotation) SetContents(s string) {
	if s == "" {
		delete(a.dict, "/Contents")
	} else {
		a.dict["/Contents"] = encodeFormString(s)
	}
	a.regenerateAP()
}

// regenerateAP rebuilds /AP/N from current properties. Stub for now —
// full visual rendering in Task 11.
func (a *FreeTextAnnotation) regenerateAP() {
	setAppearanceN(&a.annotationBase, generateFreeTextAppearance(a))
}

// RegenerateAppearance forces /AP/N to be rebuilt from current state.
func (a *FreeTextAnnotation) RegenerateAppearance() {
	a.regenerateAP()
}

// parseFreeTextAnnotation builds a FreeTextAnnotation from a parsed dict.
func parseFreeTextAnnotation(base annotationBase) *FreeTextAnnotation {
	a := &FreeTextAnnotation{drawingAnnotationBase: drawingAnnotationBase{annotationBase: base}}
	a.regenerate = a.regenerateAP
	return a
}
