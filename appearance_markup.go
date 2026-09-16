// SPDX-License-Identifier: MIT

package asposepdf

// Appearance streams for the four text-markup annotations — Highlight,
// Underline, StrikeOut and Squiggly (pdf-go-c4c4). A conforming viewer may
// synthesize one of these from /QuadPoints on the fly, and Acrobat does, but
// an /AP-only consumer paints nothing without a stream — this library's own
// renderer among them, and with it the HTML and SVG exporters. So the four
// types generate their appearance the way every other self-drawing family
// does, and the renderer synthesizes one for an /AP-less annotation that came
// from somewhere else (synthesizeAnnotationAppearance).
//
// The marked region comes from /QuadPoints, one quad per line of marked text,
// falling back to the whole /Rect when the array is absent or malformed.

// markupQuadRects returns the annotation's quads as /BBox-local rectangles.
func markupQuadRects(base *annotationBase) []Rectangle {
	rect := base.Rect()
	quads := readQuadPoints(base.dict["/QuadPoints"])
	if len(quads) == 0 {
		quads = []QuadPoint{rectAsQuadPoint(rect)}
	}
	out := make([]Rectangle, 0, len(quads))
	for _, q := range quads {
		r := localizeQuadAsRect(q, rect)
		if r.URX > r.LLX && r.URY > r.LLY {
			out = append(out, r)
		}
	}
	return out
}

// markupFormBBox is the appearance form's box: the annotation rectangle moved
// to the origin, the space markupQuadRects reports its rectangles in.
func markupFormBBox(base *annotationBase) Rectangle {
	r := base.Rect()
	return Rectangle{URX: r.URX - r.LLX, URY: r.URY - r.LLY}
}

// markupColor reads /C, falling back to def when the annotation carries none.
func markupColor(base *annotationBase, def Color) Color {
	if c := base.Color(); c != nil {
		return *c
	}
	return def
}

// markupLineWidth scales a rule to the height of the text it marks, within
// the range a reader expects of an underline or a strike-out.
func markupLineWidth(h float64) float64 {
	w := h * 0.07
	if w < 0.75 {
		w = 0.75
	}
	if w > 2 {
		w = 2
	}
	return w
}

// generateHighlightAppearance washes each marked quad in the annotation
// colour. The wash is Multiply-blended so the glyphs stay readable through it
// — what a highlighter pen does, and what viewers synthesize for /Highlight.
func generateHighlightAppearance(a *HighlightAnnotation) *pdfStream {
	base := &a.annotationBase
	rects := markupQuadRects(base)
	colour := markupColor(base, Color{R: 1, G: 1, B: 0, A: 1})

	b := newAppearanceBuilder()
	if len(rects) > 0 {
		b.SetFillColorRGB(colour)
		for _, r := range rects {
			b.Rect(r.LLX, r.LLY, r.URX-r.LLX, r.URY-r.LLY)
		}
		b.Fill()
	}
	content := append([]byte("/GSMul gs\n"), b.Bytes()...)
	resources := pdfDict{
		"/ExtGState": pdfDict{
			"/GSMul": pdfDict{
				"/Type": pdfName("/ExtGState"),
				"/BM":   pdfName("/Multiply"),
				"/ca":   1.0,
			},
		},
	}
	return makeFormXObjectWithResources(content, markupFormBBox(base), resources)
}

// generateUnderlineAppearance rules a line along the bottom of each quad.
func generateUnderlineAppearance(a *UnderlineAnnotation) *pdfStream {
	return markupRuleAppearance(&a.annotationBase, func(r Rectangle) float64 {
		return r.LLY + (r.URY-r.LLY)*0.08
	})
}

// generateStrikeOutAppearance rules a line through the middle of each quad.
func generateStrikeOutAppearance(a *StrikeOutAnnotation) *pdfStream {
	return markupRuleAppearance(&a.annotationBase, func(r Rectangle) float64 {
		return (r.LLY + r.URY) / 2
	})
}

// markupRuleAppearance strokes one horizontal rule per quad; y picks the
// rule's height within the quad, which is all that separates an underline
// from a strike-out.
func markupRuleAppearance(base *annotationBase, y func(Rectangle) float64) *pdfStream {
	rects := markupQuadRects(base)
	colour := markupColor(base, Color{A: 1})

	b := newAppearanceBuilder()
	if len(rects) > 0 {
		b.SetStrokeColorRGB(colour)
		b.SetLineWidth(markupLineWidth(rects[0].URY - rects[0].LLY))
		for _, r := range rects {
			ly := y(r)
			b.MoveTo(r.LLX, ly)
			b.LineTo(r.URX, ly)
		}
		b.Stroke()
	}
	return makeFormXObjectWithResources(b.Bytes(), markupFormBBox(base), pdfDict{})
}

// generateSquigglyAppearance draws the wavy underline spell-checkers use: a
// zigzag along the bottom of each quad, its amplitude scaled to the text.
func generateSquigglyAppearance(a *SquigglyAnnotation) *pdfStream {
	base := &a.annotationBase
	rects := markupQuadRects(base)
	colour := markupColor(base, Color{A: 1})

	b := newAppearanceBuilder()
	if len(rects) > 0 {
		b.SetStrokeColorRGB(colour)
		b.SetLineWidth(markupLineWidth(rects[0].URY - rects[0].LLY))
		for _, r := range rects {
			squigglyPath(b, r)
		}
		b.Stroke()
	}
	return makeFormXObjectWithResources(b.Bytes(), markupFormBBox(base), pdfDict{})
}

// squigglyPath traces the zigzag for one quad, starting at its left edge and
// stopping exactly at the right one so the wave never overruns the mark.
func squigglyPath(b *appearanceBuilder, r Rectangle) {
	amp := (r.URY - r.LLY) * 0.12
	if amp < 1 {
		amp = 1
	}
	if amp > 3 {
		amp = 3
	}
	base := r.LLY + amp
	step := amp * 2

	x := r.LLX
	b.MoveTo(x, base)
	for up := true; x < r.URX; up = !up {
		x += step
		if x > r.URX {
			x = r.URX
		}
		if up {
			b.LineTo(x, base+amp)
		} else {
			b.LineTo(x, base-amp)
		}
	}
}
