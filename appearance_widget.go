// SPDX-License-Identifier: MIT

package asposepdf

import (
	"math"
	"strconv"
	"strings"
)

// regenerateWidgetAppearance dispatches by the widget's field type and
// updates /AP/N on the widget dict in place. Called from AddXxx field
// constructors and from setters that mutate state visible in the
// rendered appearance (SetValue, SetChecked, AddOption, ...).
//
// When NeedAppearances=true the viewer would regenerate /AP itself, but
// that causes Acrobat to mark the document as modified on open and
// MuPDF/PyMuPDF render widgets as bare value text with no chrome. With
// proper /AP streams from this function we can leave NeedAppearances at
// its default (false) and every viewer renders the form identically.
//
// Parent radio-group dicts have /FT but no /Rect — they're not widgets
// themselves; their kids are. The function tolerates such dicts by
// returning early when the rect is empty.
func regenerateWidgetAppearance(form *Form, widget pdfDict) {
	if form == nil || form.doc == nil || widget == nil {
		return
	}
	ft := widgetFieldType(form, widget)
	switch ft {
	case "/Tx":
		setWidgetAPN(form.doc, widget, generateTextFieldAppearance(form, widget), "")
	case "/Btn":
		ff := widgetFieldFlags(form, widget)
		switch {
		case ff&fieldFlagPushbutton != 0:
			setWidgetAPN(form.doc, widget, generatePushButtonAppearance(form, widget), "")
		case ff&fieldFlagRadio != 0:
			regenerateRadioWidget(form, widget)
		default:
			regenerateCheckboxWidget(form, widget)
		}
	case "/Ch":
		ff := widgetFieldFlags(form, widget)
		if ff&fieldFlagCombo != 0 {
			setWidgetAPN(form.doc, widget, generateComboBoxAppearance(form, widget), "")
		} else {
			setWidgetAPN(form.doc, widget, generateListBoxAppearance(form, widget), "")
		}
	}
}

// regenerateFieldAppearance regenerates /AP for every widget belonging
// to a field node. Used from noteFormMutated so callers that touch a
// field handle (not the widget dict directly) still get fresh /AP.
func regenerateFieldAppearance(n *fieldNode) {
	if n == nil || n.form == nil {
		return
	}
	for _, w := range n.widgets {
		regenerateWidgetAppearance(n.form, w)
	}
}

// widgetFieldType returns the /FT of a widget, walking up to /Parent
// for kids of radio groups (where /FT lives on the parent).
func widgetFieldType(form *Form, widget pdfDict) pdfName {
	if ft, ok := widget["/FT"].(pdfName); ok {
		return ft
	}
	if parentRef, ok := widget["/Parent"].(pdfRef); ok {
		if obj, exists := form.doc.objects[parentRef.Num]; exists {
			if parent, ok := obj.Value.(pdfDict); ok {
				if ft, ok := parent["/FT"].(pdfName); ok {
					return ft
				}
			}
		}
	}
	return ""
}

// widgetFieldFlags returns the effective /Ff bitfield, walking up to
// /Parent so radio-group kids see the parent's flags.
func widgetFieldFlags(form *Form, widget pdfDict) int {
	if ff, ok := widget["/Ff"].(int); ok {
		return ff
	}
	if parentRef, ok := widget["/Parent"].(pdfRef); ok {
		if obj, exists := form.doc.objects[parentRef.Num]; exists {
			if parent, ok := obj.Value.(pdfDict); ok {
				if ff, ok := parent["/Ff"].(int); ok {
					return ff
				}
			}
		}
	}
	return 0
}

// widgetSize returns the width and height of the widget's /Rect.
// Returns (0, 0) if /Rect is missing or malformed.
func widgetSize(widget pdfDict) (float64, float64) {
	arr, ok := widget["/Rect"].(pdfArray)
	if !ok || len(arr) != 4 {
		return 0, 0
	}
	llx, _ := toFloat(arr[0])
	lly, _ := toFloat(arr[1])
	urx, _ := toFloat(arr[2])
	ury, _ := toFloat(arr[3])
	return urx - llx, ury - lly
}

