// SPDX-License-Identifier: MIT

package asposepdf

import "math"

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

// makeFormXObjectWithResources is a variant of makeFormXObject that
// accepts an explicit /Resources dict. Used by FreeText and Stamp /AP
// generators that reference fonts or image XObjects.
func makeFormXObjectWithResources(content []byte, bbox Rectangle, resources pdfDict) *pdfStream {
	if resources == nil {
		resources = pdfDict{}
	}
	return &pdfStream{
		Dict: pdfDict{
			"/Type":      pdfName("/XObject"),
			"/Subtype":   pdfName("/Form"),
			"/BBox":      pdfArray{bbox.LLX, bbox.LLY, bbox.URX, bbox.URY},
			"/Resources": resources,
		},
		Data:    content,
		Decoded: true,
	}
}

// generateSquareAppearance produces /AP/N for a Square annotation.
// Supports all five border styles: Solid, Dashed, Beveled, Inset, Underline.
// InteriorColor (/IC) is applied as a fill for Solid/Dashed styles and as a
// background rectangle for Beveled/Inset; it is ignored for Underline per
// spec convention.
func generateSquareAppearance(a *SquareAnnotation) *pdfStream {
	rect := a.Rect()
	width := rect.URX - rect.LLX
	height := rect.URY - rect.LLY

	bw := a.BorderWidth()
	style := a.BorderStyle()

	b := newAppearanceBuilder()

	switch style {
	case BorderBeveled, BorderInset:
		// Two-pass color render. Fill first if /IC is set.
		if ic := a.InteriorColor(); ic != nil {
			b.PushState()
			b.SetFillColorRGB(*ic)
			inset := bw
			b.Rect(inset, inset, width-2*bw, height-2*bw)
			b.Fill()
			b.PopState()
		}
		drawBeveledRectBorder(b, width, height, bw, a.Color(), style == BorderInset)

	case BorderUnderline:
		b.PushState()
		b.SetLineWidth(bw)
		if c := a.Color(); c != nil {
			b.SetStrokeColorRGB(*c)
		}
		// Bottom edge only.
		b.MoveTo(0, bw/2)
		b.LineTo(width, bw/2)
		b.Stroke()
		b.PopState()
		// Underline ignores /IC by spec convention.

	default:
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
		hasFill := false
		if ic := a.InteriorColor(); ic != nil {
			b.SetFillColorRGB(*ic)
			hasFill = true
		}
		if hasFill {
			b.FillStroke()
		} else {
			b.Stroke()
		}
		b.PopState()
	}

	return makeFormXObject(b.Bytes(), Rectangle{URX: width, URY: height})
}

// drawBeveledRectBorder emits a two-pass beveled (or inset) border on a
// rectangle of size (width, height). Top + left edges use the light
// color; bottom + right edges use the dark color (inverted for Inset).
func drawBeveledRectBorder(b *appearanceBuilder, width, height, bw float64, baseColor *Color, inverted bool) {
	base := Color{R: 0, G: 0, B: 0, A: 1}
	if baseColor != nil {
		base = *baseColor
	}
	light, dark := beveledColorPair(base, inverted)

	// Light pass: top + left edges as filled trapezoids.
	b.PushState()
	b.SetFillColorRGB(light)
	// Outer top-left corner → outer top-right → inner top-right → inner top-left.
	b.MoveTo(0, height)
	b.LineTo(width, height)
	b.LineTo(width-bw, height-bw)
	b.LineTo(bw, height-bw)
	b.ClosePath()
	b.Fill()
	// Outer top-left → outer bottom-left → inner bottom-left → inner top-left.
	b.MoveTo(0, height)
	b.LineTo(0, 0)
	b.LineTo(bw, bw)
	b.LineTo(bw, height-bw)
	b.ClosePath()
	b.Fill()
	b.PopState()

	// Dark pass: bottom + right edges.
	b.PushState()
	b.SetFillColorRGB(dark)
	// Outer bottom-left → outer bottom-right → inner bottom-right → inner bottom-left.
	b.MoveTo(0, 0)
	b.LineTo(width, 0)
	b.LineTo(width-bw, bw)
	b.LineTo(bw, bw)
	b.ClosePath()
	b.Fill()
	// Outer bottom-right → outer top-right → inner top-right → inner bottom-right.
	b.MoveTo(width, 0)
	b.LineTo(width, height)
	b.LineTo(width-bw, height-bw)
	b.LineTo(width-bw, bw)
	b.ClosePath()
	b.Fill()
	b.PopState()
}

