// SPDX-License-Identifier: MIT

package asposepdf_test

import (
	"bytes"
	"encoding/json"
	"testing"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

// builtForm holds a fresh document with one field of each type, for JSON
// round-trip tests. valued reports whether the fields start with values set.
type builtForm struct {
	doc   *pdf.Document
	text  *pdf.TextBoxField
	check *pdf.CheckboxField
	combo *pdf.ComboBoxField
	list  *pdf.ListBoxField
	radio *pdf.RadioButtonField
}

func buildJSONForm(t *testing.T, valued bool) builtForm {
	t.Helper()
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	form := doc.Form()

	tb, err := form.AddTextField(1, pdf.Rectangle{LLX: 50, LLY: 700, URX: 250, URY: 720}, "name")
	if err != nil {
		t.Fatal(err)
	}
	cb, err := form.AddCheckbox(1, pdf.Rectangle{LLX: 50, LLY: 660, URX: 70, URY: 680}, "agree")
	if err != nil {
		t.Fatal(err)
	}
	combo, err := form.AddComboBox(1, pdf.Rectangle{LLX: 50, LLY: 620, URX: 250, URY: 640}, "country",
		[]pdf.ChoiceOption{{Value: "USA"}, {Value: "Canada"}, {Value: "Mexico"}})
	if err != nil {
		t.Fatal(err)
	}
	list, err := form.AddListBox(1, pdf.Rectangle{LLX: 50, LLY: 540, URX: 250, URY: 600}, "langs",
		[]pdf.ChoiceOption{{Value: "en"}, {Value: "fr"}, {Value: "de"}})
	if err != nil {
		t.Fatal(err)
	}
	list.SetMultiSelect(true)
	radio, err := form.AddRadioGroup("color", []pdf.RadioItem{
		{PageNum: 1, Rect: pdf.Rectangle{LLX: 50, LLY: 500, URX: 70, URY: 520}, Export: "red"},
		{PageNum: 1, Rect: pdf.Rectangle{LLX: 90, LLY: 500, URX: 110, URY: 520}, Export: "green"},
		{PageNum: 1, Rect: pdf.Rectangle{LLX: 130, LLY: 500, URX: 150, URY: 520}, Export: "blue"},
	})
	if err != nil {
		t.Fatal(err)
	}

	if valued {
		mustNoErr(t, tb.SetValue("Alice"))
		cb.SetChecked(true)
		mustNoErr(t, combo.SetValue("Canada"))
		mustNoErr(t, list.SetSelected(0, 2)) // en, de
		mustNoErr(t, radio.SetValue("green"))
	} else {
		// A freshly created listbox auto-selects its first option; clear it so
		// the "unvalued" form is genuinely empty.
		mustNoErr(t, list.SetSelected())
	}
	return builtForm{doc: doc, text: tb, check: cb, combo: combo, list: list, radio: radio}
}

// TestFormJSONExportShape: export produces the typed {type,value} shape with the
// right JSON kinds per field type.
func TestFormJSONExportShape(t *testing.T) {
	bf := buildJSONForm(t, true)
	data, err := bf.doc.Form().ExportJSON() // compact, so values compare verbatim
	if err != nil {
		t.Fatal(err)
	}

	var got map[string]struct {
		Type  string          `json:"type"`
		Value json.RawMessage `json:"value"`
	}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("export is not valid JSON: %v\n%s", err, data)
	}

	want := map[string]struct {
		typ string
		val string
	}{
		"name":    {"text", `"Alice"`},
		"agree":   {"checkbox", `true`},
		"country": {"combobox", `"Canada"`},
		"langs":   {"listbox", `["en","de"]`},
		"color":   {"radio", `"green"`},
	}
	for name, w := range want {
		g, ok := got[name]
		if !ok {
			t.Errorf("missing field %q in export", name)
			continue
		}
		if g.Type != w.typ {
			t.Errorf("%q: type = %q, want %q", name, g.Type, w.typ)
		}
		if s := string(g.Value); s != w.val {
			t.Errorf("%q: value = %s, want %s", name, s, w.val)
		}
	}
}

