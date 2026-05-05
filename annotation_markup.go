package asposepdf

// QuadPoint is one quadrilateral within a markup annotation's
// /QuadPoints array. Per ISO 32000-1 §12.5.6.10 the eight floats name
// the corners in this order: (X1,Y1)=upper-left, (X2,Y2)=upper-right,
// (X3,Y3)=lower-left, (X4,Y4)=lower-right (in default user space, so
// "upper" means higher Y).
type QuadPoint struct {
	X1, Y1, X2, Y2, X3, Y3, X4, Y4 float64
}

// HighlightAnnotation marks a region with semi-transparent highlight
// color. Renders natively in spec-conforming viewers from /Subtype +
// /QuadPoints + /C — no /AP needed.
type HighlightAnnotation struct {
	annotationBase
}

func (a *HighlightAnnotation) AnnotationType() AnnotationType { return AnnotationTypeHighlight }

// QuadPoints returns the array of quads describing the selection.
// Returns nil if /QuadPoints is absent or its array length is not a
// multiple of 8 (malformed).
func (a *HighlightAnnotation) QuadPoints() []QuadPoint {
	return readQuadPoints(a.dict["/QuadPoints"])
}

// SetQuadPoints writes /QuadPoints. nil or empty slice removes the entry.
func (a *HighlightAnnotation) SetQuadPoints(qp []QuadPoint) {
	if len(qp) == 0 {
		delete(a.dict, "/QuadPoints")
		return
	}
	a.dict["/QuadPoints"] = quadPointsToPDFArray(qp)
}

// NewHighlightAnnotation builds an unbound highlight annotation. Page
// must be non-nil.
func NewHighlightAnnotation(page *Page, rect Rectangle) *HighlightAnnotation {
	return &HighlightAnnotation{annotationBase: newMarkupBase("NewHighlightAnnotation", page, rect, "/Highlight")}
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
	annotationBase
}

func (a *UnderlineAnnotation) AnnotationType() AnnotationType { return AnnotationTypeUnderline }

// QuadPoints returns the array of quads describing the underlined region.
// Returns nil if /QuadPoints is absent or its array length is not a
// multiple of 8 (malformed).
func (a *UnderlineAnnotation) QuadPoints() []QuadPoint {
	return readQuadPoints(a.dict["/QuadPoints"])
}

// SetQuadPoints writes /QuadPoints. nil or empty slice removes the entry.
func (a *UnderlineAnnotation) SetQuadPoints(qp []QuadPoint) {
	if len(qp) == 0 {
		delete(a.dict, "/QuadPoints")
		return
	}
	a.dict["/QuadPoints"] = quadPointsToPDFArray(qp)
}

// NewUnderlineAnnotation builds an unbound underline annotation.
func NewUnderlineAnnotation(page *Page, rect Rectangle) *UnderlineAnnotation {
	return &UnderlineAnnotation{annotationBase: newMarkupBase("NewUnderlineAnnotation", page, rect, "/Underline")}
}

// StrikeOutAnnotation draws a horizontal line through text.
type StrikeOutAnnotation struct {
	annotationBase
}

func (a *StrikeOutAnnotation) AnnotationType() AnnotationType { return AnnotationTypeStrikeOut }

// QuadPoints returns the array of quads describing the strike-out region.
// Returns nil if /QuadPoints is absent or its array length is not a
// multiple of 8 (malformed).
func (a *StrikeOutAnnotation) QuadPoints() []QuadPoint {
	return readQuadPoints(a.dict["/QuadPoints"])
}

// SetQuadPoints writes /QuadPoints. nil or empty slice removes the entry.
func (a *StrikeOutAnnotation) SetQuadPoints(qp []QuadPoint) {
	if len(qp) == 0 {
		delete(a.dict, "/QuadPoints")
		return
	}
	a.dict["/QuadPoints"] = quadPointsToPDFArray(qp)
}

// NewStrikeOutAnnotation builds an unbound strike-out annotation.
func NewStrikeOutAnnotation(page *Page, rect Rectangle) *StrikeOutAnnotation {
	return &StrikeOutAnnotation{annotationBase: newMarkupBase("NewStrikeOutAnnotation", page, rect, "/StrikeOut")}
}

// SquigglyAnnotation draws a wavy underline under text (typically used
// for spell-check style hints).
type SquigglyAnnotation struct {
	annotationBase
}

func (a *SquigglyAnnotation) AnnotationType() AnnotationType { return AnnotationTypeSquiggly }

// QuadPoints returns the array of quads describing the squiggly region.
// Returns nil if /QuadPoints is absent or its array length is not a
// multiple of 8 (malformed).
func (a *SquigglyAnnotation) QuadPoints() []QuadPoint {
	return readQuadPoints(a.dict["/QuadPoints"])
}

// SetQuadPoints writes /QuadPoints. nil or empty slice removes the entry.
func (a *SquigglyAnnotation) SetQuadPoints(qp []QuadPoint) {
	if len(qp) == 0 {
		delete(a.dict, "/QuadPoints")
		return
	}
	a.dict["/QuadPoints"] = quadPointsToPDFArray(qp)
}

// NewSquigglyAnnotation builds an unbound squiggly-underline annotation.
func NewSquigglyAnnotation(page *Page, rect Rectangle) *SquigglyAnnotation {
	return &SquigglyAnnotation{annotationBase: newMarkupBase("NewSquigglyAnnotation", page, rect, "/Squiggly")}
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
