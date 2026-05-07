package asposepdf

// generateFreeTextAppearance produces /AP/N for a FreeText annotation.
// This task is the skeleton — produces an empty Form XObject. Full
// rendering with background, border, and text is added across Tasks
// 11-17.
func generateFreeTextAppearance(a *FreeTextAnnotation) *pdfStream {
	rect := a.Rect()
	width := rect.URX - rect.LLX
	height := rect.URY - rect.LLY

	b := newAppearanceBuilder()
	// Empty /AP/N for skeleton.
	return makeFormXObjectWithResources(b.Bytes(), Rectangle{URX: width, URY: height}, pdfDict{})
}
