package asposepdf

import (
	"fmt"
	"math"
	"strings"
)

// wrapText splits text into lines that fit within maxWidth points.
// It breaks at spaces; words longer than maxWidth are broken by character.
// Explicit newlines in the input force a line break.
func wrapText(text string, widths [256]float64, fontSize, maxWidth float64) []string {
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
			wordWidth := measureString(word, widths, fontSize)

			if lineWidth == 0 {
				// First word on line.
				if wordWidth <= maxWidth {
					line = word
					lineWidth = wordWidth
				} else {
					// Word too long — break by character.
					broken := breakWord(word, widths, fontSize, maxWidth)
					for i, part := range broken {
						if i < len(broken)-1 {
							result = append(result, part)
						} else {
							line = part
							lineWidth = measureString(part, widths, fontSize)
						}
					}
				}
			} else {
				spaceWidth := widths[' '] / 1000.0 * fontSize
				if lineWidth+spaceWidth+wordWidth <= maxWidth {
					line += " " + word
					lineWidth += spaceWidth + wordWidth
				} else {
					result = append(result, line)
					if wordWidth <= maxWidth {
						line = word
						lineWidth = wordWidth
					} else {
						broken := breakWord(word, widths, fontSize, maxWidth)
						for i, part := range broken {
							if i < len(broken)-1 {
								result = append(result, part)
							} else {
								line = part
								lineWidth = measureString(part, widths, fontSize)
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
func measureString(s string, widths [256]float64, fontSize float64) float64 {
	var w float64
	for i := 0; i < len(s); i++ {
		w += widths[s[i]] / 1000.0 * fontSize
	}
	return w
}

// breakWord breaks a single word into parts that each fit within maxWidth.
func breakWord(word string, widths [256]float64, fontSize, maxWidth float64) []string {
	var parts []string
	start := 0
	var w float64
	for i := 0; i < len(word); i++ {
		cw := widths[word[i]] / 1000.0 * fontSize
		if w+cw > maxWidth && i > start {
			parts = append(parts, word[start:i])
			start = i
			w = 0
		}
		w += cw
	}
	if start < len(word) {
		parts = append(parts, word[start:])
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

// isValidFont returns true if f is one of the 14 standard font constants.
func isValidFont(f Font) bool {
	return f >= FontHelvetica && f <= FontZapfDingbats
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
	if !isValidFont(style.Font) {
		return fmt.Errorf("add text: invalid font constant %d", style.Font)
	}

	// Apply defaults.
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

	// Get font metrics.
	pdfFontName := fontPDFName(style.Font)
	widths, _ := standard14Widths(pdfFontName)

	// Word wrap.
	rectWidth := rect.URX - rect.LLX
	rectHeight := rect.URY - rect.LLY
	lines := wrapText(text, widths, fontSize, rectWidth)
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

	// Register font resource.
	fontResName, err := p.ensureFontResource(pdfFontName)
	if err != nil {
		return err
	}

	// Register ExtGState resources if needed.
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

	// Coordinate offsets: when rotated, all positions are relative to pivot (0,0).
	// When not rotated, positions are absolute.
	ox := 0.0 // offset X to subtract from all coordinates
	oy := 0.0 // offset Y to subtract from all coordinates

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
		// All subsequent coordinates are relative to pivot.
		ox = rect.LLX
		oy = rect.LLY
	}

	// Clipping path.
	buf.WriteString(fmt.Sprintf("%s %s %s %s re W n\n",
		formatFloat(rect.LLX-ox), formatFloat(rect.LLY-oy),
		formatFloat(rectWidth), formatFloat(rectHeight)))

	// Background fill.
	if style.Background != nil && style.Background.A > 0 {
		if bgGSName != "" {
			buf.WriteString(fmt.Sprintf("%s gs\n", bgGSName))
		}
		buf.WriteString(fmt.Sprintf("%s %s %s rg\n",
			formatFloat(style.Background.R), formatFloat(style.Background.G), formatFloat(style.Background.B)))
		buf.WriteString(fmt.Sprintf("%s %s %s %s re f\n",
			formatFloat(rect.LLX-ox), formatFloat(rect.LLY-oy),
			formatFloat(rectWidth), formatFloat(rectHeight)))
	}

	// Text opacity.
	if textGSName != "" {
		buf.WriteString(fmt.Sprintf("%s gs\n", textGSName))
	}

	// Text block.
	buf.WriteString("BT\n")
	buf.WriteString(fmt.Sprintf("%s %s Tf\n", fontResName, formatFloat(fontSize)))
	buf.WriteString(fmt.Sprintf("%s %s %s rg\n",
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
		lineWidth := measureString(line, widths, fontSize)

		// Horizontal alignment.
		var x float64
		switch style.HAlign {
		case HAlignCenter:
			x = rect.LLX + (rectWidth-lineWidth)/2
		case HAlignRight:
			x = rect.LLX + (rectWidth - lineWidth)
		default: // HAlignLeft
			x = rect.LLX
		}

		// Baseline Y: top of line area minus ascent.
		ascent := 0.8 * fontSize
		y := startY - float64(i)*lineHeight - ascent

		// Apply coordinate offset for rotation.
		adjX := x - ox
		adjY := y - oy

		if len(linePositions) == 0 {
			buf.WriteString(fmt.Sprintf("%s %s Td\n", formatFloat(adjX), formatFloat(adjY)))
		} else {
			prevX := linePositions[len(linePositions)-1].x
			prevY := linePositions[len(linePositions)-1].y
			buf.WriteString(fmt.Sprintf("%s %s Td\n", formatFloat(adjX-prevX), formatFloat(adjY-prevY)))
		}

		buf.WriteString(fmt.Sprintf("(%s) Tj\n", escapeStringPDF(line)))
		linePositions = append(linePositions, linePos{x: adjX, y: adjY, width: lineWidth})
	}

	buf.WriteString("ET\n")

	// Underline.
	if style.Underline && len(linePositions) > 0 {
		buf.WriteString(fmt.Sprintf("%s %s %s rg\n",
			formatFloat(textColor.R), formatFloat(textColor.G), formatFloat(textColor.B)))
		thickness := fontSize * 0.05
		for _, lp := range linePositions {
			ulY := lp.y - fontSize*0.1
			buf.WriteString(fmt.Sprintf("%s %s %s %s re f\n",
				formatFloat(lp.x), formatFloat(ulY),
				formatFloat(lp.width), formatFloat(thickness)))
		}
	}

	// Strikethrough.
	if style.Strikethrough && len(linePositions) > 0 {
		buf.WriteString(fmt.Sprintf("%s %s %s rg\n",
			formatFloat(textColor.R), formatFloat(textColor.G), formatFloat(textColor.B)))
		thickness := fontSize * 0.05
		for _, lp := range linePositions {
			stY := lp.y + fontSize*0.3
			buf.WriteString(fmt.Sprintf("%s %s %s %s re f\n",
				formatFloat(lp.x), formatFloat(stY),
				formatFloat(lp.width), formatFloat(thickness)))
		}
	}

	// Restore state.
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

// ensureFontResource registers a Type1 font in the page's /Resources /Font dict.
// If a font with the same /BaseFont already exists, its resource name is returned.
func (p *Page) ensureFontResource(pdfFontName string) (string, error) {
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

	// Create new font object.
	fontObjDict := pdfDict{
		"/Type":     pdfName("/Font"),
		"/Subtype":  pdfName("/Type1"),
		"/BaseFont": pdfName(pdfFontName),
	}
	fontID := p.doc.nextID
	p.doc.nextID++
	p.doc.objects[fontID] = &pdfObject{Num: fontID, Value: fontObjDict}

	name := nextFontName(fontDict)
	fontDict[name] = pdfRef{Num: fontID}

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

// ensureExtGState registers an ExtGState with the given fill opacity.
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

	// Check if an ExtGState with the same /ca already exists.
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
		if ca, err := toFloat(dict["/ca"]); err == nil && ca == alpha {
			return name, nil
		}
	}

	gsObjDict := pdfDict{
		"/ca": alpha,
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
