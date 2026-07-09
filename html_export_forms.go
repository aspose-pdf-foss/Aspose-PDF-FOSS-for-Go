// SPDX-License-Identifier: MIT

package asposepdf

import (
	"fmt"
	"html"
	"strconv"
	"strings"
	"time"
)

// Interactive AcroForm export for the HTML exporter (epic pdf-go-dvte,
// phase 1). With HTMLSaveOptions.InteractiveForms set (HTMLModeText /
// HTMLModeNative only), form fields become real, fillable HTML controls
// positioned over the page:
//
//   text box → <input type="text"/"password"> or <textarea> (multiline),
//   checkbox → <input type="checkbox">, radio group → one
//   <input type="radio"> per option widget sharing the field's name,
//   combo box → <select> (editable combos → <input> + <datalist>),
//   list box → <select [multiple]>.
//
// Values, required and read-only flags, MaxLen, and FieldStyle (/MK /BS
// /DA /Q — border, background, text colour/size, alignment) carry over.
// The converted widgets' /AP appearances are suppressed from the page
// background render (renderer.hideFormWidgets) so the control is not drawn
// twice. Push buttons and signature widgets are not converted and keep
// their static appearance. The result can be filled in and printed in a
// browser without any JavaScript; writing values back into the PDF is out
// of scope (the JSON/FDF/XFDF interchange covers the server side).

// convertibleWidget reports whether a widget annotation belongs to a field
// the interactive-forms exporter converts to an HTML control: text (/Tx),
// choice (/Ch), and non-pushbutton buttons (/Btn checkboxes and radios).
// Signature fields and push buttons keep their rendered appearance.
func convertibleWidget(objects map[int]*pdfObject, ad pdfDict) bool {
	if dictGetName(ad, "/Subtype") != "/Widget" {
		return false
	}
	switch inheritedFieldType(objects, ad) {
	case "/Tx", "/Ch":
		return true
	case "/Btn":
		return inheritedFieldFlags(objects, ad)&fieldFlagPushbutton == 0
	}
	return false
}

// inheritedFieldFlags returns the /Ff flags of a widget's field, walking
// the /Parent chain like inheritedFieldType.
func inheritedFieldFlags(objects map[int]*pdfObject, ad pdfDict) int {
	for i := 0; i < 32 && ad != nil; i++ {
		if v, ok := ad["/Ff"]; ok {
			return toInt(resolveRef(objects, v))
		}
		parent, ok := resolveRefToDict(objects, ad["/Parent"])
		if !ok {
			return 0
		}
		ad = parent
	}
	return 0
}

// widgetDicts exposes a field's widget dictionaries to the HTML exporter
// (promoted onto every concrete field type through fieldBase).
func (b *fieldBase) widgetDicts() []pdfDict {
	if b.node == nil {
		return nil
	}
	return b.node.widgets
}

// pageTabOrder maps each /Annots entry (by dict identity) to its 1-based
// position — the PDF default tab order — and returns the entry count.
func pageTabOrder(p *Page) (map[string]int, int) {
	m := map[string]int{}
	pd := p.pageDict()
	if pd == nil {
		return m, 0
	}
	annots, ok := resolveRefToArray(p.doc.objects, pd["/Annots"])
	if !ok {
		return m, 0
	}
	for i, a := range annots {
		if ad, ok := resolveRefToDict(p.doc.objects, a); ok {
			m[fmt.Sprintf("%p", ad)] = i + 1
		}
	}
	return m, len(annots)
}

