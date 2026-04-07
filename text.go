package asposepdf

import (
	"math"
	"strings"
)

// ExtractText returns the text content of the page.
// Characters from fonts with unrecognized encodings are replaced with U+FFFD.
func (p *Page) ExtractText() (string, error) {
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
	return ext.text(), nil
}

// ExtractText returns the text content of each page.
// The returned slice has one entry per page (0-indexed).
func (d *Document) ExtractText() ([]string, error) {
	pages := d.Pages()
	result := make([]string, len(pages))
	for i, p := range pages {
		text, err := p.ExtractText()
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

type textExtractor struct {
	objects map[int]*pdfObject
	fonts   map[string]fontInfo

	// Text state.
	font         fontInfo
	fontSize     float64
	charSpace    float64
	wordSpace    float64
	leading      float64
	horizScaling float64 // Tz / 100; default 1.0
	tm           [6]float64 // text matrix
	lm           [6]float64 // line matrix
	ctm          [6]float64 // current transformation matrix
	ctmStack     [][6]float64

	// Output.
	buf    strings.Builder
	lastX  float64
	lastY  float64
	hasPos bool
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

func (e *textExtractor) text() string {
	return cleanExtractedText(e.buf.String())
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
			e.ctmStack = append(e.ctmStack, e.ctm)

		case "Q":
			if len(e.ctmStack) > 0 {
				e.ctm = e.ctmStack[len(e.ctmStack)-1]
				e.ctmStack = e.ctmStack[:len(e.ctmStack)-1]
			}

		case "Do":
			if len(op.Operands) >= 1 {
				e.doFormXObject(op.Operands[0], resources)
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
	for i := 0; i < len(s); i++ {
		code := s[i]
		r := e.font.encoding[code]
		if r == 0 {
			r = '\uFFFD'
		}
		e.emitRune(r)
		e.advanceGlyph(code)
	}
}

func (e *textExtractor) showTJ(operand pdfValue) {
	arr, ok := operand.(pdfArray)
	if !ok {
		return
	}
	for _, elem := range arr {
		switch v := elem.(type) {
		case string:
			for i := 0; i < len(v); i++ {
				code := v[i]
				r := e.font.encoding[code]
				if r == 0 {
					r = '\uFFFD'
				}
				e.emitRune(r)
				e.advanceGlyph(code)
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
	x, y := e.currentPos()

	if e.hasPos {
		dx := x - e.lastX
		dy := e.lastY - y

		// Use the font's space character width for space detection.
		spaceWidth := e.font.widths[32] / 1000.0 * e.fontSize
		if spaceWidth < 1 {
			spaceWidth = e.fontSize * 0.25
		}
		if spaceWidth < 1 {
			spaceWidth = 1
		}

		if math.Abs(dy) > e.fontSize*0.5 {
			e.buf.WriteByte('\n')
		} else if dx > spaceWidth*0.3 {
			e.buf.WriteByte(' ')
		}
	}

	e.buf.WriteRune(r)
	e.lastX = x
	e.lastY = y
	e.hasPos = true
}

func (e *textExtractor) currentPos() (float64, float64) {
	m := matMul(e.tm, e.ctm)
	return m[4], m[5]
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
