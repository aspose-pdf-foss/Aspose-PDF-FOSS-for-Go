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

func TestFormAddListBoxRoundTrip(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	options := []pdf.ChoiceOption{
		{Value: "Red"},
		{Value: "Green"},
		{Value: "Blue"},
	}
	lb, err := doc.Form().AddListBox(1, pdf.Rectangle{LLX: 50, LLY: 500, URX: 250, URY: 580}, "color", options)
	if err != nil {
		t.Fatalf("AddListBox: %v", err)
	}
	if err := lb.SetSelected(0); err != nil {
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
	lb2 := doc2.Form().Field("color").(*pdf.ListBoxField)
	if got := len(lb2.Options()); got != 3 {
		t.Errorf("Options count = %d, want 3", got)
	}
	sel := lb2.Selected()
	if len(sel) != 1 || sel[0] != 0 {
		t.Errorf("Selected = %v, want [0]", sel)
	}
}

func TestFormAddPushButtonRoundTrip(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	bt, err := doc.Form().AddPushButton(1, pdf.Rectangle{LLX: 50, LLY: 450, URX: 200, URY: 480}, "submit", "Submit")
	if err != nil {
		t.Fatalf("AddPushButton: %v", err)
	}
	if bt == nil {
		t.Fatal("nil returned")
	}

	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatalf("WriteTo: %v", err)
	}
	doc2, err := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("OpenStream: %v", err)
	}
	if pdf.FieldType(doc2.Form().Field("submit")) != pdf.FormFieldTypePushButton {
		t.Error("after roundtrip, type is not PushButton")
	}
}

func TestFormAddRadioGroupSinglePage(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	items := []pdf.RadioItem{
		{PageNum: 1, Rect: pdf.Rectangle{LLX: 50, LLY: 400, URX: 70, URY: 420}, Export: "basic"},
		{PageNum: 1, Rect: pdf.Rectangle{LLX: 50, LLY: 370, URX: 70, URY: 390}, Export: "premium"},
	}
	rb, err := doc.Form().AddRadioGroup("plan", items)
	if err != nil {
		t.Fatalf("AddRadioGroup: %v", err)
	}
	rb.Options()[0].SetSelected(true)

	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatalf("WriteTo: %v", err)
	}
	doc2, err := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("OpenStream: %v", err)
	}
	rb2 := doc2.Form().Field("plan").(*pdf.RadioButtonField)
	opts := rb2.Options()
	if len(opts) != 2 {
		t.Fatalf("Options count = %d, want 2", len(opts))
	}
	if !opts[0].Selected() {
		t.Error("opt 0 should be selected")
	}
	if opts[1].Selected() {
		t.Error("opt 1 should not be selected")
	}
}

func TestFormAddRadioGroupCrossPage(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	if err := doc.AddBlankPage(595, 842); err != nil {
		t.Fatalf("AddBlankPage: %v", err)
	}
	items := []pdf.RadioItem{
		{PageNum: 1, Rect: pdf.Rectangle{LLX: 50, LLY: 400, URX: 70, URY: 420}, Export: "page1opt"},
		{PageNum: 2, Rect: pdf.Rectangle{LLX: 50, LLY: 400, URX: 70, URY: 420}, Export: "page2opt"},
	}
	rb, err := doc.Form().AddRadioGroup("xpage", items)
	if err != nil {
		t.Fatalf("AddRadioGroup cross-page: %v", err)
	}
	rb.Options()[1].SetSelected(true)

	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatalf("WriteTo: %v", err)
	}
	doc2, err := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("OpenStream: %v", err)
	}
	rb2 := doc2.Form().Field("xpage").(*pdf.RadioButtonField)
	if !rb2.Options()[1].Selected() {
		t.Error("opt 1 should be selected after cross-page roundtrip")
	}
}

func TestFormAddDuplicateNameReturnsError(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	if _, err := doc.Form().AddTextField(1, pdf.Rectangle{LLX: 50, LLY: 700, URX: 545, URY: 730}, "x"); err != nil {
		t.Fatalf("first AddTextField: %v", err)
	}
	if _, err := doc.Form().AddTextField(1, pdf.Rectangle{LLX: 50, LLY: 660, URX: 545, URY: 690}, "x"); err == nil {
		t.Error("second AddTextField with same name should return error")
	}
	if _, err := doc.Form().AddCheckbox(1, pdf.Rectangle{LLX: 50, LLY: 620, URX: 70, URY: 640}, "x"); err == nil {
		t.Error("AddCheckbox with same name as existing TextField should return error")
	}
}

func TestFormAddInvalidPageNumError(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	if _, err := doc.Form().AddTextField(0, pdf.Rectangle{LLX: 50, LLY: 700, URX: 545, URY: 730}, "x"); err == nil {
		t.Error("pageNum=0 should error")
	}
	if _, err := doc.Form().AddTextField(2, pdf.Rectangle{LLX: 50, LLY: 700, URX: 545, URY: 730}, "y"); err == nil {
		t.Error("pageNum=2 on single-page doc should error")
	}
}

func TestFormAddEmptyNameError(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	if _, err := doc.Form().AddTextField(1, pdf.Rectangle{LLX: 50, LLY: 700, URX: 545, URY: 730}, ""); err == nil {
		t.Error("empty name should error")
	}
}

func TestFormAddRadioGroupEmptyItemsError(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	if _, err := doc.Form().AddRadioGroup("rg", nil); err == nil {
		t.Error("empty items should error")
	}
}

func TestFormAddRadioGroupDuplicateExportError(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	items := []pdf.RadioItem{
		{PageNum: 1, Rect: pdf.Rectangle{LLX: 50, LLY: 400, URX: 70, URY: 420}, Export: "a"},
		{PageNum: 1, Rect: pdf.Rectangle{LLX: 50, LLY: 370, URX: 70, URY: 390}, Export: "a"},
	}
	if _, err := doc.Form().AddRadioGroup("rg", items); err == nil {
		t.Error("duplicate export should error")
	}
}

func TestFormRemoveFieldSimple(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	if _, err := doc.Form().AddTextField(1, pdf.Rectangle{LLX: 50, LLY: 700, URX: 545, URY: 730}, "x"); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if !doc.Form().RemoveField("x") {
		t.Fatal("RemoveField returned false on existing field")
	}
	if doc.Form().HasField("x") {
		t.Error("HasField('x') still true after RemoveField")
	}
}

func TestFormRemoveFieldNotFound(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	if doc.Form().RemoveField("ghost") {
		t.Error("RemoveField returned true for nonexistent field")
	}
}

func TestFormRemoveFieldRadioCascade(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	if err := doc.AddBlankPage(595, 842); err != nil {
		t.Fatalf("AddBlankPage: %v", err)
	}
	items := []pdf.RadioItem{
		{PageNum: 1, Rect: pdf.Rectangle{LLX: 50, LLY: 400, URX: 70, URY: 420}, Export: "a"},
		{PageNum: 2, Rect: pdf.Rectangle{LLX: 50, LLY: 400, URX: 70, URY: 420}, Export: "b"},
	}
	if _, err := doc.Form().AddRadioGroup("rg", items); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if !doc.Form().RemoveField("rg") {
		t.Fatal("Remove failed")
	}
	if doc.Form().HasField("rg") {
		t.Error("HasField still true after Remove")
	}

	// Verify save+reopen still consistent.
	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatalf("WriteTo: %v", err)
	}
	doc2, err := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("OpenStream: %v", err)
	}
	if doc2.Form().HasField("rg") {
		t.Error("HasField returned true after Remove + roundtrip")
	}
}