// beveledColorPair returns a (light, dark) color pair for Beveled and
// Inset border rendering. Light = base × 0.5 + white × 0.5; Dark =
// base × 0.5. When inverted is true (Inset style) the pair is swapped.
//
// PDF spec doesn't precisely fix the algorithm; this matches Acrobat
// output for the same input.
func beveledColorPair(base Color, inverted bool) (light, dark Color) {
	light = Color{
		R: base.R*0.5 + 0.5,
		G: base.G*0.5 + 0.5,
		B: base.B*0.5 + 0.5,
		A: 1,
	}
	dark = Color{
		R: base.R * 0.5,
		G: base.G * 0.5,
		B: base.B * 0.5,
		A: 1,
	}
	if inverted {
		return dark, light
	}
	return light, dark
}

// generateCircleAppearance produces /AP/N for a Circle annotation.
// Geometry: an ellipse inscribed in the local bbox. Border styles
// match SquareAnnotation: Solid, Dashed, Beveled, Inset, Underline
// (Underline = lower semicircle only).
func generateCircleAppearance(a *CircleAnnotation) *pdfStream {
	rect := a.Rect()
	width := rect.URX - rect.LLX
	height := rect.URY - rect.LLY

	bw := a.BorderWidth()
	style := a.BorderStyle()

	b := newAppearanceBuilder()

	cx := width / 2
	cy := height / 2
	rx := width/2 - bw/2
	ry := height/2 - bw/2

	switch style {
	case BorderBeveled, BorderInset:
		drawBeveledEllipseBorder(b, cx, cy, rx, ry, bw, a.Color(), style == BorderInset, a.InteriorColor())

	case BorderUnderline:
		// Lower semicircle only: from (cx-rx, cy) clockwise to (cx+rx, cy).
		b.PushState()
		b.SetLineWidth(bw)
		if c := a.Color(); c != nil {
			b.SetStrokeColorRGB(*c)
		}
		// Bottom half ellipse: 2 cubic Beziers.
		dx := rx * kappa
		dy := ry * kappa
		b.MoveTo(cx-rx, cy)
		b.CurveTo(cx-rx, cy-dy, cx-dx, cy-ry, cx, cy-ry)
		b.CurveTo(cx+dx, cy-ry, cx+rx, cy-dy, cx+rx, cy)
		b.Stroke()
		b.PopState()

	default:
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
		hasFill := false
		if ic := a.InteriorColor(); ic != nil {
			b.SetFillColorRGB(*ic)
			hasFill = true
		}
		b.Ellipse(cx, cy, rx, ry)
		if hasFill {
			b.FillStroke()
		} else {
			b.Stroke()
		}
		b.PopState()
	}

	return makeFormXObject(b.Bytes(), Rectangle{URX: width, URY: height})
}