// parseDA extracts the font size, fill colour, and font resource name
// from a widget's /DA string ("0.2 0.2 0.6 rg /Helv 12 Tf"). The font
// resource name (without the leading slash) lets the generator resolve
// the caller's chosen font via resolveWidgetFont so the rendered /AP
// uses it, not just viewers that synthesise their own appearance.
//
// Defaults: 12pt, black, "Helv". Supports the two common colour ops:
// "g" (gray) and "rg" (RGB). Unknown colour space → black.
func parseDA(da string) (size float64, color Color, fontRes string) {
	size = 12
	color = Color{R: 0, G: 0, B: 0, A: 1}
	fontRes = "Helv"
	toks := strings.Fields(da)
	for i, tok := range toks {
		switch tok {
		case "Tf":
			if i >= 2 {
				if s, err := strconv.ParseFloat(toks[i-1], 64); err == nil && s > 0 {
					size = s
				}
				fontRes = strings.TrimPrefix(toks[i-2], "/")
			}
		case "g":
			if i >= 1 {
				if v, err := strconv.ParseFloat(toks[i-1], 64); err == nil {
					color = Color{R: v, G: v, B: v, A: 1}
				}
			}
		case "rg":
			if i >= 3 {
				r, e1 := strconv.ParseFloat(toks[i-3], 64)
				g, e2 := strconv.ParseFloat(toks[i-2], 64)
				b, e3 := strconv.ParseFloat(toks[i-1], 64)
				if e1 == nil && e2 == nil && e3 == nil {
					color = Color{R: r, G: g, B: b, A: 1}
				}
			}
		}
	}
	return size, color, fontRes
}

// widgetQuadAlign maps /Q (0 = left, 1 = centred, 2 = right) to HAlign.
// Default is left when /Q is absent.
func widgetQuadAlign(widget pdfDict) HAlign {
	if q, ok := widget["/Q"].(int); ok {
		switch q {
		case 1:
			return HAlignCenter
		case 2:
			return HAlignRight
		}
	}
	return HAlignLeft
}

// setWidgetAPN updates /AP/N on the widget. When stateName == "" the
// stream is stored as a direct reference at /AP/N. Otherwise /AP/N is a
// dict and the stream lives under "/<stateName>" (used for /Yes /Off on
// checkboxes and the per-option states of radio buttons).
//
// Existing XObjects referenced by /AP/N (or /AP/N/<state>) are mutated
// in place so the object number stays stable across regenerations and
// no orphan objects accumulate in doc.objects.
func setWidgetAPN(doc *Document, widget pdfDict, stream *pdfStream, stateName string) {
	if doc == nil || stream == nil {
		return
	}
	ap, _ := widget["/AP"].(pdfDict)
	if ap == nil {
		ap = pdfDict{}
	}
	if stateName == "" {
		if ref, ok := ap["/N"].(pdfRef); ok {
			if obj, exists := doc.objects[ref.Num]; exists {
				obj.Value = stream
				widget["/AP"] = ap
				return
			}
		}
		id := doc.nextID
		doc.nextID++
		doc.objects[id] = &pdfObject{Num: id, Value: stream}
		ap["/N"] = pdfRef{Num: id}
		widget["/AP"] = ap
		return
	}
	// Multi-state dict.
	n, _ := ap["/N"].(pdfDict)
	if n == nil {
		n = pdfDict{}
	}
	key := "/" + stateName
	if ref, ok := n[key].(pdfRef); ok {
		if obj, exists := doc.objects[ref.Num]; exists {
			obj.Value = stream
			n[key] = ref
			ap["/N"] = n
			widget["/AP"] = ap
			return
		}
	}
	id := doc.nextID
	doc.nextID++
	doc.objects[id] = &pdfObject{Num: id, Value: stream}
	n[key] = pdfRef{Num: id}
	ap["/N"] = n
	widget["/AP"] = ap
}

