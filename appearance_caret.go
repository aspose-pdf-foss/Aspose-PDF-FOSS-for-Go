// SPDX-License-Identifier: MIT

package asposepdf

// generateCaretAppearance produces /AP/N for a Caret annotation: an
// upward chevron ("^") filling the /Rect, painted with the annotation
// colour (/C, default black). The shape is a simple non-self-intersecting
// quadrilateral — outer apex at top centre, inner notch rising from the
// bottom centre — so it reads as a caret insertion marker in any viewer.
func generateCaretAppearance(a *CaretAnnotation) *pdfStream {
	rect := a.Rect()
	width := rect.URX - rect.LLX
	height := rect.URY - rect.LLY

	b := newAppearanceBuilder()
	if width > 0 && height > 0 {
		fill := Color{A: 1} // default black
		if c := a.Color(); c != nil {
			fill = *c
		}
		b.PushState()
		b.SetFillColorRGB(fill)
		b.MoveTo(0, 0)                // bottom-left
		b.LineTo(width/2, height)     // apex (top centre)
		b.LineTo(width, 0)            // bottom-right
		b.LineTo(width/2, height*0.4) // inner notch (rising from bottom centre)
		b.ClosePath()
		b.Fill()
		b.PopState()
	}

	return makeFormXObject(b.Bytes(), Rectangle{URX: width, URY: height})
}