// drawBeveledEllipseBorder emits a two-pass beveled (or inset) border
// on an ellipse. The upper half of the ring is filled with the light
// color and the lower half with the dark color, producing a "pillow"
// effect — a simpler convention than the rect bevel because a circle
// has no top-left vs bottom-right corners. Optional /IC fill is
// rendered first.
func drawBeveledEllipseBorder(b *appearanceBuilder, cx, cy, rx, ry, bw float64, baseColor *Color, inverted bool, fill *Color) {
	if fill != nil {
		b.PushState()
		b.SetFillColorRGB(*fill)
		// Inner ellipse for the fill region.
		innerRx := rx - bw/2
		innerRy := ry - bw/2
		if innerRx > 0 && innerRy > 0 {
			b.Ellipse(cx, cy, innerRx, innerRy)
			b.Fill()
		}
		b.PopState()
	}
	base := Color{R: 0, G: 0, B: 0, A: 1}
	if baseColor != nil {
		base = *baseColor
	}
	light, dark := beveledColorPair(base, inverted)

	dx := rx * kappa
	dy := ry * kappa
	innerRx := rx - bw
	innerRy := ry - bw
	innerDx := innerRx * kappa
	innerDy := innerRy * kappa

	// Light pass: upper half ring.
	b.PushState()
	b.SetFillColorRGB(light)
	// Outer top half (left → top → right).
	b.MoveTo(cx-rx, cy)
	b.CurveTo(cx-rx, cy+dy, cx-dx, cy+ry, cx, cy+ry)
	b.CurveTo(cx+dx, cy+ry, cx+rx, cy+dy, cx+rx, cy)
	// Step in to inner ellipse, retrace top half backwards.
	b.LineTo(cx+innerRx, cy)
	b.CurveTo(cx+innerRx, cy+innerDy, cx+innerDx, cy+innerRy, cx, cy+innerRy)
	b.CurveTo(cx-innerDx, cy+innerRy, cx-innerRx, cy+innerDy, cx-innerRx, cy)
	b.ClosePath()
	b.Fill()
	b.PopState()

	// Dark pass: lower half ring.
	b.PushState()
	b.SetFillColorRGB(dark)
	b.MoveTo(cx-rx, cy)
	b.CurveTo(cx-rx, cy-dy, cx-dx, cy-ry, cx, cy-ry)
	b.CurveTo(cx+dx, cy-ry, cx+rx, cy-dy, cx+rx, cy)
	b.LineTo(cx+innerRx, cy)
	b.CurveTo(cx+innerRx, cy-innerDy, cx+innerDx, cy-innerRy, cx, cy-innerRy)
	b.CurveTo(cx-innerDx, cy-innerRy, cx-innerRx, cy-innerDy, cx-innerRx, cy)
	b.ClosePath()
	b.Fill()
	b.PopState()
}

// generateLineAppearance produces /AP/N for a Line annotation.
// Renders the line stroke and then both line-ending shapes (if set).
func generateLineAppearance(a *LineAnnotation) *pdfStream {
	rect := a.Rect()
	width := rect.URX - rect.LLX
	height := rect.URY - rect.LLY

	start := a.Start()
	end := a.End()
	bw := a.BorderWidth()
	style := a.BorderStyle()

	// Translate page-space endpoints to local /BBox-space.
	sx := start.X - rect.LLX
	sy := start.Y - rect.LLY
	ex := end.X - rect.LLX
	ey := end.Y - rect.LLY

	dx := ex - sx
	dy := ey - sy
	theta := math.Atan2(dy, dx)

	b := newAppearanceBuilder()
	// A single state carries the stroke colour, width and dash so the shaft
	// and the line endings all share them — Acrobat/MuPDF stroke the arrow
	// legs with the same dash as the shaft (38485.pdf). Filled endings
	// (ClosedArrow/Diamond) keep a solid fill; only their outline dashes.
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
	b.MoveTo(sx, sy)
	b.LineTo(ex, ey)
	b.Stroke()

	// Line endings. theta is the line direction (start → end). The end
	// ending uses theta directly (apex at the endpoint, pointing outward
	// along the line); the start ending mirrors with theta+π. They inherit
	// the stroke colour, width and dash set above.
	ic := a.InteriorColor()
	drawLineEnding(b, a.StartLineEnding(), sx, sy, theta+math.Pi, bw, ic)
	drawLineEnding(b, a.EndLineEnding(), ex, ey, theta, bw, ic)
	b.PopState()

	return makeFormXObject(b.Bytes(), Rectangle{URX: width, URY: height})
}

