package asposepdf

// Point is a single point in PDF user-space coordinates.
type Point struct {
	X, Y float64
}

// BorderStyle controls the /BS dict for drawing annotations per
// ISO 32000-1 §12.5.4 Table 168.
type BorderStyle int

const (
	BorderSolid     BorderStyle = iota // /S = /S
	BorderDashed                       // /S = /D + /D dash array
	BorderBeveled                      // /S = /B (3D raised effect)
	BorderInset                        // /S = /I (3D recessed effect)
	BorderUnderline                    // /S = /U (only the bottom edge)
)

// LineEndingStyle is one of the 10 line-ending shapes per ISO 32000-1
// §12.5.6.7 Table 176, used in /Line annotations' /LE entry.
type LineEndingStyle int

const (
	LineEndingNone         LineEndingStyle = iota
	LineEndingSquare
	LineEndingCircle
	LineEndingDiamond
	LineEndingOpenArrow
	LineEndingClosedArrow
	LineEndingButt
	LineEndingROpenArrow   // OpenArrow rotated 180° (away from line)
	LineEndingRClosedArrow // ClosedArrow rotated 180°
	LineEndingSlash
)

// SquareAnnotation draws a rectangular annotation with stroked border
// and optional interior fill. Renders natively from /AP/N — Solid,
// Dashed, Beveled, Inset, and Underline border styles supported.
type SquareAnnotation struct {
	annotationBase
}

func (a *SquareAnnotation) AnnotationType() AnnotationType { return AnnotationTypeSquare }

// NewSquareAnnotation builds an unbound square annotation. Page must be
// non-nil. The annotation is not added to the document until
// page.Annotations().Add(square) succeeds.
func NewSquareAnnotation(page *Page, rect Rectangle) *SquareAnnotation {
	if page == nil {
		panic("NewSquareAnnotation: nil page")
	}
	dict := pdfDict{
		"/Type":    pdfName("/Annot"),
		"/Subtype": pdfName("/Square"),
		"/Rect":    pdfArray{rect.LLX, rect.LLY, rect.URX, rect.URY},
	}
	a := &SquareAnnotation{annotationBase: annotationBase{
		dict: dict,
		doc:  page.doc,
		page: page,
	}}
	a.regenerateAP()
	return a
}

// BorderWidth returns the stroke line width. Reads /BS/W (preferred) or
// /Border[2] (legacy fallback). Defaults to 1 if neither is present.
func (a *SquareAnnotation) BorderWidth() float64 {
	if bs, ok := a.dict["/BS"].(pdfDict); ok {
		if w, err := toFloat(bs["/W"]); err == nil {
			return w
		}
	}
	if border, ok := a.dict["/Border"].(pdfArray); ok && len(border) >= 3 {
		if w, err := toFloat(border[2]); err == nil {
			return w
		}
	}
	return 1
}

// SetBorderWidth writes /BS/W and clears any legacy /Border array.
func (a *SquareAnnotation) SetBorderWidth(w float64) {
	bs, _ := a.dict["/BS"].(pdfDict)
	if bs == nil {
		bs = pdfDict{}
	}
	bs["/W"] = w
	a.dict["/BS"] = bs
	delete(a.dict, "/Border")
	a.regenerateAP()
}

// SetRect overrides annotationBase.SetRect to regenerate /AP/N after
// the rectangle changes (the appearance stream's BBox is derived from
// /Rect).
func (a *SquareAnnotation) SetRect(r Rectangle) {
	a.annotationBase.SetRect(r)
	a.regenerateAP()
}

// SetColor overrides annotationBase.SetColor to regenerate /AP/N after
// the stroke color changes.
func (a *SquareAnnotation) SetColor(c *Color) {
	a.annotationBase.SetColor(c)
	a.regenerateAP()
}

// regenerateAP rebuilds /AP/N from the annotation's current properties.
func (a *SquareAnnotation) regenerateAP() {
	setAppearanceN(&a.annotationBase, generateSquareAppearance(a))
}

// RegenerateAppearance forces /AP/N to be rebuilt from current properties.
// Useful when the underlying dict was mutated directly (bypassing setters).
func (a *SquareAnnotation) RegenerateAppearance() {
	a.regenerateAP()
}

// BorderStyle returns the /BS/S style. Defaults to BorderSolid if absent.
func (a *SquareAnnotation) BorderStyle() BorderStyle {
	bs, _ := a.dict["/BS"].(pdfDict)
	if bs == nil {
		return BorderSolid
	}
	switch n, _ := bs["/S"].(pdfName); n {
	case "/D":
		return BorderDashed
	case "/B":
		return BorderBeveled
	case "/I":
		return BorderInset
	case "/U":
		return BorderUnderline
	}
	return BorderSolid
}

// SetBorderStyle writes /BS/S using the PDF spec name codes.
func (a *SquareAnnotation) SetBorderStyle(s BorderStyle) {
	bs, _ := a.dict["/BS"].(pdfDict)
	if bs == nil {
		bs = pdfDict{}
	}
	bs["/S"] = borderStyleName(s)
	a.dict["/BS"] = bs
	delete(a.dict, "/Border")
	a.regenerateAP()
}

// DashPattern returns a defensive copy of /BS/D (dash array). Returns
// nil if /BS/D is absent or empty.
func (a *SquareAnnotation) DashPattern() []float64 {
	bs, _ := a.dict["/BS"].(pdfDict)
	if bs == nil {
		return nil
	}
	arr, _ := bs["/D"].(pdfArray)
	if len(arr) == 0 {
		return nil
	}
	out := make([]float64, 0, len(arr))
	for _, v := range arr {
		f, _ := toFloat(v)
		out = append(out, f)
	}
	return out
}

// SetDashPattern writes /BS/D. The slice is copied; the caller may
// safely mutate p after this returns.
func (a *SquareAnnotation) SetDashPattern(p []float64) {
	bs, _ := a.dict["/BS"].(pdfDict)
	if bs == nil {
		bs = pdfDict{}
	}
	if len(p) == 0 {
		delete(bs, "/D")
	} else {
		arr := make(pdfArray, 0, len(p))
		for _, v := range p {
			arr = append(arr, v)
		}
		bs["/D"] = arr
	}
	a.dict["/BS"] = bs
	delete(a.dict, "/Border")
	a.regenerateAP()
}

// borderStyleName maps a BorderStyle to its PDF name code per Table 168.
func borderStyleName(s BorderStyle) pdfName {
	switch s {
	case BorderDashed:
		return "/D"
	case BorderBeveled:
		return "/B"
	case BorderInset:
		return "/I"
	case BorderUnderline:
		return "/U"
	}
	return "/S"
}
