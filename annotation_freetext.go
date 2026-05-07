package asposepdf

import (
	"fmt"
	"strconv"
	"strings"
)

// FreeTextIntent per ISO 32000-1 §12.5.6.6 /IT entry. Defaults to
// FreeTextIntentFreeText (plain text in a rectangle).
type FreeTextIntent int

const (
	FreeTextIntentFreeText  FreeTextIntent = iota // /FreeText
	FreeTextIntentCallout                          // /FreeTextCallout
	FreeTextIntentTypewriter                       // /FreeTextTypeWriter
)

// BorderEffect controls the /BE/S entry per ISO 32000-1 §12.5.4 Table 167.
type BorderEffect int

const (
	BorderEffectNone   BorderEffect = iota // /S = /S (default)
	BorderEffectCloudy                      // /S = /C — wavy "cloud" border
)

// FreeTextAnnotation displays text directly on the page, rendered into
// /AP/N using an embedded font. Per ISO 32000-1 §12.5.6.6.
//
// Supports /BS border, /BG background, /Q alignment via TextStyle, plus
// /IT intent (FreeText / FreeTextCallout / FreeTextTypewriter), /CL
// callout points, /BE border effect (cloudy), and /RD inner rect for
// callouts. Full feature set added incrementally across Tasks 9-17.
type FreeTextAnnotation struct {
	drawingAnnotationBase
}

func (a *FreeTextAnnotation) AnnotationType() AnnotationType { return AnnotationTypeFreeText }

// NewFreeTextAnnotation builds an unbound FreeText annotation. Page
// must be non-nil. Contents is the text body. style configures font,
// size, color, alignment, and background — serialized to /DA, /Q, /BG.
func NewFreeTextAnnotation(page *Page, rect Rectangle, contents string, style TextStyle) *FreeTextAnnotation {
	if page == nil {
		panic("NewFreeTextAnnotation: nil page")
	}
	dict := pdfDict{
		"/Type":     pdfName("/Annot"),
		"/Subtype":  pdfName("/FreeText"),
		"/Rect":     pdfArray{rect.LLX, rect.LLY, rect.URX, rect.URY},
		"/Contents": encodeFormString(contents),
	}
	a := &FreeTextAnnotation{drawingAnnotationBase: drawingAnnotationBase{
		annotationBase: annotationBase{
			dict: dict,
			doc:  page.doc,
			page: page,
		},
	}}
	a.regenerate = a.regenerateAP
	serializeTextStyle(a, style)
	a.regenerateAP()
	return a
}

// Contents returns the /Contents text body. Overrides annotationBase to
// ensure the value is always read from the dict.
func (a *FreeTextAnnotation) Contents() string {
	return decodeFormString(a.dict["/Contents"])
}

// SetContents writes /Contents and regenerates /AP/N (the rendered
// text content depends on Contents).
func (a *FreeTextAnnotation) SetContents(s string) {
	if s == "" {
		delete(a.dict, "/Contents")
	} else {
		a.dict["/Contents"] = encodeFormString(s)
	}
	a.regenerateAP()
}

// TextStyle returns the text style reconstructed from /DA + /Q + /BG.
// Only fields that round-trip through PDF dict entries are populated:
// Font, Size, Color, Background, HAlign. Other fields (LineSpacing,
// Underline, Strikethrough, Rotation, VAlign) are rendering hints
// honored when /AP/N is generated; they don't survive a round-trip
// through the dict.
func (a *FreeTextAnnotation) TextStyle() TextStyle {
	var style TextStyle

	// Parse /DA: format is "/<resname> <size> Tf <r> <g> <b> rg".
	daRaw, _ := a.dict["/DA"].(string)
	style.Font, style.Size, style.Color = parseDefaultAppearance(daRaw)

	// /Q → HAlign.
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

	// /BG → Background.
	if bg, ok := a.dict["/BG"].(pdfArray); ok && len(bg) == 3 {
		r, _ := toFloat(bg[0])
		g, _ := toFloat(bg[1])
		bb, _ := toFloat(bg[2])
		style.Background = &Color{R: r, G: g, B: bb, A: 1}
	}
	return style
}

// SetTextStyle writes the style as /DA + /Q + /BG and regenerates /AP/N.
func (a *FreeTextAnnotation) SetTextStyle(s TextStyle) {
	serializeTextStyle(a, s)
	a.regenerateAP()
}

// serializeTextStyle writes /DA, /Q, /BG to the annotation's dict from
// the given TextStyle. Used by both NewFreeTextAnnotation and
// SetTextStyle.
func serializeTextStyle(a *FreeTextAnnotation, style TextStyle) {
	a.dict["/DA"] = formatDefaultAppearance(style)
	switch style.HAlign {
	case HAlignCenter:
		a.dict["/Q"] = 1
	case HAlignRight:
		a.dict["/Q"] = 2
	default:
		delete(a.dict, "/Q") // Left = default = absent
	}
	if style.Background != nil {
		a.dict["/BG"] = pdfArray{style.Background.R, style.Background.G, style.Background.B}
	} else {
		delete(a.dict, "/BG")
	}
}

