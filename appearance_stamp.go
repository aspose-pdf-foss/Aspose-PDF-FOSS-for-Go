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

// generateStampAppearance produces /AP/N for a Stamp annotation. This
// task is the skeleton — predefined visuals are rendered fully in
// Task 7, custom-image support in Task 8.
func generateStampAppearance(a *StampAnnotation) *pdfStream {
	rect := a.Rect()
	width := rect.URX - rect.LLX
	height := rect.URY - rect.LLY

	b := newAppearanceBuilder()
	primary, _, _ := stampVisualParams(a.Name())

	// Skeleton: just a colored border rect, no fill, no label.
	b.PushState()
	b.SetLineWidth(2)
	b.SetStrokeColorRGB(primary)
	drawRoundedRect(b, 2, 2, width-4, height-4, 5)
	b.Stroke()
	b.PopState()

	return makeFormXObjectWithResources(b.Bytes(), Rectangle{URX: width, URY: height}, pdfDict{})
}
