// SPDX-License-Identifier: MIT

package asposepdf

import (
	"fmt"
	"math"
	"strings"
)

// nonFinite reports whether v cannot be written as a PDF number at all
// (ISO 32000-1 §7.3.3 defines no NaN/Infinity encoding) — used to reject
// a caller-supplied coordinate/size/angle before it reaches formatFloat,
// which would otherwise write the literal text "NaN"/"+Inf"/"-Inf" into
// a content stream, unparseable PDF syntax no reader can read back
// (GitHub issue #19, shadowhunter1470-svg).
func nonFinite(v float64) bool {
	return math.IsNaN(v) || math.IsInf(v, 0)
}

// minRepresentableSize is the smallest positive font size that survives
// formatFloat's 6-decimal-place rounding (appearance_builder.go) without
// collapsing to the literal text "0" — half the smallest representable
// unit at that precision. A caller-supplied size below this (e.g. 1e-9)
// is neither zero (which AddText/AddTextAt/MeasureText/GetFontMetrics
// all treat as "unspecified, use 12") nor large enough to survive
// rounding, so it would silently write `/F0 0 Tf` — a real, present font
// size of zero — making the text invisible rather than merely small
// (GitHub issue #19, shadowhunter1470-svg).
const minRepresentableSize = 5e-7

// fontHasGlyph reports whether font can show r, used by
// applyMissingGlyphPolicy to detect a character that would otherwise
// draw .notdef (or, for Type3, be silently dropped) with no signal at
// all (TextStyle.MissingGlyph, GitHub issue #19 point 2). Mirrors the
// exact per-type glyph resolution fontWidthAndAscent already uses, so
// "has a glyph" here always agrees with what actually gets drawn.
func fontHasGlyph(font Font, r rune) bool {
	switch f := font.(type) {
	case standardFont:
		_, ok := encodeRuneForStandardFont("/"+f.name, r)
		return ok
	case *embeddedFont:
		return f.ttf.glyphID(r) != 0
	case *Type3Font:
		_, ok := f.byRune[r]
		return ok
	default:
		return false
	}
}

// applyMissingGlyphPolicy implements TextStyle.MissingGlyph. The zero
// value (MissingGlyphAllow) is a no-op — text passes through unchanged,
// so a caller who never sets this field pays nothing for the check.
// Otherwise it scans every rune in text (skipping default-ignorables,
// which draw nothing by design, not because they're missing) for one
// font has no glyph for: MissingGlyphError names the first such
// character and returns an error instead of the text; MissingGlyphReplace
// substitutes style.ReplacementRune (U+FFFD when zero) for each one and
// returns the modified text to draw/measure instead of the original.
func applyMissingGlyphPolicy(font Font, text string, style TextStyle) (string, error) {
	if style.MissingGlyph == MissingGlyphAllow {
		return text, nil
	}
	var sb strings.Builder
	changed := false
	for _, r := range text {
		if isDefaultIgnorable(r) || fontHasGlyph(font, r) {
			sb.WriteRune(r)
			continue
		}
		if style.MissingGlyph == MissingGlyphError {
			return "", fmt.Errorf("text contains %q (%U), which the font has no glyph for", r, r)
		}
		rep := style.ReplacementRune
		if rep == 0 {
			rep = '�'
		}
		sb.WriteRune(rep)
		changed = true
	}
	if !changed {
		return text, nil
	}
	return sb.String(), nil
}

func checkFontSize(context string, size float64) error {
	if nonFinite(size) {
		return fmt.Errorf("%s: font size must be finite, got %v", context, size)
	}
	if size < 0 {
		return fmt.Errorf("%s: font size must be non-negative, got %g", context, size)
	}
	if size > 0 && size < minRepresentableSize {
		return fmt.Errorf("%s: font size %g is too small to represent (rounds to 0)", context, size)
	}
	return nil
}

// widthFn returns advance width in points for a single rune.
type widthFn func(r rune) float64

// lineMeasurer measures candidate text for word-wrap decisions so they
// agree with what will actually be drawn (GitHub issue #14: AddText used
// to wrap by the unshaped per-rune sum even for a font that OpenType-shapes
// — kerning/ligatures then make the drawn line narrower or wider than the
// width used to wrap it, causing an early wrap or a clipped overflow).
//
// For an unshaped font (no GSUB/GPOS, or Standard-14/Type3 — shaper is
// nil) word/space/exact are all the plain per-rune advance sum: that is
// also exactly what gets drawn, so the wrapping decision is already exact
// and output stays byte-identical to before shaping existed.
//
// For a font that shapes (fontHasShaping), word/space are a FAST estimate
// — each word (or a single space) shaped on its own and cached, which is
// exact for everything shaping does WITHIN a word (ligatures, contextual
// forms, intra-word kerning — joining never crosses a space) but can miss
// kerning BETWEEN two glyphs either side of the inter-word space, which a
// font rarely defines. exact always re-shapes the whole candidate exactly
// as renderTextInBuilder's draw loop will, so a caller can verify a
// greedily-packed line before committing to it and back a word off (into
// the next line) on the rare mismatch — see wrapText/nextLine. This keeps
// the common case cheap (one shape call per word, cached; one exact
// reshape per finished line, not per word added to it) instead of
// reshaping a growing candidate line from scratch for every word.
//
// A second, narrower gap (code review of this fix, unconfirmed against a
// real font): shaping a word alone picks its OpenType script from that
// word's own characters (otScriptTag), which could in principle disagree
// with the script the real line-level shape applies to the SAME
// characters, if the word's bidi embedding level in context splits it
// across a run boundary that shaping it alone does not see. The direction
// that matters is an OVERestimate from this: exact's backoff corrects an
// UNDERestimate just fine (a line it let overflow still gets re-measured
// and popped back correctly, since exact always uses the real
// baseLevel/bidi), but it has no way to ADD a word back that the greedy
// pack, discouraged by an inflated word() estimate, never tried to fit in
// the first place — so the only possible symptom is a wrap one word
// earlier than strictly necessary, never an overflow. Whether this is
// reachable at all is not established.
type lineMeasurer struct {
	width     widthFn // unshaped per-rune advance; always set, used as-is when shaper is nil
	shaper    *ttfFont
	size      float64
	baseLevel int
	bidi      bool

	wordCache   map[string]float64
	spaceW      float64
	spaceSet    bool
	shapedCache map[string]shapedLine // populated by exact(); reused by renderTextInBuilder's draw loop so a finished line is shaped only once
}

