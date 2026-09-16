// SPDX-License-Identifier: MIT

package asposepdf

// QuadPoint is one quadrilateral within a markup annotation's
// /QuadPoints array. Per ISO 32000-1 §12.5.6.10 the eight floats name
// the corners in this order: (X1,Y1)=upper-left, (X2,Y2)=upper-right,
// (X3,Y3)=lower-left, (X4,Y4)=lower-right (in default user space, so
// "upper" means higher Y).
type QuadPoint struct {
	X1, Y1, X2, Y2, X3, Y3, X4, Y4 float64
}

// markupAnnotationBase is the shared body of the four text-markup
// annotations (Highlight, Underline, StrikeOut, Squiggly), which differ only
// in /Subtype and in how they draw themselves. The quad accessors live here,
// and — as on drawingAnnotationBase for the shape annotations — every setter
// that changes what the annotation looks like calls the regenerate hook the
// concrete type installs, so /AP/N always matches the properties.
type markupAnnotationBase struct {
	annotationBase
	regenerate func()
}

// QuadPoints returns the array of quads describing the marked region.
// Returns nil if /QuadPoints is absent or its array length is not a
// multiple of 8 (malformed).
func (m *markupAnnotationBase) QuadPoints() []QuadPoint {
	return readQuadPoints(m.dict["/QuadPoints"])
}

// SetQuadPoints writes /QuadPoints. nil or empty slice removes the entry.
func (m *markupAnnotationBase) SetQuadPoints(qp []QuadPoint) {
	if len(qp) == 0 {
		delete(m.dict, "/QuadPoints")
	} else {
		m.dict["/QuadPoints"] = quadPointsToPDFArray(qp)
	}
	m.regenerateAppearance()
}

// SetColor writes /C and redraws the appearance in the new colour.
func (m *markupAnnotationBase) SetColor(c *Color) {
	m.annotationBase.SetColor(c)
	m.regenerateAppearance()
}

// SetRect moves the annotation and redraws its appearance, whose coordinates
// are relative to the rectangle.
func (m *markupAnnotationBase) SetRect(r Rectangle) {
	m.annotationBase.SetRect(r)
	m.regenerateAppearance()
}

// regenerateAppearance runs the concrete type's generator, if one is
// installed. Annotations parsed from a file get theirs in annotationFromDict.
func (m *markupAnnotationBase) regenerateAppearance() {
	if m.regenerate != nil {
		m.regenerate()
	}
}

// HighlightAnnotation marks a region with a semi-transparent highlight
// colour, washed over the text the quads cover.
type HighlightAnnotation struct {
	markupAnnotationBase
}

func (a *HighlightAnnotation) AnnotationType() AnnotationType { return AnnotationTypeHighlight }

func (a *HighlightAnnotation) regenerateAP() {
	setAppearanceN(&a.annotationBase, generateHighlightAppearance(a))
}

// RegenerateAppearance forces /AP/N to be rebuilt from current properties.
func (a *HighlightAnnotation) RegenerateAppearance() { a.regenerateAP() }

// NewHighlightAnnotation builds an unbound highlight annotation. Page
// must be non-nil.
func NewHighlightAnnotation(page *Page, rect Rectangle) *HighlightAnnotation {
	a := &HighlightAnnotation{markupAnnotationBase{
		annotationBase: newMarkupBase("NewHighlightAnnotation", page, rect, "/Highlight"),
	}}
	a.regenerate = a.regenerateAP
	a.regenerateAP()
	return a
}

// newMarkupBase is the shared constructor body for the four markup
// types. Only /Subtype differs; everything else is identical. The
// callerName argument identifies the public entry point for panic
// diagnostics ("NewHighlightAnnotation: nil page", etc.).
func newMarkupBase(callerName string, page *Page, rect Rectangle, subtype pdfName) annotationBase {
	if page == nil {
		panic(callerName + ": nil page")
	}
	dict := pdfDict{
		"/Type":    pdfName("/Annot"),
		"/Subtype": subtype,
		"/Rect":    pdfArray{rect.LLX, rect.LLY, rect.URX, rect.URY},
	}
	return annotationBase{dict: dict, doc: page.doc, page: page}
}

