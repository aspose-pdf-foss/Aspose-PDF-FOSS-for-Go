package asposepdf

// generateRedactAppearance produces /AP/N for mark-mode display.
// Stub for now — full visual (quad fills + overlay text preview) in
// Task 7.
func generateRedactAppearance(a *RedactAnnotation) *pdfStream {
	rect := a.Rect()
	width := rect.URX - rect.LLX
	height := rect.URY - rect.LLY
	b := newAppearanceBuilder()
	return makeFormXObjectWithResources(b.Bytes(), Rectangle{URX: width, URY: height}, pdfDict{})
}