// writeHTMLFormFields emits the interactive controls for every convertible
// field whose widget sits on page p. Controls carry a tabindex following the
// page /Annots order (the PDF default tab order), offset so it stays
// ascending across pages.
func writeHTMLFormFields(b *strings.Builder, p *Page, pageH float64, ctx *htmlWriteCtx) {
	pageNum := p.Number()
	tabs, annotCount := pageTabOrder(p)
	tab := func(w pdfDict) string {
		if pos, ok := tabs[fmt.Sprintf("%p", w)]; ok {
			return fmt.Sprintf(` tabindex="%d"`, ctx.tabBase+pos)
		}
		return ""
	}
	for _, f := range p.doc.Form().Fields() {
		switch fld := f.(type) {
		case *RadioButtonField:
			// A radio group owns one widget per option, possibly across
			// pages — place each option's input from its own widget.
			for _, opt := range fld.Options() {
				writeHTMLRadioOption(b, p, fld, opt, pageH, pageNum, tab(opt.widget))
			}
		case *ButtonField:
			if f.PageIndex() != pageNum {
				continue
			}
			writeHTMLPushButton(b, fld, pageH, tab)
		default:
			if f.PageIndex() != pageNum {
				continue
			}
			tabAttr := ""
			if ws := fld.(interface{ widgetDicts() []pdfDict }).widgetDicts(); len(ws) > 0 {
				tabAttr = tab(ws[0])
			}
			writeHTMLFormControl(b, f, pageH, ctx, tabAttr)
		}
	}
	ctx.tabBase += annotCount
}

// pushButtonHTMLAction returns a push-button widget's Submit/Reset form
// action (walking /Parent for an inherited /A), or nil — buttons with other
// actions (JavaScript, GoTo, …) or none keep their static appearance.
func pushButtonHTMLAction(objects map[int]*pdfObject, ad pdfDict) Action {
	for i := 0; i < 32 && ad != nil; i++ {
		if d, ok := resolveRefToDict(objects, ad["/A"]); ok {
			switch act := parseAction(d).(type) {
			case *SubmitFormAction, *ResetFormAction:
				return act
			default:
				return nil
			}
		}
		parent, ok := resolveRefToDict(objects, ad["/Parent"])
		if !ok {
			return nil
		}
		ad = parent
	}
	return nil
}

// htmlFormEnvelope decides whether the exported pages need a document-level
// <form> wrapper (any convertible submit/reset push button exists) and
// returns the default action URL and method from the first submit action.
func htmlFormEnvelope(d *Document) (action, method string, wrap bool) {
	for _, f := range d.Form().Fields() {
		btn, ok := f.(*ButtonField)
		if !ok {
			continue
		}
		for _, w := range btn.widgetDicts() {
			switch act := pushButtonHTMLAction(d.objects, w).(type) {
			case *SubmitFormAction:
				if !wrap || action == "" {
					action = act.URL()
					method = submitMethod(act)
				}
				wrap = true
			case *ResetFormAction:
				wrap = true
			}
		}
	}
	return action, method, wrap
}

// submitMethod maps the SubmitForm GET flag to the HTML form method.
func submitMethod(a *SubmitFormAction) string {
	if a.Flags()&SubmitGetMethod != 0 {
		return "get"
	}
	return "post"
}

// writeHTMLPushButton emits a submit/reset <button> for a push button whose
// action is a form action; other push buttons keep their rendered look.
// Submission posts standard HTML form data (urlencoded/multipart) — the
// PDF-side FDF/XFDF export formats do not apply to the converted form.
func writeHTMLPushButton(b *strings.Builder, fld *ButtonField, pageH float64, tab func(pdfDict) string) {
	ws := fld.widgetDicts()
	if len(ws) == 0 {
		return
	}
	objects := fld.node.form.doc.objects
	act := pushButtonHTMLAction(objects, ws[0])
	if act == nil {
		return
	}
	r := fld.Rect()
	if r.URX <= r.LLX || r.URY <= r.LLY {
		return
	}
	caption := fld.PartialName()
	if mk, ok := resolveRefToDict(objects, ws[0]["/MK"]); ok {
		if ca := decodeFormString(mk["/CA"]); ca != "" {
			caption = ca
		}
	}
	attrs := tab(ws[0])
	if fld.IsReadOnly() {
		attrs += " disabled"
	}
	switch a := act.(type) {
	case *SubmitFormAction:
		attrs = ` type="submit"` + attrs
		if a.URL() != "" {
			attrs += fmt.Sprintf(` formaction="%s" formmethod="%s"`, html.EscapeString(a.URL()), submitMethod(a))
		}
	case *ResetFormAction:
		attrs = ` type="reset"` + attrs
	}
	fmt.Fprintf(b, "<button class=\"fw\"%s style=\"%s\">%s</button>\n",
		attrs, controlPosCSS(r, pageH)+fieldStyleCSS(fld.Style()), html.EscapeString(caption))
}

