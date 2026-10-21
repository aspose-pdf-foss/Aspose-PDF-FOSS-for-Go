// SPDX-License-Identifier: MIT

package asposepdf

import (
	"fmt"
	"math"
	"strings"
)

// singleLineShape carries whatever a single already-finished line needs for
// measuring/drawing without any word-wrapping: either a shapedLine (for a
// font with OpenType GSUB/GPOS tables) or the plain, possibly
// bidi-reordered, text plus a widthFn. Shared by AddTextAt and MeasureText
// so both apply the exact same Arabic-shaping/bidi/OpenType-shaping
// pipeline renderTextInBuilder's per-line draw step does for one already-
// wrapped line (GitHub issue #18) — the only piece deliberately left out
// here is wrapText itself.
type singleLineShape struct {
	shaper *embeddedFont
	shaped shapedLine // valid only when shaper != nil
	line   string     // final (bidi-reordered if needed) text; valid only when shaper == nil
	width  widthFn    // valid only when shaper == nil
}

// resolveSingleLine builds the singleLineShape for text in font/size,
// applying Arabic Presentation-Forms shaping and bidi reordering exactly
// as renderTextInBuilder does before its own per-line draw step — but,
// unlike renderTextInBuilder, never wrapping: text is always the whole,
// single line.
func resolveSingleLine(font Font, size float64, text string, rtl bool, width widthFn) singleLineShape {
	var shaper *embeddedFont
	if ef, ok := font.(*embeddedFont); ok && fontHasShaping(ef.ttf) {
		shaper = ef
	}
	if bidiHasArabic(text) && (shaper == nil || !fontShapesArabic(shaper.ttf)) {
		text = shapeArabic(text)
	}
	needBidi := rtl || bidiHasStrongRTL(text)
	baseLevel := 0
	if needBidi {
		baseLevel = bidiBaseLevel(text, rtl)
	}
	if shaper != nil {
		return singleLineShape{shaper: shaper, shaped: shapeLine(shaper.ttf, text, baseLevel, needBidi)}
	}
	if needBidi {
		text = bidiVisualString(text, baseLevel)
	}
	return singleLineShape{line: text, width: width}
}

// widthAt returns the line's real drawn width at size.
func (s singleLineShape) widthAt(size float64) float64 {
	if s.shaper != nil {
		return s.shaped.width(size)
	}
	return measureString(s.line, s.width)
}

