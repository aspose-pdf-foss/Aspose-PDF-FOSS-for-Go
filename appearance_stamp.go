package asposepdf

// drawRoundedRect adds a closed rounded-rectangle subpath to the
// builder. Corner radius is clamped to min(w/2, h/2). Geometry: m at
// bottom-edge (just past the bottom-left corner), then 4 cubic Beziers
// for the corners interleaved with 4 line segments for the sides,
// closed with h.
func drawRoundedRect(b *appearanceBuilder, x, y, w, h, radius float64) {
	r := radius
	if r > w/2 {
		r = w / 2
	}
	if r > h/2 {
		r = h / 2
	}
	rk := r * kappa // control-point distance for quarter-circle Bezier

	// Start at bottom-edge, just past the bottom-left corner.
	b.MoveTo(x+r, y)
	// Bottom edge to bottom-right corner start.
	b.LineTo(x+w-r, y)
	// Bottom-right corner.
	b.CurveTo(x+w-r+rk, y, x+w, y+r-rk, x+w, y+r)
	// Right edge.
	b.LineTo(x+w, y+h-r)
	// Top-right corner.
	b.CurveTo(x+w, y+h-r+rk, x+w-r+rk, y+h, x+w-r, y+h)
	// Top edge.
	b.LineTo(x+r, y+h)
	// Top-left corner.
	b.CurveTo(x+r-rk, y+h, x, y+h-r+rk, x, y+h-r)
	// Left edge.
	b.LineTo(x, y+r)
	// Bottom-left corner.
	b.CurveTo(x, y+r-rk, x+r-rk, y, x+r, y)
	b.ClosePath()
}

// stampVisualParams returns the (primary, fill, label) triple used to
// generate a default /AP/N visual for a predefined StampName. Color
// scheme: green=positive, red=warning, orange=informational, gray=neutral.
// Unknown returns the Draft (orange) defaults with empty label.
func stampVisualParams(n StampName) (primary, fill Color, label string) {
	green := Color{R: 0.13, G: 0.52, B: 0.13, A: 1}
	greenFill := Color{R: 0.85, G: 0.95, B: 0.85, A: 1}
	red := Color{R: 0.78, G: 0.13, B: 0.13, A: 1}
	redFill := Color{R: 0.99, G: 0.85, B: 0.85, A: 1}
	orange := Color{R: 0.85, G: 0.55, B: 0.13, A: 1}
	orangeFill := Color{R: 0.99, G: 0.92, B: 0.78, A: 1}
	gray := Color{R: 0.40, G: 0.40, B: 0.40, A: 1}
	grayFill := Color{R: 0.92, G: 0.92, B: 0.92, A: 1}

	switch n {
	case StampNameApproved:
		return green, greenFill, "APPROVED"
	case StampNameFinal:
		return green, greenFill, "FINAL"
	case StampNameForPublicRelease:
		return green, greenFill, "FOR PUBLIC RELEASE"
	case StampNameConfidential:
		return red, redFill, "CONFIDENTIAL"
	case StampNameExpired:
		return red, redFill, "EXPIRED"
	case StampNameNotApproved:
		return red, redFill, "NOT APPROVED"
	case StampNameNotForPublicRelease:
		return red, redFill, "NOT FOR PUBLIC RELEASE"
	case StampNameTopSecret:
		return red, redFill, "TOP SECRET"
	case StampNameAsIs:
		return orange, orangeFill, "AS IS"
	case StampNameDraft:
		return orange, orangeFill, "DRAFT"
	case StampNameExperimental:
		return orange, orangeFill, "EXPERIMENTAL"
	case StampNameForComment:
		return orange, orangeFill, "FOR COMMENT"
	case StampNameSold:
		return orange, orangeFill, "SOLD"
	case StampNameDepartmental:
		return gray, grayFill, "DEPARTMENTAL"
	}
	// Unknown / fallback: orange (Draft), no label.
	return orange, orangeFill, ""
}

// stampNameToPDF converts a StampName to its /Name entry value.
func stampNameToPDF(n StampName) pdfName {
	switch n {
	case StampNameApproved:
		return "/Approved"
	case StampNameAsIs:
		return "/AsIs"
	case StampNameConfidential:
		return "/Confidential"
	case StampNameDepartmental:
		return "/Departmental"
	case StampNameDraft:
		return "/Draft"
	case StampNameExperimental:
		return "/Experimental"
	case StampNameExpired:
		return "/Expired"
	case StampNameFinal:
		return "/Final"
	case StampNameForComment:
		return "/ForComment"
	case StampNameForPublicRelease:
		return "/ForPublicRelease"
	case StampNameNotApproved:
		return "/NotApproved"
	case StampNameNotForPublicRelease:
		return "/NotForPublicRelease"
	case StampNameSold:
		return "/Sold"
	case StampNameTopSecret:
		return "/TopSecret"
	}
	return "/Draft"
}

