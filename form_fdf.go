// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"fmt"
	"io"
	"strings"
)

// Form-data interchange — FDF (this file) and XFDF (form_xfdf.go) export/import,
// the Acrobat-interoperable counterparts to the typed JSON in form_json.go. All
// three share the format-neutral value model below (formValues / applyFormValues),
// so a value set from any format regenerates the widget appearance via the
// existing field setters. Mirrors Aspose.PDF for .NET's
// Facades.Form.Export/Import{Fdf,Xfdf}.

// formValues extracts one field as a format-neutral value: its kind, the
// value(s) as strings (one for text/combo/checkbox/radio, N for a list box), and
// whether the value is a PDF name (checkbox/radio on-state) rather than a string.
// ok is false for fields with no value (push buttons).
func formValues(fld Field) (kind string, values []string, asName, ok bool) {
	if tb, ok := asTextField(fld); ok {
		return "text", []string{tb.Value()}, false, true
	}
	switch x := fld.(type) {
	case *ComboBoxField:
		return "combobox", []string{x.Value()}, false, true
	case *CheckboxField:
		name := strings.TrimPrefix(x.Value(), "/")
		if name == "" {
			name = "Off"
		}
		return "checkbox", []string{name}, true, true
	case *RadioButtonField:
		name := radioSelectedName(x)
		if name == "" {
			name = "Off"
		}
		return "radio", []string{name}, true, true
	case *ListBoxField:
		return "listbox", listSelectedValues(x), false, true
	default:
		return "", nil, false, false
	}
}

// applyFormValues sets a field from format-neutral string values (the inverse of
// formValues), dispatching on the field's concrete type. A checkbox is "on"
// unless the value is empty or "Off"; a list box selects every matching option.
func applyFormValues(fld Field, values []string) error {
	if tb, ok := asTextField(fld); ok {
		return tb.SetValue(first(values))
	}
	switch x := fld.(type) {
	case *ComboBoxField:
		if len(values) == 0 {
			return nil
		}
		return x.SetValue(values[0])
	case *RadioButtonField:
		return x.SetValue(first(values))
	case *CheckboxField:
		on := len(values) > 0 && values[0] != "" && values[0] != "Off"
		x.SetChecked(on)
		return nil
	case *ListBoxField:
		return setListByValues(x, values)
	default:
		return fmt.Errorf("form: cannot import into field %q (unsupported type)", fld.FullName())
	}
}

func first(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

// pdfValueToStrings normalises a parsed /V (string, name, or array of those)
// into format-neutral strings, decoding text and stripping name slashes.
func pdfValueToStrings(v pdfValue) []string {
	switch x := v.(type) {
	case string:
		return []string{decodeFormString(x)}
	case pdfName:
		return []string{strings.TrimPrefix(string(x), "/")}
	case pdfArray:
		out := make([]string, 0, len(x))
		for _, it := range x {
			switch y := it.(type) {
			case string:
				out = append(out, decodeFormString(y))
			case pdfName:
				out = append(out, strings.TrimPrefix(string(y), "/"))
			}
		}
		return out
	}
	return nil
}

// ExportFDF serialises the form's field values as an FDF (Forms Data Format)
// document — a small PDF-syntax file (`%FDF-1.2` … `/FDF /Fields [ … ]`) that
// Acrobat and other readers import. Field names are written flat (the full name
// in each `/T`); push buttons are skipped. Mirrors Aspose.PDF for .NET's
// `Facades.Form.ExportFdf`.
func (f *Form) ExportFDF() ([]byte, error) {
	var fields bytes.Buffer
	for _, fld := range f.Fields() {
		_, values, asName, ok := formValues(fld)
		if !ok {
			continue
		}
		fields.WriteString("<< /T ")
		fields.WriteString(fdfString(fld.FullName()))
		fields.WriteString(" /V ")
		fields.WriteString(fdfFieldValue(values, asName))
		fields.WriteString(" >>\n")
	}
	var b bytes.Buffer
	b.WriteString("%FDF-1.2\n")
	b.WriteString("1 0 obj\n<< /FDF << /Fields [\n")
	b.Write(fields.Bytes())
	b.WriteString("] >> >>\nendobj\n")
	b.WriteString("trailer\n<< /Root 1 0 R >>\n%%EOF\n")
	return b.Bytes(), nil
}

// WriteFDF writes ExportFDF output to w.
func (f *Form) WriteFDF(w io.Writer) error {
	data, err := f.ExportFDF()
	if err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}

// ImportFDF applies field values from an FDF document and returns the number of
// fields set. Names are matched by full name (FDF `/Kids` nesting is resolved to
// dotted names); unknown names and non-applying values are skipped. Mirrors
// Aspose.PDF for .NET's `Facades.Form.ImportFdf`.
func (f *Form) ImportFDF(data []byte) (int, error) {
	fields, err := parseFDFFields(data)
	if err != nil {
		return 0, err
	}
	applied := 0
	for name, v := range fields {
		fld := f.Field(name)
		if fld == nil {
			continue
		}
		if err := applyFormValues(fld, pdfValueToStrings(v)); err != nil {
			continue
		}
		applied++
	}
	return applied, nil
}

// ReadFDF reads an FDF document from r and applies it via ImportFDF.
func (f *Form) ReadFDF(r io.Reader) (int, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return 0, err
	}
	return f.ImportFDF(data)
}

