// SPDX-License-Identifier: MIT

package asposepdf_test

import (
	"bytes"
	"testing"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
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

func TestComboBoxFieldRead(t *testing.T) {
	doc, _ := pdf.Open(testFile(t))
	cb := doc.Form().Field("comboboxField").(*pdf.ComboBoxField)
	opts := cb.Options()
	if len(opts) == 0 {
		t.Fatal("comboboxField has zero options")
	}
	idx := cb.Selected()
	if idx < 0 || idx >= len(opts) {
		t.Errorf("Selected() = %d out of range [0,%d)", idx, len(opts))
	}
}

func TestComboBoxFieldRoundTrip(t *testing.T) {
	src := testFile(t)
	doc, _ := pdf.Open(src)
	cb := doc.Form().Field("comboboxField").(*pdf.ComboBoxField)
	opts := cb.Options()
	if len(opts) < 2 {
		t.Skip("need at least 2 options")
	}
	target := 1
	if cb.Selected() == 1 {
		target = 0
	}
	if err := cb.SetSelected(target); err != nil {
		t.Fatalf("SetSelected: %v", err)
	}
	var buf bytes.Buffer
	doc.WriteTo(&buf)
	doc2, _ := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	cb2 := doc2.Form().Field("comboboxField").(*pdf.ComboBoxField)
	if cb2.Selected() != target {
		t.Errorf("after roundtrip Selected() = %d, want %d", cb2.Selected(), target)
	}
}

func TestListBoxFieldRead(t *testing.T) {
	doc, _ := pdf.Open(testFile(t))
	lb := doc.Form().Field("listboxField").(*pdf.ListBoxField)
	opts := lb.Options()
	if len(opts) == 0 {
		t.Fatal("listboxField has zero options")
	}
	sel := lb.Selected()
	for _, idx := range sel {
		if idx < 0 || idx >= len(opts) {
			t.Errorf("Selected() index %d out of range [0,%d)", idx, len(opts))
		}
	}
}

func TestListBoxFieldRoundTrip(t *testing.T) {
	src := testFile(t)
	doc, _ := pdf.Open(src)
	lb := doc.Form().Field("listboxField").(*pdf.ListBoxField)
	opts := lb.Options()
	if len(opts) < 2 {
		t.Skip("need at least 2 options")
	}
	if err := lb.SetSelected(0, 1); err != nil {
		// Single-select listboxes reject multi-set; fall back to single.
		if err := lb.SetSelected(1); err != nil {
			t.Fatalf("SetSelected single-arg: %v", err)
		}
	}
	var buf bytes.Buffer
	doc.WriteTo(&buf)
	doc2, _ := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	lb2 := doc2.Form().Field("listboxField").(*pdf.ListBoxField)
	got := lb2.Selected()
	if len(got) == 0 {
		t.Error("after SetSelected + reopen, Selected() returned empty")
	}
}

func TestFormSetValueAutoNeedAppearances(t *testing.T) {
	src := testFile(t)
	doc, _ := pdf.Open(src)
	tf := doc.Form().Field("textField").(*pdf.TextBoxField)
	tf.SetValue("triggers needappearances")

	var buf bytes.Buffer
	doc.WriteTo(&buf)
	if !bytes.Contains(buf.Bytes(), []byte("/NeedAppearances")) {
		t.Error("/NeedAppearances not present in saved bytes after SetValue")
	}
	if !bytes.Contains(buf.Bytes(), []byte("/NeedAppearances true")) {
		t.Error("/NeedAppearances not set to true after SetValue")
	}
}

func TestFormManualNeedAppearancesToggle(t *testing.T) {
	src := testFile(t)
	doc, _ := pdf.Open(src)
	if !doc.Form().NeedAppearances() {
		// Original file may or may not have it; flip from current state.
		doc.Form().SetNeedAppearances(true)
		if !doc.Form().NeedAppearances() {
			t.Error("after SetNeedAppearances(true), getter still returned false")
		}
	}
	doc.Form().SetNeedAppearances(false)
	if doc.Form().NeedAppearances() {
		t.Error("after SetNeedAppearances(false), getter still returned true")
	}
}

func TestSetNeedAppearancesFalseOnBlankDocDoesNotCreateAcroForm(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	doc.Form().SetNeedAppearances(false)
	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatalf("WriteTo: %v", err)
	}
	if bytes.Contains(buf.Bytes(), []byte("/AcroForm")) {
		t.Error("blank document grew an /AcroForm dict after SetNeedAppearances(false)")
	}
}