// AddTextAt draws text as a single line whose baseline starts at (x, y):
// no word-wrapping, no clipping to any rectangle, and whitespace is kept
// exactly as given — unlike AddText, which normalizes runs of whitespace
// through strings.Fields while wrapping. For a caller that already does
// its own line-breaking and positioning and only needs this library to
// place and shape individual lines (GitHub issue #18, `pdf-go-xe3l`).
// Uses the same font resources, OpenType shaping (GSUB/GPOS) and bidi
// reordering as AddText, so MeasureText/GetFontMetrics below always agree
// with what this draws.
//
// HAlign anchors the line at x the way SVG's text-anchor does:
// HAlignLeft (the default) starts the line at x, HAlignCenter centers it
// on x, HAlignRight ends it at x. y is always the exact baseline — no
// ascent-based offset is applied, unlike AddText's rect-relative vertical
// placement (see GetFontMetrics to compute that offset yourself, the same
// way AddText does internally).
//
// Color, Underline, Strikethrough, Rotation (pivot = (x, y) instead of a
// rectangle corner), Skew, Invisible, Behind and RTL from TextStyle all
// apply the same way they do in AddText. Background and VAlign have no
// meaning without a rectangle to fill or align within and are ignored.
func (p *Page) AddTextAt(text string, style TextStyle, x, y float64) error {
	if text == "" {
		return nil
	}
	if err := checkFontSize("add text at", style.Size); err != nil {
		return err
	}
	if nonFinite(x) || nonFinite(y) {
		return fmt.Errorf("add text at: x and y must be finite, got (%v, %v)", x, y)
	}
	if nonFinite(style.Rotation) {
		return fmt.Errorf("add text at: rotation must be finite, got %v", style.Rotation)
	}
	if nonFinite(style.Skew) {
		return fmt.Errorf("add text at: skew must be finite, got %v", style.Skew)
	}

	fontSize := style.Size
	if fontSize == 0 {
		fontSize = 12
	}
	font := style.Font
	if font == nil {
		font = FontHelvetica
	}
	text, err := applyMissingGlyphPolicy(font, text, style)
	if err != nil {
		return fmt.Errorf("add text at: %w", err)
	}

	resName, width, encode, _, _, err := p.resolveFontForPage(font, fontSize)
	if err != nil {
		return err
	}

	shape := resolveSingleLine(font, fontSize, text, style.RTL, width)
	lineWidth := shape.widthAt(fontSize)

	textColor := Color{R: 0, G: 0, B: 0, A: 1}
	if style.Color != nil {
		textColor = *style.Color
	}
	var textGSName string
	if textColor.A < 1 {
		name, err := p.ensureExtGState(textColor.A)
		if err != nil {
			return err
		}
		textGSName = name
	}

	var buf strings.Builder
	buf.WriteString("\nq\n")

	// Rotation pivots about (x, y) itself (contrast AddText, whose pivot is
	// a rectangle corner): translate the origin there first, then rotate,
	// so every coordinate from this point on is pivot-relative.
	originX, originY := x, y
	if style.Rotation != 0 {
		buf.WriteString(fmt.Sprintf("1 0 0 1 %s %s cm\n", formatFloat(x), formatFloat(y)))
		rad := style.Rotation * math.Pi / 180.0
		cos := math.Cos(rad)
		sin := math.Sin(rad)
		buf.WriteString(fmt.Sprintf("%s %s %s %s 0 0 cm\n",
			formatFloat(cos), formatFloat(sin), formatFloat(-sin), formatFloat(cos)))
		originX, originY = 0, 0
	}

	var drawX float64
	switch style.HAlign {
	case HAlignCenter:
		drawX = originX - lineWidth/2
	case HAlignRight:
		drawX = originX - lineWidth
	default: // HAlignLeft
		drawX = originX
	}
	drawY := originY

	if textGSName != "" {
		buf.WriteString(fmt.Sprintf("%s gs\n", textGSName))
	}

	buf.WriteString("BT\n")
	if style.Invisible {
		buf.WriteString("3 Tr\n")
	}
	buf.WriteString(fmt.Sprintf("%s %s Tf\n", resName, formatFloat(fontSize)))
	buf.WriteString(fmt.Sprintf("%s %s %s rg\n",
		formatFloat(textColor.R), formatFloat(textColor.G), formatFloat(textColor.B)))

	if style.Skew != 0 {
		tan := math.Tan(style.Skew * math.Pi / 180)
		buf.WriteString(fmt.Sprintf("1 0 %s 1 %s %s Tm\n",
			formatFloat(tan), formatFloat(drawX), formatFloat(drawY)))
	} else {
		buf.WriteString(fmt.Sprintf("%s %s Td\n", formatFloat(drawX), formatFloat(drawY)))
	}

	if shape.shaper != nil {
		buf.WriteString(shape.shaped.showOps(shape.shaper, fontSize))
	} else {
		buf.WriteString(fmt.Sprintf("%s Tj\n", encode(shape.line)))
	}
	buf.WriteString("ET\n")

	if style.Underline && !style.Invisible {
		buf.WriteString(fmt.Sprintf("%s %s %s rg\n",
			formatFloat(textColor.R), formatFloat(textColor.G), formatFloat(textColor.B)))
		thickness := fontSize * 0.05
		ulY := drawY - fontSize*0.1
		buf.WriteString(fmt.Sprintf("%s %s %s %s re f\n",
			formatFloat(drawX), formatFloat(ulY), formatFloat(lineWidth), formatFloat(thickness)))
	}
	if style.Strikethrough && !style.Invisible {
		buf.WriteString(fmt.Sprintf("%s %s %s rg\n",
			formatFloat(textColor.R), formatFloat(textColor.G), formatFloat(textColor.B)))
		thickness := fontSize * 0.05
		stY := drawY + fontSize*0.3
		buf.WriteString(fmt.Sprintf("%s %s %s %s re f\n",
			formatFloat(drawX), formatFloat(stY), formatFloat(lineWidth), formatFloat(thickness)))
	}

	buf.WriteString("Q\n")

	if style.Behind {
		return p.prependToContentStream([]byte(buf.String()))
	}
	return p.appendToContentStream([]byte(buf.String()))
}

