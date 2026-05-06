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

// DashPattern returns a defensive copy of the dash array. Reads /BS/D
// (preferred) or falls back to the legacy /Border[3] entry per ISO
// 32000-1 §12.5.4. Returns nil if neither is present.
func (d *drawingAnnotationBase) DashPattern() []float64 {
	if bs, ok := d.dict["/BS"].(pdfDict); ok {
		if arr, ok := bs["/D"].(pdfArray); ok && len(arr) > 0 {
			out := make([]float64, 0, len(arr))
			for _, v := range arr {
				f, _ := toFloat(v)
				out = append(out, f)
			}
			return out
		}
	}
	if border, ok := d.dict["/Border"].(pdfArray); ok && len(border) >= 4 {
		if arr, ok := border[3].(pdfArray); ok && len(arr) > 0 {
			out := make([]float64, 0, len(arr))
			for _, v := range arr {
				f, _ := toFloat(v)
				out = append(out, f)
			}
			return out
		}
	}
	return nil
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

// CircleAnnotation draws an elliptical annotation. Mirrors
// SquareAnnotation API; only the rendered shape and /Subtype differ.
// Border styles (Solid/Dashed/Beveled/Inset/Underline) and
// BorderWidth/Color/DashPattern are inherited from drawingAnnotationBase.
type CircleAnnotation struct {
	drawingAnnotationBase
}

func (a *CircleAnnotation) AnnotationType() AnnotationType { return AnnotationTypeCircle }

// NewCircleAnnotation builds an unbound circle annotation. Page must be
// non-nil. The ellipse is inscribed in the given rectangle.
func NewCircleAnnotation(page *Page, rect Rectangle) *CircleAnnotation {
	if page == nil {
		panic("NewCircleAnnotation: nil page")
	}
	dict := pdfDict{
		"/Type":    pdfName("/Annot"),
		"/Subtype": pdfName("/Circle"),
		"/Rect":    pdfArray{rect.LLX, rect.LLY, rect.URX, rect.URY},
	}
	a := &CircleAnnotation{drawingAnnotationBase: drawingAnnotationBase{
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
func (a *CircleAnnotation) InteriorColor() *Color {
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
func (a *CircleAnnotation) SetInteriorColor(c *Color) {
	if c == nil {
		delete(a.dict, "/IC")
	} else {
		a.dict["/IC"] = pdfArray{c.R, c.G, c.B}
	}
	a.regenerateAP()
}

// regenerateAP rebuilds /AP/N from the annotation's current properties.
func (a *CircleAnnotation) regenerateAP() {
	setAppearanceN(&a.annotationBase, generateCircleAppearance(a))
}

// RegenerateAppearance forces /AP/N to be rebuilt from current properties.
func (a *CircleAnnotation) RegenerateAppearance() {
	a.regenerateAP()
}

// LineAnnotation draws a straight line between two points, with
// optional line endings on each end (arrows, circles, etc. — added in
// Task 14). The /Rect entry is auto-computed from the endpoints +
// padding for line endings.
type LineAnnotation struct {
	drawingAnnotationBase
}

func (a *LineAnnotation) AnnotationType() AnnotationType { return AnnotationTypeLine }

// NewLineAnnotation builds an unbound line annotation. Page must be
// non-nil. The /Rect is auto-computed as the bounding box of the line
// plus padding equal to 9 × BorderWidth (Acrobat convention) on each
// side.
func NewLineAnnotation(page *Page, start, end Point) *LineAnnotation {
	if page == nil {
		panic("NewLineAnnotation: nil page")
	}
	dict := pdfDict{
		"/Type":    pdfName("/Annot"),
		"/Subtype": pdfName("/Line"),
		"/L":       pdfArray{start.X, start.Y, end.X, end.Y},
	}
	a := &LineAnnotation{drawingAnnotationBase: drawingAnnotationBase{
		annotationBase: annotationBase{
			dict: dict,
			doc:  page.doc,
			page: page,
		},
	}}
	a.regenerate = a.regenerateAP
	a.recomputeRect()
	a.regenerateAP()
	return a
}

// Start returns the line's start point.
func (a *LineAnnotation) Start() Point {
	arr, ok := a.dict["/L"].(pdfArray)
	if !ok || len(arr) < 4 {
		return Point{}
	}
	x, _ := toFloat(arr[0])
	y, _ := toFloat(arr[1])
	return Point{X: x, Y: y}
}

// End returns the line's end point.
func (a *LineAnnotation) End() Point {
	arr, ok := a.dict["/L"].(pdfArray)
	if !ok || len(arr) < 4 {
		return Point{}
	}
	x, _ := toFloat(arr[2])
	y, _ := toFloat(arr[3])
	return Point{X: x, Y: y}
}

// SetStart updates the line's start point and recomputes /Rect.
func (a *LineAnnotation) SetStart(p Point) {
	end := a.End()
	a.dict["/L"] = pdfArray{p.X, p.Y, end.X, end.Y}
	a.recomputeRect()
	a.regenerateAP()
}

// SetEnd updates the line's end point and recomputes /Rect.
func (a *LineAnnotation) SetEnd(p Point) {
	start := a.Start()
	a.dict["/L"] = pdfArray{start.X, start.Y, p.X, p.Y}
	a.recomputeRect()
	a.regenerateAP()
}

// SetBorderWidth overrides drawingAnnotationBase.SetBorderWidth so /Rect
// (which depends on BorderWidth via line-ending padding) is recomputed
// before /AP/N regenerates — ensuring the appearance stream's /BBox
// matches the new bounds in a single pass.
func (a *LineAnnotation) SetBorderWidth(w float64) {
	bs, _ := a.dict["/BS"].(pdfDict)
	if bs == nil {
		bs = pdfDict{}
	}
	bs["/W"] = w
	a.dict["/BS"] = bs
	delete(a.dict, "/Border")
	a.recomputeRect()
	a.regenerateAP()
}

// recomputeRect updates /Rect to the bounding box of the line plus
// padding equal to 9 × BorderWidth (Acrobat convention) on each side.
func (a *LineAnnotation) recomputeRect() {
	start := a.Start()
	end := a.End()
	pad := 9 * a.BorderWidth()
	llx := min(start.X, end.X) - pad
	lly := min(start.Y, end.Y) - pad
	urx := max(start.X, end.X) + pad
	ury := max(start.Y, end.Y) + pad
	a.dict["/Rect"] = pdfArray{llx, lly, urx, ury}
}

// regenerateAP rebuilds /AP/N from the annotation's current properties.
func (a *LineAnnotation) regenerateAP() {
	setAppearanceN(&a.annotationBase, generateLineAppearance(a))
}

// RegenerateAppearance forces /AP/N to be rebuilt from current properties.
func (a *LineAnnotation) RegenerateAppearance() {
	a.regenerateAP()
}

// StartLineEnding returns the style applied to the start of the line.
// Defaults to LineEndingNone if /LE is absent or malformed.
func (a *LineAnnotation) StartLineEnding() LineEndingStyle {
	arr, _ := a.dict["/LE"].(pdfArray)
	if len(arr) < 1 {
		return LineEndingNone
	}
	n, _ := arr[0].(pdfName)
	return parseLineEndingName(n)
}

// EndLineEnding returns the style applied to the end of the line.
func (a *LineAnnotation) EndLineEnding() LineEndingStyle {
	arr, _ := a.dict["/LE"].(pdfArray)
	if len(arr) < 2 {
		return LineEndingNone
	}
	n, _ := arr[1].(pdfName)
	return parseLineEndingName(n)
}

// SetStartLineEnding sets the start-side line-ending style.
func (a *LineAnnotation) SetStartLineEnding(s LineEndingStyle) {
	end := a.EndLineEnding()
	a.dict["/LE"] = pdfArray{lineEndingName(s), lineEndingName(end)}
	a.regenerateAP()
}

// SetEndLineEnding sets the end-side line-ending style.
func (a *LineAnnotation) SetEndLineEnding(s LineEndingStyle) {
	start := a.StartLineEnding()
	a.dict["/LE"] = pdfArray{lineEndingName(start), lineEndingName(s)}
	a.regenerateAP()
}

// InteriorColor returns the /IC fill color (used for filled line
// endings: ClosedArrow, RClosedArrow, Square, Circle, Diamond).
// Returns nil if absent.
func (a *LineAnnotation) InteriorColor() *Color {
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
func (a *LineAnnotation) SetInteriorColor(c *Color) {
	if c == nil {
		delete(a.dict, "/IC")
	} else {
		a.dict["/IC"] = pdfArray{c.R, c.G, c.B}
	}
	a.regenerateAP()
}

// LeaderLineLength returns the /LL value (0 if absent). Used for
// dimension-line annotations where the line is offset from the
// measured points by this much.
func (a *LineAnnotation) LeaderLineLength() float64 {
	v, err := toFloat(a.dict["/LL"])
	if err != nil {
		return 0
	}
	return v
}

// SetLeaderLineLength writes /LL. Zero removes the entry.
func (a *LineAnnotation) SetLeaderLineLength(l float64) {
	if l == 0 {
		delete(a.dict, "/LL")
	} else {
		a.dict["/LL"] = l
	}
	a.regenerateAP()
}

// lineEndingName maps a LineEndingStyle to its PDF spec name per Table 176.
func lineEndingName(s LineEndingStyle) pdfName {
	switch s {
	case LineEndingSquare:
		return "/Square"
	case LineEndingCircle:
		return "/Circle"
	case LineEndingDiamond:
		return "/Diamond"
	case LineEndingOpenArrow:
		return "/OpenArrow"
	case LineEndingClosedArrow:
		return "/ClosedArrow"
	case LineEndingButt:
		return "/Butt"
	case LineEndingROpenArrow:
		return "/ROpenArrow"
	case LineEndingRClosedArrow:
		return "/RClosedArrow"
	case LineEndingSlash:
		return "/Slash"
	}
	return "/None"
}

// parseLineEndingName reverses lineEndingName.
func parseLineEndingName(n pdfName) LineEndingStyle {
	switch n {
	case "/Square":
		return LineEndingSquare
	case "/Circle":
		return LineEndingCircle
	case "/Diamond":
		return LineEndingDiamond
	case "/OpenArrow":
		return LineEndingOpenArrow
	case "/ClosedArrow":
		return LineEndingClosedArrow
	case "/Butt":
		return LineEndingButt
	case "/ROpenArrow":
		return LineEndingROpenArrow
	case "/RClosedArrow":
		return LineEndingRClosedArrow
	case "/Slash":
		return LineEndingSlash
	}
	return LineEndingNone
}

// InkAnnotation draws a series of free-form strokes — typically used to
// represent handwritten ink. Each stroke is a sequence of points
// rendered as a polyline; in Task 16 strokes with 3+ points get
// Catmull-Rom smoothing for visual quality.
type InkAnnotation struct {
	drawingAnnotationBase
}

func (a *InkAnnotation) AnnotationType() AnnotationType { return AnnotationTypeInk }

// NewInkAnnotation builds an unbound ink annotation. Page must be
// non-nil. Each inner slice is one continuous stroke. The /Rect entry
// is auto-computed as the bounding box of all stroke points plus
// padding equal to BorderWidth.
func NewInkAnnotation(page *Page, strokes [][]Point) *InkAnnotation {
	if page == nil {
		panic("NewInkAnnotation: nil page")
	}
	a := &InkAnnotation{drawingAnnotationBase: drawingAnnotationBase{
		annotationBase: annotationBase{
			dict: pdfDict{
				"/Type":    pdfName("/Annot"),
				"/Subtype": pdfName("/Ink"),
			},
			doc:  page.doc,
			page: page,
		},
	}}
	a.regenerate = a.regenerateAP
	a.SetStrokes(strokes)
	return a
}

// Strokes returns a deep copy of /InkList. Mutating the result does
// not affect the annotation.
func (a *InkAnnotation) Strokes() [][]Point {
	arr, _ := a.dict["/InkList"].(pdfArray)
	out := make([][]Point, 0, len(arr))
	for _, strokeAny := range arr {
		strokeArr, _ := strokeAny.(pdfArray)
		stroke := make([]Point, 0, len(strokeArr)/2)
		for i := 0; i+1 < len(strokeArr); i += 2 {
			x, _ := toFloat(strokeArr[i])
			y, _ := toFloat(strokeArr[i+1])
			stroke = append(stroke, Point{X: x, Y: y})
		}
		out = append(out, stroke)
	}
	return out
}

// SetStrokes writes /InkList. The slices are deep-copied; the caller
// may safely mutate after this returns.
func (a *InkAnnotation) SetStrokes(strokes [][]Point) {
	if len(strokes) == 0 {
		delete(a.dict, "/InkList")
	} else {
		outer := make(pdfArray, 0, len(strokes))
		for _, stroke := range strokes {
			inner := make(pdfArray, 0, len(stroke)*2)
			for _, p := range stroke {
				inner = append(inner, p.X, p.Y)
			}
			outer = append(outer, inner)
		}
		a.dict["/InkList"] = outer
	}
	a.recomputeRect()
	a.regenerateAP()
}

// AddStroke appends a single stroke to the annotation. Convenience for
// incremental construction.
func (a *InkAnnotation) AddStroke(stroke []Point) {
	current := a.Strokes()
	current = append(current, stroke)
	a.SetStrokes(current)
}

// SetBorderWidth overrides drawingAnnotationBase.SetBorderWidth so /Rect
// (whose padding scales with BorderWidth) is recomputed before /AP/N
// regenerates — single-pass /BBox sync.
func (a *InkAnnotation) SetBorderWidth(w float64) {
	bs, _ := a.dict["/BS"].(pdfDict)
	if bs == nil {
		bs = pdfDict{}
	}
	bs["/W"] = w
	a.dict["/BS"] = bs
	delete(a.dict, "/Border")
	a.recomputeRect()
	a.regenerateAP()
}

// recomputeRect updates /Rect to the bounding box of all points across
// all strokes plus padding equal to the stroke width.
func (a *InkAnnotation) recomputeRect() {
	strokes := a.Strokes()
	if len(strokes) == 0 {
		a.dict["/Rect"] = pdfArray{0.0, 0.0, 0.0, 0.0}
		return
	}
	first := true
	var llx, lly, urx, ury float64
	for _, stroke := range strokes {
		for _, p := range stroke {
			if first {
				llx, lly, urx, ury = p.X, p.Y, p.X, p.Y
				first = false
				continue
			}
			llx = min(llx, p.X)
			lly = min(lly, p.Y)
			urx = max(urx, p.X)
			ury = max(ury, p.Y)
		}
	}
	pad := a.BorderWidth()
	a.dict["/Rect"] = pdfArray{llx - pad, lly - pad, urx + pad, ury + pad}
}

// regenerateAP rebuilds /AP/N from the annotation's current properties.
func (a *InkAnnotation) regenerateAP() {
	setAppearanceN(&a.annotationBase, generateInkAppearance(a))
}

// RegenerateAppearance forces /AP/N to be rebuilt from current properties.
func (a *InkAnnotation) RegenerateAppearance() {
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