// drawWidgetChrome paints the background fill and border for a text-like
// widget. Colours and border come from the widget's /MK (/BG, /BC) and
// /BS (/W, /S, /D) when present (set via Field.SetStyle); otherwise it
// falls back to the library default — white interior with a light grey
// 0.5pt hairline border — so unstyled fields look the same as before.
func drawWidgetChrome(b *appearanceBuilder, widget pdfDict, width, height float64) {
	// Background.
	b.PushState()
	if bg := mkColor(widget, "/BG"); bg != nil {
		b.SetFillColorRGB(*bg)
	} else {
		b.SetFillGray(1)
	}
	b.Rect(0, 0, width, height)
	b.Fill()
	b.PopState()

	// Border.
	bc := mkColor(widget, "/BC")
	bw, bstyle, dash := readBS(widget)
	if bc != nil {
		if bw <= 0 {
			bw = 1
		}
		drawStandardRectBorder(b, width, height, bstyle, bw, dash, bc)
	} else {
		b.PushState()
		b.SetLineWidth(0.5)
		b.SetStrokeGray(0.7)
		b.Rect(0.25, 0.25, width-0.5, height-0.5)
		b.Stroke()
		b.PopState()
	}
}

// Helvetica metric fractions used to lay out widget text exactly the way
// Acrobat does, so that our pre-generated /AP coincides with whatever a
// viewer would draw (no ghosting if it overlays its own form layer).
//   - helvLineHeight: Helvetica FontBBox height (1156/1000) — the per-row
//     advance Acrobat uses for list boxes and the line box for centring.
//   - helvDescentFrac: Helvetica descent magnitude (207/1000) — the gap
//     between a row's bottom and the text baseline.
//
// Verified against Acrobat-generated /AP streams in testdata/PdfWithAcroForm.pdf.
const (
	helvLineHeight  = 1.156
	helvDescentFrac = 0.207
)

// generateTextFieldAppearance renders the value into a /Tx-marked
// appearance stream matching Acrobat's single-line layout: white fill +
// border, clip to the 1pt-inset interior, value vertically centred via
// baseline = (h-rowH)/2 + descent. Multiline fields keep the wrapping
// renderTextInBuilder path (still wrapped in /Tx BMC).
func generateTextFieldAppearance(form *Form, widget pdfDict) *pdfStream {
	width, height := widgetSize(widget)
	if width <= 0 || height <= 0 {
		return makeFormXObject(nil, Rectangle{})
	}
	fontSize, textColor, fontRes := parseDA(dictGetString(widget, "/DA"))
	font := resolveWidgetFont(form, fontRes)
	value := decodeFormString(widget["/V"])
	multiline := widgetFieldFlags(form, widget)&fieldFlagMultiline != 0

	b := newAppearanceBuilder()
	drawWidgetChrome(b, widget, width, height)

	if value == "" {
		return makeFormXObject(b.Bytes(), Rectangle{URX: width, URY: height})
	}

	resources := pdfDict{}

	if multiline {
		b.BeginMarkedContentText()
		const pad = 2.0
		inner := Rectangle{LLX: pad, LLY: pad, URX: width - pad, URY: height - pad}
		style := TextStyle{Font: font, Size: fontSize, Color: &textColor,
			HAlign: widgetQuadAlign(widget), VAlign: VAlignTop}
		resolve := func(fn Font, _ pdfDict) (string, widthFn, encodeFn, float64, float64, error) {
			return resolveFontForXObject(fn, fontSize, form.doc, resources)
		}
		_ = renderTextInBuilder(b, resources, value, style, inner, resolve, "", "")
		b.EndMarkedContent()
		return makeFormXObjectWithResources(b.Bytes(), Rectangle{URX: width, URY: height}, resources)
	}

	resName, widthOf, encode, _, _, err := resolveFontForXObject(font, fontSize, form.doc, resources)
	if err != nil {
		return makeFormXObjectWithResources(b.Bytes(), Rectangle{URX: width, URY: height}, resources)
	}
	rowH := helvLineHeight * fontSize
	descent := helvDescentFrac * fontSize
	baseline := (height-rowH)/2 + descent
	x := widgetTextX(value, widthOf, widgetQuadAlign(widget), 2, width-2)

	b.BeginMarkedContentText()
	b.PushState()
	b.ClipRect(1, 1, width-2, height-2)
	b.TextLine(resName, fontSize, textColor, x, baseline, encode(value))
	b.PopState()
	b.EndMarkedContent()
	return makeFormXObjectWithResources(b.Bytes(), Rectangle{URX: width, URY: height}, resources)
}