// drawLineEnding renders one line ending shape at (x, y) rotated by
// theta radians. theta is the line direction at the endpoint, pointing
// outward from the line interior — for the end side this is the
// natural direction of travel from start toward end; the start side
// passes theta+π so the shape mirrors that direction. Stroke color is
// inherited from the surrounding state; fill color (used by filled
// shapes Square/Circle/Diamond/ClosedArrow/RClosedArrow) is provided
// explicitly. Ending span = 9 × lineWidth (Acrobat convention).
//
// The ending is emitted inside a q ... Q block with a local cm so that
// shapes are authored in axis-aligned coordinates and rotated via the
// matrix.
func drawLineEnding(b *appearanceBuilder, style LineEndingStyle, x, y, theta, lineWidth float64, fill *Color) {
	if style == LineEndingNone {
		return
	}
	span := 9 * lineWidth
	half := span / 2

	cos := math.Cos(theta)
	sin := math.Sin(theta)

	b.PushState()
	// cm: rotate by theta then translate to (x, y). PDF cm matrix is
	// [a b c d e f] = [cos sin -sin cos x y].
	b.ConcatMatrix(cos, sin, -sin, cos, x, y)

	switch style {
	case LineEndingSquare:
		b.Rect(-half, -half, span, span)
		paintShape(b, fill)
	case LineEndingCircle:
		b.Ellipse(0, 0, half, half)
		paintShape(b, fill)
	case LineEndingDiamond:
		b.MoveTo(half, 0)
		b.LineTo(0, half)
		b.LineTo(-half, 0)
		b.LineTo(0, -half)
		b.ClosePath()
		paintShape(b, fill)
	case LineEndingOpenArrow:
		// Apex (tip) at the endpoint pointing outward (+x = line direction);
		// the two legs sweep BACK toward the line interior (−x), so the arrow
		// points the way the line travels.
		b.MoveTo(-span, half)
		b.LineTo(0, 0)
		b.LineTo(-span, -half)
		b.Stroke()
	case LineEndingClosedArrow:
		// Filled triangle: tip at the endpoint, base corners back toward the
		// line interior at (−span, ±half) so the arrow points outward.
		b.MoveTo(0, 0)
		b.LineTo(-span, half)
		b.LineTo(-span, -half)
		b.ClosePath()
		paintShape(b, fill)
	case LineEndingButt:
		// Short perpendicular segment across the point.
		b.MoveTo(0, half)
		b.LineTo(0, -half)
		b.Stroke()
	case LineEndingROpenArrow:
		// OpenArrow's mirror — legs sweep outward (+x), so the arrow points
		// back toward the line interior.
		b.MoveTo(span, half)
		b.LineTo(0, 0)
		b.LineTo(span, -half)
		b.Stroke()
	case LineEndingRClosedArrow:
		// ClosedArrow rotated 180° — base corners outward at (span, ±half).
		b.MoveTo(0, 0)
		b.LineTo(span, half)
		b.LineTo(span, -half)
		b.ClosePath()
		paintShape(b, fill)
	case LineEndingSlash:
		// Diagonal at 60° (cos 60° = 0.5, sin 60° ≈ 0.866). Length = span.
		dx := half
		dy := half * math.Sqrt(3)
		b.MoveTo(-dx, -dy)
		b.LineTo(dx, dy)
		b.Stroke()
	}

	b.PopState()
}

// paintShape paints the current subpath. With fill: FillStroke (B).
// Without fill: just Stroke (S). Used by line endings that have a
// filled body.
func paintShape(b *appearanceBuilder, fill *Color) {
	if fill != nil {
		b.PushState()
		b.SetFillColorRGB(*fill)
		b.FillStroke()
		b.PopState()
	} else {
		b.Stroke()
	}
}

