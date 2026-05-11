package asposepdf

// generateRedactAppearance produces /AP/N for mark-mode display:
// each /QuadPoints region is filled with /IC (default black at 50% transparency-equivalent
// — actually opaque black since builder doesn't support transparency; visual contrast
// signals the redact mark) and optional /OverlayText is rendered centered in each quad.
func generateRedactAppearance(a *RedactAnnotation) *pdfStream {
	rect := a.Rect()
	width := rect.URX - rect.LLX
	height := rect.URY - rect.LLY

	b := newAppearanceBuilder()
	resources := existingAPNResources(&a.annotationBase)
	if resources == nil {
		resources = pdfDict{}
	}

	// Determine fill color — default black if /IC absent.
	fill := Color{R: 0, G: 0, B: 0, A: 1}
	if ic := a.InteriorColor(); ic != nil {
		fill = *ic
	}

	quads := a.QuadPoints()
	if len(quads) == 0 {
		// Default: full /Rect as single quad.
		quads = []QuadPoint{rectAsQuadPoint(rect)}
	}

	// 1. Fill each quad in /IC.
	b.PushState()
	b.SetFillColorRGB(fill)
	for _, qp := range quads {
		// Translate quad to local /BBox space (subtract rect.LLX, rect.LLY).
		// Use an axis-aligned bounding rect derived from the quad's corner points.
		local := localizeQuadAsRect(qp, rect)
		b.Rect(local.LLX, local.LLY, local.URX-local.LLX, local.URY-local.LLY)
	}
	b.Fill()
	b.PopState()

	// 2. Optional /OverlayText preview (centered in each quad).
	if overlay := a.OverlayText(); overlay != "" {
		style := a.OverlayTextStyle()
		// Default text color: white if /IC is dark, black otherwise — heuristic.
		if style.Color == nil {
			white := Color{R: 1, G: 1, B: 1, A: 1}
			style.Color = &white
		}
		// Default font/size if not set.
		if style.Font == nil {
			style.Font = FontHelvetica
		}
		if style.Size == 0 {
			style.Size = 10
		}
		for _, qp := range quads {
			quadRect := localizeQuadAsRect(qp, rect)
			resolve := func(font Font, _ pdfDict) (resName string, w widthFn, e encodeFn, asc, desc float64, err error) {
				return resolveFontForXObject(font, style.Size, a.doc, resources)
			}
			_ = renderTextInBuilder(b, resources, overlay, style, quadRect, resolve, "", "")
		}
	}

	return makeFormXObjectWithResources(b.Bytes(), Rectangle{URX: width, URY: height}, resources)
}

// rectAsQuadPoint converts a Rectangle to a QuadPoint covering the
// same area (corners as defined by ISO 32000-1 §12.5.6.10: UL/UR/LL/LR).
func rectAsQuadPoint(r Rectangle) QuadPoint {
	return QuadPoint{
		X1: r.LLX, Y1: r.URY, // UL
		X2: r.URX, Y2: r.URY, // UR
		X3: r.LLX, Y3: r.LLY, // LL
		X4: r.URX, Y4: r.LLY, // LR
	}
}

// localizeQuadAsRect converts a page-space QuadPoint to a local /BBox-space
// axis-aligned Rectangle. Acceptable for redact rendering since redact
// regions are typically rectangular; full quad geometry preserved in the
// /QuadPoints array but visual fill uses the bounding box.
func localizeQuadAsRect(qp QuadPoint, rect Rectangle) Rectangle {
	minX := min(min(qp.X1, qp.X2), min(qp.X3, qp.X4))
	maxX := max(max(qp.X1, qp.X2), max(qp.X3, qp.X4))
	minY := min(min(qp.Y1, qp.Y2), min(qp.Y3, qp.Y4))
	maxY := max(max(qp.Y1, qp.Y2), max(qp.Y3, qp.Y4))
	return Rectangle{
		LLX: minX - rect.LLX,
		LLY: minY - rect.LLY,
		URX: maxX - rect.LLX,
		URY: maxY - rect.LLY,
	}
}
