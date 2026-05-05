package asposepdf

// QuadPoint is one quadrilateral within a markup annotation's
// /QuadPoints array. Each point names the four corners of one selection
// quad, in PDF default user space coordinates.
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
	return &HighlightAnnotation{annotationBase: newMarkupBase(page, rect, "/Highlight")}
}

// newMarkupBase is the shared constructor body for the four markup
// types. Only /Subtype differs; everything else is identical.
func newMarkupBase(page *Page, rect Rectangle, subtype pdfName) annotationBase {
	if page == nil {
		panic("NewMarkupAnnotation: nil page")
	}
	dict := pdfDict{
		"/Type":    pdfName("/Annot"),
		"/Subtype": subtype,
		"/Rect":    pdfArray{rect.LLX, rect.LLY, rect.URX, rect.URY},
	}
	return annotationBase{dict: dict, doc: page.doc, page: page}
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