// writeHTMLRadioOption emits one radio input for one option widget of a
// radio group, if that widget is on the current page.
func writeHTMLRadioOption(b *strings.Builder, p *Page, fld *RadioButtonField, opt *RadioButtonOptionField, pageH float64, pageNum int, tabAttr string) {
	pref, ok := opt.widget["/P"].(pdfRef)
	if !ok {
		return
	}
	onPage := 0
	for i, pg := range p.doc.pages {
		if pg.Num == pref.Num {
			onPage = i + 1
			break
		}
	}
	if onPage != pageNum {
		return
	}
	rect, ok := normRect(shFloats(p.doc.objects, opt.widget["/Rect"]))
	if !ok {
		return
	}
	attrs := fmt.Sprintf(` name="%s" value="%s"`, html.EscapeString(fld.FullName()), html.EscapeString(opt.Name())) + tabAttr
	if opt.Selected() {
		attrs += " checked"
	}
	if fld.IsRequired() {
		attrs += " required"
	}
	if fld.IsReadOnly() {
		attrs += " disabled"
	}
	fmt.Fprintf(b, "<input class=\"fw\" type=\"radio\"%s style=\"%s\">\n",
		attrs, controlPosCSS(Rectangle{LLX: rect[0], LLY: rect[1], URX: rect[2], URY: rect[3]}, pageH)+fieldStyleCSS(fld.Style()))
}

// writeHTMLFormControl emits the HTML control for one single-widget field.
func writeHTMLFormControl(b *strings.Builder, f Field, pageH float64, ctx *htmlWriteCtx, tabAttr string) {
	r := f.Rect()
	if r.URX <= r.LLX || r.URY <= r.LLY {
		return
	}
	name := html.EscapeString(f.FullName())
	style := controlPosCSS(r, pageH) + fieldStyleCSS(f.Style())
	common := fmt.Sprintf(` name="%s"`, name) + tabAttr
	if f.IsRequired() {
		common += " required"
	}

	switch fld := f.(type) {
	case *NumberField:
		// AFNumber_Format's decimal count maps to the step attribute; the
		// stored value is sanitized to the plain numeric form the HTML
		// number input accepts (currency symbols and separators dropped).
		if f.IsReadOnly() {
			common += " readonly"
		}
		step := "1"
		if d := numberFieldDecimals(fld); d > 0 {
			step = "0." + strings.Repeat("0", d-1) + "1"
		}
		val := ""
		if v, ok := sanitizeHTMLNumber(f.Value()); ok {
			val = v
		}
		fmt.Fprintf(b, "<input class=\"fw\" type=\"number\" step=\"%s\"%s value=\"%s\" style=\"%s\">\n",
			step, common, html.EscapeString(val), style)

	case *DateField:
		// The HTML date input requires ISO yyyy-mm-dd; convert the stored
		// value through the field's format mask. An unconvertible mask or
		// value falls back to a plain text input so nothing is lost.
		layout := dateMaskToGoLayout(fld.Format())
		iso, ok := "", false
		if layout != "" {
			if v := f.Value(); v == "" {
				ok = true
			} else if t, err := time.Parse(layout, v); err == nil {
				iso, ok = t.Format("2006-01-02"), true
			}
		}
		if !ok {
			writeHTMLTextControl(b, f, common, style)
			return
		}
		if f.IsReadOnly() {
			common += " readonly"
		}
		fmt.Fprintf(b, "<input class=\"fw\" type=\"date\"%s value=\"%s\" style=\"%s\">\n",
			common, iso, style)
	case *CheckboxField:
		attrs := common + fmt.Sprintf(` value="%s"`, html.EscapeString(fld.checkedExportName()))
		if fld.Checked() {
			attrs += " checked"
		}
		if f.IsReadOnly() {
			attrs += " disabled"
		}
		fmt.Fprintf(b, "<input class=\"fw\" type=\"checkbox\"%s style=\"%s\">\n", attrs, style)

	case *ComboBoxField:
		if f.IsReadOnly() {
			common += " disabled"
		}
		if fld.node.ff&fieldFlagEdit != 0 {
			// Editable combo: free text plus suggestions.
			ctx.dlSeq++
			id := ctx.dlSeq
			fmt.Fprintf(b, "<input class=\"fw\" type=\"text\"%s list=\"dl%d\" value=\"%s\" style=\"%s\">\n",
				common, id, html.EscapeString(fld.Value()), style)
			fmt.Fprintf(b, "<datalist id=\"dl%d\">\n", id)
			for _, opt := range fld.Options() {
				fmt.Fprintf(b, "<option value=\"%s\">\n", html.EscapeString(opt.Value))
			}
			b.WriteString("</datalist>\n")
			return
		}
		fmt.Fprintf(b, "<select class=\"fw\"%s style=\"%s\">\n", common, style)
		writeHTMLChoiceOptions(b, fld.Options(), []int{fld.Selected()})
		b.WriteString("</select>\n")

	case *ListBoxField:
		if f.IsReadOnly() {
			common += " disabled"
		}
		if fld.MultiSelect() {
			common += " multiple"
		}
		size := len(fld.Options())
		if size < 2 {
			size = 2 // size=1 renders as a dropdown, not a list
		}
		fmt.Fprintf(b, "<select class=\"fw\" size=\"%d\"%s style=\"%s\">\n", size, common, style)
		writeHTMLChoiceOptions(b, fld.Options(), fld.Selected())
		b.WriteString("</select>\n")

	default:
		// The text family: TextBoxField and everything embedding it
		// (Password/FileSelect/RichText render as text inputs).
		writeHTMLTextControl(b, f, common, style)
	}
}

