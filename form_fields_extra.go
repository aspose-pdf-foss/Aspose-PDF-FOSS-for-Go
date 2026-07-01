// SPDX-License-Identifier: MIT

package asposepdf

import "fmt"

// Extra AcroForm field types (form_fields_extra.go) — Password, FileSelect,
// RichText, Number and Date. Each is a text field (/FT /Tx) distinguished by a
// field flag (/Ff bit) or a JavaScript format action (/AA), so it round-trips
// through Save+Open (fieldFromNode reclassifies from the flag or the format JS).
// Each type embeds TextBoxField, inheriting the full text-field surface
// (Value/SetValue, SetMaxLen, SetReadOnly, SetStyle, Flatten, …) and adds its
// type-specific behaviour. Mirrors Aspose.PDF for .NET's NumberField / DateField
// / PasswordBoxField / RichTextBoxField / FileSelectBoxField.

// Text-field /Ff flags added here (ISO 32000-1 §12.7.4.3 Table 228).
const (
	fieldFlagFileSelect = 1 << 20 // bit 21 — the field value is a file path
	fieldFlagRichText   = 1 << 25 // bit 26 — the value is rich text (/RV)
)

// PasswordBoxField is a text field whose input is masked (Password flag). Its
// value is not stored in a saved file by conforming viewers.
type PasswordBoxField struct{ TextBoxField }

// FileSelectBoxField is a text field whose value is a file path (FileSelect
// flag), used to attach a local file on submit.
type FileSelectBoxField struct{ TextBoxField }

// RichTextBoxField is a text field that carries a rich-text value (RichText
// flag + /RV) in addition to its plain /V.
type RichTextBoxField struct{ TextBoxField }

// NumberField is a text field with a JavaScript number-format action, so viewers
// display and validate the value as a formatted number.
type NumberField struct{ TextBoxField }

// DateField is a text field with a JavaScript date-format action and a format
// mask (e.g. "mm/dd/yyyy").
type DateField struct{ TextBoxField }

// NumberFormatOptions configures a NumberField's display formatting (maps to
// Acrobat's AFNumber_Format). The zero value is a plain integer.
type NumberFormatOptions struct {
	Decimals        int    // number of decimal places (0 = integer)
	UseSeparator    bool   // group thousands (1,234)
	CurrencySymbol  string // e.g. "$", "€" (empty = none)
	CurrencyPrepend bool   // symbol before the number ("$1" vs "1$")
}

// AddPasswordField adds a masked text field.
func (f *Form) AddPasswordField(pageNum int, rect Rectangle, name string) (*PasswordBoxField, error) {
	fld, err := f.addTextFieldConfigured(pageNum, rect, name, func(dict pdfDict) {
		dict["/Ff"] = fieldFlagPassword
	})
	if err != nil {
		return nil, err
	}
	return fld.(*PasswordBoxField), nil
}

// AddFileSelectField adds a file-select text field.
func (f *Form) AddFileSelectField(pageNum int, rect Rectangle, name string) (*FileSelectBoxField, error) {
	fld, err := f.addTextFieldConfigured(pageNum, rect, name, func(dict pdfDict) {
		dict["/Ff"] = fieldFlagFileSelect
	})
	if err != nil {
		return nil, err
	}
	return fld.(*FileSelectBoxField), nil
}

// AddRichTextField adds a rich-text field.
func (f *Form) AddRichTextField(pageNum int, rect Rectangle, name string) (*RichTextBoxField, error) {
	fld, err := f.addTextFieldConfigured(pageNum, rect, name, func(dict pdfDict) {
		dict["/Ff"] = fieldFlagRichText
	})
	if err != nil {
		return nil, err
	}
	return fld.(*RichTextBoxField), nil
}

// AddNumberField adds a number field formatted per opts.
func (f *Form) AddNumberField(pageNum int, rect Rectangle, name string, opts NumberFormatOptions) (*NumberField, error) {
	fld, err := f.addTextFieldConfigured(pageNum, rect, name, func(dict pdfDict) {
		dict["/AA"] = numberFormatAA(opts)
	})
	if err != nil {
		return nil, err
	}
	return fld.(*NumberField), nil
}

// AddDateField adds a date field with the given format mask (e.g. "mm/dd/yyyy";
// empty defaults to "mm/dd/yyyy").
func (f *Form) AddDateField(pageNum int, rect Rectangle, name, format string) (*DateField, error) {
	if format == "" {
		format = "mm/dd/yyyy"
	}
	fld, err := f.addTextFieldConfigured(pageNum, rect, name, func(dict pdfDict) {
		dict["/AA"] = dateFormatAA(format)
	})
	if err != nil {
		return nil, err
	}
	return fld.(*DateField), nil
}