// newLineMeasurer wraps a plain unshaped widthFn: word/space/exact are all
// the existing per-rune sum, so output for an unshaped font is unchanged.
func newLineMeasurer(width widthFn) *lineMeasurer {
	return &lineMeasurer{width: width}
}

// newShapedLineMeasurer measures through the OpenType shaper (shaper must
// have fontHasShaping), so wrapping decisions match what
// renderTextInBuilder actually draws. baseLevel/bidi are the same values
// the draw loop uses for this text (see bidiBaseLevel/bidiHasStrongRTL).
func newShapedLineMeasurer(shaper *ttfFont, size float64, baseLevel int, bidi bool, width widthFn) *lineMeasurer {
	return &lineMeasurer{
		width: width, shaper: shaper, size: size, baseLevel: baseLevel, bidi: bidi,
		wordCache: make(map[string]float64),
	}
}

// lineMeasurerForFont builds the measurer that matches how AddText would
// wrap and draw text in this font/size: shaped when the font has GSUB/GPOS
// tables, otherwise the plain unshaped sum. For use by any caller that
// must measure or pre-wrap text before handing it to AddText — table row
// heights, Flow paragraphs, TOC entries, text flowing around floats, …—
// so the line breaks it predicts agree with what AddText actually draws.
//
// Known gap, not closed here (pdf-go-qxsz): unlike renderTextInBuilder,
// this does not apply the synthetic Arabic Presentation-Forms shaping
// pass (shapeArabic) for a font whose own GSUB doesn't shape Arabic, so
// a caller's prediction for such Arabic text can still disagree with
// what AddText draws — the same symptom class as GitHub issue #14, via a
// different, non-OpenType shaping path, pre-existing and left out of
// scope for that fix.
func lineMeasurerForFont(font Font, size float64, text string, rtl bool, width widthFn) *lineMeasurer {
	ef, ok := font.(*embeddedFont)
	if !ok || !fontHasShaping(ef.ttf) {
		return newLineMeasurer(width)
	}
	needBidi := rtl || bidiHasStrongRTL(text)
	baseLevel := 0
	if needBidi {
		baseLevel = bidiBaseLevel(text, rtl)
	}
	return newShapedLineMeasurer(ef.ttf, size, baseLevel, needBidi, width)
}

// shapedOrPlainWidth returns s's width exactly as AddText would draw it in
// this font/size: OpenType-shaped for a font with GSUB/GPOS tables,
// otherwise the plain per-rune sum. For callers that measure a single
// finished string (not a word-wrap decision) — TOC title/number widths,
// Markdown code lines and list labels — where the full lineMeasurer's
// word/space/exact split isn't needed.
func shapedOrPlainWidth(font Font, size float64, s string, width widthFn) float64 {
	if ef, ok := font.(*embeddedFont); ok && fontHasShaping(ef.ttf) {
		return shapeLine(ef.ttf, s, 0, false).width(size)
	}
	return measureString(s, width)
}

// word returns a fast width estimate for a single word (no internal
// whitespace): exact for an unshaped font, shaped-in-isolation (cached)
// for a shaped font — see lineMeasurer.
func (m *lineMeasurer) word(s string) float64 {
	if m.shaper == nil {
		return measureString(s, m.width)
	}
	if w, ok := m.wordCache[s]; ok {
		return w
	}
	w := shapeLine(m.shaper, s, 0, false).width(m.size)
	m.wordCache[s] = w
	return w
}

// space returns the fast width estimate for one inter-word space.
func (m *lineMeasurer) space() float64 {
	if m.shaper == nil {
		return m.width(' ')
	}
	if !m.spaceSet {
		m.spaceW = shapeLine(m.shaper, " ", 0, false).width(m.size)
		m.spaceSet = true
	}
	return m.spaceW
}

// exact returns a finished candidate line's real drawn width — exactly
// what renderTextInBuilder's shapeLine call will produce — so a caller can
// verify a greedily-packed line before committing to it. The shapedLine it
// computes along the way is cached (keyed by s) so the draw loop, which
// needs the same shapedLine to emit glyphs, doesn't reshape it a second
// time; see shapedLineFor.
func (m *lineMeasurer) exact(s string) float64 {
	if m.shaper == nil {
		return measureString(s, m.width)
	}
	if m.shapedCache != nil {
		if sl, ok := m.shapedCache[s]; ok {
			return sl.width(m.size)
		}
	} else {
		m.shapedCache = make(map[string]shapedLine)
	}
	sl := shapeLine(m.shaper, s, m.baseLevel, m.bidi)
	m.shapedCache[s] = sl
	return sl.width(m.size)
}

// shapedLineFor returns the shapedLine for a finished wrapped line,
// reusing the result exact() already computed for it during wrapping when
// available (so a line already measured once during wrapText isn't
// reshaped again at draw time).
func (m *lineMeasurer) shapedLineFor(line string) shapedLine {
	if m.shapedCache != nil {
		if sl, ok := m.shapedCache[line]; ok {
			return sl
		}
	}
	return shapeLine(m.shaper, line, m.baseLevel, m.bidi)
}

// encodeFn returns a PDF string operand for s — "(...)" for single-byte
// encoding, "<...>" for hex glyph IDs.
type encodeFn func(s string) string

// fontResolver registers a font into the caller's /Resources dict (when
// resources is non-nil) and returns the local resource name (e.g. "/F1")
// plus per-font callbacks. Page.AddText supplies a page-level resolver
// that delegates to p.ensureStandardFontResource /
// p.ensureEmbeddedFontResource; FreeText/Stamp /AP rendering will supply
// an XObject-level resolver in later tasks.
type fontResolver func(font Font, resources pdfDict) (resName string, width widthFn, encode encodeFn, ascent, descent float64, err error)

