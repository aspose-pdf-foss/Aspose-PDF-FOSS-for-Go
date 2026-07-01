// SPDX-License-Identifier: MIT

package asposepdf_test

import (
	"bytes"
	"strings"
	"testing"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

// buildExtraForm creates one of every extra field type with some values set.
func buildExtraForm(t *testing.T) *pdf.Document {
	t.Helper()
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	form := doc.Form()
	rect := func(y float64) pdf.Rectangle {
		return pdf.Rectangle{LLX: 50, LLY: y, URX: 250, URY: y + 18}
	}
	pw, err := form.AddPasswordField(1, rect(700), "pw")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := form.AddFileSelectField(1, rect(670), "file"); err != nil {
		t.Fatal(err)
	}
	rt, err := form.AddRichTextField(1, rect(640), "rich")
	if err != nil {
		t.Fatal(err)
	}
	num, err := form.AddNumberField(1, rect(610), "amount", pdf.NumberFormatOptions{
		Decimals: 2, UseSeparator: true, CurrencySymbol: "$", CurrencyPrepend: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	dt, err := form.AddDateField(1, rect(580), "when", "mm/dd/yyyy")
	if err != nil {
		t.Fatal(err)
	}
	mustNoErr(t, pw.SetValue("secret"))
	mustNoErr(t, num.SetValue("1234.5"))
	mustNoErr(t, dt.SetValue("12/31/2026"))
	mustNoErr(t, rt.SetRichValue("<b>Hi</b>", "Hi"))
	return doc
}

// TestExtraFieldTypesInSession: each constructor returns its concrete type and
// FieldType reports it.
func TestExtraFieldTypesInSession(t *testing.T) {
	form := buildExtraForm(t).Form()
	cases := map[string]pdf.FormFieldType{
		"pw":     pdf.FormFieldTypePassword,
		"file":   pdf.FormFieldTypeFileSelect,
		"rich":   pdf.FormFieldTypeRichText,
		"amount": pdf.FormFieldTypeNumber,
		"when":   pdf.FormFieldTypeDate,
	}
	for name, want := range cases {
		if got := pdf.FieldType(form.Field(name)); got != want {
			t.Errorf("%s: FieldType = %v, want %v", name, got, want)
		}
	}
}

// TestExtraFieldTypesRoundTrip: the types are reclassified correctly after
// Save+Open (from their /Ff flag or /AA format JavaScript), and values survive.
func TestExtraFieldTypesRoundTrip(t *testing.T) {
	var buf bytes.Buffer
	if _, err := buildExtraForm(t).WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	out, err := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	form := out.Form()

	if _, ok := form.Field("pw").(*pdf.PasswordBoxField); !ok {
		t.Errorf("pw is %T, want *PasswordBoxField", form.Field("pw"))
	}
	if _, ok := form.Field("file").(*pdf.FileSelectBoxField); !ok {
		t.Errorf("file is %T, want *FileSelectBoxField", form.Field("file"))
	}
	if _, ok := form.Field("amount").(*pdf.NumberField); !ok {
		t.Errorf("amount is %T, want *NumberField", form.Field("amount"))
	}
	if num := form.Field("amount"); num.Value() != "1234.5" {
		t.Errorf("amount value = %q, want 1234.5", num.Value())
	}

	dt, ok := form.Field("when").(*pdf.DateField)
	if !ok {
		t.Fatalf("when is %T, want *DateField", form.Field("when"))
	}
	if dt.Format() != "mm/dd/yyyy" {
		t.Errorf("date format = %q, want mm/dd/yyyy", dt.Format())
	}
	if dt.Value() != "12/31/2026" {
		t.Errorf("date value = %q, want 12/31/2026", dt.Value())
	}

	rt, ok := form.Field("rich").(*pdf.RichTextBoxField)
	if !ok {
		t.Fatalf("rich is %T, want *RichTextBoxField", form.Field("rich"))
	}
	if rt.RichValue() != "<b>Hi</b>" {
		t.Errorf("rich value = %q, want <b>Hi</b>", rt.RichValue())
	}
}

// TestNumberFieldFormatJS: the number field carries the expected AFNumber_Format
// JavaScript.
func TestNumberFieldFormatJS(t *testing.T) {
	var buf bytes.Buffer
	if _, err := buildExtraForm(t).WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	// The '(' is escaped as '\(' inside the PDF string literal, so match the
	// function name and its first argument separately.
	if !strings.Contains(buf.String(), "AFNumber_Format") {
		t.Error("number field missing AFNumber_Format JavaScript")
	}
	if !strings.Contains(buf.String(), "AFDate_FormatEx") {
		t.Error("date field missing AFDate_FormatEx JavaScript")
	}
	// Reopen and confirm the parsed JavaScript is intact (unescaped).
	out, err := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if dt, ok := out.Form().Field("when").(*pdf.DateField); !ok || dt.Format() != "mm/dd/yyyy" {
		t.Errorf("date format did not survive: %v", out.Form().Field("when"))
	}
}

// TestExtraFieldsInFormData: the extra text-family fields are exported in the
// typed JSON with their type tags, and import applies to them.
func TestExtraFieldsInFormData(t *testing.T) {
	data, err := buildExtraForm(t).Form().ExportJSON()
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	for _, want := range []string{
		`"amount":{"type":"number","value":"1234.5"}`,
		`"when":{"type":"date","value":"12/31/2026"}`,
		`"pw":{"type":"password","value":"secret"}`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("JSON missing %q\n--- got ---\n%s", want, s)
		}
	}

	// Import a new value into the number field of a fresh identical form.
	dst := buildExtraForm(t)
	n, err := dst.Form().ImportJSON([]byte(`{"amount":{"type":"number","value":"99.9"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("imported %d, want 1", n)
	}
	if got := dst.Form().Field("amount").Value(); got != "99.9" {
		t.Errorf("amount after import = %q, want 99.9", got)
	}
}