// TestFormJSONRoundTrip: export from a valued form, import into a fresh empty
// form, and confirm every field value is reproduced.
func TestFormJSONRoundTrip(t *testing.T) {
	src := buildJSONForm(t, true)
	data, err := src.doc.Form().ExportJSON()
	if err != nil {
		t.Fatal(err)
	}

	dst := buildJSONForm(t, false) // same fields, no values
	n, err := dst.doc.Form().ImportJSON(data)
	if err != nil {
		t.Fatal(err)
	}
	if n != 5 {
		t.Errorf("imported %d fields, want 5", n)
	}

	if got := dst.text.Value(); got != "Alice" {
		t.Errorf("text = %q, want Alice", got)
	}
	if !dst.check.Checked() {
		t.Error("checkbox not checked after import")
	}
	if got := dst.combo.Value(); got != "Canada" {
		t.Errorf("combo = %q, want Canada", got)
	}
	if got := dst.list.Selected(); len(got) != 2 || got[0] != 0 || got[1] != 2 {
		t.Errorf("listbox selected = %v, want [0 2]", got)
	}
	if got := dst.radio.Value(); got != "/green" && got != "green" {
		t.Errorf("radio = %q, want green", got)
	}
}

// TestFormJSONRoundTripThroughSave: values survive export → save → reopen → the
// JSON also survives a document round-trip.
func TestFormJSONRoundTripThroughSave(t *testing.T) {
	src := buildJSONForm(t, true)
	data, err := src.doc.Form().ExportJSON()
	if err != nil {
		t.Fatal(err)
	}

	dst := buildJSONForm(t, false)
	if _, err := dst.doc.Form().ImportJSON(data); err != nil {
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
		t.Errorf("after save+reopen, name = %q, want Alice", got)
	}
}

// TestFormJSONOmitEmpty: OmitEmpty drops valueless fields; the default keeps them.
func TestFormJSONOmitEmpty(t *testing.T) {
	bf := buildJSONForm(t, false) // no values
	full, err := bf.doc.Form().ExportJSON()
	if err != nil {
		t.Fatal(err)
	}
	var fullMap map[string]json.RawMessage
	mustNoErr(t, json.Unmarshal(full, &fullMap))
	if len(fullMap) != 5 {
		t.Errorf("default export has %d fields, want 5 (full snapshot)", len(fullMap))
	}

	omitted, err := bf.doc.Form().ExportJSON(pdf.JSONExportOptions{OmitEmpty: true})
	if err != nil {
		t.Fatal(err)
	}
	var omitMap map[string]json.RawMessage
	mustNoErr(t, json.Unmarshal(omitted, &omitMap))
	if len(omitMap) != 0 {
		t.Errorf("OmitEmpty export has %d fields, want 0 (all empty)", len(omitMap))
	}
}

// TestFormJSONImportLenient: unknown keys and a boolean-string checkbox are
// handled; the applied count excludes the unknown key.
func TestFormJSONImportLenient(t *testing.T) {
	bf := buildJSONForm(t, false)
	data := []byte(`{
		"name": {"type": "text", "value": "Bob"},
		"agree": {"type": "checkbox", "value": "yes"},
		"ghost": {"type": "text", "value": "nope"}
	}`)
	n, err := bf.doc.Form().ImportJSON(data)
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Errorf("applied %d, want 2 (ghost skipped)", n)
	}
	if got := bf.text.Value(); got != "Bob" {
		t.Errorf("text = %q, want Bob", got)
	}
	if !bf.check.Checked() {
		t.Error("checkbox should be checked from boolean string \"yes\"")
	}
}

// TestFormJSONExportRealFile: a real AcroForm exports as typed JSON with at
// least one field.
func TestFormJSONExportRealFile(t *testing.T) {
	doc, err := pdf.Open(testFile(t))
	if err != nil {
		t.Fatal(err)
	}
	data, err := doc.Form().ExportJSON(pdf.JSONExportOptions{Indent: true})
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]struct {
		Type  string          `json:"type"`
		Value json.RawMessage `json:"value"`
	}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("export not valid JSON: %v", err)
	}
	if len(m) == 0 {
		t.Error("expected at least one field in the exported form")
	}
	for name, fld := range m {
		if fld.Type == "" {
			t.Errorf("field %q exported without a type", name)
		}
	}
}