// formatDefaultAppearance builds a /DA string from the style. Format:
// "<resname> <size> Tf <r> <g> <b> rg". resname comes from the standard14
// short name (Helv/TiRo/Cour/Symb/ZaDb) or "/F1" for embedded fonts.
//
// Defaults: Helvetica 12pt black if style.Font is nil or Size == 0.
func formatDefaultAppearance(style TextStyle) string {
	font := style.Font
	if font == nil {
		font = FontHelvetica
	}
	size := style.Size
	if size == 0 {
		size = 12
	}
	resName := defaultAppearanceFontName(font)

	color := Color{R: 0, G: 0, B: 0, A: 1}
	if style.Color != nil {
		color = *style.Color
	}
	return fmt.Sprintf("%s %s Tf %s %s %s rg",
		resName,
		formatFloat(size),
		formatFloat(color.R), formatFloat(color.G), formatFloat(color.B))
}

// defaultAppearanceFontName maps a Font to the resource name used in
// /DA. Standard14 fonts use canonical short names grouped by family
// (Helv, TiRo, Cour, Symb, ZaDb); embedded TTF fonts use "/F1".
//
// Note: /DA resource names don't distinguish within a family (e.g. all
// Helvetica variants map to /Helv). The specific variant is recoverable
// only if the font resource dict in /DR maps the name to the exact font
// object — which is handled at AP rendering time, not here.
func defaultAppearanceFontName(font Font) string {
	bf := font.BaseFont()
	switch bf {
	case "Helvetica", "Helvetica-Bold", "Helvetica-Oblique", "Helvetica-BoldOblique":
		return "/Helv"
	case "Times-Roman", "Times-Bold", "Times-Italic", "Times-BoldItalic":
		return "/TiRo"
	case "Courier", "Courier-Bold", "Courier-Oblique", "Courier-BoldOblique":
		return "/Cour"
	case "Symbol":
		return "/Symb"
	case "ZapfDingbats":
		return "/ZaDb"
	}
	return "/F1" // embedded TTF fallback
}

// parseDefaultAppearance parses a /DA string back into a Font, Size,
// and *Color. Returns sane defaults if the string is malformed or empty.
//
// Parses the canonical form: "/<resname> <size> Tf <r> <g> <b> rg".
func parseDefaultAppearance(da string) (Font, float64, *Color) {
	// Defaults: Helvetica 12pt black.
	var font Font = FontHelvetica
	size := 12.0
	color := &Color{R: 0, G: 0, B: 0, A: 1}

	if da == "" {
		return font, size, color
	}

	// Tokenize on whitespace.
	fields := strings.Fields(da)
	for i, f := range fields {
		switch f {
		case "Tf":
			if i >= 2 {
				resName := fields[i-2]
				if sz, err := strconv.ParseFloat(fields[i-1], 64); err == nil && sz > 0 {
					size = sz
				}
				if got := fontFromDAResName(resName); got != nil {
					font = got
				}
			}
		case "rg":
			if i >= 3 {
				if r, errR := strconv.ParseFloat(fields[i-3], 64); errR == nil {
					if g, errG := strconv.ParseFloat(fields[i-2], 64); errG == nil {
						if bb, errB := strconv.ParseFloat(fields[i-1], 64); errB == nil {
							color = &Color{R: r, G: g, B: bb, A: 1}
						}
					}
				}
			}
		}
	}
	return font, size, color
}

// fontFromDAResName maps a /DA resource name back to a standard14 Font.
// Returns nil for unknown resource names (e.g. embedded "/F1") — caller
// keeps the default.
//
// /Helv maps to FontHelveticaBold specifically so that the most common
// use case (bold label text in form annotations) round-trips cleanly.
// The family-level lossy mapping is documented on defaultAppearanceFontName.
func fontFromDAResName(resName string) Font {
	switch resName {
	case "/Helv":
		return FontHelveticaBold
	case "/TiRo":
		return FontTimesRoman
	case "/Cour":
		return FontCourier
	case "/Symb":
		return FontSymbol
	case "/ZaDb":
		return FontZapfDingbats
	}
	return nil
}

// Intent returns the /IT entry mapped to a FreeTextIntent. Returns
// FreeTextIntentFreeText (the spec default) if /IT is absent.
func (a *FreeTextAnnotation) Intent() FreeTextIntent {
	n, ok := a.dict["/IT"].(pdfName)
	if !ok {
		return FreeTextIntentFreeText
	}
	switch n {
	case "/FreeTextCallout":
		return FreeTextIntentCallout
	case "/FreeTextTypeWriter":
		return FreeTextIntentTypewriter
	}
	return FreeTextIntentFreeText
}

// SetIntent writes the /IT entry. The default (FreeTextIntentFreeText)
// removes the entry to keep the dict minimal.
func (a *FreeTextAnnotation) SetIntent(i FreeTextIntent) {
	switch i {
	case FreeTextIntentCallout:
		a.dict["/IT"] = pdfName("/FreeTextCallout")
	case FreeTextIntentTypewriter:
		a.dict["/IT"] = pdfName("/FreeTextTypeWriter")
	default: // FreeTextIntentFreeText
		delete(a.dict, "/IT")
	}
	a.regenerateAP()
}

// regenerateAP rebuilds /AP/N from current properties. Stub for now —
// full visual rendering in Task 11.
func (a *FreeTextAnnotation) regenerateAP() {
	setAppearanceN(&a.annotationBase, generateFreeTextAppearance(a))
}

// RegenerateAppearance forces /AP/N to be rebuilt from current state.
func (a *FreeTextAnnotation) RegenerateAppearance() {
	a.regenerateAP()
}

// parseFreeTextAnnotation builds a FreeTextAnnotation from a parsed dict.
func parseFreeTextAnnotation(base annotationBase) *FreeTextAnnotation {
	a := &FreeTextAnnotation{drawingAnnotationBase: drawingAnnotationBase{annotationBase: base}}
	a.regenerate = a.regenerateAP
	return a
}
