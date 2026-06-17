// SPDX-License-Identifier: MIT

package asposepdf

// polyAnnotationBase is the shared embedded base for the two
// vertex-based annotation types, Polygon and PolyLine. It provides the
// /Vertices accessor, /IC interior colour, /Rect recomputation from the
// vertices, and the BorderWidth override that keeps /Rect in sync — all
// identical across the two types. The concrete type sets the regenerate
// closure in its constructor so /AP/N stays current after any mutation.
//
// Mirrors the shape shared by Aspose.PDF for .NET's PolygonAnnotation and
// PolylineAnnotation (both expose Vertices + InteriorColor).
type polyAnnotationBase struct {
	drawingAnnotationBase
}

// Vertices returns a copy of the annotation's vertices in PDF user space.
// Mutating the result does not affect the annotation.
func (a *polyAnnotationBase) Vertices() []Point {
	arr, _ := a.dict["/Vertices"].(pdfArray)
	out := make([]Point, 0, len(arr)/2)
	for i := 0; i+1 < len(arr); i += 2 {
		x, _ := toFloat(arr[i])
		y, _ := toFloat(arr[i+1])
		out = append(out, Point{X: x, Y: y})
	}
	return out
}

// SetVertices writes /Vertices (a flat x y x y … array), recomputes
// /Rect, and regenerates /AP/N. The slice is copied; the caller may
// safely mutate it after this returns.
func (a *polyAnnotationBase) SetVertices(v []Point) {
	if len(v) == 0 {
		delete(a.dict, "/Vertices")
	} else {
		arr := make(pdfArray, 0, len(v)*2)
		for _, p := range v {
			arr = append(arr, p.X, p.Y)
		}
		a.dict["/Vertices"] = arr
	}
	a.recomputeRect()
	if a.regenerate != nil {
		a.regenerate()
	}
}

// InteriorColor returns the /IC fill colour, or nil if absent. For a
// Polygon this fills the closed shape; for a PolyLine it fills any
// closed line endings (ClosedArrow/Square/Circle/Diamond).
func (a *polyAnnotationBase) InteriorColor() *Color {
	arr, _ := a.dict["/IC"].(pdfArray)
	return annotColorFromComponents(arr)
}

// SetInteriorColor writes /IC as an RGB array; nil removes the entry.
func (a *polyAnnotationBase) SetInteriorColor(c *Color) {
	if c == nil {
		delete(a.dict, "/IC")
	} else {
		a.dict["/IC"] = pdfArray{c.R, c.G, c.B}
	}
	if a.regenerate != nil {
		a.regenerate()
	}
}

// SetBorderWidth overrides drawingAnnotationBase.SetBorderWidth so /Rect
// (whose padding scales with BorderWidth) is recomputed before /AP/N
// regenerates — single-pass /BBox sync, matching Line/Ink.
func (a *polyAnnotationBase) SetBorderWidth(w float64) {
	bs, _ := a.dict["/BS"].(pdfDict)
	if bs == nil {
		bs = pdfDict{}
	}
	bs["/W"] = w
	a.dict["/BS"] = bs
	delete(a.dict, "/Border")
	a.recomputeRect()
	if a.regenerate != nil {
		a.regenerate()
	}
}

// recomputeRect updates /Rect to the bounding box of all vertices plus
// padding equal to 9 × BorderWidth (Acrobat convention — leaves room for
// line endings on a PolyLine).
func (a *polyAnnotationBase) recomputeRect() {
	verts := a.Vertices()
	if len(verts) == 0 {
		a.dict["/Rect"] = pdfArray{0.0, 0.0, 0.0, 0.0}
		return
	}
	llx, lly, urx, ury := verts[0].X, verts[0].Y, verts[0].X, verts[0].Y
	for _, p := range verts[1:] {
		llx = min(llx, p.X)
		lly = min(lly, p.Y)
		urx = max(urx, p.X)
		ury = max(ury, p.Y)
	}
	pad := 9 * a.BorderWidth()
	a.dict["/Rect"] = pdfArray{llx - pad, lly - pad, urx + pad, ury + pad}
}

// PolygonAnnotation draws a closed polygon through a list of vertices,
// with a stroked border and optional interior fill. Mirrors
// Aspose.PDF for .NET's PolygonAnnotation.
type PolygonAnnotation struct {
	polyAnnotationBase
}