// catmullRomControlPoints returns the cubic-Bezier control points C1, C2
// for a Catmull-Rom segment from P1 to P2 with neighbors P0 (before P1)
// and P3 (after P2). Tension factor 0.5 (standard Catmull-Rom).
func catmullRomControlPoints(p0, p1, p2, p3 Point) (c1, c2 Point) {
	c1 = Point{
		X: p1.X + (p2.X-p0.X)/6,
		Y: p1.Y + (p2.Y-p0.Y)/6,
	}
	c2 = Point{
		X: p2.X - (p3.X-p1.X)/6,
		Y: p2.Y - (p3.Y-p1.Y)/6,
	}
	return c1, c2
}

// generateInkAppearance produces /AP/N for an Ink annotation. Strokes
// with 3+ points are rendered as Catmull-Rom smoothed cubic Beziers;
// 2-point strokes are rendered as straight lines; shorter strokes are
// skipped.
func generateInkAppearance(a *InkAnnotation) *pdfStream {
	rect := a.Rect()
	width := rect.URX - rect.LLX
	height := rect.URY - rect.LLY

	bw := a.BorderWidth()
	style := a.BorderStyle()
	strokes := a.Strokes()

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

	for _, stroke := range strokes {
		if len(stroke) < 2 {
			continue
		}
		// Translate to local /BBox-space.
		local := make([]Point, len(stroke))
		for i, p := range stroke {
			local[i] = Point{X: p.X - rect.LLX, Y: p.Y - rect.LLY}
		}
		emitInkStroke(b, local)
	}
	b.PopState()

	return makeFormXObject(b.Bytes(), Rectangle{URX: width, URY: height})
}

// emitInkStroke renders one stroke. With 2 points: simple m+l. With 3+
// points: Catmull-Rom smoothing into cubic Beziers. Phantom points at
// the ends are produced by mirroring the first / last segment via
// duplication (P[-1] = P[0], P[n] = P[n-1]).
func emitInkStroke(b *appearanceBuilder, points []Point) {
	n := len(points)
	if n == 0 {
		return
	}
	if n == 1 {
		// A single point produces no visible stroke; skip.
		return
	}
	if n == 2 {
		b.MoveTo(points[0].X, points[0].Y)
		b.LineTo(points[1].X, points[1].Y)
		b.Stroke()
		return
	}
	// 3+ points: Catmull-Rom. Phantom points: P[-1] = P[0], P[n] = P[n-1].
	getPoint := func(i int) Point {
		if i < 0 {
			return points[0]
		}
		if i >= n {
			return points[n-1]
		}
		return points[i]
	}
	b.MoveTo(points[0].X, points[0].Y)
	for i := 0; i < n-1; i++ {
		c1, c2 := catmullRomControlPoints(getPoint(i-1), getPoint(i), getPoint(i+1), getPoint(i+2))
		b.CurveTo(c1.X, c1.Y, c2.X, c2.Y, points[i+1].X, points[i+1].Y)
	}
	b.Stroke()
}

// existingAPNResources returns the /Resources dict from the current /AP/N
// XObject for an annotation, or nil if no such XObject exists yet.
// Used by text-bearing annotation appearance generators to reuse already-
// registered font objects rather than allocating duplicates on each
// regeneration call.
func existingAPNResources(base *annotationBase) pdfDict {
	if base.doc == nil {
		return nil
	}
	apDict, _ := base.dict["/AP"].(pdfDict)
	ref, ok := apDict["/N"].(pdfRef)
	if !ok {
		return nil
	}
	obj, exists := base.doc.objects[ref.Num]
	if !exists {
		return nil
	}
	stream, ok := obj.Value.(*pdfStream)
	if !ok {
		return nil
	}
	res, _ := stream.Dict["/Resources"].(pdfDict)
	return res
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