// renderTextInBuilder draws wrapped/aligned text into b. Font references
// are accumulated into resources["/Font"] via the resolver. The caller is
// responsible for rotation wrapping (style.Rotation) and the
// behind-content flag (style.Behind); this helper emits only the
// clipping / background / text / underline / strikethrough operators,
// wrapped in one q … Q block.
//
// textGSName and bgGSName are optional ExtGState resource names for
// fill-opacity transparency (empty string = opaque). Page-level callers
// set these up via ensureExtGState before calling; XObject callers pass
// empty strings.
func renderTextInBuilder(
	b *appearanceBuilder,
	resources pdfDict,
	text string,
	style TextStyle,
	rect Rectangle,
	resolve fontResolver,
	textGSName, bgGSName string,
) error {
	if text == "" {
		return nil
	}
	if err := rect.validate(); err != nil {
		return fmt.Errorf("render text: %w", err)
	}
	if err := checkFontSize("render text", style.Size); err != nil {
		return err
	}
	if nonFinite(style.Skew) {
		return fmt.Errorf("render text: skew must be finite, got %v", style.Skew)
	}

	font := style.Font
	if font == nil {
		font = FontHelvetica
	}
	text, err := applyMissingGlyphPolicy(font, text, style)
	if err != nil {
		return fmt.Errorf("render text: %w", err)
	}
	fontSize := style.Size
	if fontSize == 0 {
		fontSize = 12
	}
	lineSpacing := style.LineSpacing
	if lineSpacing == 0 {
		lineSpacing = 1.2
	}
	textColor := Color{R: 0, G: 0, B: 0, A: 1}
	if style.Color != nil {
		textColor = *style.Color
	}

	rectWidth := rect.URX - rect.LLX
	rectHeight := rect.URY - rect.LLY

	resName, width, encode, ascentFactor, _, err := resolve(font, resources)
	if err != nil {
		return err
	}

	// OpenType shaping (phase 3): an embedded font with GSUB/GPOS tables
	// shapes each line itself — contextual forms, ligatures, kerning, mark
	// positioning. Every other font keeps the rune-by-rune path.
	var shaper *embeddedFont
	if ef, ok := font.(*embeddedFont); ok && fontHasShaping(ef.ttf) {
		shaper = ef
	}

	// Arabic contextual shaping (phase 2): map letters to their Presentation
	// Forms-B glyphs in logical order, before wrapping and BiDi reordering —
	// unless the font's own GSUB chooses the forms, which would otherwise be
	// handed glyphs that are already shaped.
	if bidiHasArabic(text) && (shaper == nil || !fontShapesArabic(shaper.ttf)) {
		text = shapeArabic(text)
	}

	// Bidirectional (RTL) layout: when the text is explicitly RTL or contains
	// any strong right-to-left character, reorder each line into visual order
	// (so the left-to-right glyph emission below draws it correctly) and flip
	// the default alignment to the right for an RTL base paragraph. Computed
	// before wrapping (not just before the draw loop) so the lineMeasurer
	// below verifies wrapped lines' EXACT width with the same baseLevel/bidi
	// the draw loop itself uses — wrapping decisions must agree with what
	// gets drawn (GitHub issue #14).
	needBidi := style.RTL || bidiHasStrongRTL(text)
	baseLevel := 0
	if needBidi {
		baseLevel = bidiBaseLevel(text, style.RTL)
	}
	hAlign := style.HAlign
	if baseLevel%2 == 1 && hAlign == HAlignLeft {
		hAlign = HAlignRight
	}

	var m *lineMeasurer
	if shaper != nil {
		m = newShapedLineMeasurer(shaper.ttf, fontSize, baseLevel, needBidi, width)
	} else {
		m = newLineMeasurer(width)
	}
	lines := wrapText(text, m, rectWidth)
	if len(lines) == 0 {
		return nil
	}

	// Line height and total text height.
	lineHeight := fontSize * lineSpacing
	totalTextHeight := float64(len(lines)) * lineHeight

	// Vertical start position (top of first line, in PDF coordinates).
	var startY float64
	switch style.VAlign {
	case VAlignMiddle:
		startY = rect.URY - (rectHeight-totalTextHeight)/2
	case VAlignBottom:
		startY = rect.LLY + totalTextHeight
	default: // VAlignTop
		startY = rect.URY
	}

	// Save state.
	b.PushState()

	// Clipping path.
	b.buf.WriteString(fmt.Sprintf("%s %s %s %s re W n\n",
		formatFloat(rect.LLX), formatFloat(rect.LLY),
		formatFloat(rectWidth), formatFloat(rectHeight)))

	// Background fill.
	if style.Background != nil && style.Background.A > 0 {
		if bgGSName != "" {
			b.buf.WriteString(fmt.Sprintf("%s gs\n", bgGSName))
		}
		b.buf.WriteString(fmt.Sprintf("%s %s %s rg\n",
			formatFloat(style.Background.R), formatFloat(style.Background.G), formatFloat(style.Background.B)))
		b.buf.WriteString(fmt.Sprintf("%s %s %s %s re f\n",
			formatFloat(rect.LLX), formatFloat(rect.LLY),
			formatFloat(rectWidth), formatFloat(rectHeight)))
	}

	// Text opacity.
	if textGSName != "" {
		b.buf.WriteString(fmt.Sprintf("%s gs\n", textGSName))
	}

	// Text block.
	b.buf.WriteString("BT\n")
	if style.Invisible {
		b.buf.WriteString("3 Tr\n")
	}
	b.buf.WriteString(fmt.Sprintf("%s %s Tf\n", resName, formatFloat(fontSize)))
	b.buf.WriteString(fmt.Sprintf("%s %s %s rg\n",
		formatFloat(textColor.R), formatFloat(textColor.G), formatFloat(textColor.B)))

	// Track positions for underline/strikethrough.
	type linePos struct {
		x, y, width float64
	}
	var linePositions []linePos

	for i, line := range lines {
		if line == "" {
			continue
		}
		var shaped shapedLine
		var lineWidth float64
		if shaper != nil {
			shaped = m.shapedLineFor(line) // reuses the shapedLine wrapText already computed verifying this line's exact width, if any
			lineWidth = shaped.width(fontSize)
		} else {
			if needBidi {
				line = bidiVisualString(line, baseLevel)
			}
			lineWidth = measureString(line, width)
		}

		// Horizontal alignment.
		var x float64
		switch hAlign {
		case HAlignCenter:
			x = rect.LLX + (rectWidth-lineWidth)/2
		case HAlignRight:
			x = rect.LLX + (rectWidth - lineWidth)
		default: // HAlignLeft
			x = rect.LLX
		}

		// Baseline Y: top of line area minus ascent.
		ascent := ascentFactor * fontSize
		y := startY - float64(i)*lineHeight - ascent

		if style.Skew != 0 {
			// Synthetic oblique: an absolute text matrix with a shear
			// component per line, so glyphs slant about their own baseline
			// (browser-style faux italic) instead of the whole block
			// leaning.
			tan := math.Tan(style.Skew * math.Pi / 180)
			b.buf.WriteString(fmt.Sprintf("1 0 %s 1 %s %s Tm\n",
				formatFloat(tan), formatFloat(x), formatFloat(y)))
		} else if len(linePositions) == 0 {
			b.buf.WriteString(fmt.Sprintf("%s %s Td\n", formatFloat(x), formatFloat(y)))
		} else {
			prevX := linePositions[len(linePositions)-1].x
			prevY := linePositions[len(linePositions)-1].y
			b.buf.WriteString(fmt.Sprintf("%s %s Td\n", formatFloat(x-prevX), formatFloat(y-prevY)))
		}

		if shaper != nil {
			b.buf.WriteString(shaped.showOps(shaper, fontSize))
		} else {
			b.buf.WriteString(fmt.Sprintf("%s Tj\n", encode(line)))
		}
		linePositions = append(linePositions, linePos{x: x, y: y, width: lineWidth})
	}

	b.buf.WriteString("ET\n")

	// Underline. Suppressed for invisible text: the decorations are plain
	// fills, not glyphs, so mode 3 would not hide them.
	if style.Underline && !style.Invisible && len(linePositions) > 0 {
		b.buf.WriteString(fmt.Sprintf("%s %s %s rg\n",
			formatFloat(textColor.R), formatFloat(textColor.G), formatFloat(textColor.B)))
		thickness := fontSize * 0.05
		for _, lp := range linePositions {
			ulY := lp.y - fontSize*0.1
			b.buf.WriteString(fmt.Sprintf("%s %s %s %s re f\n",
				formatFloat(lp.x), formatFloat(ulY),
				formatFloat(lp.width), formatFloat(thickness)))
		}
	}

	// Strikethrough. Suppressed for invisible text (see Underline above).
	if style.Strikethrough && !style.Invisible && len(linePositions) > 0 {
		b.buf.WriteString(fmt.Sprintf("%s %s %s rg\n",
			formatFloat(textColor.R), formatFloat(textColor.G), formatFloat(textColor.B)))
		thickness := fontSize * 0.05
		for _, lp := range linePositions {
			stY := lp.y + fontSize*0.3
			b.buf.WriteString(fmt.Sprintf("%s %s %s %s re f\n",
				formatFloat(lp.x), formatFloat(stY),
				formatFloat(lp.width), formatFloat(thickness)))
		}
	}

	// Restore state.
	b.PopState()

	return nil
}