// widgetTextX returns the x origin for a single line of text given the
// alignment, the usable [left,right] band, and a per-rune width function.
// Left alignment returns left; centre/right measure the string so it sits
// flush-centre or flush-right within the band.
func widgetTextX(text string, widthOf widthFn, align HAlign, left, right float64) float64 {
	if align == HAlignLeft {
		return left
	}
	var w float64
	for _, r := range text {
		w += widthOf(r)
	}
	switch align {
	case HAlignCenter:
		x := left + (right-left-w)/2
		if x < left {
			return left
		}
		return x
	case HAlignRight:
		x := right - w
		if x < left {
			return left
		}
		return x
	}
	return left
}

// regenerateCheckboxWidget writes both /AP/N/Off and /AP/N/<onName>
// streams. The on-name is read from the existing /AP/N dict (preserving
// custom export values from previous calls) and falls back to "Yes".
func regenerateCheckboxWidget(form *Form, widget pdfDict) {
	width, height := widgetSize(widget)
	if width <= 0 || height <= 0 {
		return
	}

	fill, stroke, mark := widgetMarkColors(widget)

	off := newAppearanceBuilder()
	drawCheckboxBox(off, width, height, fill, stroke)
	setWidgetAPN(form.doc, widget, makeFormXObject(off.Bytes(), Rectangle{URX: width, URY: height}), "Off")

	on := newAppearanceBuilder()
	drawCheckboxBox(on, width, height, fill, stroke)
	drawCheckMark(on, width, height, mark)
	onName := widgetOnStateName(widget)
	if onName == "" {
		onName = "Yes"
	}
	setWidgetAPN(form.doc, widget, makeFormXObject(on.Bytes(), Rectangle{URX: width, URY: height}), onName)
}

// widgetMarkColors resolves the fill, border, and mark colours used by
// checkbox / radio chrome from the widget's /MK (/BG, /BC) and /DA text
// colour, falling back to the library defaults (white fill, dark grey
// border, near-black mark) when unstyled.
func widgetMarkColors(widget pdfDict) (fill, stroke, mark Color) {
	fill = Color{R: 1, G: 1, B: 1, A: 1}
	stroke = Color{R: 0.35, G: 0.35, B: 0.35, A: 1}
	if bg := mkColor(widget, "/BG"); bg != nil {
		fill = *bg
	}
	if bc := mkColor(widget, "/BC"); bc != nil {
		stroke = *bc
	}
	_, mark, _ = parseDA(dictGetString(widget, "/DA"))
	return fill, stroke, mark
}

// drawCheckboxBox draws the interior fill + border that surrounds both
// states of a checkbox.
func drawCheckboxBox(b *appearanceBuilder, width, height float64, fill, stroke Color) {
	b.PushState()
	b.SetFillColorRGB(fill)
	b.Rect(0, 0, width, height)
	b.Fill()
	b.PopState()
	b.PushState()
	b.SetLineWidth(0.8)
	b.SetStrokeColorRGB(stroke)
	b.Rect(0.4, 0.4, width-0.8, height-0.8)
	b.Stroke()
	b.PopState()
}

// drawCheckMark paints a two-segment tick across the box from
// (~20% w, 50% h) to (~42% w, 28% h) to (~80% w, 75% h) — close to the
// standard Acrobat check geometry.
func drawCheckMark(b *appearanceBuilder, width, height float64, mark Color) {
	b.PushState()
	b.SetLineWidth(math.Min(width, height) * 0.12)
	b.SetStrokeColorRGB(mark)
	b.SetLineCap(LineCapRound)
	b.SetLineJoin(LineJoinRound)
	b.MoveTo(width*0.20, height*0.50)
	b.LineTo(width*0.42, height*0.28)
	b.LineTo(width*0.80, height*0.75)
	b.Stroke()
	b.PopState()
}

