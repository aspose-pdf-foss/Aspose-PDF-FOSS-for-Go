// SPDX-License-Identifier: MIT

package asposepdf

import (
	"encoding/json"
	"fmt"
	"io"
)

// JSONExportOptions controls (*Form).ExportJSON / WriteJSON.
type JSONExportOptions struct {
	// Indent pretty-prints the JSON with two-space indentation.
	Indent bool
	// OmitEmpty skips fields with no value (empty text, unchecked checkbox,
	// unselected radio/choice). By default every non-button field is exported,
	// producing a full snapshot of the form.
	OmitEmpty bool
}

// jsonFieldOut is the marshalled shape of one field on export: a type tag plus
// a value whose JSON kind follows the type (string, bool, or []string).
type jsonFieldOut struct {
	Type  string      `json:"type"`
	Value interface{} `json:"value"`
}

// jsonFieldIn is the parsed shape of one field on import. Value is kept raw so
// it can be decoded against the concrete type of the matching field in the
// target document (which is authoritative — the JSON "type" is advisory).
type jsonFieldIn struct {
	Type  string          `json:"type"`
	Value json.RawMessage `json:"value"`
}

// ExportJSON serialises the form's field values to a typed JSON document keyed
// by each field's full name: `{"name": {"type": "...", "value": ...}}`. The
// value's JSON kind follows the field type — text/radio/combobox → string,
// checkbox → bool, listbox → array of strings. Push buttons (which hold no
// value) are skipped. Keys are emitted in sorted order. Mirrors the intent of
// Aspose.PDF for .NET's `Document.Form.ExportToJson`.
func (f *Form) ExportJSON(opts ...JSONExportOptions) ([]byte, error) {
	opt := JSONExportOptions{}
	if len(opts) > 0 {
		opt = opts[0]
	}
	out := make(map[string]jsonFieldOut)
	for _, fld := range f.Fields() {
		jf, ok := exportFieldJSON(fld, opt)
		if !ok {
			continue
		}
		out[fld.FullName()] = jf
	}
	if opt.Indent {
		return json.MarshalIndent(out, "", "  ")
	}
	return json.Marshal(out)
}

// WriteJSON writes ExportJSON output to w.
func (f *Form) WriteJSON(w io.Writer, opts ...JSONExportOptions) error {
	data, err := f.ExportJSON(opts...)
	if err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}

// ImportJSON applies field values from a typed JSON document (the shape
// ExportJSON produces) and returns the number of fields actually set. Each key
// is matched against the form by full name; unknown names and values that don't
// apply to their target field are skipped (not an error). Values are interpreted
// by the matching field's concrete type, so a checkbox accepts a bool or a
// boolean string and a listbox accepts an array or a single string. Setting a
// value regenerates the widget appearance, so imported values render in any
// viewer. Mirrors the intent of Aspose.PDF for .NET's `Document.Form.ImportFromJson`.
func (f *Form) ImportJSON(data []byte) (int, error) {
	var in map[string]jsonFieldIn
	if err := json.Unmarshal(data, &in); err != nil {
		return 0, err
	}
	applied := 0
	for name, jf := range in {
		fld := f.Field(name)
		if fld == nil {
			continue
		}
		if err := applyFieldJSON(fld, jf); err != nil {
			continue
		}
		applied++
	}
	return applied, nil
}

// ReadJSON reads a JSON document from r and applies it via ImportJSON.
func (f *Form) ReadJSON(r io.Reader) (int, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return 0, err
	}
	return f.ImportJSON(data)
}

// exportFieldJSON builds the typed JSON entry for one field, or reports false to
// skip it (push buttons, and — when OmitEmpty — valueless fields).
func exportFieldJSON(fld Field, opt JSONExportOptions) (jsonFieldOut, bool) {
	switch x := fld.(type) {
	case *TextBoxField:
		v := x.Value()
		if v == "" && opt.OmitEmpty {
			return jsonFieldOut{}, false
		}
		return jsonFieldOut{Type: "text", Value: v}, true
	case *CheckboxField:
		b := x.Checked()
		if !b && opt.OmitEmpty {
			return jsonFieldOut{}, false
		}
		return jsonFieldOut{Type: "checkbox", Value: b}, true
	case *RadioButtonField:
		v := radioSelectedName(x)
		if v == "" && opt.OmitEmpty {
			return jsonFieldOut{}, false
		}
		return jsonFieldOut{Type: "radio", Value: v}, true
	case *ComboBoxField:
		v := x.Value()
		if v == "" && opt.OmitEmpty {
			return jsonFieldOut{}, false
		}
		return jsonFieldOut{Type: "combobox", Value: v}, true
	case *ListBoxField:
		vals := listSelectedValues(x)
		if len(vals) == 0 && opt.OmitEmpty {
			return jsonFieldOut{}, false
		}
		// Always emit an array (possibly empty) so the type stays stable.
		if vals == nil {
			vals = []string{}
		}
		return jsonFieldOut{Type: "listbox", Value: vals}, true
	default:
		return jsonFieldOut{}, false // push button / unsupported — no value
	}
}

// applyFieldJSON sets one field from its parsed JSON entry, dispatching on the
// target field's concrete type (authoritative over the JSON "type" tag).
func applyFieldJSON(fld Field, jf jsonFieldIn) error {
	switch x := fld.(type) {
	case *TextBoxField:
		s, err := jsonAsString(jf.Value)
		if err != nil {
			return err
		}
		return x.SetValue(s)
	case *ComboBoxField:
		s, err := jsonAsString(jf.Value)
		if err != nil {
			return err
		}
		return x.SetValue(s)
	case *RadioButtonField:
		s, err := jsonAsString(jf.Value)
		if err != nil {
			return err
		}
		return x.SetValue(s)
	case *CheckboxField:
		var b bool
		if json.Unmarshal(jf.Value, &b) == nil {
			x.SetChecked(b)
			return nil
		}
		s, err := jsonAsString(jf.Value)
		if err != nil {
			return err
		}
		return x.SetValue(s)
	case *ListBoxField:
		var arr []string
		if json.Unmarshal(jf.Value, &arr) == nil {
			return setListByValues(x, arr)
		}
		s, err := jsonAsString(jf.Value)
		if err != nil {
			return err
		}
		return x.SetValue(s)
	default:
		return fmt.Errorf("form: cannot import into field %q (unsupported type)", fld.FullName())
	}
}

// radioSelectedName returns the export name of the selected radio option (empty
// if none selected).
func radioSelectedName(f *RadioButtonField) string {
	for _, o := range f.Options() {
		if o.Selected() {
			return o.Name()
		}
	}
	return ""
}

// listSelectedValues returns the export values of a listbox's selected options.
func listSelectedValues(f *ListBoxField) []string {
	opts := f.Options()
	var out []string
	for _, idx := range f.Selected() {
		if idx < 0 || idx >= len(opts) {
			continue
		}
		v := opts[idx].Value
		if opts[idx].Export != "" {
			v = opts[idx].Export
		}
		out = append(out, v)
	}
	return out
}

// setListByValues selects the listbox options whose value/export matches each of
// vals (an empty slice clears the selection).
func setListByValues(f *ListBoxField, vals []string) error {
	opts := f.Options()
	var idx []int
	for _, v := range vals {
		for i, o := range opts {
			if o.Value == v || o.Export == v {
				idx = append(idx, i)
				break
			}
		}
	}
	return f.SetSelected(idx...)
}

// jsonAsString decodes a raw JSON value as a string.
func jsonAsString(raw json.RawMessage) (string, error) {
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return "", err
	}
	return s, nil
}
