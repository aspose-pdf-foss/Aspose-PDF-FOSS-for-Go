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

// drawingAnnotationBase is the shared embedded base for the four
// geometric drawing annotation types (Square/Circle/Line/Ink). It
// provides the BorderStyle/DashPattern/BorderWidth accessors and the
// regen-aware SetRect/SetColor overrides — all of which are identical
// across the four drawing types.
//
// Concrete types embed drawingAnnotationBase and set the regenerate
// field in their constructor to a closure that calls the type-specific
// generator (e.g. setAppearanceN(&a.annotationBase, generateSquareAppearance(a))).
// Setters on this base call regenerate() after mutating the dict, so
// /AP/N stays in sync without per-type accessor duplication.
type drawingAnnotationBase struct {
	annotationBase
	regenerate func()
}

// BorderWidth returns the stroke line width. Reads /BS/W (preferred) or
// /Border[2] (legacy fallback). Defaults to 1 if neither is present.
func (d *drawingAnnotationBase) BorderWidth() float64 {
	if bs, ok := d.dict["/BS"].(pdfDict); ok {
		if w, err := toFloat(bs["/W"]); err == nil {
			return w
		}
	}
	if border, ok := d.dict["/Border"].(pdfArray); ok && len(border) >= 3 {
		if w, err := toFloat(border[2]); err == nil {
			return w
		}
	}
	return 1
}

// SetBorderWidth writes /BS/W and clears any legacy /Border array.
func (d *drawingAnnotationBase) SetBorderWidth(w float64) {
	bs, _ := d.dict["/BS"].(pdfDict)
	if bs == nil {
		bs = pdfDict{}
	}
	bs["/W"] = w
	d.dict["/BS"] = bs
	delete(d.dict, "/Border")
	if d.regenerate != nil {
		d.regenerate()
	}
}

// BorderStyle returns the /BS/S style. Defaults to BorderSolid if absent.
func (d *drawingAnnotationBase) BorderStyle() BorderStyle {
	bs, _ := d.dict["/BS"].(pdfDict)
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
func (d *drawingAnnotationBase) SetBorderStyle(s BorderStyle) {
	bs, _ := d.dict["/BS"].(pdfDict)
	if bs == nil {
		bs = pdfDict{}
	}
	bs["/S"] = borderStyleName(s)
	d.dict["/BS"] = bs
	delete(d.dict, "/Border")
	if d.regenerate != nil {
		d.regenerate()
	}
}

// DashPattern returns a defensive copy of /BS/D (dash array). Returns
// nil if /BS/D is absent or empty.
func (d *drawingAnnotationBase) DashPattern() []float64 {
	bs, _ := d.dict["/BS"].(pdfDict)
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
func (d *drawingAnnotationBase) SetDashPattern(p []float64) {
	bs, _ := d.dict["/BS"].(pdfDict)
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
	d.dict["/BS"] = bs
	delete(d.dict, "/Border")
	if d.regenerate != nil {
		d.regenerate()
	}
}

// SetRect overrides annotationBase.SetRect to regenerate /AP/N after
// the rectangle changes (the appearance stream's BBox is derived from
// /Rect).
func (d *drawingAnnotationBase) SetRect(r Rectangle) {
	d.annotationBase.SetRect(r)
	if d.regenerate != nil {
		d.regenerate()
	}
}

// SetColor overrides annotationBase.SetColor to regenerate /AP/N after
// the stroke color changes.
func (d *drawingAnnotationBase) SetColor(c *Color) {
	d.annotationBase.SetColor(c)
	if d.regenerate != nil {
		d.regenerate()
	}
}

// SquareAnnotation draws a rectangular annotation with stroked border
// and optional interior fill. Renders natively from /AP/N — Solid,
// Dashed, Beveled, Inset, and Underline border styles supported.
type SquareAnnotation struct {
	drawingAnnotationBase
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
	a := &SquareAnnotation{drawingAnnotationBase: drawingAnnotationBase{
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

// InteriorColor returns the /IC fill color, or nil if absent.
func (a *SquareAnnotation) InteriorColor() *Color {
	arr, ok := a.dict["/IC"].(pdfArray)
	if !ok || len(arr) != 3 {
		return nil
	}
	r, _ := toFloat(arr[0])
	g, _ := toFloat(arr[1])
	bl, _ := toFloat(arr[2])
	return &Color{R: r, G: g, B: bl, A: 1}
}

// SetInteriorColor writes /IC as an RGB array; nil removes the entry.
func (a *SquareAnnotation) SetInteriorColor(c *Color) {
	if c == nil {
		delete(a.dict, "/IC")
	} else {
		a.dict["/IC"] = pdfArray{c.R, c.G, c.B}
	}
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