// fdfFieldValue serialises a field's value(s) as the FDF `/V`: a name for
// checkbox/radio (`/Yes`, `/Off`), a string literal for single text/choice
// values, or an array of strings for a multi-select list box.
func fdfFieldValue(values []string, asName bool) string {
	if asName {
		name := "Off"
		if len(values) > 0 && values[0] != "" {
			name = values[0]
		}
		return "/" + name
	}
	if len(values) <= 1 {
		return fdfString(first(values))
	}
	parts := make([]string, len(values))
	for i, v := range values {
		parts[i] = fdfString(v)
	}
	return "[ " + strings.Join(parts, " ") + " ]"
}

// fdfString serialises a Go string as a PDF string object: a `( … )` literal for
// ASCII / PDFDocEncoding, or a `<FEFF…>` hex string for UTF-16BE (non-ASCII).
func fdfString(s string) string {
	enc := encodeFormString(s)
	if len(enc) >= 2 && enc[0] == 0xFE && enc[1] == 0xFF {
		var h strings.Builder
		h.WriteByte('<')
		for i := 0; i < len(enc); i++ {
			fmt.Fprintf(&h, "%02X", enc[i])
		}
		h.WriteByte('>')
		return h.String()
	}
	return "(" + escapeStringPDF(enc) + ")"
}

// parseFDFFields parses an FDF document into a full-name → /V value map. It scans
// indirect objects, locates the `/FDF` dictionary's `/Fields` array (resolving
// references), and walks it (descending `/Kids`, joining partial `/T` names with
// ".").
func parseFDFFields(data []byte) (map[string]pdfValue, error) {
	objs := map[int]pdfValue{}
	for _, loc := range objHeaderRE.FindAllSubmatchIndex(data, -1) {
		num := atoiBytes(data[loc[2]:loc[3]])
		v, err := parseValue(newLexerAt(data, loc[1]))
		if err != nil {
			continue // skip an unparseable object; others may still resolve
		}
		objs[num] = v
	}
	resolve := func(v pdfValue) pdfValue {
		for i := 0; i < 32; i++ {
			r, ok := v.(pdfRef)
			if !ok {
				return v
			}
			nv, ok := objs[r.Num]
			if !ok {
				return nil
			}
			v = nv
		}
		return v
	}

	// Find the /FDF dictionary among the parsed objects.
	var fdf pdfDict
	for _, v := range objs {
		d, ok := v.(pdfDict)
		if !ok {
			continue
		}
		if inner, ok := resolve(d["/FDF"]).(pdfDict); ok {
			fdf = inner
			break
		}
	}
	if fdf == nil {
		return nil, fmt.Errorf("FDF: no /FDF dictionary found")
	}
	arr, ok := resolve(fdf["/Fields"]).(pdfArray)
	if !ok {
		return map[string]pdfValue{}, nil // valid FDF with no fields
	}
	out := map[string]pdfValue{}
	walkFDFFields(arr, "", resolve, out)
	return out, nil
}

// walkFDFFields flattens an FDF /Fields array into full-name → /V, descending
// /Kids and joining partial names with ".".
func walkFDFFields(arr pdfArray, prefix string, resolve func(pdfValue) pdfValue, out map[string]pdfValue) {
	for _, it := range arr {
		d, ok := resolve(it).(pdfDict)
		if !ok {
			continue
		}
		full := decodeFormString(d["/T"])
		if prefix != "" {
			full = prefix + "." + full
		}
		if v, ok := d["/V"]; ok {
			out[full] = resolve(v)
		}
		if kids, ok := resolve(d["/Kids"]).(pdfArray); ok {
			walkFDFFields(kids, full, resolve, out)
		}
	}
}

// atoiBytes parses a non-negative integer from ASCII digit bytes.
func atoiBytes(b []byte) int {
	n := 0
	for _, c := range b {
		if c < '0' || c > '9' {
			break
		}
		n = n*10 + int(c-'0')
	}
	return n
}
