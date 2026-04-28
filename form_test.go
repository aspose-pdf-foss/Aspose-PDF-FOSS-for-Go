package asposepdf_test

import (
	"bytes"
	"testing"

	pdf "github.com/aspose/pdf-for-go"
)

func TestFormFieldsCount(t *testing.T) {
	doc, err := pdf.Open(testFile(t))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	got := len(doc.Form().Fields())
	if got != 6 {
		t.Errorf("Fields() returned %d entries, want 6 (PdfWithAcroForm.pdf has 6 leaf fields)", got)
	}
}

func TestFormFieldsTypes(t *testing.T) {
	doc, err := pdf.Open(testFile(t))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	wantByName := map[string]pdf.FormFieldType{
		"textField":        pdf.FormFieldTypeText,
		"checkboxField":    pdf.FormFieldTypeCheckbox,
		"radiobuttonField": pdf.FormFieldTypeRadioButton,
		"listboxField":     pdf.FormFieldTypeListBox,
		"comboboxField":    pdf.FormFieldTypeComboBox,
		"buttonField":      pdf.FormFieldTypePushButton,
	}
	for _, f := range doc.Form().Fields() {
		want, ok := wantByName[f.FullName()]
		if !ok {
			t.Errorf("unexpected field FullName %q", f.FullName())
			continue
		}
		got := pdf.FieldType(f)
		if got != want {
			t.Errorf("field %q: type = %v, want %v", f.FullName(), got, want)
		}
		delete(wantByName, f.FullName())
	}
	for name := range wantByName {
		t.Errorf("missing expected field: %q", name)
	}
}

func TestFormFieldAndFieldsSameInstance(t *testing.T) {
	doc, err := pdf.Open(testFile(t))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	form := doc.Form()
	fields := form.Fields()
	for _, f := range fields {
		got := form.Field(f.FullName())
		if got != f {
			t.Errorf("Field(%q) returned different instance than Fields()", f.FullName())
		}
	}
}

func TestDocumentFormNonNilOnPlainPDF(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	form := doc.Form()
	if form == nil {
		t.Fatal("Form() returned nil for plain document; expected non-nil empty form")
	}
	if got := form.Fields(); len(got) != 0 {
		t.Errorf("plain document Form().Fields() = %d entries, want 0", len(got))
	}
	if form.HasField("anything") {
		t.Error("plain document HasField returned true")
	}
	if form.Field("anything") != nil {
		t.Error("plain document Field() returned non-nil")
	}
}

func TestTextBoxFieldRead(t *testing.T) {
	doc, err := pdf.Open(testFile(t))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	f := doc.Form().Field("textField")
	if f == nil {
		t.Fatal("Field('textField') returned nil")
	}
	tf, ok := f.(*pdf.TextBoxField)
	if !ok {
		t.Fatalf("Field('textField') = %T, want *pdf.TextBoxField", f)
	}
	if got := tf.Value(); got != "this is the text field" {
		t.Errorf("Value() = %q, want %q", got, "this is the text field")
	}
	if tf.IsMultiline() {
		t.Error("IsMultiline() = true; PdfWithAcroForm.pdf textField is single-line")
	}
	if tf.IsPassword() {
		t.Error("IsPassword() = true; PdfWithAcroForm.pdf textField is plain")
	}
}

func TestTextBoxFieldRoundTrip(t *testing.T) {
	src := testFile(t)
	doc, err := pdf.Open(src)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	tf := doc.Form().Field("textField").(*pdf.TextBoxField)
	const newValue = "filled by go test"
	if err := tf.SetValue(newValue); err != nil {
		t.Fatalf("SetValue: %v", err)
	}

	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatalf("WriteTo: %v", err)
	}
	doc2, err := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	tf2 := doc2.Form().Field("textField").(*pdf.TextBoxField)
	if got := tf2.Value(); got != newValue {
		t.Errorf("after roundtrip Value() = %q, want %q", got, newValue)
	}
}

func TestCheckboxFieldRead(t *testing.T) {
	doc, err := pdf.Open(testFile(t))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	cb := doc.Form().Field("checkboxField").(*pdf.CheckboxField)
	if !cb.Checked() {
		t.Error("checkboxField.Checked() = false; PdfWithAcroForm.pdf has it checked (/V /Yes)")
	}
}

func TestCheckboxFieldRoundTrip(t *testing.T) {
	src := testFile(t)
	doc, _ := pdf.Open(src)
	cb := doc.Form().Field("checkboxField").(*pdf.CheckboxField)
	cb.SetChecked(false)
	var buf bytes.Buffer
	doc.WriteTo(&buf)
	doc2, _ := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	cb2 := doc2.Form().Field("checkboxField").(*pdf.CheckboxField)
	if cb2.Checked() {
		t.Error("after SetChecked(false) + reopen, Checked() still true")
	}
}

func TestRadioButtonFieldRead(t *testing.T) {
	doc, _ := pdf.Open(testFile(t))
	rb := doc.Form().Field("radiobuttonField").(*pdf.RadioButtonField)
	opts := rb.Options()
	if len(opts) == 0 {
		t.Fatal("radiobuttonField has zero options")
	}
	selectedCount := 0
	for _, o := range opts {
		if o.Selected() {
			selectedCount++
		}
	}
	if selectedCount != 1 {
		t.Errorf("expected exactly one selected option, got %d", selectedCount)
	}
}

func TestRadioButtonFieldRoundTrip(t *testing.T) {
	src := testFile(t)
	doc, _ := pdf.Open(src)
	rb := doc.Form().Field("radiobuttonField").(*pdf.RadioButtonField)
	opts := rb.Options()
	if len(opts) < 2 {
		t.Skip("need at least 2 options for round-trip")
	}
	target := 1
	if opts[1].Selected() {
		target = 0
	}
	opts[target].SetSelected(true)

	var buf bytes.Buffer
	doc.WriteTo(&buf)
	doc2, _ := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	rb2 := doc2.Form().Field("radiobuttonField").(*pdf.RadioButtonField)
	opts2 := rb2.Options()
	if !opts2[target].Selected() {
		t.Errorf("after SetSelected(true) + reopen, option %d not selected", target)
	}
	for i, o := range opts2 {
		if i == target {
			continue
		}
		if o.Selected() {
			t.Errorf("after SetSelected(true) + reopen, sibling option %d also selected", i)
		}
	}
}