// UnderlineAnnotation draws a horizontal line under text.
type UnderlineAnnotation struct {
	markupAnnotationBase
}

func (a *UnderlineAnnotation) AnnotationType() AnnotationType { return AnnotationTypeUnderline }

func (a *UnderlineAnnotation) regenerateAP() {
	setAppearanceN(&a.annotationBase, generateUnderlineAppearance(a))
}

// RegenerateAppearance forces /AP/N to be rebuilt from current properties.
func (a *UnderlineAnnotation) RegenerateAppearance() { a.regenerateAP() }

// NewUnderlineAnnotation builds an unbound underline annotation.
func NewUnderlineAnnotation(page *Page, rect Rectangle) *UnderlineAnnotation {
	a := &UnderlineAnnotation{markupAnnotationBase{
		annotationBase: newMarkupBase("NewUnderlineAnnotation", page, rect, "/Underline"),
	}}
	a.regenerate = a.regenerateAP
	a.regenerateAP()
	return a
}

// StrikeOutAnnotation draws a horizontal line through text.
type StrikeOutAnnotation struct {
	markupAnnotationBase
}

func (a *StrikeOutAnnotation) AnnotationType() AnnotationType { return AnnotationTypeStrikeOut }

func (a *StrikeOutAnnotation) regenerateAP() {
	setAppearanceN(&a.annotationBase, generateStrikeOutAppearance(a))
}

// RegenerateAppearance forces /AP/N to be rebuilt from current properties.
func (a *StrikeOutAnnotation) RegenerateAppearance() { a.regenerateAP() }

// NewStrikeOutAnnotation builds an unbound strike-out annotation.
func NewStrikeOutAnnotation(page *Page, rect Rectangle) *StrikeOutAnnotation {
	a := &StrikeOutAnnotation{markupAnnotationBase{
		annotationBase: newMarkupBase("NewStrikeOutAnnotation", page, rect, "/StrikeOut"),
	}}
	a.regenerate = a.regenerateAP
	a.regenerateAP()
	return a
}

// SquigglyAnnotation draws a wavy underline under text (typically used
// for spell-check style hints).
type SquigglyAnnotation struct {
	markupAnnotationBase
}

func (a *SquigglyAnnotation) AnnotationType() AnnotationType { return AnnotationTypeSquiggly }

func (a *SquigglyAnnotation) regenerateAP() {
	setAppearanceN(&a.annotationBase, generateSquigglyAppearance(a))
}

// RegenerateAppearance forces /AP/N to be rebuilt from current properties.
func (a *SquigglyAnnotation) RegenerateAppearance() { a.regenerateAP() }

// NewSquigglyAnnotation builds an unbound squiggly-underline annotation.
func NewSquigglyAnnotation(page *Page, rect Rectangle) *SquigglyAnnotation {
	a := &SquigglyAnnotation{markupAnnotationBase{
		annotationBase: newMarkupBase("NewSquigglyAnnotation", page, rect, "/Squiggly"),
	}}
	a.regenerate = a.regenerateAP
	a.regenerateAP()
	return a
}

func readQuadPoints(v pdfValue) []QuadPoint {
	arr, ok := v.(pdfArray)
	if !ok || len(arr)%8 != 0 {
		return nil
	}
	out := make([]QuadPoint, 0, len(arr)/8)
	for i := 0; i+7 < len(arr); i += 8 {
		var qp QuadPoint
		qp.X1, _ = toFloat(arr[i])
		qp.Y1, _ = toFloat(arr[i+1])
		qp.X2, _ = toFloat(arr[i+2])
		qp.Y2, _ = toFloat(arr[i+3])
		qp.X3, _ = toFloat(arr[i+4])
		qp.Y3, _ = toFloat(arr[i+5])
		qp.X4, _ = toFloat(arr[i+6])
		qp.Y4, _ = toFloat(arr[i+7])
		out = append(out, qp)
	}
	return out
}

func quadPointsToPDFArray(qp []QuadPoint) pdfArray {
	arr := make(pdfArray, 0, len(qp)*8)
	for _, q := range qp {
		arr = append(arr, q.X1, q.Y1, q.X2, q.Y2, q.X3, q.Y3, q.X4, q.Y4)
	}
	return arr
}