// Format returns the date field's format mask (parsed back from its format JS).
func (d *DateField) Format() string {
	return formatJSArg(d.node.dict, "AFDate_FormatEx")
}

// SetRichValue sets the rich-text value (/RV, XHTML) and mirrors a plain-text
// fallback into /V.
func (r *RichTextBoxField) SetRichValue(rv, plain string) error {
	r.node.dict["/RV"] = encodeFormString(rv)
	noteFormMutated(r.node)
	return r.SetValue(plain)
}

// RichValue returns the field's /RV rich-text value.
func (r *RichTextBoxField) RichValue() string {
	return decodeFormString(r.node.dict["/RV"])
}

// numberFormatAA builds the /AA dictionary (format + keystroke JavaScript) for a
// number field.
func numberFormatAA(opts NumberFormatOptions) pdfDict {
	sepStyle := 1 // no thousands separator
	if opts.UseSeparator {
		sepStyle = 0
	}
	prepend := "false"
	if opts.CurrencyPrepend {
		prepend = "true"
	}
	args := fmt.Sprintf("%d, %d, 0, 0, \"%s\", %s",
		opts.Decimals, sepStyle, jsQuote(opts.CurrencySymbol), prepend)
	return formatAA("AFNumber_Format("+args+");", "AFNumber_Keystroke("+args+");")
}

// dateFormatAA builds the /AA dictionary for a date field.
func dateFormatAA(format string) pdfDict {
	q := jsQuote(format)
	return formatAA(
		"AFDate_FormatEx(\""+q+"\");",
		"AFDate_KeystrokeEx(\""+q+"\");")
}

// formatAA assembles an /AA dict with /F (format) and /K (keystroke) JavaScript
// actions.
func formatAA(formatJS, keystrokeJS string) pdfDict {
	return pdfDict{
		"/F": pdfDict{"/S": pdfName("/JavaScript"), "/JS": formatJS},
		"/K": pdfDict{"/S": pdfName("/JavaScript"), "/JS": keystrokeJS},
	}
}

// jsQuote escapes a string for embedding inside a double-quoted JavaScript
// string literal in the format actions.
func jsQuote(s string) string {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		if s[i] == '"' || s[i] == '\\' {
			out = append(out, '\\')
		}
		out = append(out, s[i])
	}
	return string(out)
}

// nodeHasFormatJS reports whether the field's /AA/F (or /K) JavaScript contains
// the given function name — used by fieldFromNode to recognise Number/Date
// fields on read.
func nodeHasFormatJS(n *fieldNode, fn string) bool {
	aa, ok := n.dict["/AA"].(pdfDict)
	if !ok {
		return false
	}
	for _, key := range []string{"/F", "/K"} {
		if act, ok := aa[key].(pdfDict); ok {
			if js, ok := act["/JS"].(string); ok && containsSub(js, fn) {
				return true
			}
		}
	}
	return false
}

// formatJSArg extracts the first double-quoted argument of the named JS format
// function from the field's /AA/F action (used by DateField.Format).
func formatJSArg(dict pdfDict, fn string) string {
	aa, ok := dict["/AA"].(pdfDict)
	if !ok {
		return ""
	}
	act, ok := aa["/F"].(pdfDict)
	if !ok {
		return ""
	}
	js, ok := act["/JS"].(string)
	if !ok || !containsSub(js, fn) {
		return ""
	}
	// Return the substring between the first pair of double quotes.
	start := -1
	for i := 0; i < len(js); i++ {
		if js[i] == '"' {
			if start < 0 {
				start = i + 1
			} else {
				return js[start:i]
			}
		}
	}
	return ""
}

// containsSub reports whether s contains sub (small helper to avoid importing
// strings just for this).
func containsSub(s, sub string) bool {
	if len(sub) == 0 {
		return true
	}
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// textFieldJSONType returns the JSON "type" tag for a text-family field.
func textFieldJSONType(f Field) string {
	switch FieldType(f) {
	case FormFieldTypePassword:
		return "password"
	case FormFieldTypeFileSelect:
		return "fileselect"
	case FormFieldTypeRichText:
		return "richtext"
	case FormFieldTypeNumber:
		return "number"
	case FormFieldTypeDate:
		return "date"
	}
	return "text"
}

// asTextField returns the embedded *TextBoxField for any text-family field (the
// base type or one of the typed variants), so shared code — form-data
// export/import — can treat them uniformly.
func asTextField(f Field) (*TextBoxField, bool) {
	switch x := f.(type) {
	case *TextBoxField:
		return x, true
	case *PasswordBoxField:
		return &x.TextBoxField, true
	case *FileSelectBoxField:
		return &x.TextBoxField, true
	case *RichTextBoxField:
		return &x.TextBoxField, true
	case *NumberField:
		return &x.TextBoxField, true
	case *DateField:
		return &x.TextBoxField, true
	}
	return nil, false
}
