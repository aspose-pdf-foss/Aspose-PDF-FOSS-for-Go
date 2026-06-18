// SPDX-License-Identifier: MIT

package asposepdf

import (
	"strings"
	"time"
)

// generateSignatureAppearance builds the /AP/N Form XObject for a visible
// signature: a small text block ("Digitally signed by …", date, optional
// reason/location) drawn with a Standard-14 font so the built-in renderer
// (and any viewer) draws it. Mirrors the text Aspose.PDF for .NET's
// SignatureCustomAppearance produces.
func generateSignatureAppearance(cfg *signConfig, doc *Document, when time.Time, w, h float64) *pdfStream {
	ap := cfg.appearance
	showName := ap == nil || ap.ShowName
	showDate := ap == nil || ap.ShowDate
	showReason := ap == nil || ap.ShowReason
	showLocation := ap == nil || ap.ShowLocation

	var lines []string
	if showName {
		name := cfg.cert.Subject.CommonName
		if name == "" {
			name = cfg.cert.Subject.String()
		}
		lines = append(lines, "Digitally signed by "+name)
	}
	if showDate {
		lines = append(lines, "Date: "+when.Format("2006.01.02 15:04:05 -07'00'"))
	}
	if showReason && cfg.reason != "" {
		lines = append(lines, "Reason: "+cfg.reason)
	}
	if showLocation && cfg.location != "" {
		lines = append(lines, "Location: "+cfg.location)
	}
	text := strings.Join(lines, "\n")

	font := Font(FontHelvetica)
	if ap != nil && ap.Font != nil {
		font = ap.Font
	}
	color := &Color{R: 0.18, G: 0.27, B: 0.55, A: 1} // dark blue, echoing the familiar look
	if ap != nil && ap.Color != nil {
		color = ap.Color
	}
	size := 0.0
	if ap != nil {
		size = ap.FontSize
	}
	if size <= 0 {
		// Auto-fit: each line gets h/n vertical space; size ≈ that / 1.25,
		// clamped to a readable range.
		n := len(lines)
		if n < 1 {
			n = 1
		}
		size = (h / float64(n)) / 1.25
		if size > 11 {
			size = 11
		}
		if size < 5 {
			size = 5
		}
	}

	b := newAppearanceBuilder()
	resources := pdfDict{}
	style := TextStyle{Font: font, Size: size, Color: color, VAlign: VAlignTop, LineSpacing: 1.25}
	resolve := func(f Font, _ pdfDict) (string, widthFn, encodeFn, float64, float64, error) {
		return resolveFontForXObject(f, size, doc, resources)
	}
	inner := Rectangle{LLX: 2, LLY: 2, URX: w - 2, URY: h - 2}
	if text != "" {
		_ = renderTextInBuilder(b, resources, text, style, inner, resolve, "", "")
	}
	return makeFormXObjectWithResources(b.Bytes(), Rectangle{URX: w, URY: h}, resources)
}