// writeHTMLTextControl emits the input/textarea for a text-family field —
// also the fallback for a DateField whose mask or value can't be converted.
func writeHTMLTextControl(b *strings.Builder, f Field, common, style string) {
	tf, ok := f.(interface {
		IsMultiline() bool
		IsPassword() bool
		MaxLen() int
	})
	if !ok {
		return
	}
	if f.IsReadOnly() {
		common += " readonly"
	}
	if ml := tf.MaxLen(); ml > 0 {
		common += fmt.Sprintf(` maxlength="%d"`, ml)
	}
	if tf.IsMultiline() {
		fmt.Fprintf(b, "<textarea class=\"fw\"%s style=\"%s\">%s</textarea>\n",
			common, style, html.EscapeString(f.Value()))
		return
	}
	typ := "text"
	if tf.IsPassword() {
		typ = "password"
	}
	fmt.Fprintf(b, "<input class=\"fw\" type=\"%s\"%s value=\"%s\" style=\"%s\">\n",
		typ, common, html.EscapeString(f.Value()), style)
}

// numberFieldDecimals extracts the decimal count from the field's
// AFNumber_Format JavaScript (its first argument).
func numberFieldDecimals(f *NumberField) int {
	aa, ok := f.node.dict["/AA"].(pdfDict)
	if !ok {
		return 0
	}
	for _, key := range []string{"/F", "/K"} {
		act, ok := aa[key].(pdfDict)
		if !ok {
			continue
		}
		js, ok := act["/JS"].(string)
		if !ok {
			continue
		}
		i := strings.Index(js, "AFNumber_Format(")
		if i < 0 {
			continue
		}
		rest := js[i+len("AFNumber_Format("):]
		n, seen := 0, false
		for _, c := range rest {
			if c >= '0' && c <= '9' {
				n, seen = n*10+int(c-'0'), true
				continue
			}
			break
		}
		if seen {
			return n
		}
	}
	return 0
}

// sanitizeHTMLNumber reduces a stored number-field value (possibly carrying
// a currency symbol and thousands separators) to the plain form the HTML
// number input accepts. Returns ok=false when nothing numeric remains.
func sanitizeHTMLNumber(v string) (string, bool) {
	var out strings.Builder
	for _, c := range v {
		if c >= '0' && c <= '9' || c == '.' || c == '-' {
			out.WriteRune(c)
		}
	}
	s := out.String()
	if s == "" {
		return "", false
	}
	if _, err := strconv.ParseFloat(s, 64); err != nil {
		return "", false
	}
	return s, true
}

