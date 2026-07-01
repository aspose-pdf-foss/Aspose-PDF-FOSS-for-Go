// SPDX-License-Identifier: MIT

package asposepdf

import (
	"math"
	"strings"
)

// TextExtractionMode selects how ExtractText orders the extracted text. Mirrors
// the intent of Aspose.PDF for .NET's TextExtractionOptions.TextFormattingMode.
type TextExtractionMode int

const (
	// TextExtractReading returns text in visual reading order (fragments sorted
	// top-to-bottom, left-to-right, grouped into lines). This is the default.
	TextExtractReading TextExtractionMode = iota
	// TextExtractRaw returns text in the order the content stream emits it,
	// without visual sorting — useful when the emission order is significant or
	// when the reading-order heuristics reorder columned/overlapping content.
	TextExtractRaw
)

// TextExtractOptions configures ExtractText. The zero value is reading order.
type TextExtractOptions struct {
	Mode TextExtractionMode
}

// ExtractText returns the text content of the page. Characters from fonts with
// unrecognized encodings are replaced with U+FFFD. An optional TextExtractOptions
// selects reading order (default) or raw content-stream order.
func (p *Page) ExtractText(opts ...TextExtractOptions) (string, error) {
	data, err := p.contentStreams()
	if err != nil {
		return "", err
	}
	if len(data) == 0 {
		return "", nil
	}

	ops, err := parseContentStream(data)
	if err != nil {
		return "", err
	}

	resources := p.pageResources()
	fonts := resolveFontResources(p.doc.objects, resources)

	ext := newTextExtractor(p.doc.objects, fonts)
	ext.process(ops, resources)
	if len(opts) > 0 && opts[0].Mode == TextExtractRaw {
		return ext.textRaw(), nil
	}
	return ext.text(), nil
}

// ExtractText returns the text content of each page (one entry per page,
// 0-indexed). An optional TextExtractOptions selects the extraction mode.
func (d *Document) ExtractText(opts ...TextExtractOptions) ([]string, error) {
	pages := d.Pages()
	result := make([]string, len(pages))
	for i, p := range pages {
		text, err := p.ExtractText(opts...)
		if err != nil {
			return nil, err
		}
		result[i] = text
	}
	return result, nil
}

// resolveFontResources resolves all fonts in /Resources /Font.
func resolveFontResources(objects map[int]*pdfObject, resources pdfDict) map[string]fontInfo {
	fonts := make(map[string]fontInfo)
	if resources == nil {
		return fonts
	}
	fontVal, ok := resources["/Font"]
	if !ok {
		return fonts
	}
	fontDict, ok := resolveRefToDict(objects, fontVal)
	if !ok {
		return fonts
	}
	for name, val := range fontDict {
		fd, ok := resolveRefToDict(objects, val)
		if !ok {
			continue
		}
		fonts[name] = resolveFont(objects, fd)
	}
	return fonts
}

// textFragment is a contiguous run of text at a single position.
type textFragment struct {
	text        strings.Builder
	x, y        float64   // device-space position of first rune
	runeX       []float64 // device-space x at the start of each rune (len == rune count)
	endX        float64   // device-space x after last glyph advance
	fontName    string
	fontSize    float64 // effective font size (fontSize * textScaleX)
	height      float64 // (ascent - descent) / 1000 * fontSize
	bold        bool
	italic      bool
	charSpacing float64
	colorR      float64 // fill color RGB (0-1)
	colorG      float64
	colorB      float64
}

type textExtractor struct {
	objects map[int]*pdfObject
	fonts   map[string]fontInfo

	// Text state.
	font         fontInfo
	fontSize     float64
	charSpace    float64
	wordSpace    float64
	leading      float64
	horizScaling float64    // Tz / 100; default 1.0
	tm           [6]float64 // text matrix
	lm           [6]float64 // line matrix
	ctm          [6]float64 // current transformation matrix
	gsStack      []extractorGState

	// Fill color (for text rendering).
	fillR, fillG, fillB float64 // RGB, 0-1

	// Marked content stack for ActualText support (BDC/BMC/EMC).
	mcStack []markedContentEntry

	// Output: collected text fragments.
	fragments []textFragment
	curFrag   *textFragment // current fragment being built
	lastX     float64       // x after last glyph advance
	lastY     float64       // y after last glyph advance
	hasPos    bool
}

