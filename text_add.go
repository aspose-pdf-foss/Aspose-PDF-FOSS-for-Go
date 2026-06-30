// SPDX-License-Identifier: MIT

package asposepdf

import (
	"fmt"
	"math"
	"strings"
)

// widthFn returns advance width in points for a single rune.
type widthFn func(r rune) float64

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
	if style.Size < 0 {
		return fmt.Errorf("render text: font size must be non-negative, got %g", style.Size)
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

	lines := wrapText(text, width, rectWidth)
	if len(lines) == 0 {
		return nil
	}

	// Bidirectional (RTL) layout: when the text is explicitly RTL or contains
	// any strong right-to-left character, reorder each line into visual order
	// (so the left-to-right glyph emission below draws it correctly) and flip
	// the default alignment to the right for an RTL base paragraph.
	needBidi := style.RTL || bidiHasStrongRTL(text)
	baseLevel := 0
	if needBidi {
		baseLevel = bidiBaseLevel(text, style.RTL)
	}
	hAlign := style.HAlign
	if baseLevel%2 == 1 && hAlign == HAlignLeft {
		hAlign = HAlignRight
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
		if needBidi {
			line = bidiVisualString(line, baseLevel)
		}
		lineWidth := measureString(line, width)

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

		if len(linePositions) == 0 {
			b.buf.WriteString(fmt.Sprintf("%s %s Td\n", formatFloat(x), formatFloat(y)))
		} else {
			prevX := linePositions[len(linePositions)-1].x
			prevY := linePositions[len(linePositions)-1].y
			b.buf.WriteString(fmt.Sprintf("%s %s Td\n", formatFloat(x-prevX), formatFloat(y-prevY)))
		}

		b.buf.WriteString(fmt.Sprintf("%s Tj\n", encode(line)))
		linePositions = append(linePositions, linePos{x: x, y: y, width: lineWidth})
	}

	b.buf.WriteString("ET\n")

	// Underline.
	if style.Underline && len(linePositions) > 0 {
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

	// Strikethrough.
	if style.Strikethrough && len(linePositions) > 0 {
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

// wrapText splits text into lines that fit within maxWidth points.
// It breaks at spaces; words longer than maxWidth are broken on rune boundaries.
// Explicit newlines in the input force a line break.
func wrapText(text string, width widthFn, maxWidth float64) []string {
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

		var line string
		var lineWidth float64

		for _, word := range words {
			wordWidth := measureString(word, width)

			if lineWidth == 0 {
				if wordWidth <= maxWidth {
					line = word
					lineWidth = wordWidth
				} else {
					broken := breakWord(word, width, maxWidth)
					for i, part := range broken {
						if i < len(broken)-1 {
							result = append(result, part)
						} else {
							line = part
							lineWidth = measureString(part, width)
						}
					}
				}
			} else {
				spaceWidth := width(' ')
				if lineWidth+spaceWidth+wordWidth <= maxWidth {
					line += " " + word
					lineWidth += spaceWidth + wordWidth
				} else {
					result = append(result, line)
					if wordWidth <= maxWidth {
						line = word
						lineWidth = wordWidth
					} else {
						broken := breakWord(word, width, maxWidth)
						for i, part := range broken {
							if i < len(broken)-1 {
								result = append(result, part)
							} else {
								line = part
								lineWidth = measureString(part, width)
							}
						}
					}
				}
			}
		}
		if line != "" || lineWidth == 0 {
			result = append(result, line)
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
	wrapped := wrapText(text, width, maxWidth)
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
	if err := rect.validate(); err != nil {
		return fmt.Errorf("add text: %w", err)
	}
	if style.Size < 0 {
		return fmt.Errorf("add text: font size must be non-negative, got %g", style.Size)
	}

	// Default Font if unset.
	font := style.Font
	if font == nil {
		font = FontHelvetica
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

// prependToContentStream inserts data before the existing page content.
func (p *Page) prependToContentStream(data []byte) error {
	existing, err := p.contentStreams()
	if err != nil {
		return err
	}

	newData := make([]byte, 0, len(data)+len(existing))
	newData = append(newData, data...)
	newData = append(newData, existing...)
	newStream := &pdfStream{
		Dict:    pdfDict{},
		Data:    newData,
		Decoded: true,
	}

	newID := p.doc.nextID
	p.doc.nextID++
	p.doc.objects[newID] = &pdfObject{Num: newID, Value: newStream}

	pageDict := p.pageDict()
	pageDict["/Contents"] = pdfRef{Num: newID}
	return nil
}

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

	default:
		return "", nil, nil, 0, 0, fmt.Errorf("resolve font: unsupported font type %T", font)
	}
}