// wrapText splits text into lines that fit within maxWidth points, as
// measured by m. It breaks at spaces; words longer than maxWidth are
// broken on rune boundaries. Explicit newlines in the input force a line
// break.
//
// For a shaped font (m.shaper != nil) the greedy packing below uses m's
// fast per-word/per-space estimate, then verifies the finished line's
// EXACT shaped width (m.exact) and backs the last word off onto the next
// line if cross-word kerning pushed it over maxWidth — see lineMeasurer.
// For an unshaped font this reduces to exactly the old per-rune-sum
// wrapping (m.word/m.space/m.exact are all measureString, and the verify
// step is skipped since it can never disagree).
func wrapText(text string, m *lineMeasurer, maxWidth float64) []string {
	if text == "" {
		return nil
	}

	var result []string
	paragraphs := strings.Split(text, "\n")

	for _, para := range paragraphs {
		if para == "" {
			result = append(result, "")
			continue
		}
		words := strings.Fields(para)
		if len(words) == 0 {
			result = append(result, "")
			continue
		}

		var lineWidth float64
		var lineWords []string

		// finalizeLine commits the accumulated line to result. For a
		// shaped font it re-verifies the exact shaped width and pops
		// words off the end — each one reconsidered for the next line,
		// via rewinding wi — until what remains actually fits.
		var wi int
		finalizeLine := func() {
			kept := lineWords
			pushedBack := 0
			if m.shaper != nil {
				for {
					candidate := strings.Join(kept, " ")
					if m.exact(candidate) <= maxWidth || len(kept) <= 1 {
						break
					}
					kept = kept[:len(kept)-1]
					pushedBack++
				}
			}
			result = append(result, strings.Join(kept, " "))
			wi -= pushedBack
			lineWords, lineWidth = nil, 0
		}

		for wi = 0; wi < len(words) || len(lineWords) > 0; {
			if wi >= len(words) {
				finalizeLine()
				continue
			}
			word := words[wi]
			wordWidth := m.word(word)

			// len(lineWords) == 0, not lineWidth == 0, decides whether a
			// line is pending: a zero-ADVANCE word (ZWSP, an unmapped
			// glyph, a combining mark) must still occupy a slot in
			// lineWords, or it — and any line consisting only of such
			// words — is silently dropped instead of committed by
			// finalizeLine (code review of this fix).
			if len(lineWords) == 0 {
				if wordWidth <= maxWidth {
					lineWords = []string{word}
					lineWidth = wordWidth
					wi++
				} else {
					broken := breakWordMeasured(word, m, maxWidth)
					for i, part := range broken {
						if i < len(broken)-1 {
							result = append(result, part)
						} else {
							lineWords = []string{part}
							lineWidth = m.word(part)
						}
					}
					wi++
				}
			} else {
				spaceWidth := m.space()
				if lineWidth+spaceWidth+wordWidth <= maxWidth {
					lineWords = append(lineWords, word)
					lineWidth += spaceWidth + wordWidth
					wi++
				} else {
					// The line is full; finalize it (word is NOT consumed
					// — wi still points at it, or at an earlier word if
					// backoff pushed one back) and loop around to start a
					// fresh line at the current wi.
					finalizeLine()
				}
			}
		}
	}

	return result
}

// measureString returns the width of a string in points.
func measureString(s string, width widthFn) float64 {
	var w float64
	for _, r := range s {
		w += width(r)
	}
	return w
}

// breakWord breaks a single word into parts that each fit within maxWidth.
// Splits on rune boundaries so multi-byte UTF-8 is never cut mid-sequence.
func breakWord(word string, width widthFn, maxWidth float64) []string {
	var parts []string
	var buf strings.Builder
	var w float64
	for _, r := range word {
		cw := width(r)
		if w+cw > maxWidth && buf.Len() > 0 {
			parts = append(parts, buf.String())
			buf.Reset()
			w = 0
		}
		buf.WriteRune(r)
		w += cw
	}
	if buf.Len() > 0 {
		parts = append(parts, buf.String())
	}
	return parts
}