// dateMaskToGoLayout converts an AFDate format mask (e.g. "dd.mm.yyyy") to
// a Go time layout. Masks with textual months or time parts return "" (the
// field then falls back to a plain text input).
func dateMaskToGoLayout(mask string) string {
	if mask == "" {
		return ""
	}
	var out strings.Builder
	haveY, haveM, haveD := false, false, false
	for i := 0; i < len(mask); {
		rest := mask[i:]
		switch {
		case strings.HasPrefix(rest, "yyyy"):
			out.WriteString("2006")
			haveY = true
			i += 4
		case strings.HasPrefix(rest, "yy"):
			out.WriteString("06")
			haveY = true
			i += 2
		case strings.HasPrefix(rest, "mmm"): // textual month — not converted
			return ""
		case strings.HasPrefix(rest, "mm"):
			out.WriteString("01")
			haveM = true
			i += 2
		case strings.HasPrefix(rest, "m"):
			out.WriteString("1")
			haveM = true
			i++
		case strings.HasPrefix(rest, "dd"):
			out.WriteString("02")
			haveD = true
			i += 2
		case strings.HasPrefix(rest, "d"):
			out.WriteString("2")
			haveD = true
			i++
		case (rest[0] >= 'a' && rest[0] <= 'z') || (rest[0] >= 'A' && rest[0] <= 'Z'):
			return "" // time parts (h/H/M/t) and other letters — not converted
		default:
			out.WriteByte(rest[0])
			i++
		}
	}
	if !haveY || !haveM || !haveD {
		return ""
	}
	return out.String()
}

// writeHTMLChoiceOptions emits <option> elements, marking selected indices.
func writeHTMLChoiceOptions(b *strings.Builder, opts []ChoiceOption, selected []int) {
	sel := make(map[int]bool, len(selected))
	for _, i := range selected {
		sel[i] = true
	}
	for i, opt := range opts {
		val := opt.Value
		if opt.Export != "" {
			val = opt.Export
		}
		mark := ""
		if sel[i] {
			mark = " selected"
		}
		fmt.Fprintf(b, "<option value=\"%s\"%s>%s</option>\n",
			html.EscapeString(val), mark, html.EscapeString(opt.Value))
	}
}

// controlPosCSS positions a control absolutely over its widget rect.
func controlPosCSS(r Rectangle, pageH float64) string {
	return fmt.Sprintf("left:%spt;top:%spt;width:%spt;height:%spt",
		htmlNum(r.LLX), htmlNum(pageH-r.URY), htmlNum(r.URX-r.LLX), htmlNum(r.URY-r.LLY))
}

// fieldStyleCSS maps a FieldStyle (/MK /BS /DA /Q) onto inline CSS. Only
// properties the field actually sets are emitted; browser defaults cover
// the rest.
func fieldStyleCSS(s FieldStyle) string {
	css := ""
	if s.BorderColor != nil {
		w := s.BorderWidth
		if w <= 0 {
			w = 1
		}
		line := "solid"
		switch s.BorderStyle {
		case BorderDashed:
			line = "dashed"
		case BorderBeveled:
			line = "outset"
		case BorderInset:
			line = "inset"
		}
		if s.BorderStyle == BorderUnderline {
			css += fmt.Sprintf(";border:none;border-bottom:%spt solid %s", htmlNum(w), htmlColor(*s.BorderColor))
		} else {
			css += fmt.Sprintf(";border:%spt %s %s", htmlNum(w), line, htmlColor(*s.BorderColor))
		}
	}
	if s.BackgroundColor != nil {
		css += ";background-color:" + htmlColor(*s.BackgroundColor)
	}
	if s.TextColor != nil {
		css += ";color:" + htmlColor(*s.TextColor)
	}
	if s.TextSize > 0 {
		css += ";font-size:" + htmlNum(s.TextSize) + "pt"
	}
	switch s.TextAlign {
	case HAlignCenter:
		css += ";text-align:center"
	case HAlignRight:
		css += ";text-align:right"
	}
	return css
}
