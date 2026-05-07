package asposepdf

import "math"

// generateFreeTextAppearance produces /AP/N for a FreeText annotation.
//
// Order:
//  1. Optional /BG background fill (full rect) — skipped for Typewriter.
//  2. Standard rectangle border — skipped for Typewriter.
//  3. Text rendered inside an inner rect via renderTextInBuilder, using
//     the XObject's own /Resources/Font.
//
// Typewriter intent renders bare text with no background or border and
// zero padding (text fills the full bbox), matching Acrobat behavior.
//
// Callout intent (Task 14) will add leader-line drawing here later.
// Cloudy border (BorderEffect) is wired in Tasks 15-16.
// VAlign in /AP is verified end-to-end in Task 17 (renderTextInBuilder
// already supports VAlign from Task 1).
func generateFreeTextAppearance(a *FreeTextAnnotation) *pdfStream {
	rect := a.Rect()
	width := rect.URX - rect.LLX
	height := rect.URY - rect.LLY
	style := a.TextStyle()
	intent := a.Intent()

	b := newAppearanceBuilder()

	// Reuse existing /Resources from the current /AP/N XObject so that
	// font objects already registered in doc.objects are reused rather
	// than duplicated on each regeneration call.
	resources := existingAPNResources(&a.annotationBase)
	if resources == nil {
		resources = pdfDict{}
	}

	// Typewriter intent: bare text, no background or border per Acrobat behavior.
	skipChrome := intent == FreeTextIntentTypewriter

	// 1. Background fill (skip for typewriter).
	if !skipChrome && style.Background != nil {
		b.PushState()
		b.SetFillColorRGB(*style.Background)
		b.Rect(0, 0, width, height)
		b.Fill()
		b.PopState()
	}

	// 2. Border (skip for typewriter).
	bw := a.BorderWidth()
	if !skipChrome && bw > 0 {
		if a.BorderEffect() == BorderEffectCloudy {
			intensity := a.BorderEffectIntensity()
			if intensity == 0 {
				intensity = 1.0 // default
			}
			drawCloudyRectBorder(b, width, height, bw, a.Color(), intensity)
		} else {
			drawStandardRectBorder(b, width, height, a.BorderStyle(), bw, a.DashPattern(), a.Color())
		}
	}

	// 3. Determine text rendering rect.
	var innerLocal Rectangle
	if intent == FreeTextIntentCallout {
		// Use /RD-derived inner rect, translated to local /BBox space.
		innerPage := a.InnerRect()
		innerLocal = Rectangle{
			LLX: innerPage.LLX - rect.LLX,
			LLY: innerPage.LLY - rect.LLY,
			URX: innerPage.URX - rect.LLX,
			URY: innerPage.URY - rect.LLY,
		}
	} else {
		var pad float64
		if skipChrome {
			pad = 0 // typewriter has no border/padding chrome
		} else {
			pad = bw
			if pad < 2 {
				pad = 2 // at least 2 pt of margin even with 0-width border
			}
		}
		innerLocal = Rectangle{LLX: pad, LLY: pad, URX: width - pad, URY: height - pad}
	}

	// 4. Render text.
	contents := a.Contents()
	if contents != "" {
		// renderTextInBuilder uses style.Color for text color (separate
		// from a.Color() which is the BORDER color).
		// The second arg (pdfDict) to the resolver is ignored by
		// resolveFontForXObject — it writes to the captured `resources`
		// via closure instead.
		resolve := func(font Font, _ pdfDict) (resName string, w widthFn, e encodeFn, asc, desc float64, err error) {
			return resolveFontForXObject(font, style.Size, a.doc, resources)
		}
		// Empty ExtGState names — opaque text/bg.
		_ = renderTextInBuilder(b, resources, contents, style, innerLocal, resolve, "", "")
	}

	// 5. Callout line (only for callout intent).
	if intent == FreeTextIntentCallout {
		ptsPage := a.CalloutPoints()
		if len(ptsPage) >= 2 {
			ptsLocal := make([]Point, len(ptsPage))
			for i, p := range ptsPage {
				ptsLocal[i] = Point{X: p.X - rect.LLX, Y: p.Y - rect.LLY}
			}
			startLocal := nearestInnerEdgeMidpoint(innerLocal, ptsLocal[0])
			drawCalloutLine(b, startLocal, ptsLocal, bw, a.Color(), a.EndLineEnding())
		}
	}

	return makeFormXObjectWithResources(b.Bytes(), Rectangle{URX: width, URY: height}, resources)
}