// breakWordMeasured is breakWord, but for a shaped font (m.shaper != nil)
// each growing candidate piece is checked by its real shaped width
// (m.word) instead of the plain per-rune sum — so a forced split still
// respects kerning/ligatures within the word (e.g. two letters that kern
// wider together than their unshaped advances sum to would otherwise both
// land in one piece that, once shaped, is still too wide for maxWidth;
// see GitHub issue #14). Falls back to plain breakWord for an unshaped
// font.
//
// Cost: this is only reached for a single word that alone exceeds
// maxWidth — rare in prose. Each candidate's own length is capped by
// however many runes fit under maxWidth (buf resets on every flush), not
// by the word's total length, so a long unbroken shaped token (e.g. a URL
// with no spaces) costs one bounded-length shaped-width call per rune —
// O(n) shape calls for an n-rune word, same order as breakWord's O(n)
// plain-sum calls, just each one more expensive (code review of this
// fix corrected an earlier, overstated O(n²) claim here).
//
// A single glyph whose own shaped width already exceeds maxWidth is
// still emitted alone (the `buf.Len() > 0` guard never fires for it) and
// overflows when drawn — inherent, not something any wrap-level fix can
// correct, same as breakWord's pre-existing behavior for an oversized
// rune.
func breakWordMeasured(word string, m *lineMeasurer, maxWidth float64) []string {
	if m.shaper == nil {
		return breakWord(word, m.width, maxWidth)
	}
	var parts []string
	var buf strings.Builder
	for _, r := range word {
		if buf.Len() > 0 && m.word(buf.String()+string(r)) > maxWidth {
			parts = append(parts, buf.String())
			buf.Reset()
		}
		buf.WriteRune(r)
	}
	if buf.Len() > 0 {
		parts = append(parts, buf.String())
	}
	return parts
}

// escapeStringPDF escapes special characters for a PDF literal string.
func escapeStringPDF(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '(':
			b.WriteString("\\(")
		case ')':
			b.WriteString("\\)")
		case '\\':
			b.WriteString("\\\\")
		default:
			b.WriteByte(s[i])
		}
	}
	return b.String()
}

// fontWidthAndAscent returns a per-glyph width function and the ascent
// factor (ascent / unitsPerEm) for a Font, without any side effects on a
// Page. Used by measureText (which has no Page) and informs the values
// returned by resolveFontForPage.
func fontWidthAndAscent(font Font, size float64) (widthFn, float64, error) {
	switch f := font.(type) {
	case standardFont:
		pdfFontName := "/" + f.name
		widths, _ := standard14Widths(pdfFontName)
		encodeRune := func(r rune) (byte, bool) {
			return encodeRuneForStandardFont(pdfFontName, r)
		}
		width := func(r rune) float64 {
			code, ok := encodeRune(r)
			if !ok {
				code = byte('?')
			}
			return widths[code] / 1000.0 * size
		}
		return width, 0.8, nil

	case *embeddedFont:
		width := func(r rune) float64 {
			gid := f.ttf.glyphID(r)
			if int(gid) >= len(f.ttf.glyphWidths) {
				return 0
			}
			return float64(f.ttf.glyphWidths[gid]) / float64(f.ttf.unitsPerEm) * size
		}
		ascentVal := float64(f.ttf.ascent) / float64(f.ttf.unitsPerEm)
		return width, ascentVal, nil

	case *Type3Font:
		return f.widthFn(size), 0.8, nil

	default:
		return nil, 0, fmt.Errorf("font: unsupported type %T", font)
	}
}

// measureText returns the number of wrapped lines and the per-line height
// (in points) that AddText would produce for the given text + style +
// maxWidth.
//
// Used by table rendering to compute auto-fit row heights without first
// committing the text to the page.
func measureText(text string, style TextStyle, maxWidth float64) (lines int, lineHeight float64, err error) {
	if text == "" {
		return 0, 0, nil
	}
	font := style.Font
	if font == nil {
		font = FontHelvetica
	}
	fontSize := style.Size
	if fontSize == 0 {
		fontSize = 12
	}
	lineSpacing := style.LineSpacing
	if lineSpacing == 0 {
		lineSpacing = 1.2
	}
	width, _, err := fontWidthAndAscent(font, fontSize)
	if err != nil {
		return 0, 0, err
	}
	m := lineMeasurerForFont(font, fontSize, text, style.RTL, width)
	wrapped := wrapText(text, m, maxWidth)
	return len(wrapped), fontSize * lineSpacing, nil
}

// resolveFontForPage handles page-level font registration and returns the
// callbacks needed by renderTextInBuilder. It is extracted from the
// original AddText monolith so that AddText becomes a thin wrapper.
func (p *Page) resolveFontForPage(font Font, size float64) (resName string, width widthFn, encode encodeFn, ascent, descent float64, err error) {
	switch f := font.(type) {
	case standardFont:
		pdfFontName := "/" + f.name
		widths, _ := standard14Widths(pdfFontName)
		// Symbol and ZapfDingbats use their built-in encodings (widths and
		// byte codes share the same native positions). Everything else uses
		// WinAnsi, which matches the /Encoding we set on the font resource.
		encodeRune := func(r rune) (byte, bool) {
			return encodeRuneForStandardFont(pdfFontName, r)
		}
		width = func(r rune) float64 {
			code, ok := encodeRune(r)
			if !ok {
				code = byte('?')
			}
			return widths[code] / 1000.0 * size
		}
		encode = func(s string) string {
			var b strings.Builder
			b.WriteByte('(')
			for _, r := range s {
				code, ok := encodeRune(r)
				if !ok {
					code = byte('?')
				}
				switch code {
				case '(', ')', '\\':
					b.WriteByte('\\')
				}
				b.WriteByte(code)
			}
			b.WriteByte(')')
			return b.String()
		}
		name, e := p.ensureStandardFontResource(pdfFontName)
		if e != nil {
			return "", nil, nil, 0, 0, e
		}
		return name, width, encode, 0.8, 0, nil

	case *embeddedFont:
		width = func(r rune) float64 {
			gid := f.ttf.glyphID(r)
			if int(gid) >= len(f.ttf.glyphWidths) {
				return 0
			}
			return float64(f.ttf.glyphWidths[gid]) / float64(f.ttf.unitsPerEm) * size
		}
		encode = func(s string) string {
			var b strings.Builder
			b.WriteByte('<')
			for _, r := range s {
				gid := f.ttf.glyphID(r)
				f.useGlyph(gid)
				fmt.Fprintf(&b, "%04X", gid)
			}
			b.WriteByte('>')
			return b.String()
		}
		name, e := p.ensureEmbeddedFontResource(f)
		if e != nil {
			return "", nil, nil, 0, 0, e
		}
		ascentVal := float64(f.ttf.ascent) / float64(f.ttf.unitsPerEm)
		return name, width, encode, ascentVal, 0, nil

	case *Type3Font:
		name, e := p.ensureType3FontResource(f)
		if e != nil {
			return "", nil, nil, 0, 0, e
		}
		return name, f.widthFn(size), f.encodeString, 0.8, 0, nil

	default:
		return "", nil, nil, 0, 0, fmt.Errorf("add text: unsupported font type %T", font)
	}
}

