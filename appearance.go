package asposepdf

// makeFormXObject builds a Form XObject stream wrapping the given content
// bytes and bbox. The returned stream is ready for storage in
// doc.objects and reference from /AP/N.
//
// /Resources is empty — drawing annotations (Square/Circle/Line/Ink)
// don't use fonts or images. Future subepics (FreeText, Stamp) will
// extend this helper or supply their own.
func makeFormXObject(content []byte, bbox Rectangle) *pdfStream {
	return &pdfStream{
		Dict: pdfDict{
			"/Type":      pdfName("/XObject"),
			"/Subtype":   pdfName("/Form"),
			"/BBox":      pdfArray{bbox.LLX, bbox.LLY, bbox.URX, bbox.URY},
			"/Resources": pdfDict{},
		},
		Data:    content,
		Decoded: true,
	}
}

// generateSquareAppearance produces /AP/N for a Square annotation.
// Supports Solid and Dashed border styles; Beveled/Inset/Underline
// are added in subsequent tasks.
func generateSquareAppearance(a *SquareAnnotation) *pdfStream {
	rect := a.Rect()
	width := rect.URX - rect.LLX
	height := rect.URY - rect.LLY

	bw := a.BorderWidth()
	style := a.BorderStyle()

	b := newAppearanceBuilder()
	b.PushState()
	b.SetLineWidth(bw)
	if c := a.Color(); c != nil {
		b.SetStrokeColorRGB(*c)
	}
	if style == BorderDashed {
		dp := a.DashPattern()
		if len(dp) == 0 {
			dp = []float64{3, 3}
		}
		b.SetDashPattern(dp, 0)
	}
	inset := bw / 2
	b.Rect(inset, inset, width-bw, height-bw)
	b.Stroke()
	b.PopState()

	return makeFormXObject(b.Bytes(), Rectangle{URX: width, URY: height})
}

// setAppearanceN replaces /AP/N on the annotation. If /AP/N already
// references an XObject in doc.objects, that object is mutated in place
// (no new objID allocated, no orphans). Otherwise a fresh XObject is
// allocated and /AP/N updated to reference it.
//
// No-op when base.doc is nil (annotation not yet doc-linked — should
// not normally happen because constructors set base.doc immediately).
func setAppearanceN(base *annotationBase, stream *pdfStream) {
	if base.doc == nil {
		return
	}
	apDict, _ := base.dict["/AP"].(pdfDict)
	if ref, ok := apDict["/N"].(pdfRef); ok {
		if obj, exists := base.doc.objects[ref.Num]; exists {
			obj.Value = stream
			return
		}
	}
	objID := base.doc.nextID
	base.doc.nextID++
	base.doc.objects[objID] = &pdfObject{Num: objID, Value: stream}
	if apDict == nil {
		apDict = pdfDict{}
	}
	apDict["/N"] = pdfRef{Num: objID}
	base.dict["/AP"] = apDict
}
