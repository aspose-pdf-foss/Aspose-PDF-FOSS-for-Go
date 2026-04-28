package asposepdf_test

import (
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
