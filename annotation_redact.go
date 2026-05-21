// SPDX-License-Identifier: MIT

package asposepdf

// RedactAnnotation marks regions for redaction. Mark mode (this type)
// renders a semi-transparent fill of /QuadPoints regions. The
// destructive content removal happens when (*Document).ApplyRedactions
// is called — this annotation is then removed and the underlying page
// content is irreversibly rewritten. Per ISO 32000-1 §12.5.6.20.
type RedactAnnotation struct {
	drawingAnnotationBase
}

func (a *RedactAnnotation) AnnotationType() AnnotationType { return AnnotationTypeRedact }

// NewRedactAnnotation builds an unbound redact annotation. Page must
// be non-nil. By default, /QuadPoints is empty (rendering uses /Rect
// as a single quad). Callers typically call SetQuadPoints to specify
// multiple disjoint regions.
func NewRedactAnnotation(page *Page, rect Rectangle) *RedactAnnotation {
	if page == nil {
		panic("NewRedactAnnotation: nil page")
	}
	dict := pdfDict{
		"/Type":    pdfName("/Annot"),
		"/Subtype": pdfName("/Redact"),
		"/Rect":    pdfArray{rect.LLX, rect.LLY, rect.URX, rect.URY},
	}
	a := &RedactAnnotation{drawingAnnotationBase: drawingAnnotationBase{
		annotationBase: annotationBase{
			dict: dict,
			doc:  page.doc,
			page: page,
		},
	}}
	a.regenerate = a.regenerateAP
	a.regenerateAP()
	return a
}

// QuadPoints returns the regions to redact in page space. Returns nil
// if /QuadPoints is absent (Apply uses /Rect as the single region).
func (a *RedactAnnotation) QuadPoints() []QuadPoint {
	return readQuadPoints(a.dict["/QuadPoints"])
}

// SetQuadPoints writes /QuadPoints. nil/empty slice removes the entry
// (Apply will then use /Rect as single region).
func (a *RedactAnnotation) SetQuadPoints(qp []QuadPoint) {
	if len(qp) == 0 {
		delete(a.dict, "/QuadPoints")
	} else {
		a.dict["/QuadPoints"] = quadPointsToPDFArray(qp)
	}
	a.regenerateAP()
}

// regenerateAP rebuilds /AP/N for mark-mode visual.
func (a *RedactAnnotation) regenerateAP() {
	setAppearanceN(&a.annotationBase, generateRedactAppearance(a))
}

// RegenerateAppearance forces /AP/N to be rebuilt.
func (a *RedactAnnotation) RegenerateAppearance() {
	a.regenerateAP()
}

// parseRedactAnnotation builds a RedactAnnotation from a parsed dict.
func parseRedactAnnotation(base annotationBase) *RedactAnnotation {
	a := &RedactAnnotation{drawingAnnotationBase: drawingAnnotationBase{annotationBase: base}}
	a.regenerate = a.regenerateAP
	return a
}

// InteriorColor returns the /IC fill color (used for both mark visual
// and post-apply overlay). Returns nil if absent.
func (a *RedactAnnotation) InteriorColor() *Color {
	arr, ok := a.dict["/IC"].(pdfArray)
	if !ok || len(arr) != 3 {
		return nil
	}
	r, _ := toFloat(arr[0])
	g, _ := toFloat(arr[1])
	bl, _ := toFloat(arr[2])
	return &Color{R: r, G: g, B: bl, A: 1}
}

// SetInteriorColor writes /IC. nil deletes the entry.
func (a *RedactAnnotation) SetInteriorColor(c *Color) {
	if c == nil {
		delete(a.dict, "/IC")
	} else {
		a.dict["/IC"] = pdfArray{c.R, c.G, c.B}
	}
	a.regenerateAP()
}

// OverlayText returns /OverlayText. Empty string if absent.
func (a *RedactAnnotation) OverlayText() string {
	return decodeFormString(a.dict["/OverlayText"])
}

// SetOverlayText writes /OverlayText. Empty string deletes the entry.
func (a *RedactAnnotation) SetOverlayText(s string) {
	if s == "" {
		delete(a.dict, "/OverlayText")
	} else {
		a.dict["/OverlayText"] = encodeFormString(s)
	}
	a.regenerateAP()
}

// RepeatOverlayText returns /Repeat. False if absent.
func (a *RedactAnnotation) RepeatOverlayText() bool {
	v, _ := a.dict["/Repeat"].(bool)
	return v
}

// SetRepeatOverlayText writes /Repeat. False removes the entry.
func (a *RedactAnnotation) SetRepeatOverlayText(repeat bool) {
	if repeat {
		a.dict["/Repeat"] = true
	} else {
		delete(a.dict, "/Repeat")
	}
	a.regenerateAP()
}

// OverlayTextStyle returns the style reconstructed from /DA + /Q.
// Background is not relevant for redact overlay.
func (a *RedactAnnotation) OverlayTextStyle() TextStyle {
	var style TextStyle
	daRaw, _ := a.dict["/DA"].(string)
	style.Font, style.Size, style.Color = parseDefaultAppearance(daRaw)
	if q, ok := a.dict["/Q"]; ok {
		switch toInt(q) {
		case 1:
			style.HAlign = HAlignCenter
		case 2:
			style.HAlign = HAlignRight
		default:
			style.HAlign = HAlignLeft
		}
	}
	return style
}

// SetOverlayTextStyle writes /DA + /Q.
func (a *RedactAnnotation) SetOverlayTextStyle(s TextStyle) {
	a.dict["/DA"] = formatDefaultAppearance(s)
	switch s.HAlign {
	case HAlignCenter:
		a.dict["/Q"] = 1
	case HAlignRight:
		a.dict["/Q"] = 2
	default:
		delete(a.dict, "/Q")
	}
	a.regenerateAP()
}