// nearestInnerEdgeMidpoint returns the midpoint of the inner rect's
// edge nearest to target. Used as the implicit "start" point for a
// callout line, per ISO 32000-1 §12.5.6.6.
func nearestInnerEdgeMidpoint(inner Rectangle, target Point) Point {
	midX := (inner.LLX + inner.URX) / 2
	midY := (inner.LLY + inner.URY) / 2
	candidates := []Point{
		{X: midX, Y: inner.LLY},  // bottom
		{X: inner.URX, Y: midY},  // right
		{X: midX, Y: inner.URY},  // top
		{X: inner.LLX, Y: midY},  // left
	}
	bestIdx := 0
	bestDist := math.Inf(1)
	for i, p := range candidates {
		dx := p.X - target.X
		dy := p.Y - target.Y
		d := dx*dx + dy*dy
		if d < bestDist {
			bestDist = d
			bestIdx = i
		}
	}
	return candidates[bestIdx]
}

// drawStandardRectBorder renders a rectangular border using the given
// BorderStyle. Dispatches:
//   - Solid: simple stroked rect.
//   - Dashed: same with dash pattern.
//   - Beveled / Inset: two-pass color render (uses Subepic 3's
//     drawBeveledRectBorder).
//   - Underline: just the bottom edge.
func drawStandardRectBorder(b *appearanceBuilder, width, height float64, style BorderStyle, lineWidth float64, dashPattern []float64, strokeColor *Color) {
	switch style {
	case BorderBeveled, BorderInset:
		drawBeveledRectBorder(b, width, height, lineWidth, strokeColor, style == BorderInset)
	case BorderUnderline:
		b.PushState()
		b.SetLineWidth(lineWidth)
		if strokeColor != nil {
			b.SetStrokeColorRGB(*strokeColor)
		}
		b.MoveTo(0, lineWidth/2)
		b.LineTo(width, lineWidth/2)
		b.Stroke()
		b.PopState()
	default: // BorderSolid, BorderDashed
		b.PushState()
		b.SetLineWidth(lineWidth)
		if strokeColor != nil {
			b.SetStrokeColorRGB(*strokeColor)
		}
		if style == BorderDashed {
			dp := dashPattern
			if len(dp) == 0 {
				dp = []float64{3, 3}
			}
			b.SetDashPattern(dp, 0)
		}
		inset := lineWidth / 2
		b.Rect(inset, inset, width-lineWidth, height-lineWidth)
		b.Stroke()
		b.PopState()
	}
}

// drawCloudyRectBorder renders an Acrobat-style wavy "cloudy" border
// around a rectangle of size (width, height). Each side is subdivided
// into segments of length ~10×intensity×lineWidth; each segment
// renders as a half-circle bulge protruding outward from the rect.
//
// All 4 sides drawn as one path (via sequential drawCloudySide calls),
// closed and stroked at the end. Color and line width are applied here.
//
// intensity controls the bulge size and segment density (spec range
// ~0.5–2.0; default 1.0). Lower = tighter waves, higher = larger bulges
// with fewer per side.
func drawCloudyRectBorder(b *appearanceBuilder, width, height, lineWidth float64, color *Color, intensity float64) {
	if intensity <= 0 {
		intensity = 1.0
	}
	radius := 5.0 * intensity * lineWidth
	if radius < 2 {
		radius = 2
	}
	bulgeStep := radius * 2.0

	b.PushState()
	b.SetLineWidth(lineWidth)
	if color != nil {
		b.SetStrokeColorRGB(*color)
	}

	// Inset by lineWidth/2 so the stroke stays inside the bbox.
	inset := lineWidth / 2
	x0, y0 := inset, inset
	x1, y1 := width-inset, height-inset

	// Trace 4 sides with bulges going outward.
	// Side 1: bottom (left → right), bulge direction = -y (down)
	drawCloudySide(b, Point{X: x0, Y: y0}, Point{X: x1, Y: y0}, Point{X: 0, Y: -1}, bulgeStep, radius, true)
	// Side 2: right (bottom → top), bulge direction = +x (right)
	drawCloudySide(b, Point{X: x1, Y: y0}, Point{X: x1, Y: y1}, Point{X: 1, Y: 0}, bulgeStep, radius, false)
	// Side 3: top (right → left), bulge direction = +y (up)
	drawCloudySide(b, Point{X: x1, Y: y1}, Point{X: x0, Y: y1}, Point{X: 0, Y: 1}, bulgeStep, radius, false)
	// Side 4: left (top → bottom), bulge direction = -x (left)
	drawCloudySide(b, Point{X: x0, Y: y1}, Point{X: x0, Y: y0}, Point{X: -1, Y: 0}, bulgeStep, radius, false)

	b.ClosePath()
	b.Stroke()
	b.PopState()
}

