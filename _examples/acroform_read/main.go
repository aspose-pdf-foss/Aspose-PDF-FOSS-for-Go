// Reads every form field from a PDF and emits a JSON dump to stdout.
//
// Usage:
//
//	go run ./my_examples/acroform_read/                                   # reads result_files/acroform_build.pdf
//	go run ./my_examples/acroform_read/ path/to/filled.pdf                # any PDF with /AcroForm
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

type fieldDump struct {
	FullName    string         `json:"fullName"`
	PartialName string         `json:"partialName"`
	Type        string         `json:"type"`
	Value       string         `json:"value,omitempty"`
	IsReadOnly  bool           `json:"readOnly,omitempty"`
	IsRequired  bool           `json:"required,omitempty"`
	PageIndex   int            `json:"page,omitempty"`
	Rect        *rectDump      `json:"rect,omitempty"`
	TypeData    map[string]any `json:"-"` // flattened into the same object on marshal
}

type rectDump struct {
	LLX, LLY, URX, URY float64
}

// MarshalJSON merges TypeData into the top-level object so each field
// type's specifics (MaxLen, Options, Selected, etc.) appear at the
// same level as the common keys without nesting.
func (f fieldDump) MarshalJSON() ([]byte, error) {
	out := map[string]any{
		"fullName":    f.FullName,
		"partialName": f.PartialName,
		"type":        f.Type,
	}
	if f.Value != "" {
		out["value"] = f.Value
	}
	if f.IsReadOnly {
		out["readOnly"] = true
	}
	if f.IsRequired {
		out["required"] = true
	}
	if f.PageIndex != 0 {
		out["page"] = f.PageIndex
	}
	if f.Rect != nil {
		out["rect"] = f.Rect
	}
	for k, v := range f.TypeData {
		out[k] = v
	}
	return json.Marshal(out)
}

func main() {
	path := "result_files/acroform_build.pdf"
	if len(os.Args) > 1 {
		path = os.Args[1]
	}

	doc, err := pdf.Open(path)
	if err != nil {
		log.Fatalf("open %s: %v", path, err)
	}
	form := doc.Form()

	fields := form.Fields()
	dumps := make([]fieldDump, 0, len(fields))
	for _, f := range fields {
		dumps = append(dumps, dumpField(f))
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(map[string]any{
		"source":          path,
		"needAppearances": form.NeedAppearances(),
		"fieldCount":      len(fields),
		"fields":          dumps,
	}); err != nil {
		log.Fatalf("encode: %v", err)
	}
}

func dumpField(f pdf.Field) fieldDump {
	d := fieldDump{
		FullName:    f.FullName(),
		PartialName: f.PartialName(),
		Value:       f.Value(),
		IsReadOnly:  f.IsReadOnly(),
		IsRequired:  f.IsRequired(),
		PageIndex:   f.PageIndex(),
		TypeData:    map[string]any{},
	}
	if r := f.Rect(); r != (pdf.Rectangle{}) {
		d.Rect = &rectDump{LLX: r.LLX, LLY: r.LLY, URX: r.URX, URY: r.URY}
	}

	switch x := f.(type) {
	case *pdf.TextBoxField:
		d.Type = "TextBox"
		if n := x.MaxLen(); n > 0 {
			d.TypeData["maxLen"] = n
		}
		if x.IsMultiline() {
			d.TypeData["multiline"] = true
		}
		if x.IsPassword() {
			d.TypeData["password"] = true
		}

	case *pdf.CheckboxField:
		d.Type = "Checkbox"
		d.TypeData["checked"] = x.Checked()

	case *pdf.RadioButtonField:
		d.Type = "RadioButton"
		opts := x.Options()
		buf := make([]map[string]any, 0, len(opts))
		for _, o := range opts {
			buf = append(buf, map[string]any{
				"name":     o.Name(),
				"selected": o.Selected(),
			})
		}
		d.TypeData["options"] = buf

	case *pdf.ComboBoxField:
		d.Type = "ComboBox"
		d.TypeData["selectedIndex"] = x.Selected()
		d.TypeData["options"] = optionList(x.Options())

	case *pdf.ListBoxField:
		d.Type = "ListBox"
		d.TypeData["selected"] = x.Selected()
		d.TypeData["multiSelect"] = x.MultiSelect()
		d.TypeData["options"] = optionList(x.Options())

	case *pdf.ButtonField:
		d.Type = "PushButton"

	default:
		d.Type = fmt.Sprintf("%T", f)
	}
	return d
}

func optionList(opts []pdf.ChoiceOption) []map[string]string {
	out := make([]map[string]string, 0, len(opts))
	for _, o := range opts {
		entry := map[string]string{"value": o.Value}
		if o.Export != "" {
			entry["export"] = o.Export
		}
		out = append(out, entry)
	}
	return out
}
