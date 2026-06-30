// SPDX-License-Identifier: MIT

package asposepdf

import (
	"encoding/xml"
	"io"
)

// XFDF (XML Forms Data Format, ISO 19444-1) export/import — the XML counterpart
// to FDF (form_fdf.go), built on the shared formValues / applyFormValues model.

const xfdfNamespace = "http://ns.adobe.com/xfdf/"

// xfdfRoot mirrors <xfdf xmlns="http://ns.adobe.com/xfdf/"><fields>…</fields></xfdf>.
type xfdfRoot struct {
	XMLName xml.Name   `xml:"xfdf"`
	Xmlns   string     `xml:"xmlns,attr"`
	Fields  xfdfFields `xml:"fields"`
}

type xfdfFields struct {
	Field []xfdfField `xml:"field"`
}

// xfdfField is one <field name="..."> with <value> children and/or nested
// <field> children (hierarchical names).
type xfdfField struct {
	Name  string      `xml:"name,attr"`
	Value []string    `xml:"value"`
	Kids  []xfdfField `xml:"field"`
}

// ExportXFDF serialises the form's field values as an XFDF document. Field names
// are written flat (the full name in each `name` attribute); push buttons are
// skipped. Mirrors Aspose.PDF for .NET's `Facades.Form.ExportXfdf`.
func (f *Form) ExportXFDF() ([]byte, error) {
	root := xfdfRoot{Xmlns: xfdfNamespace}
	for _, fld := range f.Fields() {
		_, values, _, ok := formValues(fld)
		if !ok {
			continue
		}
		if values == nil {
			values = []string{}
		}
		root.Fields.Field = append(root.Fields.Field, xfdfField{Name: fld.FullName(), Value: values})
	}
	body, err := xml.MarshalIndent(root, "", "  ")
	if err != nil {
		return nil, err
	}
	out := append([]byte(xml.Header), body...)
	return append(out, '\n'), nil
}

// WriteXFDF writes ExportXFDF output to w.
func (f *Form) WriteXFDF(w io.Writer) error {
	data, err := f.ExportXFDF()
	if err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}

// ImportXFDF applies field values from an XFDF document and returns the number
// of fields set. Names are matched by full name (nested `<field>` elements are
// resolved to dotted names); unknown names and non-applying values are skipped.
// Mirrors Aspose.PDF for .NET's `Facades.Form.ImportXfdf`.
func (f *Form) ImportXFDF(data []byte) (int, error) {
	var root xfdfRoot
	if err := xml.Unmarshal(data, &root); err != nil {
		return 0, err
	}
	flat := map[string][]string{}
	flattenXFDF(root.Fields.Field, "", flat)
	applied := 0
	for name, vals := range flat {
		fld := f.Field(name)
		if fld == nil {
			continue
		}
		if err := applyFormValues(fld, vals); err != nil {
			continue
		}
		applied++
	}
	return applied, nil
}

// ReadXFDF reads an XFDF document from r and applies it via ImportXFDF.
func (f *Form) ReadXFDF(r io.Reader) (int, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return 0, err
	}
	return f.ImportXFDF(data)
}

// flattenXFDF flattens nested <field> elements into full-name → values, joining
// partial names with ".".
func flattenXFDF(fields []xfdfField, prefix string, out map[string][]string) {
	for _, fl := range fields {
		full := fl.Name
		if prefix != "" {
			full = prefix + "." + fl.Name
		}
		if len(fl.Value) > 0 {
			out[full] = fl.Value
		}
		if len(fl.Kids) > 0 {
			flattenXFDF(fl.Kids, full, out)
		}
	}
}