// AddText draws text inside the rectangle using the given style.
// Text is wrapped at word boundaries to fit the rectangle width.
// Content exceeding the rectangle height is clipped.
func (p *Page) AddText(text string, style TextStyle, rect Rectangle) error {
	if text == "" {
		return nil
	}
	// Checked here, not only in appendToContentStream below: font
	// resolution (resolveFontForPage, a few lines down) registers the
	// font into /Resources and, via the embedded-font encode closure's
	// useGlyph call, permanently marks glyphs used in the FONT OBJECT
	// ITSELF (a document-wide resource, affecting /ToUnicode and /W at
	// save time) — both happen before this method ever reaches its own
	// appendToContentStream call, so a frozen-canvas rejection there
	// alone would still leave those traces of rejected content in the
	// saved file (GitHub issue #20 point 2, shadowhunter1470-svg).
	if err := p.checkCanvasNotFrozen(); err != nil {
		return err
	}
	if err := rect.validate(); err != nil {
		return fmt.Errorf("add text: %w", err)
	}
	if err := checkFontSize("add text", style.Size); err != nil {
		return err
	}
	if nonFinite(style.Rotation) {
		return fmt.Errorf("add text: rotation must be finite, got %v", style.Rotation)
	}

	fontSize := style.Size
	if fontSize == 0 {
		fontSize = 12
	}

	// Build the page-level font resolver closure.
	resolve := func(f Font, _ pdfDict) (string, widthFn, encodeFn, float64, float64, error) {
		return p.resolveFontForPage(f, fontSize)
	}

	// Set up ExtGState resources for transparency (page-level concern).
	textColor := Color{R: 0, G: 0, B: 0, A: 1}
	if style.Color != nil {
		textColor = *style.Color
	}
	var textGSName, bgGSName string
	if textColor.A < 1 {
		name, err := p.ensureExtGState(textColor.A)
		if err != nil {
			return err
		}
		textGSName = name
	}
	if style.Background != nil && style.Background.A > 0 && style.Background.A < 1 {
		name, err := p.ensureExtGState(style.Background.A)
		if err != nil {
			return err
		}
		bgGSName = name
	}

	// Build content stream operators.
	var buf strings.Builder

	// Save state + optional rotation transform.
	buf.WriteString("\nq\n")

	if style.Rotation != 0 {
		// Translate origin to pivot point (LLX, LLY), then rotate.
		buf.WriteString(fmt.Sprintf("1 0 0 1 %s %s cm\n",
			formatFloat(rect.LLX), formatFloat(rect.LLY)))
		rad := style.Rotation * math.Pi / 180.0
		cos := math.Cos(rad)
		sin := math.Sin(rad)
		buf.WriteString(fmt.Sprintf("%s %s %s %s 0 0 cm\n",
			formatFloat(cos), formatFloat(sin), formatFloat(-sin), formatFloat(cos)))
	}

	// When rotation is active, the cm transform already translates the
	// coordinate origin to (rect.LLX, rect.LLY). renderTextInBuilder must
	// therefore use pivot-relative coordinates starting at (0, 0); otherwise
	// the offset would be applied twice (double-translation bug).
	renderRect := rect
	if style.Rotation != 0 {
		width := rect.URX - rect.LLX
		height := rect.URY - rect.LLY
		renderRect = Rectangle{LLX: 0, LLY: 0, URX: width, URY: height}
	}

	// Render the text body into a sub-builder, then embed it.
	b := newAppearanceBuilder()
	if err := renderTextInBuilder(b, pdfDict{}, text, style, renderRect, resolve, textGSName, bgGSName); err != nil {
		return err
	}
	buf.Write(b.Bytes())

	// Close outer state.
	buf.WriteString("Q\n")

	if style.Behind {
		return p.prependToContentStream([]byte(buf.String()))
	}
	return p.appendToContentStream([]byte(buf.String()))
}

// AddTextWatermark adds a text watermark to selected pages of the document.
// If no page numbers are given, the watermark is applied to all pages.
// Page numbers are 1-based. The watermark covers the full page area (MediaBox).
// The caller controls all styling via TextStyle (rotation, behind, color, etc.).
func (d *Document) AddTextWatermark(text string, style TextStyle, pageNums ...int) error {
	if text == "" {
		return nil
	}
	indices, err := resolvePageIndices(len(d.pages), pageNums)
	if err != nil {
		return fmt.Errorf("add text watermark: %w", err)
	}
	for _, i := range indices {
		page := &Page{doc: d, index: i}
		size, err := page.Size()
		if err != nil {
			return fmt.Errorf("add text watermark: page %d: %w", i+1, err)
		}
		rect := Rectangle{LLX: 0, LLY: 0, URX: size.Width, URY: size.Height}
		if err := page.AddText(text, style, rect); err != nil {
			return fmt.Errorf("add text watermark: page %d: %w", i+1, err)
		}
	}
	return nil
}

// prependToContentStream is defined in content_append.go.

// ensureStandardFontResource registers a Type1 font in the page's /Resources /Font dict.
// If a font with the same /BaseFont already exists, its resource name is returned.
func (p *Page) ensureStandardFontResource(pdfFontName string) (string, error) {
	pageDict := p.pageDict()
	if pageDict == nil {
		return "", fmt.Errorf("add text: page has no dict")
	}

	resources := p.pageResources()
	if resources == nil {
		resources = pdfDict{}
		pageDict["/Resources"] = resources
	}

	fontVal := resolveRef(p.doc.objects, resources["/Font"])
	fontDict, _ := fontVal.(pdfDict)
	if fontDict == nil {
		fontDict = pdfDict{}
		resources["/Font"] = fontDict
	}

	// Check if font with this BaseFont already registered.
	for name, val := range fontDict {
		ref, ok := val.(pdfRef)
		if !ok {
			continue
		}
		obj := p.doc.objects[ref.Num]
		if obj == nil {
			continue
		}
		dict, ok := obj.Value.(pdfDict)
		if !ok {
			continue
		}
		if bf, ok := dict["/BaseFont"].(pdfName); ok && string(bf) == pdfFontName {
			return name, nil
		}
	}

	// Create new font object. /Encoding must match how AddText encodes strings
	// (WinAnsi); without it viewers fall back to the font's built-in encoding
	// (StandardEncoding for most), where several WinAnsi bytes — e.g. 0x97 em-dash
	// — are undefined and render as a missing glyph. Symbol and ZapfDingbats
	// keep their built-in encodings — WinAnsi is not applicable there.
	fontObjDict := pdfDict{
		"/Type":     pdfName("/Font"),
		"/Subtype":  pdfName("/Type1"),
		"/BaseFont": pdfName(pdfFontName),
	}
	if pdfFontName != "/Symbol" && pdfFontName != "/ZapfDingbats" {
		fontObjDict["/Encoding"] = pdfName("/WinAnsiEncoding")
	}
	fontID := p.doc.nextID
	p.doc.nextID++
	p.doc.objects[fontID] = &pdfObject{Num: fontID, Value: fontObjDict}

	name := nextFontName(fontDict)
	fontDict[name] = pdfRef{Num: fontID}

	return name, nil
}

