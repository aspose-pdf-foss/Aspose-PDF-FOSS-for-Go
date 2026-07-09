// SPDX-License-Identifier: MIT

package asposepdf

import (
	"fmt"
	"html"
	"strings"
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

// writeHTMLFormFields emits the interactive controls for every convertible
// field whose widget sits on page p. dlSeq numbers <datalist> ids across
// the whole document.
func writeHTMLFormFields(b *strings.Builder, p *Page, pageH float64, dlSeq *int) {
	pageNum := p.Number()
	for _, f := range p.doc.Form().Fields() {
		switch fld := f.(type) {
		case *RadioButtonField:
			// A radio group owns one widget per option, possibly across
			// pages — place each option's input from its own widget.
			for _, opt := range fld.Options() {
				writeHTMLRadioOption(b, p, fld, opt, pageH, pageNum)
			}
		case *ButtonField:
			// Push buttons: actions are PDF-side; keep the rendered look.
		default:
			if f.PageIndex() != pageNum {
				continue
			}
			writeHTMLFormControl(b, f, pageH, dlSeq)
		}
	}
}

// writeHTMLRadioOption emits one radio input for one option widget of a
// radio group, if that widget is on the current page.
func writeHTMLRadioOption(b *strings.Builder, p *Page, fld *RadioButtonField, opt *RadioButtonOptionField, pageH float64, pageNum int) {
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
	attrs := fmt.Sprintf(` name="%s" value="%s"`, html.EscapeString(fld.FullName()), html.EscapeString(opt.Name()))
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
func writeHTMLFormControl(b *strings.Builder, f Field, pageH float64, dlSeq *int) {
	r := f.Rect()
	if r.URX <= r.LLX || r.URY <= r.LLY {
		return
	}
	name := html.EscapeString(f.FullName())
	style := controlPosCSS(r, pageH) + fieldStyleCSS(f.Style())
	common := fmt.Sprintf(` name="%s"`, name)
	if f.IsRequired() {
		common += " required"
	}

	switch fld := f.(type) {
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
			*dlSeq++
			id := *dlSeq
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
		// (Password/FileSelect/RichText/Number/Date render as text inputs
		// in phase 1; typed inputs are phase 2).
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