func TestFormFillIntegration(t *testing.T) {
	src := testFile(t)
	doc, err := pdf.Open(src)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	// Fill every field type with a known value.
	doc.Form().Field("textField").(*pdf.TextBoxField).SetValue("integration test value")
	doc.Form().Field("checkboxField").(*pdf.CheckboxField).SetChecked(false)
	rb := doc.Form().Field("radiobuttonField").(*pdf.RadioButtonField)
	rb.Options()[0].SetSelected(true)
	var comboTarget int = -1
	cb := doc.Form().Field("comboboxField").(*pdf.ComboBoxField)
	if len(cb.Options()) >= 2 {
		comboTarget = 1
		cb.SetSelected(comboTarget)
	}
	var listTarget int = -1
	lb := doc.Form().Field("listboxField").(*pdf.ListBoxField)
	if len(lb.Options()) >= 1 {
		listTarget = 0
		lb.SetSelected(listTarget)
	}

	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatalf("WriteTo: %v", err)
	}
	doc2, err := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}

	if got := doc2.Form().Field("textField").(*pdf.TextBoxField).Value(); got != "integration test value" {
		t.Errorf("textField round-trip: got %q", got)
	}
	if doc2.Form().Field("checkboxField").(*pdf.CheckboxField).Checked() {
		t.Error("checkboxField round-trip: still checked")
	}
	rb2 := doc2.Form().Field("radiobuttonField").(*pdf.RadioButtonField)
	if !rb2.Options()[0].Selected() {
		t.Error("radiobuttonField round-trip: option 0 not selected")
	}
	if comboTarget != -1 {
		cb2 := doc2.Form().Field("comboboxField").(*pdf.ComboBoxField)
		if got := cb2.Selected(); got != comboTarget {
			t.Errorf("comboboxField round-trip: Selected() = %d, want %d", got, comboTarget)
		}
	}
	if listTarget != -1 {
		lb2 := doc2.Form().Field("listboxField").(*pdf.ListBoxField)
		sel := lb2.Selected()
		found := false
		for _, idx := range sel {
			if idx == listTarget {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("listboxField round-trip: Selected() = %v, want includes %d", sel, listTarget)
		}
	}
}

func TestRadioButtonFieldSetValueInvalidReturnsError(t *testing.T) {
	doc, _ := pdf.Open(testFile(t))
	rb := doc.Form().Field("radiobuttonField").(*pdf.RadioButtonField)
	beforeV := rb.Value()
	if err := rb.SetValue("not-an-option"); err == nil {
		t.Error("SetValue with invalid option returned nil error")
	}
	if rb.Value() != beforeV {
		t.Errorf("SetValue error path mutated state: before=%q after=%q", beforeV, rb.Value())
	}
}

func TestRadioButtonFieldSetValueEmptyClearsSelection(t *testing.T) {
	doc, _ := pdf.Open(testFile(t))
	rb := doc.Form().Field("radiobuttonField").(*pdf.RadioButtonField)
	if err := rb.SetValue(""); err != nil {
		t.Errorf("SetValue('') returned error: %v", err)
	}
	for _, opt := range rb.Options() {
		if opt.Selected() {
			t.Error("after SetValue(''), some option still appears selected")
		}
	}
}

func TestComboBoxFieldSetValueInvalidNonEditableReturnsError(t *testing.T) {
	doc, _ := pdf.Open(testFile(t))
	cb := doc.Form().Field("comboboxField").(*pdf.ComboBoxField)
	if err := cb.SetValue("not-an-option"); err == nil {
		t.Error("SetValue with invalid option on non-editable combo returned nil error")
	}
}

func TestCheckboxFieldSetValueAcceptsBooleanStrings(t *testing.T) {
	doc, _ := pdf.Open(testFile(t))
	cb := doc.Form().Field("checkboxField").(*pdf.CheckboxField)
	if err := cb.SetValue("on"); err != nil {
		t.Errorf("SetValue('on') returned error: %v", err)
	}
	if !cb.Checked() {
		t.Error("after SetValue('on'), Checked() = false")
	}
	if err := cb.SetValue("OFF"); err != nil {
		t.Errorf("SetValue('OFF') returned error: %v", err)
	}
	if cb.Checked() {
		t.Error("after SetValue('OFF'), Checked() = true")
	}
}

func TestCheckboxFieldSetValueInvalidReturnsError(t *testing.T) {
	doc, _ := pdf.Open(testFile(t))
	cb := doc.Form().Field("checkboxField").(*pdf.CheckboxField)
	if err := cb.SetValue("garbage"); err == nil {
		t.Error("SetValue('garbage') returned nil error")
	}
}

func TestButtonFieldSetValueReturnsError(t *testing.T) {
	doc, _ := pdf.Open(testFile(t))
	bf := doc.Form().Field("buttonField").(*pdf.ButtonField)
	if err := bf.SetValue("anything"); err == nil {
		t.Error("ButtonField.SetValue returned nil error; push button has no value")
	}
}

func TestListBoxFieldSetSelectedMultiOnSingleSelectReturnsError(t *testing.T) {
	doc, _ := pdf.Open(testFile(t))
	lb := doc.Form().Field("listboxField").(*pdf.ListBoxField)
	if lb.MultiSelect() {
		t.Skip("listboxField is multi-select; this test requires single-select")
	}
	if err := lb.SetSelected(0, 1); err == nil {
		t.Error("SetSelected(0,1) on single-select returned nil error")
	}
}

func TestFormCyrillicRoundTrip(t *testing.T) {
	loaded, err := pdf.Open(testFile(t))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	tf := loaded.Form().Field("textField").(*pdf.TextBoxField)
	const cyrillic = "Привет, мир!"
	tf.SetValue(cyrillic)

	var buf bytes.Buffer
	if _, err := loaded.WriteTo(&buf); err != nil {
		t.Fatalf("WriteTo: %v", err)
	}
	doc2, err := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("OpenStream: %v", err)
	}
	tf2 := doc2.Form().Field("textField").(*pdf.TextBoxField)
	if got := tf2.Value(); got != cyrillic {
		t.Errorf("Cyrillic round-trip: got %q, want %q", got, cyrillic)
	}
}
