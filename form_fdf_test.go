// SPDX-License-Identifier: MIT

package asposepdf_test

import (
	"bytes"
	"strings"
	"testing"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

// assertFilled checks the standard valued-form expectations after an import.
func assertFilled(t *testing.T, bf builtForm) {
	t.Helper()
	if got := bf.text.Value(); got != "Alice" {
		t.Errorf("text = %q, want Alice", got)
	}
	if !bf.check.Checked() {
		t.Error("checkbox not checked after import")
	}
	if got := bf.combo.Value(); got != "Canada" {
		t.Errorf("combo = %q, want Canada", got)
	}
	if got := bf.list.Selected(); len(got) != 2 || got[0] != 0 || got[1] != 2 {
		t.Errorf("listbox selected = %v, want [0 2]", got)
	}
	if got := bf.radio.Value(); got != "/green" && got != "green" {
		t.Errorf("radio = %q, want green", got)
	}
}

// TestFDFRoundTrip: export a valued form to FDF, import into a fresh form,
// confirm every field reproduces.
func TestFDFRoundTrip(t *testing.T) {
	src := buildJSONForm(t, true)
	data, err := src.doc.Form().ExportFDF()
	if err != nil {
		t.Fatal(err)
	}
	dst := buildJSONForm(t, false)
	n, err := dst.doc.Form().ImportFDF(data)
	if err != nil {
		t.Fatal(err)
	}
	if n != 5 {
		t.Errorf("imported %d fields, want 5", n)
	}
	assertFilled(t, dst)
}

// TestXFDFRoundTrip: same for XFDF.
func TestXFDFRoundTrip(t *testing.T) {
	src := buildJSONForm(t, true)
	data, err := src.doc.Form().ExportXFDF()
	if err != nil {
		t.Fatal(err)
	}
	dst := buildJSONForm(t, false)
	n, err := dst.doc.Form().ImportXFDF(data)
	if err != nil {
		t.Fatal(err)
	}
	if n != 5 {
		t.Errorf("imported %d fields, want 5", n)
	}
	assertFilled(t, dst)
}

// TestFDFExportShape: the FDF body carries the expected PDF-syntax tokens.
func TestFDFExportShape(t *testing.T) {
	data, err := buildJSONForm(t, true).doc.Form().ExportFDF()
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	for _, want := range []string{
		"%FDF-1.2", "/FDF", "/Fields",
		"/T (name)", "/V (Alice)",
		"/V /Yes",          // checkbox on-state name
		"/V /green",        // radio selected name
		"/V [ (en) (de) ]", // multi-select list box
		"trailer", "/Root 1 0 R",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("FDF missing %q\n--- got ---\n%s", want, s)
		}
	}
}

// TestXFDFExportShape: the XFDF carries the namespace and field/value elements.
func TestXFDFExportShape(t *testing.T) {
	data, err := buildJSONForm(t, true).doc.Form().ExportXFDF()
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	for _, want := range []string{
		`xmlns="http://ns.adobe.com/xfdf/"`,
		`<field name="name">`, `<value>Alice</value>`,
		`<field name="langs">`, `<value>en</value>`, `<value>de</value>`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("XFDF missing %q\n--- got ---\n%s", want, s)
		}
	}
}

// TestFDFImportExternal: a hand-written FDF (the shape Acrobat produces) fills
// the form.
func TestFDFImportExternal(t *testing.T) {
	bf := buildJSONForm(t, false)
	fdf := []byte("%FDF-1.2\n1 0 obj\n<< /FDF << /Fields [\n" +
		"<< /T (name) /V (Bob) >>\n" +
		"<< /T (agree) /V /Yes >>\n" +
		"<< /T (country) /V (Mexico) >>\n" +
		"] >> >>\nendobj\ntrailer\n<< /Root 1 0 R >>\n%%EOF\n")
	n, err := bf.doc.Form().ImportFDF(fdf)
	if err != nil {
		t.Fatal(err)
	}
	if n != 3 {
		t.Errorf("applied %d, want 3", n)
	}
	if got := bf.text.Value(); got != "Bob" {
		t.Errorf("text = %q, want Bob", got)
	}
	if !bf.check.Checked() {
		t.Error("checkbox not checked")
	}
	if got := bf.combo.Value(); got != "Mexico" {
		t.Errorf("combo = %q, want Mexico", got)
	}
}

// TestXFDFImportExternalNested: a hierarchical <field> nesting resolves to a
// dotted full name.
func TestXFDFImportExternalNested(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	form := doc.Form()
	// A field whose full name contains a dot, matched by nested XFDF elements.
	tf, err := form.AddTextField(1, pdf.Rectangle{LLX: 50, LLY: 700, URX: 250, URY: 720}, "group.sub")
	if err != nil {
		t.Fatal(err)
	}
	xfdf := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<xfdf xmlns="http://ns.adobe.com/xfdf/">
  <fields>
    <field name="group">
      <field name="sub"><value>Nested</value></field>
    </field>
  </fields>
</xfdf>`)
	n, err := form.ImportXFDF(xfdf)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("applied %d, want 1", n)
	}
	if got := tf.Value(); got != "Nested" {
		t.Errorf("group.sub = %q, want Nested", got)
	}
}

// TestFDFXFDFThroughSave: values imported from each format survive Save+reopen.
func TestFDFXFDFThroughSave(t *testing.T) {
	for _, tc := range []struct {
		name   string
		export func(*pdf.Form) ([]byte, error)
		imp    func(*pdf.Form, []byte) (int, error)
	}{
		{"fdf", (*pdf.Form).ExportFDF, (*pdf.Form).ImportFDF},
		{"xfdf", (*pdf.Form).ExportXFDF, (*pdf.Form).ImportXFDF},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data, err := tc.export(buildJSONForm(t, true).doc.Form())
			if err != nil {
				t.Fatal(err)
			}
			dst := buildJSONForm(t, false)
			if _, err := tc.imp(dst.doc.Form(), data); err != nil {
				t.Fatal(err)
			}
			var buf bytes.Buffer
			if _, err := dst.doc.WriteTo(&buf); err != nil {
				t.Fatal(err)
			}
			out, err := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
			if err != nil {
				t.Fatal(err)
			}
			if got := out.Form().Field("name").Value(); got != "Alice" {
				t.Errorf("%s: after save+reopen name = %q, want Alice", tc.name, got)
			}
		})
	}
}