func (a *PolygonAnnotation) AnnotationType() AnnotationType { return AnnotationTypePolygon }

// NewPolygonAnnotation builds an unbound polygon annotation through the
// given vertices (PDF user space). Page must be non-nil. The /Rect is
// auto-computed from the vertices; the annotation is not added to the
// document until page.Annotations().Add(poly) succeeds.
func NewPolygonAnnotation(page *Page, vertices []Point) *PolygonAnnotation {
	if page == nil {
		panic("NewPolygonAnnotation: nil page")
	}
	a := &PolygonAnnotation{polyAnnotationBase: polyAnnotationBase{drawingAnnotationBase: drawingAnnotationBase{
		annotationBase: annotationBase{
			dict: pdfDict{
				"/Type":    pdfName("/Annot"),
				"/Subtype": pdfName("/Polygon"),
			},
			doc:  page.doc,
			page: page,
		},
	}}}
	a.regenerate = a.regenerateAP
	a.SetVertices(vertices)
	return a
}

func (a *PolygonAnnotation) regenerateAP() {
	setAppearanceN(&a.annotationBase, generatePolygonAppearance(a))
}

// RegenerateAppearance forces /AP/N to be rebuilt from current properties.
func (a *PolygonAnnotation) RegenerateAppearance() { a.regenerateAP() }

// PolylineAnnotation draws an open polyline through a list of vertices,
// with a stroked border, optional line endings at the first and last
// vertex, and optional interior fill for closed endings. Mirrors
// Aspose.PDF for .NET's PolylineAnnotation.
type PolylineAnnotation struct {
	polyAnnotationBase
}

func (a *PolylineAnnotation) AnnotationType() AnnotationType { return AnnotationTypePolyLine }

// NewPolylineAnnotation builds an unbound polyline annotation through the
// given vertices (PDF user space). Page must be non-nil. The /Rect is
// auto-computed from the vertices.
func NewPolylineAnnotation(page *Page, vertices []Point) *PolylineAnnotation {
	if page == nil {
		panic("NewPolylineAnnotation: nil page")
	}
	a := &PolylineAnnotation{polyAnnotationBase: polyAnnotationBase{drawingAnnotationBase: drawingAnnotationBase{
		annotationBase: annotationBase{
			dict: pdfDict{
				"/Type":    pdfName("/Annot"),
				"/Subtype": pdfName("/PolyLine"),
			},
			doc:  page.doc,
			page: page,
		},
	}}}
	a.regenerate = a.regenerateAP
	a.SetVertices(vertices)
	return a
}

// StartLineEnding returns the style applied to the first vertex.
func (a *PolylineAnnotation) StartLineEnding() LineEndingStyle {
	arr, _ := a.dict["/LE"].(pdfArray)
	if len(arr) < 1 {
		return LineEndingNone
	}
	n, _ := arr[0].(pdfName)
	return parseLineEndingName(n)
}

// EndLineEnding returns the style applied to the last vertex.
func (a *PolylineAnnotation) EndLineEnding() LineEndingStyle {
	arr, _ := a.dict["/LE"].(pdfArray)
	if len(arr) < 2 {
		return LineEndingNone
	}
	n, _ := arr[1].(pdfName)
	return parseLineEndingName(n)
}

// SetStartLineEnding sets the start-side (first vertex) line-ending style.
func (a *PolylineAnnotation) SetStartLineEnding(s LineEndingStyle) {
	a.dict["/LE"] = pdfArray{lineEndingName(s), lineEndingName(a.EndLineEnding())}
	a.regenerateAP()
}

// SetEndLineEnding sets the end-side (last vertex) line-ending style.
func (a *PolylineAnnotation) SetEndLineEnding(s LineEndingStyle) {
	a.dict["/LE"] = pdfArray{lineEndingName(a.StartLineEnding()), lineEndingName(s)}
	a.regenerateAP()
}

func (a *PolylineAnnotation) regenerateAP() {
	setAppearanceN(&a.annotationBase, generatePolylineAppearance(a))
}

// RegenerateAppearance forces /AP/N to be rebuilt from current properties.
func (a *PolylineAnnotation) RegenerateAppearance() { a.regenerateAP() }