// regenerateRadioWidget paints a circle outline for the /Off state and
// a circle with an inner filled dot for the on state. The on-name is
// read from the widget's existing /AP/N keys.
func regenerateRadioWidget(form *Form, widget pdfDict) {
	width, height := widgetSize(widget)
	if width <= 0 || height <= 0 {
		return
	}
	cx, cy := width/2, height/2
	r := math.Min(width, height)/2 - 0.6
	fill, stroke, mark := widgetMarkColors(widget)

	off := newAppearanceBuilder()
	drawRadioRing(off, cx, cy, r, fill, stroke)
	setWidgetAPN(form.doc, widget, makeFormXObject(off.Bytes(), Rectangle{URX: width, URY: height}), "Off")

	on := newAppearanceBuilder()
	drawRadioRing(on, cx, cy, r, fill, stroke)
	on.PushState()
	on.SetFillColorRGB(mark)
	on.Ellipse(cx, cy, r*0.5, r*0.5)
	on.Fill()
	on.PopState()
	onName := widgetOnStateName(widget)
	if onName == "" {
		onName = "Yes"
	}
	setWidgetAPN(form.doc, widget, makeFormXObject(on.Bytes(), Rectangle{URX: width, URY: height}), onName)
}

// drawRadioRing paints the filled, stroked circle that forms the visual
// base of both radio states.
func drawRadioRing(b *appearanceBuilder, cx, cy, r float64, fill, stroke Color) {
	b.PushState()
	b.SetLineWidth(0.8)
	b.SetStrokeColorRGB(stroke)
	b.SetFillColorRGB(fill)
	b.Ellipse(cx, cy, r, r)
	b.FillStroke()
	b.PopState()
}

// generateComboBoxAppearance renders the selected value as a single
// centred line, matching Acrobat's combo /AP layout. No dropdown chevron
// is baked in — the viewer draws the dropdown button itself as widget
// chrome, so baking one would double it. Mirrors the reference combo /AP
// in testdata/PdfWithAcroForm.pdf (white fill, /Tx BMC, clipped, value
// at baseline = (h-rowH)/2 + descent).
func generateComboBoxAppearance(form *Form, widget pdfDict) *pdfStream {
	width, height := widgetSize(widget)
	if width <= 0 || height <= 0 {
		return makeFormXObject(nil, Rectangle{})
	}
	fontSize, textColor, fontRes := parseDA(dictGetString(widget, "/DA"))
	font := resolveWidgetFont(form, fontRes)
	value := decodeFormString(widget["/V"])

	b := newAppearanceBuilder()
	drawWidgetChrome(b, widget, width, height)

	if value == "" {
		return makeFormXObject(b.Bytes(), Rectangle{URX: width, URY: height})
	}
	resources := pdfDict{}
	resName, widthOf, encode, _, _, err := resolveFontForXObject(font, fontSize, form.doc, resources)
	if err != nil {
		return makeFormXObjectWithResources(b.Bytes(), Rectangle{URX: width, URY: height}, resources)
	}
	rowH := helvLineHeight * fontSize
	descent := helvDescentFrac * fontSize
	baseline := (height-rowH)/2 + descent
	x := widgetTextX(value, widthOf, widgetQuadAlign(widget), 2, width-2)

	b.BeginMarkedContentText()
	b.PushState()
	b.ClipRect(1, 1, width-2, height-2)
	b.TextLine(resName, fontSize, textColor, x, baseline, encode(value))
	b.PopState()
	b.EndMarkedContent()
	return makeFormXObjectWithResources(b.Bytes(), Rectangle{URX: width, URY: height}, resources)
}