// ensureEmbeddedFontResource registers an already-embedded font (created by LoadFont)
// in the page's /Resources /Font dict and returns the resource name.
func (p *Page) ensureEmbeddedFontResource(ef *embeddedFont) (string, error) {
	if ef.doc != p.doc {
		return "", fmt.Errorf("add text: font was loaded into a different document")
	}
	pageDict := p.pageDict()
	if pageDict == nil {
		return "", fmt.Errorf("add text: page has no dict")
	}
	resources := p.pageResources()
	if resources == nil {
		resources = pdfDict{}
		pageDict["/Resources"] = resources
	}
	fontVal := resolveRef(p.doc.objects, resources["/Font"])
	fontDict, _ := fontVal.(pdfDict)
	if fontDict == nil {
		fontDict = pdfDict{}
		resources["/Font"] = fontDict
	}

	for name, val := range fontDict {
		if ref, ok := val.(pdfRef); ok && ref.Num == ef.fontObjectID {
			return name, nil
		}
	}
	name := nextFontName(fontDict)
	fontDict[name] = pdfRef{Num: ef.fontObjectID}
	return name, nil
}

// nextFontName returns the next available font resource name.
func nextFontName(fontDict pdfDict) string {
	for i := 0; ; i++ {
		name := fmt.Sprintf("/F%d", i)
		if _, exists := fontDict[name]; !exists {
			return name
		}
	}
}

// ensureExtGState registers an ExtGState with the given opacity for both
// stroke (/CA) and fill (/ca) — same value for both, matching the common
// case of "draw this entire shape semi-transparent". For per-property
// transparency, a future helper can split.
func (p *Page) ensureExtGState(alpha float64) (string, error) {
	pageDict := p.pageDict()
	if pageDict == nil {
		return "", fmt.Errorf("add text: page has no dict")
	}

	resources := p.pageResources()
	if resources == nil {
		resources = pdfDict{}
		pageDict["/Resources"] = resources
	}

	gsVal := resolveRef(p.doc.objects, resources["/ExtGState"])
	gsDict, _ := gsVal.(pdfDict)
	if gsDict == nil {
		gsDict = pdfDict{}
		resources["/ExtGState"] = gsDict
	}

	// Check if an ExtGState with matching /ca AND /CA already exists.
	for name, val := range gsDict {
		ref, ok := val.(pdfRef)
		if !ok {
			continue
		}
		obj := p.doc.objects[ref.Num]
		if obj == nil {
			continue
		}
		dict, ok := obj.Value.(pdfDict)
		if !ok {
			continue
		}
		caOK, ca := false, 0.0
		if v, err := toFloat(dict["/ca"]); err == nil {
			ca = v
			caOK = true
		}
		bigCAOK, bigCA := false, 0.0
		if v, err := toFloat(dict["/CA"]); err == nil {
			bigCA = v
			bigCAOK = true
		}
		if caOK && bigCAOK && ca == alpha && bigCA == alpha {
			return name, nil
		}
	}

	gsObjDict := pdfDict{
		"/ca": alpha,
		"/CA": alpha,
	}
	gsID := p.doc.nextID
	p.doc.nextID++
	p.doc.objects[gsID] = &pdfObject{Num: gsID, Value: gsObjDict}

	name := nextGSName(gsDict)
	gsDict[name] = pdfRef{Num: gsID}

	return name, nil
}

// nextGSName returns the next available ExtGState resource name.
func nextGSName(gsDict pdfDict) string {
	for i := 0; ; i++ {
		name := fmt.Sprintf("/GS%d", i)
		if _, exists := gsDict[name]; !exists {
			return name
		}
	}
}

// ensurePatternResource registers the gradient as a PDF shading pattern in the page's
// /Resources/Pattern dictionary and returns the resource name (e.g. "/P0").
//
// It mirrors ensureExtGState: allocates indirect object numbers via p.doc.nextID,
// stores objects in p.doc.objects, and inserts pdfRef values into the page resources.
//
// The gradient's /Function is registered as one indirect object; the /Shading dict
// as another; the /Pattern dict as a third. The matrix transforms the shading
// coordinate space into page user space (passed as the PDF /Matrix entry).
//
// Returns "" and an error if the gradient type is unsupported or the page has no dict.
func (p *Page) ensurePatternResource(grad svgGradient, matrix svgMatrix) (string, error) {
	pageDict := p.pageDict()
	if pageDict == nil {
		return "", fmt.Errorf("ensurePatternResource: page has no dict")
	}

	resources := p.pageResources()
	if resources == nil {
		resources = pdfDict{}
		pageDict["/Resources"] = resources
	}

	// Ensure /Pattern subdict exists.
	patVal := resolveRef(p.doc.objects, resources["/Pattern"])
	patDict, _ := patVal.(pdfDict)
	if patDict == nil {
		patDict = pdfDict{}
		resources["/Pattern"] = patDict
	}

	// --- Register the function object as an indirect object. ---
	var stops []svgGradientStop
	switch g := grad.(type) {
	case *svgLinearGradient:
		stops = g.stops
	case *svgRadialGradient:
		stops = g.stops
	default:
		return "", fmt.Errorf("ensurePatternResource: unsupported gradient type %T", grad)
	}

	fnObj := buildShadingFunction(stops)
	fnID := p.doc.nextID
	p.doc.nextID++
	fnObj.Num = fnID
	p.doc.objects[fnID] = fnObj

	// --- Build and register the /Shading indirect object. ---
	// We bake `matrix` (gradientUnits + gradientTransform) directly into the
	// shading's /Coords and omit /Matrix on the parent /Pattern dict — this
	// avoids cross-viewer ambiguity around how /Matrix composes with the CTM
	// for Type 3 shadings. See gradientToShadingObject for the full rationale.
	shadingObj := gradientToShadingObject(grad, pdfRef{Num: fnID}, matrix)
	if shadingObj == nil {
		return "", fmt.Errorf("ensurePatternResource: unsupported gradient type %T", grad)
	}
	shadingID := p.doc.nextID
	p.doc.nextID++
	shadingObj.Num = shadingID
	p.doc.objects[shadingID] = shadingObj

	// --- Build and register the /Pattern (shading pattern, PatternType 2) indirect object. ---
	// /Matrix is identity because gradientUnits + gradientTransform are already
	// baked into the shading's /Coords. We still emit identity explicitly: PDF
	// spec lists /Matrix as optional with identity default, but some viewers
	// behave differently when it's omitted, and "[1 0 0 1 0 0]" is unambiguous.
	patternDict := pdfDict{
		"/Type":        pdfName("/Pattern"),
		"/PatternType": 2,
		"/Shading":     pdfRef{Num: shadingID},
		"/Matrix":      pdfArray{1.0, 0.0, 0.0, 1.0, 0.0, 0.0},
	}
	patternID := p.doc.nextID
	p.doc.nextID++
	p.doc.objects[patternID] = &pdfObject{Num: patternID, Value: patternDict}

	// --- Insert into /Resources/Pattern. ---
	name := nextPatternName(patDict)
	patDict[name] = pdfRef{Num: patternID}

	return name, nil
}

