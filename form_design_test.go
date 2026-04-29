package asposepdf_test

import (
	"bytes"
	"testing"

	pdf "github.com/aspose/pdf-for-go"
)

func TestFormAddTextFieldRoundTrip(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	tf, err := doc.Form().AddTextField(1, pdf.Rectangle{LLX: 50, LLY: 700, URX: 545, URY: 730}, "name")
	if err != nil {
		t.Fatalf("AddTextField: %v", err)
	}
	if tf == nil {
		t.Fatal("AddTextField returned nil *TextBoxField")
	}
	tf.SetValue("Jane Doe")

	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatalf("WriteTo: %v", err)
	}

	doc2, err := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("OpenStream: %v", err)
	}
	if doc2.Form().HasField("name") == false {
		t.Fatal("HasField('name') = false after roundtrip")
	}
	tf2 := doc2.Form().Field("name").(*pdf.TextBoxField)
	if got := tf2.Value(); got != "Jane Doe" {
		t.Errorf("Value() = %q, want %q", got, "Jane Doe")
	}
}

func TestFormAddCheckboxRoundTrip(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	cb, err := doc.Form().AddCheckbox(1, pdf.Rectangle{LLX: 50, LLY: 650, URX: 70, URY: 670}, "subscribe")
	if err != nil {
		t.Fatalf("AddCheckbox: %v", err)
	}
	cb.SetChecked(true)

	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatalf("WriteTo: %v", err)
	}
	doc2, err := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("OpenStream: %v", err)
	}
	cb2 := doc2.Form().Field("subscribe").(*pdf.CheckboxField)
	if !cb2.Checked() {
		t.Error("checkbox not checked after roundtrip")
	}
}

func TestFormAddComboBoxRoundTrip(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	options := []pdf.ChoiceOption{
		{Value: "USA"},
		{Value: "Canada"},
		{Value: "Mexico"},
	}
	cb, err := doc.Form().AddComboBox(1, pdf.Rectangle{LLX: 50, LLY: 600, URX: 250, URY: 625}, "country", options)
	if err != nil {
		t.Fatalf("AddComboBox: %v", err)
	}
	if err := cb.SetSelected(1); err != nil {
		t.Fatalf("SetSelected: %v", err)
	}

	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatalf("WriteTo: %v", err)
	}
	doc2, err := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("OpenStream: %v", err)
	}
	cb2 := doc2.Form().Field("country").(*pdf.ComboBoxField)
	if got := len(cb2.Options()); got != 3 {
		t.Errorf("Options count = %d, want 3", got)
	}
	if got := cb2.Selected(); got != 1 {
		t.Errorf("Selected = %d, want 1", got)
	}
}