// generateListBoxAppearance lays out each /Opt entry as a row exactly the
// way Acrobat does, so the pre-generated /AP coincides with the viewer's
// own list rendering and never ghosts. Layout (verified against the
// reference list box /AP in testdata/PdfWithAcroForm.pdf):
//   - white fill + border, then "/Tx BMC", then clip to the 1pt interior.
//   - rowH = 1.156*fontSize (Helvetica FontBBox height).
//   - row i spans y in [h-1-(i+1)*rowH, h-1-i*rowH]; baseline sits
//     `descent` (0.207*fontSize) above each row's bottom.
//   - selected rows (from /I, falling back to /V) get a light-blue
//     0.6/0.757/0.855 highlight band; ALL text is drawn black on top,
//     matching Acrobat (no white-on-blue).
//
// Rows past the bottom of the box are clipped.
func generateListBoxAppearance(form *Form, widget pdfDict) *pdfStream {
	width, height := widgetSize(widget)
	if width <= 0 || height <= 0 {
		return makeFormXObject(nil, Rectangle{})
	}
	fontSize, textColor, fontRes := parseDA(dictGetString(widget, "/DA"))
	font := resolveWidgetFont(form, fontRes)
	options := readChoiceOptions(widget["/Opt"])
	selected := widgetSelectedIndexSet(widget, options)

	b := newAppearanceBuilder()
	drawWidgetChrome(b, widget, width, height)

	resources := pdfDict{}
	resName, _, encode, _, _, err := resolveFontForXObject(font, fontSize, form.doc, resources)
	if err != nil {
		return makeFormXObjectWithResources(b.Bytes(), Rectangle{URX: width, URY: height}, resources)
	}
	rowH := helvLineHeight * fontSize
	descent := helvDescentFrac * fontSize
	highlight := Color{R: 0.600006, G: 0.756866, B: 0.854904, A: 1}

	b.BeginMarkedContentText()
	b.PushState()
	b.ClipRect(1, 1, width-2, height-2)

	// Highlight bands for selected rows first (so text lands on top).
	for i := range options {
		if !selected[i] {
			continue
		}
		yBot := height - 1 - float64(i+1)*rowH
		if yBot >= height-1 || yBot+rowH <= 1 {
			continue
		}
		b.PushState()
		b.SetFillColorRGB(highlight)
		b.Rect(1, yBot, width-2, rowH)
		b.Fill()
		b.PopState()
	}

	// Option text — black, one BT/ET per row at the Acrobat baseline.
	for i, opt := range options {
		baseline := height - 1 - float64(i+1)*rowH + descent
		if baseline > height-1 || baseline+rowH < 1 {
			continue // row fully outside the visible interior
		}
		b.TextLine(resName, fontSize, textColor, 2, baseline, encode(opt.Value))
	}

	b.PopState()
	b.EndMarkedContent()
	return makeFormXObjectWithResources(b.Bytes(), Rectangle{URX: width, URY: height}, resources)
}

// widgetSelectedIndexSet returns the set of selected /Opt indices for a
// choice widget. Prefers the /I (selected-indices) array; falls back to
// matching /V values against the option list when /I is absent.
func widgetSelectedIndexSet(widget pdfDict, options []ChoiceOption) map[int]bool {
	set := map[int]bool{}
	if iarr, ok := widget["/I"].(pdfArray); ok && len(iarr) > 0 {
		for _, v := range iarr {
			idx := toInt(v)
			if idx >= 0 && idx < len(options) {
				set[idx] = true
			}
		}
		return set
	}
	sel := widgetSelectedValues(widget)
	for i, opt := range options {
		if isOptionSelected(opt, sel) {
			set[i] = true
		}
	}
	return set
}

// widgetSelectedValues returns the strings in /V — handles string,
// array, and /Name forms. /Off returns nil. Used for listbox selection
// highlighting; combo and text values are read via decodeFormString.
func widgetSelectedValues(widget pdfDict) []string {
	switch v := widget["/V"].(type) {
	case string:
		s := decodeFormString(v)
		if s == "" {
			return nil
		}
		return []string{s}
	case pdfHexString:
		s := decodeFormString(v)
		if s == "" {
			return nil
		}
		return []string{s}
	case pdfArray:
		var out []string
		for _, item := range v {
			if s := decodeFormString(item); s != "" {
				out = append(out, s)
			}
		}
		return out
	case pdfName:
		if v == "/Off" || v == "" {
			return nil
		}
		return []string{string(v)[1:]}
	}
	return nil
}