// extractorGState is the slice of state that q/Q save and restore. Per ISO
// 32000-1 Table 52 the text-state parameters (font, size, char/word spacing,
// leading, horizontal scaling) and the fill colour are part of the graphics
// state, so q/Q must roll them back along with the CTM. The text matrices
// (tm/lm) are text-object state, not graphics state, so they are NOT saved here
// — a q/Q may nest inside a BT/ET block. Without restoring the font, text drawn
// after a "q … /Fsub Tf … Q" block kept the inner font and decoded through the
// wrong encoding (Binder1.pdf: TOTAL rows became "3?3>;").
type extractorGState struct {
	ctm          [6]float64
	font         fontInfo
	fontSize     float64
	charSpace    float64
	wordSpace    float64
	leading      float64
	horizScaling float64
	fillR        float64
	fillG        float64
	fillB        float64
}

// markedContentEntry tracks a BDC/BMC nesting level.
// When actualText is non-nil, glyphs inside are suppressed and the pointed-to
// string is emitted instead when the matching EMC is encountered.
type markedContentEntry struct {
	actualText *string // nil means no /ActualText replacement
}

func newTextExtractor(objects map[int]*pdfObject, fonts map[string]fontInfo) *textExtractor {
	return &textExtractor{
		objects:      objects,
		fonts:        fonts,
		ctm:          identityMatrix(),
		horizScaling: 1.0,
	}
}

func identityMatrix() [6]float64 {
	return [6]float64{1, 0, 0, 1, 0, 0}
}

// insideActualText returns true if the extractor is currently inside a
// marked content sequence that carries /ActualText (glyph emission suppressed).
func (e *textExtractor) insideActualText() bool {
	for i := len(e.mcStack) - 1; i >= 0; i-- {
		if e.mcStack[i].actualText != nil {
			return true
		}
	}
	return false
}

func (e *textExtractor) text() string {
	e.flushFragment()
	return cleanExtractedText(buildTextFromFragments(e.fragments))
}

// textRaw returns the fragments joined in content-stream emission order (no
// visual sorting), inserting a newline on a vertical shift and a space on a
// horizontal gap.
func (e *textExtractor) textRaw() string {
	e.flushFragment()
	return cleanExtractedText(buildRawTextFromFragments(e.fragments))
}

// buildRawTextFromFragments joins fragments in their emission order. A newline
// is inserted when a fragment's baseline shifts from the previous one by more
// than half its font size; otherwise a space is inserted when there is a
// positive horizontal gap.
func buildRawTextFromFragments(frags []textFragment) string {
	var buf strings.Builder
	var prev *textFragment
	for i := range frags {
		f := &frags[i]
		s := f.text.String()
		if s == "" {
			continue
		}
		if prev != nil {
			fs := f.fontSize
			if fs == 0 {
				fs = 12
			}
			dy := prev.y - f.y
			if dy > fs*0.5 || dy < -fs*0.5 {
				buf.WriteByte('\n')
			} else if f.x-prev.endX > fs*0.2 {
				buf.WriteByte(' ')
			}
		}
		buf.WriteString(s)
		prev = f
	}
	return buf.String()
}

