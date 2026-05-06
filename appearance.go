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