// pdfNameToStampName reverses stampNameToPDF; returns Unknown for non-spec names.
func pdfNameToStampName(n pdfName) StampName {
	switch n {
	case "/Approved":
		return StampNameApproved
	case "/AsIs":
		return StampNameAsIs
	case "/Confidential":
		return StampNameConfidential
	case "/Departmental":
		return StampNameDepartmental
	case "/Draft":
		return StampNameDraft
	case "/Experimental":
		return StampNameExperimental
	case "/Expired":
		return StampNameExpired
	case "/Final":
		return StampNameFinal
	case "/ForComment":
		return StampNameForComment
	case "/ForPublicRelease":
		return StampNameForPublicRelease
	case "/NotApproved":
		return StampNameNotApproved
	case "/NotForPublicRelease":
		return StampNameNotForPublicRelease
	case "/Sold":
		return StampNameSold
	case "/TopSecret":
		return StampNameTopSecret
	}
	return StampNameUnknown
}

// generateStampAppearance produces /AP/N for a Stamp annotation.
// Dispatches to predefined visual rendering or custom-image stub.
func generateStampAppearance(a *StampAnnotation) *pdfStream {
	if a.HasCustomImage() {
		return generateCustomImageStamp(a)
	}
	return generatePredefinedStamp(a)
}

// generatePredefinedStamp renders a default visual based on /Name:
// filled rounded rect + inner decorative border + uppercase italic
// label centered inside.
func generatePredefinedStamp(a *StampAnnotation) *pdfStream {
	rect := a.Rect()
	width := rect.URX - rect.LLX
	height := rect.URY - rect.LLY

	primary, fill, label := stampVisualParams(a.Name())

	b := newAppearanceBuilder()
	resources := existingAPNResources(&a.annotationBase)
	if resources == nil {
		resources = pdfDict{}
	}

	// 1. Filled rounded rect (background + outer border).
	b.PushState()
	b.SetFillColorRGB(fill)
	b.SetStrokeColorRGB(primary)
	b.SetLineWidth(2)
	drawRoundedRect(b, 1, 1, width-2, height-2, 5)
	b.FillStroke()
	b.PopState()

	// 2. Inner border (decorative double-line look).
	b.PushState()
	b.SetStrokeColorRGB(primary)
	b.SetLineWidth(1)
	drawRoundedRect(b, 4, 4, width-8, height-8, 3)
	b.Stroke()
	b.PopState()

	// 3. Centered label (using renderTextInBuilder).
	if label != "" {
		fontSize := fitStampFontSize(label, width-12, height-12)
		primaryCopy := primary
		style := TextStyle{
			Font:   FontHelveticaBoldOblique,
			Size:   fontSize,
			Color:  &primaryCopy,
			HAlign: HAlignCenter,
			VAlign: VAlignMiddle,
		}
		textRect := Rectangle{LLX: 6, LLY: 6, URX: width - 6, URY: height - 6}
		// XObject-context resolver: register font in resources, not page.
		resolve := func(font Font, _ pdfDict) (resName string, w widthFn, e encodeFn, asc, desc float64, err error) {
			return resolveFontForXObject(font, fontSize, a.doc, resources)
		}
		// Empty ExtGState names — opaque text, no transparency.
		_ = renderTextInBuilder(b, resources, label, style, textRect, resolve, "", "")
	}

	return makeFormXObjectWithResources(b.Bytes(), Rectangle{URX: width, URY: height}, resources)
}

// fitStampFontSize chooses a font size that fits label within the
// available rect dimensions. Heuristic: start at height/2, reduce until
// the label width fits maxWidth (rough estimate).
func fitStampFontSize(label string, maxWidth, maxHeight float64) float64 {
	size := maxHeight * 0.6
	if size > 24 {
		size = 24
	}
	// Estimate label width at this font size. Helvetica-Bold-Oblique
	// average char width is roughly 0.55 × fontSize.
	estWidth := float64(len(label)) * 0.55 * size
	if estWidth > maxWidth {
		size = maxWidth / (float64(len(label)) * 0.55)
	}
	if size < 6 {
		size = 6
	}
	return size
}

// generateCustomImageStamp wraps the embedded Image XObject (allocated
// during SetCustomImage) into a Form XObject /AP/N. The image is
// scaled to fit /BBox via cm transform and invoked with Do.
func generateCustomImageStamp(a *StampAnnotation) *pdfStream {
	rect := a.Rect()
	width := rect.URX - rect.LLX
	height := rect.URY - rect.LLY

	if a.customImageObjID == 0 {
		// Should not happen — caller checks HasCustomImage. Defensive fallback.
		return generatePredefinedStamp(a)
	}

	imgRef := pdfRef{Num: a.customImageObjID}
	resources := pdfDict{
		"/XObject": pdfDict{"/Im0": imgRef},
	}

	b := newAppearanceBuilder()
	b.PushState()
	// Scale image to fill BBox: cm matrix [width 0 0 height 0 0].
	b.ConcatMatrix(width, 0, 0, height, 0, 0)
	b.DoXObject(pdfName("/Im0"))
	b.PopState()

	return makeFormXObjectWithResources(b.Bytes(), Rectangle{URX: width, URY: height}, resources)
}