// cleanExtractedText trims trailing whitespace from each line and
// collapses runs of more than two consecutive blank lines into two.
func cleanExtractedText(s string) string {
	lines := strings.Split(s, "\n")
	blank := 0
	var out []string
	for _, line := range lines {
		line = strings.TrimRight(line, " \t")
		if line == "" {
			blank++
			if blank > 2 {
				continue
			}
		} else {
			blank = 0
		}
		out = append(out, line)
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

func (e *textExtractor) process(ops []contentOp, resources pdfDict) {
	for _, op := range ops {
		switch op.Operator {
		case "BT":
			e.tm = identityMatrix()
			e.lm = identityMatrix()

		case "ET":
			// End of text object.

		case "Tf":
			if len(op.Operands) >= 2 {
				fontName := operandName(op.Operands[0])
				if fi, ok := e.fonts[fontName]; ok {
					e.font = fi
				} else {
					// Font absent from /Resources/Font (e.g. an empty dict,
					// 44963.pdf): viewers substitute a default text face
					// rather than dropping the text — decode via Helvetica.
					e.font = defaultFontInfo(e.objects)
				}
				e.fontSize = operandFloat(op.Operands[1])
			}

		case "Td":
			if len(op.Operands) >= 2 {
				tx := operandFloat(op.Operands[0])
				ty := operandFloat(op.Operands[1])
				e.lm = matMul(translateMatrix(tx, ty), e.lm)
				e.tm = e.lm
			}

		case "TD":
			if len(op.Operands) >= 2 {
				tx := operandFloat(op.Operands[0])
				ty := operandFloat(op.Operands[1])
				e.leading = -ty
				e.lm = matMul(translateMatrix(tx, ty), e.lm)
				e.tm = e.lm
			}

		case "Tm":
			if len(op.Operands) >= 6 {
				for i := 0; i < 6; i++ {
					e.tm[i] = operandFloat(op.Operands[i])
				}
				e.lm = e.tm
			}

		case "T*":
			e.lm = matMul(translateMatrix(0, -e.leading), e.lm)
			e.tm = e.lm

		case "Tj":
			if len(op.Operands) >= 1 {
				e.showString(op.Operands[0])
			}

		case "TJ":
			if len(op.Operands) >= 1 {
				e.showTJ(op.Operands[0])
			}

		case "'":
			e.lm = matMul(translateMatrix(0, -e.leading), e.lm)
			e.tm = e.lm
			if len(op.Operands) >= 1 {
				e.showString(op.Operands[0])
			}

		case "\"":
			if len(op.Operands) >= 3 {
				e.wordSpace = operandFloat(op.Operands[0])
				e.charSpace = operandFloat(op.Operands[1])
				e.lm = matMul(translateMatrix(0, -e.leading), e.lm)
				e.tm = e.lm
				e.showString(op.Operands[2])
			}

		case "Tc":
			if len(op.Operands) >= 1 {
				e.charSpace = operandFloat(op.Operands[0])
			}

		case "Tw":
			if len(op.Operands) >= 1 {
				e.wordSpace = operandFloat(op.Operands[0])
			}

		case "TL":
			if len(op.Operands) >= 1 {
				e.leading = operandFloat(op.Operands[0])
			}

		case "Tz":
			if len(op.Operands) >= 1 {
				e.horizScaling = operandFloat(op.Operands[0]) / 100.0
			}

		case "cm":
			if len(op.Operands) >= 6 {
				var m [6]float64
				for i := 0; i < 6; i++ {
					m[i] = operandFloat(op.Operands[i])
				}
				e.ctm = matMul(m, e.ctm)
			}

		case "q":
			e.gsStack = append(e.gsStack, extractorGState{
				ctm: e.ctm, font: e.font, fontSize: e.fontSize,
				charSpace: e.charSpace, wordSpace: e.wordSpace, leading: e.leading,
				horizScaling: e.horizScaling, fillR: e.fillR, fillG: e.fillG, fillB: e.fillB,
			})

		case "Q":
			if n := len(e.gsStack); n > 0 {
				s := e.gsStack[n-1]
				e.gsStack = e.gsStack[:n-1]
				e.ctm, e.font, e.fontSize = s.ctm, s.font, s.fontSize
				e.charSpace, e.wordSpace, e.leading = s.charSpace, s.wordSpace, s.leading
				e.horizScaling = s.horizScaling
				e.fillR, e.fillG, e.fillB = s.fillR, s.fillG, s.fillB
			}

		case "BMC":
			e.mcStack = append(e.mcStack, markedContentEntry{})

		case "BDC":
			entry := markedContentEntry{}
			if len(op.Operands) >= 2 {
				entry.actualText = e.resolveActualText(op.Operands[1], resources)
			}
			e.mcStack = append(e.mcStack, entry)

		case "EMC":
			if len(e.mcStack) > 0 {
				top := e.mcStack[len(e.mcStack)-1]
				e.mcStack = e.mcStack[:len(e.mcStack)-1]
				if top.actualText != nil {
					for _, r := range *top.actualText {
						e.emitRune(r)
					}
				}
			}

		case "Do":
			if len(op.Operands) >= 1 {
				e.doFormXObject(op.Operands[0], resources)
			}

		// Fill color operators (text uses fill color).
		case "g": // gray
			if len(op.Operands) >= 1 {
				gray := operandFloat(op.Operands[0])
				e.fillR, e.fillG, e.fillB = gray, gray, gray
			}
		case "rg": // RGB
			if len(op.Operands) >= 3 {
				e.fillR = operandFloat(op.Operands[0])
				e.fillG = operandFloat(op.Operands[1])
				e.fillB = operandFloat(op.Operands[2])
			}
		case "k": // CMYK → RGB
			if len(op.Operands) >= 4 {
				c := operandFloat(op.Operands[0])
				m := operandFloat(op.Operands[1])
				y := operandFloat(op.Operands[2])
				k := operandFloat(op.Operands[3])
				e.fillR = (1 - c) * (1 - k)
				e.fillG = (1 - m) * (1 - k)
				e.fillB = (1 - y) * (1 - k)
			}
		case "sc", "scn": // generic fill color (DeviceRGB assumed if 3 operands)
			if len(op.Operands) == 1 {
				gray := operandFloat(op.Operands[0])
				e.fillR, e.fillG, e.fillB = gray, gray, gray
			} else if len(op.Operands) >= 3 {
				e.fillR = operandFloat(op.Operands[0])
				e.fillG = operandFloat(op.Operands[1])
				e.fillB = operandFloat(op.Operands[2])
			}
		}
	}
}

func (e *textExtractor) advanceGlyph(code byte) {
	w0 := e.font.widths[code]
	tx := (w0/1000.0*e.fontSize + e.charSpace) * e.horizScaling
	if code == 32 {
		tx += e.wordSpace * e.horizScaling
	}
	e.tm = matMul(translateMatrix(tx, 0), e.tm)
	// Update lastX/lastY to the post-advance position so that the next
	// emitRune sees only the true inter-glyph gap (not the glyph width).
	e.lastX, e.lastY = e.currentPos()
}

func (e *textExtractor) showString(operand pdfValue) {
	s, ok := operand.(string)
	if !ok {
		return
	}
	if e.font.isType0 {
		e.showStringMultiByte(s)
	} else {
		e.showStringSingleByte(s)
	}
}

func (e *textExtractor) showStringSingleByte(s string) {
	for i := 0; i < len(s); i++ {
		code := s[i]
		r := e.font.encoding[code]
		// If toUnicode is available, prefer it for single-byte fonts too.
		if e.font.toUnicode != nil {
			if tr, ok := e.font.toUnicode[uint16(code)]; ok {
				r = tr
			}
		}
		if r == 0 {
			r = '\uFFFD'
		}
		e.emitRune(r)
		e.advanceGlyph(code)
	}
}

func (e *textExtractor) showStringMultiByte(s string) {
	if e.font.cidCMap != nil {
		e.showStringCMap(s)
		return
	}
	// Identity-H/V: two-byte codes, code == CID.
	for i := 0; i+1 < len(s); i += 2 {
		code := uint16(s[i])<<8 | uint16(s[i+1])
		r := rune(0)
		if e.font.toUnicode != nil {
			r = e.font.toUnicode[code]
		}
		if r == 0 {
			r = '\uFFFD'
		}
		e.emitRune(r)
		e.advanceGlyphCID(code, 2)
	}
}

// showStringCMap decodes a string through a composite font's CMap, where the
// codespace dictates each code's byte length (mixed 1-byte Latin / 2-byte CJK).
func (e *textExtractor) showStringCMap(s string) {
	b := []byte(s)
	for len(b) > 0 {
		code, cid, n := e.font.cidCMap.next(b)
		r := rune(0)
		if e.font.toUnicode != nil {
			r = e.font.toUnicode[uint16(code)]
		}
		if r == 0 && e.font.cidToUni != nil {
			r = e.font.cidToUni[cid]
		}
		// Latin inside a CJK font: many CMaps map ASCII to proportional-Latin
		// CIDs that carry no Unicode in Adobe's tables \u2014 use the code itself.
		if r == 0 && n == 1 && code >= 0x20 && code < 0x7f {
			r = rune(code)
		}
		if r == 0 {
			r = '\uFFFD'
		}
		e.emitRune(r)
		e.advanceGlyphCID(cid, n)
		b = b[n:]
	}
}

func (e *textExtractor) advanceGlyphCID(cid uint16, nbytes int) {
	w0 := e.font.defaultW
	if cw, ok := e.font.cidWidths[cid]; ok {
		w0 = cw
	}
	tx := (w0/1000.0*e.fontSize + e.charSpace) * e.horizScaling
	if nbytes == 1 && cid == 32 {
		tx += e.wordSpace * e.horizScaling
	}
	e.tm = matMul(translateMatrix(tx, 0), e.tm)
	// Update lastX/lastY to the post-advance position so that the next
	// emitRune sees only the true inter-glyph gap (not the glyph width).
	e.lastX, e.lastY = e.currentPos()
}

func (e *textExtractor) showTJ(operand pdfValue) {
	arr, ok := operand.(pdfArray)
	if !ok {
		return
	}
	for _, elem := range arr {
		switch v := elem.(type) {
		case string:
			if e.font.isType0 {
				e.showStringMultiByte(v)
			} else {
				e.showStringSingleByte(v)
			}
		case int:
			displacement := -float64(v) / 1000.0 * e.fontSize
			e.tm = matMul(translateMatrix(displacement, 0), e.tm)
		case float64:
			displacement := -v / 1000.0 * e.fontSize
			e.tm = matMul(translateMatrix(displacement, 0), e.tm)
		}
	}
}

func (e *textExtractor) emitRune(r rune) {
	// Suppress glyph output inside /ActualText marked content spans.
	// The replacement text is emitted at EMC instead.
	if e.insideActualText() {
		return
	}

	x, y := e.currentPos()
	effectiveFontSize := e.fontSize * e.textScaleX()
	fontName := e.font.name

	needNew := e.curFrag == nil ||
		fontName != e.curFrag.fontName ||
		math.Abs(effectiveFontSize-e.curFrag.fontSize) > 0.01

	if !needNew && e.hasPos {
		dy := e.lastY - y
		if math.Abs(dy) > effectiveFontSize*0.5 {
			needNew = true
		}
		dx := x - e.lastX
		spaceWidth := e.computeSpaceWidth()
		scale := e.textScaleX()
		// Forward gap wider than ~a third of a space, or a significant backward
		// jump (the next glyph sits well left of the previous — out-of-order
		// content, e.g. text drawn over an erased run), starts a new fragment so
		// visual ordering stays correct.
		if dx > spaceWidth*scale*0.3 || dx < -spaceWidth*scale {
			needNew = true
		}
	}

	if needNew {
		e.flushFragment()
		height := effectiveFontSize // fallback
		if e.font.ascent != 0 || e.font.descent != 0 {
			height = (e.font.ascent - e.font.descent) / 1000.0 * effectiveFontSize
		}
		frag := textFragment{
			x:           x,
			y:           y,
			fontName:    fontName,
			fontSize:    effectiveFontSize,
			height:      height,
			bold:        e.font.bold,
			italic:      e.font.italic,
			charSpacing: e.charSpace,
			colorR:      e.fillR,
			colorG:      e.fillG,
			colorB:      e.fillB,
		}
		e.fragments = append(e.fragments, frag)
		e.curFrag = &e.fragments[len(e.fragments)-1]
	}

	e.curFrag.text.WriteRune(r)
	e.curFrag.runeX = append(e.curFrag.runeX, x) // exact glyph-start X for sub-fragment positioning
	e.lastX = x
	e.lastY = y
	e.hasPos = true
}

func (e *textExtractor) flushFragment() {
	if e.curFrag != nil {
		e.curFrag.endX = e.lastX
		e.curFrag = nil
	}
}

// computeSpaceWidth returns the space character width in text space units.
func (e *textExtractor) computeSpaceWidth() float64 {
	var spaceWidth float64
	if e.font.isType0 {
		if sw, ok := e.font.cidWidths[0x0020]; ok {
			spaceWidth = sw / 1000.0 * e.fontSize
		} else {
			spaceWidth = e.font.defaultW / 1000.0 * e.fontSize
		}
	} else {
		spaceWidth = e.font.widths[32] / 1000.0 * e.fontSize
	}
	if spaceWidth < 1 {
		spaceWidth = e.fontSize * 0.25
	}
	if spaceWidth < 1 {
		spaceWidth = 1
	}
	return spaceWidth
}

func (e *textExtractor) currentPos() (float64, float64) {
	m := matMul(e.tm, e.ctm)
	return m[4], m[5]
}

// textScaleX returns the horizontal scale factor from text space to device space.
// This accounts for both the text matrix (Tm) and current transformation matrix (CTM).
func (e *textExtractor) textScaleX() float64 {
	m := matMul(e.tm, e.ctm)
	sx := math.Sqrt(m[0]*m[0] + m[1]*m[1])
	if sx < 0.001 {
		return 1
	}
	return sx
}

func (e *textExtractor) doFormXObject(operand pdfValue, parentResources pdfDict) {
	name := operandName(operand)
	if name == "" || parentResources == nil {
		return
	}

	xobjVal, ok := parentResources["/XObject"]
	if !ok {
		return
	}
	xobjDict, ok := resolveRefToDict(e.objects, xobjVal)
	if !ok {
		return
	}
	formVal, ok := xobjDict[name]
	if !ok {
		return
	}
	resolved := resolveRef(e.objects, formVal)
	stream, ok := resolved.(*pdfStream)
	if !ok {
		return
	}
	if dictGetName(stream.Dict, "/Subtype") != "/Form" {
		return
	}

	ops, err := parseContentStream(stream.Data)
	if err != nil {
		return
	}

	// Form resources override parent.
	formResources := parentResources
	if resVal, ok := stream.Dict["/Resources"]; ok {
		if rd, ok := resolveRefToDict(e.objects, resVal); ok {
			formResources = rd
		}
	}

	formFonts := resolveFontResources(e.objects, formResources)

	// Save state.
	savedCTM := e.ctm
	savedFonts := e.fonts

	// Apply form's /Matrix if present.
	if matVal, ok := stream.Dict["/Matrix"]; ok {
		if arr, ok := matVal.(pdfArray); ok && len(arr) == 6 {
			var fm [6]float64
			for i := 0; i < 6; i++ {
				fm[i] = operandFloat(arr[i])
			}
			e.ctm = matMul(fm, e.ctm)
		}
	}

	// Merge fonts (form takes precedence).
	merged := make(map[string]fontInfo, len(e.fonts)+len(formFonts))
	for k, v := range e.fonts {
		merged[k] = v
	}
	for k, v := range formFonts {
		merged[k] = v
	}
	e.fonts = merged

	e.process(ops, formResources)

	e.fonts = savedFonts
	e.ctm = savedCTM
}

// resolveActualText extracts /ActualText from a BDC property operand.
// The operand is either an inline dict or a name referencing /Properties in resources.
func (e *textExtractor) resolveActualText(operand pdfValue, resources pdfDict) *string {
	var props pdfDict

	switch v := operand.(type) {
	case pdfDict:
		props = v
	case pdfName:
		// Look up in /Properties resource dict.
		if resources == nil {
			return nil
		}
		propsVal, ok := resources["/Properties"]
		if !ok {
			return nil
		}
		propsDict, ok := resolveRefToDict(e.objects, propsVal)
		if !ok {
			return nil
		}
		entryVal, ok := propsDict[string(v)]
		if !ok {
			return nil
		}
		d, ok := resolveRefToDict(e.objects, entryVal)
		if !ok {
			return nil
		}
		props = d
	default:
		return nil
	}

	atVal, ok := props["/ActualText"]
	if !ok {
		return nil
	}
	s, ok := resolveRef(e.objects, atVal).(string)
	if !ok {
		return nil
	}
	// Decode UTF-16BE BOM if present.
	s = decodeTextString(s)
	return &s
}

// decodeTextString converts a PDF text string to Go string.
// If it starts with UTF-16BE BOM (0xFE 0xFF), it is decoded as UTF-16BE;
// otherwise it is returned as-is (PDFDocEncoding ≈ Latin-1).
func decodeTextString(s string) string {
	if len(s) >= 2 && s[0] == 0xFE && s[1] == 0xFF {
		runes := make([]rune, 0, (len(s)-2)/2)
		for i := 2; i+1 < len(s); i += 2 {
			code := uint16(s[i])<<8 | uint16(s[i+1])
			runes = append(runes, rune(code))
		}
		return string(runes)
	}
	return s
}

// operandName extracts a PDF name string from an operand.
func operandName(v pdfValue) string {
	if n, ok := v.(pdfName); ok {
		return string(n)
	}
	return ""
}

// operandFloat extracts a float64 from an operand (int or float64).
func operandFloat(v pdfValue) float64 {
	switch n := v.(type) {
	case int:
		return float64(n)
	case float64:
		return n
	}
	return 0
}

// Matrix operations for 3x3 affine transforms stored as [a b c d e f].
// The full matrix is:
//
//	| a b 0 |
//	| c d 0 |
//	| e f 1 |

func translateMatrix(tx, ty float64) [6]float64 {
	return [6]float64{1, 0, 0, 1, tx, ty}
}

func matMul(a, b [6]float64) [6]float64 {
	return [6]float64{
		a[0]*b[0] + a[1]*b[2],
		a[0]*b[1] + a[1]*b[3],
		a[2]*b[0] + a[3]*b[2],
		a[2]*b[1] + a[3]*b[3],
		a[4]*b[0] + a[5]*b[2] + b[4],
		a[4]*b[1] + a[5]*b[3] + b[5],
	}
}