// drawCloudySide draws one side of the cloudy border from start to end,
// with bulges protruding in the perpDir direction (unit vector).
// Each segment of length bulgeStep renders as a half-circle bulge via
// 2 cubic Beziers (kappa approximation).
//
// isFirst controls whether to emit a MoveTo at the beginning (true for
// the first side; subsequent sides continue the path from the prior side's
// endpoint).
func drawCloudySide(b *appearanceBuilder, start, end, perpDir Point, bulgeStep, radius float64, isFirst bool) {
	dx := end.X - start.X
	dy := end.Y - start.Y
	length := math.Sqrt(dx*dx + dy*dy)
	if length < 1 {
		return
	}
	// Direction unit vector along the side.
	dirX := dx / length
	dirY := dy / length

	// Number of bulges: at least 1, rounded to nearest integer.
	n := int(math.Max(1, math.Round(length/bulgeStep)))
	segLen := length / float64(n)

	// kappa-based control-point offset for quarter-circle approximation.
	kappaR := radius * kappa

	if isFirst {
		b.MoveTo(start.X, start.Y)
	}

	// For each segment, emit a half-circle bulge via 2 cubic Beziers.
	// Bulge: from point A to point B (along side, segLen apart), peak
	// protrudes outward by radius at midpoint.
	for i := 0; i < n; i++ {
		a := Point{
			X: start.X + dirX*float64(i)*segLen,
			Y: start.Y + dirY*float64(i)*segLen,
		}
		c := Point{
			X: start.X + dirX*float64(i+1)*segLen,
			Y: start.Y + dirY*float64(i+1)*segLen,
		}
		// Peak: midpoint of segment displaced outward by radius.
		midSide := Point{
			X: (a.X + c.X) / 2,
			Y: (a.Y + c.Y) / 2,
		}
		peak := Point{
			X: midSide.X + perpDir.X*radius,
			Y: midSide.Y + perpDir.Y*radius,
		}

		// First Bezier: A → peak.
		// c1: from A, offset along dir and perpDir by kappaR.
		// c2: from peak, pulled back along dir by kappaR.
		c1 := Point{
			X: a.X + dirX*kappaR + perpDir.X*kappaR,
			Y: a.Y + dirY*kappaR + perpDir.Y*kappaR,
		}
		c2 := Point{
			X: peak.X - dirX*kappaR,
			Y: peak.Y - dirY*kappaR,
		}
		b.CurveTo(c1.X, c1.Y, c2.X, c2.Y, peak.X, peak.Y)

		// Second Bezier: peak → C.
		// c3: from peak, offset along dir by kappaR.
		// c4: from C, pulled back along dir and offset outward by kappaR.
		c3 := Point{
			X: peak.X + dirX*kappaR,
			Y: peak.Y + dirY*kappaR,
		}
		c4 := Point{
			X: c.X - dirX*kappaR + perpDir.X*kappaR,
			Y: c.Y - dirY*kappaR + perpDir.Y*kappaR,
		}
		b.CurveTo(c3.X, c3.Y, c4.X, c4.Y, c.X, c.Y)
	}
}

// drawCalloutLine renders a FreeText callout connector line: start →
// knee(s) → endpoint, with an optional line ending at the endpoint.
//
// pts must have 2 elements (one knee + endpoint) or 3 elements (two
// knees + endpoint). With fewer than 2, this is a no-op.
//
// All coordinates are in local /BBox space (caller translates from
// page space). The start point is computed by the caller as the
// midpoint of the inner-rect edge nearest to pts[0].
//
// The endpoint is at pts[len(pts)-1]. Theta for the line ending is
// the angle of the last segment (last-knee → endpoint), pointing
// outward (matching Subepic 3 line-ending conventions).
func drawCalloutLine(b *appearanceBuilder, start Point, pts []Point, lineWidth float64, color *Color, ending LineEndingStyle) {
	if len(pts) < 2 {
		return
	}
	b.PushState()
	b.SetLineWidth(lineWidth)
	if color != nil {
		b.SetStrokeColorRGB(*color)
	}
	b.MoveTo(start.X, start.Y)
	for _, p := range pts {
		b.LineTo(p.X, p.Y)
	}
	b.Stroke()
	b.PopState()

	// Line ending at endpoint.
	if ending != LineEndingNone {
		endpoint := pts[len(pts)-1]
		prev := pts[len(pts)-2]
		theta := math.Atan2(endpoint.Y-prev.Y, endpoint.X-prev.X)
		// /IC fill is not applicable here (FreeText callout endings
		// typically use the stroke color for fill); use stroke color
		// when a fill is needed.
		var fill *Color
		if color != nil {
			fc := *color
			fill = &fc
		}
		drawLineEnding(b, ending, endpoint.X, endpoint.Y, theta, lineWidth, fill)
	}
}