// isOptionSelected returns true when either the option's display value
// or its export name appears in the selected list.
func isOptionSelected(opt ChoiceOption, selected []string) bool {
	for _, s := range selected {
		if s == opt.Value {
			return true
		}
		if opt.Export != "" && s == opt.Export {
			return true
		}
	}
	return false
}

// generatePushButtonAppearance paints a soft grey rounded-rectangle
// button with the caption from /MK/CA centred on top. Caption colour
// defaults to dark grey; no /DA so we use Helvetica Bold at a size
// proportional to the button height.
func generatePushButtonAppearance(form *Form, widget pdfDict) *pdfStream {
	width, height := widgetSize(widget)
	if width <= 0 || height <= 0 {
		return makeFormXObject(nil, Rectangle{})
	}

	b := newAppearanceBuilder()

	// Button face — light grey fill, slightly darker border.
	face := Color{R: 0.93, G: 0.93, B: 0.95, A: 1}
	border := Color{R: 0.55, G: 0.55, B: 0.60, A: 1}
	drawRoundedRectPath(b, 0.5, 0.5, width-1, height-1, math.Min(6, height/3))
	b.PushState()
	b.SetFillColorRGB(face)
	b.SetStrokeColorRGB(border)
	b.SetLineWidth(0.7)
	b.FillStroke()
	b.PopState()

	caption := readPushButtonCaption(widget)
	if caption == "" {
		return makeFormXObject(b.Bytes(), Rectangle{URX: width, URY: height})
	}
	// Caption text — Helvetica Bold, ~50% of height (capped at 14pt).
	fontSize := math.Min(height*0.5, 14)
	textColor := Color{R: 0.15, G: 0.15, B: 0.20, A: 1}
	style := TextStyle{
		Font:   FontHelveticaBold,
		Size:   fontSize,
		Color:  &textColor,
		HAlign: HAlignCenter,
		VAlign: VAlignMiddle,
	}
	resources := pdfDict{}
	resolve := func(font Font, _ pdfDict) (string, widthFn, encodeFn, float64, float64, error) {
		return resolveFontForXObject(font, fontSize, form.doc, resources)
	}
	_ = renderTextInBuilder(b, resources, caption, style,
		Rectangle{LLX: 4, LLY: 0, URX: width - 4, URY: height},
		resolve, "", "")
	return makeFormXObjectWithResources(b.Bytes(), Rectangle{URX: width, URY: height}, resources)
}

// readPushButtonCaption pulls /MK/CA from the widget dict. Returns ""
// when /MK or /CA is missing.
func readPushButtonCaption(widget pdfDict) string {
	mk, _ := widget["/MK"].(pdfDict)
	if mk == nil {
		return ""
	}
	return decodeFormString(mk["/CA"])
}

// drawRoundedRectPath appends a closed rounded-rectangle subpath to b,
// using four quarter-circle Bezier corners (kappa from
// appearance_builder.go). The radius is clamped to half the shorter
// side so the path stays valid for any input.
func drawRoundedRectPath(b *appearanceBuilder, x, y, w, h, r float64) {
	if r < 0 {
		r = 0
	}
	if r > w/2 {
		r = w / 2
	}
	if r > h/2 {
		r = h / 2
	}
	c := r * kappa
	// Move to start of bottom edge (after the bottom-left curve).
	b.MoveTo(x+r, y)
	b.LineTo(x+w-r, y)
	b.CurveTo(x+w-r+c, y, x+w, y+r-c, x+w, y+r)
	b.LineTo(x+w, y+h-r)
	b.CurveTo(x+w, y+h-r+c, x+w-r+c, y+h, x+w-r, y+h)
	b.LineTo(x+r, y+h)
	b.CurveTo(x+r-c, y+h, x, y+h-r+c, x, y+h-r)
	b.LineTo(x, y+r)
	b.CurveTo(x, y+r-c, x+r-c, y, x+r, y)
	b.ClosePath()
}
