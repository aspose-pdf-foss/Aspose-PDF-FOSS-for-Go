// SPDX-License-Identifier: MIT

package asposepdf

import (
	"fmt"
	"math"
	"strings"
	"unicode/utf8"
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
	text = arabicFormsFor(font, text)
	needBidi := rtl || bidiHasStrongRTL(text)
	baseLevel := 0
	if needBidi {
		baseLevel = bidiBaseLevel(text, rtl)
	}
	if shaper != nil {
		return singleLineShape{shaper: shaper, shaped: shapeLine(shaper.ttf, text, baseLevel, needBidi)}
	}
	// The rune-by-rune path draws a default-ignorable character as
	// nothing (the shaper hides them the same way), so it is dropped
	// before measuring and encoding (GitHub issue #24 point 2).
	text = dropDefaultIgnorables(text)
	if needBidi {
		text = bidiVisualString(text, baseLevel)
	}
	return singleLineShape{line: text, width: width}
}

// spacingText is the text whose characters character and word spacing
// apply to: the drawn line on the rune-by-rune path (default-ignorables
// dropped), the caller's text when shaped.
func (s singleLineShape) spacingText(text string) string {
	if s.shaper != nil {
		return text
	}
	return s.line
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
	// Checked here, not only in appendToContentStream below: see
	// AddText's identical comment — resolveFontForPage/useGlyph happen
	// well before this method's own appendToContentStream call
	// (GitHub issue #20 point 2, shadowhunter1470-svg).
	// Checked here, not only in appendToContentStream below: see
	// AddText's identical comment — resolveFontForPage/useGlyph happen
	// well before this method's own appendToContentStream call
	// (GitHub issue #20 point 2, shadowhunter1470-svg).
	if err := p.checkCanvasNotFrozen(); err != nil {
		return err
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
	if err := checkTextStyleValues("add text at", style); err != nil {
		return err
	}

	fontSize := style.Size
	if fontSize == 0 {
		fontSize = 12
	}
	font := style.Font
	if font == nil {
		font = FontHelvetica
	}
	text, err := applyMissingGlyphPolicy(font, text, style, false)
	if err != nil {
		return fmt.Errorf("add text at: %w", err)
	}

	resName, width, encode, _, _, err := p.resolveFontForPage(font, fontSize)
	if err != nil {
		return err
	}

	shape := resolveSingleLine(font, fontSize, text, style.RTL, width)
	lineWidth := spacedWidth(shape.widthAt(fontSize), shape.spacingText(text), style.CharSpacing, style.WordSpacing, style.HorizontalScaling)
	invisible := style.Invisible || style.RenderMode == RenderingInvisible
	_, embedded := font.(*embeddedFont)
	if pf, ok := font.(*pageFont); ok && pf.twoByte() {
		embedded = true // Tw does not reach its codes either
	}

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
	switch {
	case invisible:
		buf.WriteString("3 Tr\n")
	case style.RenderMode == RenderingStroke || style.RenderMode == RenderingFillStroke:
		if sc := style.StrokeColor; sc != nil {
			buf.WriteString(fmt.Sprintf("%s %s %s RG\n",
				formatFloat(sc.R), formatFloat(sc.G), formatFloat(sc.B)))
		}
		if style.strokeWidth > 0 {
			buf.WriteString(fmt.Sprintf("%s w\n", formatFloat(style.strokeWidth)))
		}
		buf.WriteString(fmt.Sprintf("%d Tr\n", int(style.RenderMode)))
	}
	if style.CharSpacing != 0 {
		buf.WriteString(fmt.Sprintf("%s Tc\n", formatFloat(style.CharSpacing)))
	}
	// A two-byte (embedded) font ignores Tw; its word spacing is applied
	// through TJ numbers below.
	if style.WordSpacing != 0 && !embedded {
		buf.WriteString(fmt.Sprintf("%s Tw\n", formatFloat(style.WordSpacing)))
	}
	if style.HorizontalScaling != 0 && style.HorizontalScaling != 100 {
		buf.WriteString(fmt.Sprintf("%s Tz\n", formatFloat(style.HorizontalScaling)))
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

	if shape.shaper == nil && style.Rotation == 0 && style.HAlign == HAlignLeft &&
		shape.line == text && len(style.glyphX) == utf8.RuneCountInString(text) {
		tj, w := placedTJ(text, encode, width, fontSize, drawX, embedded, style)
		buf.WriteString(tj)
		lineWidth = w
	} else if shape.shaper != nil {
		buf.WriteString(shape.shaped.showOpsSpaced(shape.shaper, fontSize, style.WordSpacing))
	} else if embedded && style.WordSpacing != 0 && strings.ContainsRune(shape.line, ' ') {
		buf.WriteString(spacedTJ(shape.line, encode, fontSize, style.WordSpacing))
	} else {
		buf.WriteString(fmt.Sprintf("%s Tj\n", encode(shape.line)))
	}
	buf.WriteString("ET\n")

	if style.Underline && !invisible {
		buf.WriteString(fmt.Sprintf("%s %s %s rg\n",
			formatFloat(textColor.R), formatFloat(textColor.G), formatFloat(textColor.B)))
		thickness := fontSize * 0.05
		ulY := drawY - fontSize*0.1
		buf.WriteString(fmt.Sprintf("%s %s %s %s re f\n",
			formatFloat(drawX), formatFloat(ulY), formatFloat(lineWidth), formatFloat(thickness)))
	}
	if style.Strikethrough && !invisible {
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

// spacedTJ shows an unshaped line in a two-byte font as a TJ array that
// advances the pen by tw points after every space (Tw does not reach
// two-byte codes).
func spacedTJ(line string, encode func(string) string, size, tw float64) string {
	var arr, run strings.Builder
	flush := func() {
		if run.Len() > 0 {
			arr.WriteString("<" + run.String() + ">")
			run.Reset()
		}
	}
	for _, r := range line {
		enc := encode(string(r))
		run.WriteString(strings.Trim(enc, "<>"))
		if r == ' ' {
			flush()
			arr.WriteString(" " + formatFloat(-tw/size*1000) + " ")
		}
	}
	flush()
	return fmt.Sprintf("[%s] TJ\n", strings.TrimSpace(arr.String()))
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
	if err := checkTextStyleValues("measure text", style); err != nil {
		return 0, err
	}
	font := style.Font
	if font == nil {
		font = FontHelvetica
	}
	text, err := applyMissingGlyphPolicy(font, text, style, false)
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
	return spacedWidth(shape.widthAt(fontSize), shape.spacingText(text), style.CharSpacing, style.WordSpacing, style.HorizontalScaling), nil
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
	if pf, ok := f.(*pageFont); ok && pf.fi.descent < 0 {
		m.Descent = pf.fi.descent / 1000 * size
	}
	return m, nil
}

// placedTJ shows text as a TJ array that starts each character's glyph at
// style.glyphX (a NaN entry follows on from the glyph before), measuring
// advances as the viewer does (width, Tc, Tw for a one-byte space, Tz). It
// returns the operator and the width drawn, from x to the end of the last
// glyph.
func placedTJ(text string, encode encodeFn, width widthFn, size, x float64, twoByte bool, style TextStyle) (string, float64) {
	th := style.HorizontalScaling / 100
	if th == 0 {
		th = 1
	}
	var arr strings.Builder
	pen := x
	for i, r := range []rune(text) {
		if want := style.glyphX[i]; i > 0 && !math.IsNaN(want) {
			if d := want - pen; math.Abs(d) > 1e-4 {
				arr.WriteString(formatFloat(-d * 1000 / (size * th)))
				arr.WriteByte(' ')
			}
			pen = want
		}
		arr.WriteString(encode(string(r)))
		arr.WriteByte(' ')
		adv := width(r) + style.CharSpacing
		if r == ' ' && !twoByte {
			adv += style.WordSpacing
		}
		pen += adv * th
	}
	return fmt.Sprintf("[%s] TJ\n", strings.TrimSpace(arr.String())), pen - x
}