// nextPatternName returns the next available Pattern resource name (/P0, /P1, …).
func nextPatternName(patDict pdfDict) string {
	for i := 0; ; i++ {
		name := fmt.Sprintf("/P%d", i)
		if _, exists := patDict[name]; !exists {
			return name
		}
	}
}

// resolveFontForXObject is the fontResolver variant for XObject /AP
// contexts. Registers the font in the supplied resources dict
// (resources["/Font"][resName] = pdfRef{...}) instead of the page's
// /Resources. Returns the local resource name, width/encode callbacks,
// and ascent/descent metrics — same shape as resolveFontForPage.
func resolveFontForXObject(font Font, size float64, doc *Document, resources pdfDict) (resName string, width widthFn, encode encodeFn, ascent, descent float64, err error) {
	// Ensure /Font subdict exists in the supplied resources.
	fontDict, _ := resources["/Font"].(pdfDict)
	if fontDict == nil {
		fontDict = pdfDict{}
		resources["/Font"] = fontDict
	}

	switch f := font.(type) {
	case standardFont:
		pdfFontName := "/" + f.name
		widths, _ := standard14Widths(pdfFontName)
		encodeRune := func(r rune) (byte, bool) {
			return encodeRuneForStandardFont(pdfFontName, r)
		}
		width = func(r rune) float64 {
			code, ok := encodeRune(r)
			if !ok {
				code = byte('?')
			}
			return widths[code] / 1000.0 * size
		}
		encode = func(s string) string {
			var b strings.Builder
			b.WriteByte('(')
			for _, r := range s {
				code, ok := encodeRune(r)
				if !ok {
					code = byte('?')
				}
				switch code {
				case '(', ')', '\\':
					b.WriteByte('\\')
				}
				b.WriteByte(code)
			}
			b.WriteByte(')')
			return b.String()
		}

		// Check if this BaseFont is already registered in the XObject resources.
		for name, val := range fontDict {
			ref, ok := val.(pdfRef)
			if !ok {
				continue
			}
			obj := doc.objects[ref.Num]
			if obj == nil {
				continue
			}
			dict, ok := obj.Value.(pdfDict)
			if !ok {
				continue
			}
			if bf, ok := dict["/BaseFont"].(pdfName); ok && string(bf) == pdfFontName {
				return name, width, encode, 0.8, 0, nil
			}
		}

		// Create new font object in doc.objects.
		fontObjDict := pdfDict{
			"/Type":     pdfName("/Font"),
			"/Subtype":  pdfName("/Type1"),
			"/BaseFont": pdfName(pdfFontName),
		}
		if pdfFontName != "/Symbol" && pdfFontName != "/ZapfDingbats" {
			fontObjDict["/Encoding"] = pdfName("/WinAnsiEncoding")
		}
		fontID := doc.nextID
		doc.nextID++
		doc.objects[fontID] = &pdfObject{Num: fontID, Value: fontObjDict}

		name := nextFontName(fontDict)
		fontDict[name] = pdfRef{Num: fontID}
		return name, width, encode, 0.8, 0, nil

	case *embeddedFont:
		if f.doc != doc {
			return "", nil, nil, 0, 0, fmt.Errorf("resolve font: font was loaded into a different document")
		}
		width = func(r rune) float64 {
			gid := f.ttf.glyphID(r)
			if int(gid) >= len(f.ttf.glyphWidths) {
				return 0
			}
			return float64(f.ttf.glyphWidths[gid]) / float64(f.ttf.unitsPerEm) * size
		}
		encode = func(s string) string {
			var b strings.Builder
			b.WriteByte('<')
			for _, r := range s {
				gid := f.ttf.glyphID(r)
				f.useGlyph(gid)
				fmt.Fprintf(&b, "%04X", gid)
			}
			b.WriteByte('>')
			return b.String()
		}

		// Check if already registered in XObject resources.
		for name, val := range fontDict {
			if ref, ok := val.(pdfRef); ok && ref.Num == f.fontObjectID {
				ascentVal := float64(f.ttf.ascent) / float64(f.ttf.unitsPerEm)
				return name, width, encode, ascentVal, 0, nil
			}
		}
		name := nextFontName(fontDict)
		fontDict[name] = pdfRef{Num: f.fontObjectID}
		ascentVal := float64(f.ttf.ascent) / float64(f.ttf.unitsPerEm)
		return name, width, encode, ascentVal, 0, nil

	case *Type3Font:
		if f.doc != doc {
			return "", nil, nil, 0, 0, fmt.Errorf("resolve font: type3 font belongs to a different document")
		}
		id, e := f.ensureBuilt()
		if e != nil {
			return "", nil, nil, 0, 0, e
		}
		name := fmt.Sprintf("/T3F%d", id)
		fontDict[name] = pdfRef{Num: id}
		return name, f.widthFn(size), f.encodeString, 0.8, 0, nil

	default:
		return "", nil, nil, 0, 0, fmt.Errorf("resolve font: unsupported font type %T", font)
	}
}