// MeasureText returns the width in points of text drawn as a single line
// with style, using the same font resolution, Arabic/bidi reordering and
// OpenType shaping AddTextAt applies — so a caller doing its own
// line-breaking can measure a candidate line exactly the way it would be
// drawn. Unlike AddText's own internal wrapping measurement, whitespace in
// text is never collapsed. style.Font nil defaults to FontHelvetica,
// style.Size 0 to 12, matching AddText/AddTextAt.
func MeasureText(text string, style TextStyle) (float64, error) {
	if text == "" {
		return 0, nil
	}
	if err := checkFontSize("measure text", style.Size); err != nil {
		return 0, err
	}
	font := style.Font
	if font == nil {
		font = FontHelvetica
	}
	text, err := applyMissingGlyphPolicy(font, text, style)
	if err != nil {
		return 0, fmt.Errorf("measure text: %w", err)
	}
	fontSize := style.Size
	if fontSize == 0 {
		fontSize = 12
	}
	width, _, err := fontWidthAndAscent(font, fontSize)
	if err != nil {
		return 0, err
	}
	shape := resolveSingleLine(font, fontSize, text, style.RTL, width)
	return shape.widthAt(fontSize), nil
}

// FontMetrics holds the vertical metrics AddText/AddTextAt use for a font
// at a given size, in points. Mirrors Aspose.PDF for .NET's
// Font.MeasureString returning the same kind of layout metrics.
type FontMetrics struct {
	// Ascent is the distance AddText subtracts from a rectangle's top
	// edge (or AddTextAt's caller would subtract from a desired top) to
	// place the baseline: rect.URY - Ascent. Always >= 0.
	Ascent float64
	// Descent is how far the font's descender reaches below the
	// baseline, as a negative value (0 if not tracked for this font —
	// see below). Mirrors the existing internal descent convention
	// (text.go) of a non-positive distance.
	Descent float64
}

// GetFontMetrics returns the ascent/descent AddText/AddTextAt use for f at
// size, in points — the quantity a caller doing its own vertical
// positioning with AddTextAt would otherwise have to reverse-engineer
// from AddText's rect.URY - ascent*size placement formula. size 0 is
// treated as 12, matching AddText/AddTextAt/MeasureText.
//
// Descent is only tracked for an embedded TrueType font (its real hhea
// descender); AddText's own placement never consults a descent value for
// the Standard-14/Type3 fallback (a fixed 0.8 em ascent, no separate
// descender), so none is fabricated here either — Descent comes back 0
// for those.
func GetFontMetrics(f Font, size float64) (FontMetrics, error) {
	if err := checkFontSize("get font metrics", size); err != nil {
		return FontMetrics{}, err
	}
	if size == 0 {
		size = 12
	}
	// AddText/AddTextAt/MeasureText all default a nil Font to
	// FontHelvetica; this stayed an error instead ("font: unsupported
	// type <nil>"), breaking a caller that pairs MeasureText (which
	// accepts nil) with GetFontMetrics (which didn't) for its own layout
	// math (GitHub issue #19, shadowhunter1470-svg).
	if f == nil {
		f = FontHelvetica
	}
	_, ascentFactor, err := fontWidthAndAscent(f, size)
	if err != nil {
		return FontMetrics{}, err
	}
	m := FontMetrics{Ascent: ascentFactor * size}
	if ef, ok := f.(*embeddedFont); ok {
		m.Descent = float64(ef.ttf.descent) / float64(ef.ttf.unitsPerEm) * size
	}
	return m, nil
}
