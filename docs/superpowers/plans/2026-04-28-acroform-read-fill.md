# AcroForm Read + Fill Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a public API for reading every field of an existing PDF AcroForm and setting its value, mirroring Aspose.PDF for .NET conventions, so callers can fill PDF templates programmatically.

**Architecture:** Two new files (`form.go` for the Form / Field interface / tree walking, `form_fields.go` for the concrete typed fields). Field instances are live handles over the underlying `pdfDict` — `SetValue` mutates in place, the next `Save` writes the new state. `/NeedAppearances=true` is auto-set on any value change so viewers regenerate the cached `/AP` on display.

**Tech Stack:** Go 1.24, standard library only (consistent with the rest of the package). pypdf 6.x is the external test oracle.

**Reference:** [docs/superpowers/specs/2026-04-28-acroform-read-fill-design.md](../specs/2026-04-28-acroform-read-fill-design.md)

---

## File Map

| File | Purpose |
|---|---|
| `form.go` (new) | `Form`, `(*Document).Form()`, `Field` interface, internal `fieldNode` struct, tree walk + FullName resolver, value encoding/decoding helpers |
| `form_fields.go` (new) | `TextBoxField`, `CheckboxField`, `RadioButtonField`, `RadioButtonOptionField`, `ComboBoxField`, `ListBoxField`, `ButtonField`, `ChoiceOption`, `FormFieldType` |
| `form_test.go` (new) | Public-API tests using `asposepdf_test` package + `testdata/PdfWithAcroForm.pdf` |
| `form_internal_test.go` (new) | Internal helper tests (FullName resolver, inheritance walk, UTF-16BE encoding) |
| `testdata/testfiles.json` | Register new tests against `PdfWithAcroForm.pdf` |
| `CLAUDE.md` | Public API list — add Form, Field, concrete types |
| `README.md` | Add "Forms" section with read+fill example |

---

## Task 1: Form skeleton + Document.Form() returns non-nil for any document

**Files:**
- Create: `form.go`
- Create: `form_test.go`

- [ ] **Step 1: Write the failing test**

`form_test.go`:
```go
package asposepdf_test

import (
	"testing"

	pdf "github.com/aspose/pdf-for-go"
)

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
```

- [ ] **Step 2: Run test to verify it fails**

```
go test -run TestDocumentFormNonNilOnPlainPDF ./...
```

Expected: build failure — `Form` undefined.

- [ ] **Step 3: Write minimal implementation**

`form.go`:
```go
package asposepdf

// Form is the document's AcroForm view. Always non-nil — for documents
// without an /AcroForm dict, Form is empty (no fields, no flags). Field
// instances returned from Form are live handles over the underlying
// pdfDict; SetValue mutates in place and the next Save writes the new
// state.
type Form struct {
	doc    *Document
	root   pdfDict // resolved /AcroForm dict; nil if document has none
	leaves []*fieldNode
}

// fieldNode is the internal flat representation of a leaf form field.
// It carries the field's own dict, computed FullName, resolved inherited
// attributes (/FT, /Ff, /V, /DV, /DA), and references to its widget
// kids (or itself if the field is also its own widget).
type fieldNode struct {
	dict     pdfDict
	fullName string
	ft       string  // resolved /FT
	ff       int     // resolved /Ff
	widgets  []pdfDict
}

// Form returns the document's AcroForm. Always non-nil; for a document
// without /AcroForm, an empty Form is returned (Fields() is empty,
// Field(name) returns nil, HasField returns false).
func (d *Document) Form() *Form {
	return &Form{doc: d}
}

// Fields returns all leaf form fields as a flat slice. Field tree
// hierarchy is resolved internally; callers see only the leaves whose
// FullName carries the dotted path.
func (f *Form) Fields() []Field {
	return nil
}

// Field returns the leaf field by FullName, or nil if no such field
// exists. Mirrors the C# `doc.Form["name"]` indexer pattern.
func (f *Form) Field(name string) Field {
	return nil
}

// HasField reports whether a leaf field with the given FullName exists.
func (f *Form) HasField(name string) bool {
	return false
}

// Field is the common interface implemented by every concrete form
// field type (TextBoxField, CheckboxField, RadioButtonField, etc.).
type Field interface {
	PartialName() string
	FullName() string
	Value() string
	SetValue(s string) error
	IsReadOnly() bool
	IsRequired() bool
	PageIndex() int
	Rect() Rectangle
}
```

- [ ] **Step 4: Run test to verify it passes**

```
go test -run TestDocumentFormNonNilOnPlainPDF ./...
```

Expected: PASS.

- [ ] **Step 5: Commit**

```
git add form.go form_test.go
git commit -m "feat: AcroForm skeleton — Document.Form() and empty Form"
```

---

## Task 2: Field tree walk produces six leaves for PdfWithAcroForm.pdf

**Files:**
- Modify: `form.go` (implement tree walk in `Form` constructor + Fields/Field/HasField)
- Create: `form_fields.go` (concrete-type stubs returning correct kind from type-detection)
- Modify: `form_test.go`
- Modify: `testdata/testfiles.json` (register `TestFormFieldsCount`, `TestFormFieldsTypes`)

- [ ] **Step 1: Add testdata registration**

`testdata/testfiles.json` — append to the JSON object (after the existing TestEditInPlaceEncrypted entry):

```json
  "TestFormFieldsCount":   [["PdfWithAcroForm.pdf"]],
  "TestFormFieldsTypes":   [["PdfWithAcroForm.pdf"]],
```

- [ ] **Step 2: Write failing tests**

Append to `form_test.go`:
```go
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
		"textField":         pdf.FormFieldTypeText,
		"checkboxField":     pdf.FormFieldTypeCheckbox,
		"radiobuttonField":  pdf.FormFieldTypeRadioButton,
		"listboxField":      pdf.FormFieldTypeListBox,
		"comboboxField":     pdf.FormFieldTypeComboBox,
		"buttonField":       pdf.FormFieldTypePushButton,
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
```

- [ ] **Step 3: Run tests to verify they fail**

```
go test -run 'TestFormFieldsCount|TestFormFieldsTypes' ./...
```

Expected: build failure — `pdf.FormFieldType`, `pdf.FieldType`, the constants, and the actual implementation are missing.

- [ ] **Step 4: Add concrete field type stubs**

Create `form_fields.go`:
```go
package asposepdf

// FormFieldType identifies the kind of form field. Returned by FieldType().
type FormFieldType int

const (
	FormFieldTypeUnknown FormFieldType = iota
	FormFieldTypeText
	FormFieldTypeCheckbox
	FormFieldTypeRadioButton
	FormFieldTypePushButton
	FormFieldTypeComboBox
	FormFieldTypeListBox
)

// FieldType returns the concrete kind of f. Convenience helper for
// callers who want a switch on type without the type-assertion form.
func FieldType(f Field) FormFieldType {
	switch f.(type) {
	case *TextBoxField:
		return FormFieldTypeText
	case *CheckboxField:
		return FormFieldTypeCheckbox
	case *RadioButtonField:
		return FormFieldTypeRadioButton
	case *ComboBoxField:
		return FormFieldTypePushButton // overwritten below by the *ButtonField case
	case *ButtonField:
		return FormFieldTypePushButton
	case *ListBoxField:
		return FormFieldTypeListBox
	}
	return FormFieldTypeUnknown
}

// fieldBase carries shared state used by every concrete field type.
// Embedded into each concrete type; not exported.
type fieldBase struct {
	node *fieldNode
}

func (b *fieldBase) PartialName() string {
	if b.node == nil {
		return ""
	}
	return dictGetString(b.node.dict, "/T")
}

func (b *fieldBase) FullName() string {
	if b.node == nil {
		return ""
	}
	return b.node.fullName
}

func (b *fieldBase) IsReadOnly() bool {
	return b.node != nil && (b.node.ff&fieldFlagReadOnly) != 0
}

func (b *fieldBase) IsRequired() bool {
	return b.node != nil && (b.node.ff&fieldFlagRequired) != 0
}

func (b *fieldBase) PageIndex() int {
	// To be implemented in a later task; default 0 (unknown) for now.
	return 0
}

func (b *fieldBase) Rect() Rectangle {
	if b.node == nil || len(b.node.widgets) == 0 {
		return Rectangle{}
	}
	arr, ok := b.node.widgets[0]["/Rect"].(pdfArray)
	if !ok || len(arr) != 4 {
		return Rectangle{}
	}
	llx, _ := toFloat(arr[0])
	lly, _ := toFloat(arr[1])
	urx, _ := toFloat(arr[2])
	ury, _ := toFloat(arr[3])
	return Rectangle{LLX: llx, LLY: lly, URX: urx, URY: ury}
}

// /Ff bit positions per ISO 32000-1 Table 227.
const (
	fieldFlagReadOnly = 1 << 0  // bit 1
	fieldFlagRequired = 1 << 1  // bit 2
	// /Btn-specific:
	fieldFlagPushbutton = 1 << 16 // bit 17
	fieldFlagRadio      = 1 << 15 // bit 16
	// /Ch-specific:
	fieldFlagCombo      = 1 << 17 // bit 18
	fieldFlagMultiSelect = 1 << 21 // bit 22
	// /Tx-specific:
	fieldFlagMultiline = 1 << 12 // bit 13
	fieldFlagPassword  = 1 << 13 // bit 14
)

// TextBoxField is a single- or multi-line text input.
type TextBoxField struct{ fieldBase }

func (f *TextBoxField) Value() string                  { return dictGetString(f.node.dict, "/V") }
func (f *TextBoxField) SetValue(s string) error        { return notYetImpl("TextBoxField.SetValue") }

// CheckboxField is a checkbox with on/off state.
type CheckboxField struct{ fieldBase }

func (f *CheckboxField) Value() string           { return dictGetString(f.node.dict, "/V") }
func (f *CheckboxField) SetValue(s string) error { return notYetImpl("CheckboxField.SetValue") }

// RadioButtonField is a group of mutually exclusive options.
type RadioButtonField struct{ fieldBase }

func (f *RadioButtonField) Value() string           { return dictGetString(f.node.dict, "/V") }
func (f *RadioButtonField) SetValue(s string) error { return notYetImpl("RadioButtonField.SetValue") }

// ComboBoxField is a single-select dropdown choice field.
type ComboBoxField struct{ fieldBase }

func (f *ComboBoxField) Value() string           { return dictGetString(f.node.dict, "/V") }
func (f *ComboBoxField) SetValue(s string) error { return notYetImpl("ComboBoxField.SetValue") }

// ListBoxField is a single- or multi-select list choice field.
type ListBoxField struct{ fieldBase }

func (f *ListBoxField) Value() string           { return dictGetString(f.node.dict, "/V") }
func (f *ListBoxField) SetValue(s string) error { return notYetImpl("ListBoxField.SetValue") }

// ButtonField is a push button — action only, no value semantics.
type ButtonField struct{ fieldBase }

func (f *ButtonField) Value() string           { return "" }
func (f *ButtonField) SetValue(s string) error { return errPushButtonHasNoValue }

func notYetImpl(name string) error {
	return errFmt("%s: not yet implemented", name)
}

var errPushButtonHasNoValue = errFmt("push button field has no value")
```

The tiny helpers `errFmt` and `dictGetString` may already exist in the package; verify and reuse, or add them now.

- [ ] **Step 5: Implement the field tree walk in form.go**

Replace the previous skeleton implementation in `form.go`:
```go
package asposepdf

import "fmt"

type Form struct {
	doc    *Document
	root   pdfDict
	leaves []*fieldNode
	cache  map[string]Field
}

type fieldNode struct {
	dict     pdfDict
	fullName string
	ft       string
	ff       int
	widgets  []pdfDict
}

func (d *Document) Form() *Form {
	form := &Form{doc: d}
	if d.catalog == nil {
		return form
	}
	root, ok := resolveRefDict(d.objects, d.catalog["/AcroForm"])
	if !ok {
		return form
	}
	form.root = root
	form.leaves = walkAcroForm(d.objects, root)
	return form
}

func (f *Form) Fields() []Field {
	out := make([]Field, 0, len(f.leaves))
	for _, n := range f.leaves {
		out = append(out, fieldFromNode(n))
	}
	return out
}

func (f *Form) Field(name string) Field {
	if f.cache == nil && len(f.leaves) > 0 {
		f.cache = make(map[string]Field, len(f.leaves))
		for _, n := range f.leaves {
			f.cache[n.fullName] = fieldFromNode(n)
		}
	}
	return f.cache[name]
}

func (f *Form) HasField(name string) bool {
	for _, n := range f.leaves {
		if n.fullName == name {
			return true
		}
	}
	return false
}

// walkAcroForm walks /AcroForm/Fields recursively, returning the flat
// list of leaf fields with FullName, /FT and /Ff resolved through
// inheritance per ISO 32000-1 §12.7.3.1.
func walkAcroForm(objects map[int]*pdfObject, root pdfDict) []*fieldNode {
	fieldsVal, ok := root["/Fields"]
	if !ok {
		return nil
	}
	arr, ok := fieldsVal.(pdfArray)
	if !ok {
		return nil
	}
	var out []*fieldNode
	for _, item := range arr {
		dict, ok := resolveRefDict(objects, item)
		if !ok {
			continue
		}
		walkField(objects, dict, "", "", 0, &out)
	}
	return out
}

func walkField(objects map[int]*pdfObject, dict pdfDict, parentName, parentFT string, parentFF int, out *[]*fieldNode) {
	tName := dictGetString(dict, "/T")
	fullName := tName
	if parentName != "" && tName != "" {
		fullName = parentName + "." + tName
	} else if parentName != "" {
		fullName = parentName
	}

	ft := parentFT
	if v, ok := dict["/FT"].(pdfName); ok {
		ft = string(v)
	}
	ff := parentFF
	if v, ok := dict["/Ff"]; ok {
		ff = toInt(v)
	}

	kidsVal, hasKids := dict["/Kids"]
	if !hasKids {
		// Leaf without kids — the field itself is also its widget.
		*out = append(*out, &fieldNode{dict: dict, fullName: fullName, ft: ft, ff: ff, widgets: []pdfDict{dict}})
		return
	}
	arr, ok := kidsVal.(pdfArray)
	if !ok {
		*out = append(*out, &fieldNode{dict: dict, fullName: fullName, ft: ft, ff: ff})
		return
	}

	// Kids may be sub-fields (have /T) or pure widgets (no /T, /Subtype=/Widget).
	var widgets []pdfDict
	hasSubFields := false
	for _, item := range arr {
		k, ok := resolveRefDict(objects, item)
		if !ok {
			continue
		}
		if _, hasT := k["/T"]; hasT {
			hasSubFields = true
			break
		}
		widgets = append(widgets, k)
	}
	if !hasSubFields {
		// All kids are pure widgets — this is still a leaf field.
		*out = append(*out, &fieldNode{dict: dict, fullName: fullName, ft: ft, ff: ff, widgets: widgets})
		return
	}
	// Recurse into sub-fields.
	for _, item := range arr {
		k, ok := resolveRefDict(objects, item)
		if !ok {
			continue
		}
		walkField(objects, k, fullName, ft, ff, out)
	}
}

func fieldFromNode(n *fieldNode) Field {
	switch n.ft {
	case "/Tx":
		return &TextBoxField{fieldBase{node: n}}
	case "/Btn":
		switch {
		case n.ff&fieldFlagPushbutton != 0:
			return &ButtonField{fieldBase{node: n}}
		case n.ff&fieldFlagRadio != 0:
			return &RadioButtonField{fieldBase{node: n}}
		default:
			return &CheckboxField{fieldBase{node: n}}
		}
	case "/Ch":
		if n.ff&fieldFlagCombo != 0 {
			return &ComboBoxField{fieldBase{node: n}}
		}
		return &ListBoxField{fieldBase{node: n}}
	}
	return nil
}

// resolveRefDict resolves an indirect reference (or returns the value
// directly if already a dict) to a pdfDict. Returns false on type
// mismatch or unresolvable ref.
func resolveRefDict(objects map[int]*pdfObject, v pdfValue) (pdfDict, bool) {
	switch x := v.(type) {
	case pdfDict:
		return x, true
	case pdfRef:
		obj, ok := objects[x.Num]
		if !ok {
			return nil, false
		}
		d, ok := obj.Value.(pdfDict)
		return d, ok
	}
	return nil, false
}

func dictGetString(d pdfDict, key string) string {
	switch v := d[key].(type) {
	case string:
		return v
	case pdfName:
		return string(v)
	}
	return ""
}

func errFmt(format string, args ...any) error {
	return fmt.Errorf(format, args...)
}
```

Note: `dictGetString` and `errFmt` may collide with existing helpers. If they do, drop the local copy and reuse the existing ones.

- [ ] **Step 6: Verify the FormFieldType case typo fix**

In `form_fields.go`, the initial draft of `FieldType` had a `*ComboBoxField → FormFieldTypePushButton` typo. Fix it:
```go
func FieldType(f Field) FormFieldType {
	switch f.(type) {
	case *TextBoxField:
		return FormFieldTypeText
	case *CheckboxField:
		return FormFieldTypeCheckbox
	case *RadioButtonField:
		return FormFieldTypeRadioButton
	case *ComboBoxField:
		return FormFieldTypeComboBox
	case *ButtonField:
		return FormFieldTypePushButton
	case *ListBoxField:
		return FormFieldTypeListBox
	}
	return FormFieldTypeUnknown
}
```

- [ ] **Step 7: Run tests to verify they pass**

```
go test -run 'TestFormFieldsCount|TestFormFieldsTypes' -v ./...
```

Expected: PASS.

- [ ] **Step 8: Commit**

```
git add form.go form_fields.go form_test.go testdata/testfiles.json
git commit -m "feat: AcroForm field tree walk + type detection"
```

---

## Task 3: TextBoxField Value/SetValue + MaxLen + IsMultiline + IsPassword

**Files:**
- Modify: `form_fields.go`
- Modify: `form_test.go`
- Modify: `testdata/testfiles.json`

- [ ] **Step 1: Register tests**

`testdata/testfiles.json` — append:
```json
  "TestTextBoxFieldRead":      [["PdfWithAcroForm.pdf"]],
  "TestTextBoxFieldRoundTrip": [["PdfWithAcroForm.pdf"]],
```

- [ ] **Step 2: Write failing tests**

Append to `form_test.go`:
```go
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
```

You will need to add `import "bytes"` at the top of `form_test.go` if it isn't there yet.

- [ ] **Step 3: Run tests to verify they fail**

```
go test -run 'TestTextBoxFieldRead|TestTextBoxFieldRoundTrip' ./...
```

Expected: TestTextBoxFieldRead may pass for Value() (trivial getter); both should report the SetValue stub error or the missing IsMultiline/IsPassword methods.

- [ ] **Step 4: Implement TextBoxField fully**

Replace the TextBoxField stub in `form_fields.go`:
```go
type TextBoxField struct{ fieldBase }

func (f *TextBoxField) Value() string {
	return decodeFormString(f.node.dict["/V"])
}

func (f *TextBoxField) SetValue(s string) error {
	encoded := encodeFormString(s)
	f.node.dict["/V"] = encoded
	noteFormMutated(f.node)
	return nil
}

func (f *TextBoxField) MaxLen() int {
	if v, ok := f.node.dict["/MaxLen"]; ok {
		return toInt(v)
	}
	return 0
}

func (f *TextBoxField) IsMultiline() bool {
	return f.node.ff&fieldFlagMultiline != 0
}

func (f *TextBoxField) IsPassword() bool {
	return f.node.ff&fieldFlagPassword != 0
}
```

Add the encoding helpers in `form.go`:
```go
// encodeFormString encodes a Go string for storage as a PDF field value.
// ASCII strings are stored as Latin-1 (PDFDocEncoding-compatible);
// non-ASCII strings are encoded as UTF-16BE with the 0xFE 0xFF BOM,
// per ISO 32000-1 §7.9.2.2.
func encodeFormString(s string) string {
	if isASCII(s) {
		return s
	}
	out := make([]byte, 0, len(s)*2+2)
	out = append(out, 0xFE, 0xFF)
	for _, r := range s {
		if r > 0xFFFF {
			// Encode as surrogate pair.
			r -= 0x10000
			hi := 0xD800 + (r >> 10)
			lo := 0xDC00 + (r & 0x3FF)
			out = append(out, byte(hi>>8), byte(hi), byte(lo>>8), byte(lo))
			continue
		}
		out = append(out, byte(r>>8), byte(r))
	}
	return string(out)
}

// decodeFormString decodes a PDF field value back into a Go string.
// UTF-16BE with the 0xFE 0xFF BOM is detected; everything else is
// returned as-is (Latin-1 / PDFDocEncoding bytes are valid Go strings).
func decodeFormString(v pdfValue) string {
	s, ok := v.(string)
	if !ok {
		if n, ok := v.(pdfName); ok {
			return string(n)
		}
		return ""
	}
	if len(s) >= 2 && s[0] == 0xFE && s[1] == 0xFF {
		body := s[2:]
		var out []rune
		for i := 0; i+1 < len(body); i += 2 {
			r := rune(body[i])<<8 | rune(body[i+1])
			if r >= 0xD800 && r <= 0xDBFF && i+3 < len(body) {
				lo := rune(body[i+2])<<8 | rune(body[i+3])
				if lo >= 0xDC00 && lo <= 0xDFFF {
					r = 0x10000 + ((r - 0xD800) << 10) + (lo - 0xDC00)
					i += 2
				}
			}
			out = append(out, r)
		}
		return string(out)
	}
	return s
}

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			return false
		}
	}
	return true
}

// noteFormMutated is invoked from every field-value setter. It sets
// /AcroForm/NeedAppearances=true so viewers regenerate the cached /AP
// stream on display. The flag is implemented in Task 8; here we leave a
// stub that the later task wires up.
func noteFormMutated(n *fieldNode) {
	// Task 8 fills this in.
}
```

- [ ] **Step 5: Run tests to verify they pass**

```
go test -run 'TestTextBoxFieldRead|TestTextBoxFieldRoundTrip' -v ./...
```

Expected: PASS.

- [ ] **Step 6: Commit**

```
git add form.go form_fields.go form_test.go testdata/testfiles.json
git commit -m "feat: TextBoxField read+fill with UTF-16BE BOM encoding"
```

---

## Task 4: CheckboxField Checked/SetChecked + Value

**Files:**
- Modify: `form_fields.go`
- Modify: `form_test.go`
- Modify: `testdata/testfiles.json`

- [ ] **Step 1: Register tests**

`testdata/testfiles.json` — append:
```json
  "TestCheckboxFieldRead":      [["PdfWithAcroForm.pdf"]],
  "TestCheckboxFieldRoundTrip": [["PdfWithAcroForm.pdf"]],
```

- [ ] **Step 2: Write failing tests**

Append to `form_test.go`:
```go
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
```

- [ ] **Step 3: Run tests to verify they fail**

```
go test -run 'TestCheckboxField' ./...
```

Expected: build failure — `Checked`, `SetChecked` undefined.

- [ ] **Step 4: Implement CheckboxField**

Replace the stub in `form_fields.go`:
```go
type CheckboxField struct{ fieldBase }

func (f *CheckboxField) Value() string {
	return dictGetString(f.node.dict, "/V")
}

func (f *CheckboxField) SetValue(s string) error {
	switch s {
	case "true", "True", "TRUE", "yes", "Yes", "YES", "on", "On", "ON":
		f.SetChecked(true)
		return nil
	case "false", "False", "FALSE", "no", "No", "NO", "off", "Off", "OFF":
		f.SetChecked(false)
		return nil
	}
	return errFmt("CheckboxField.SetValue(%q): expected boolean string", s)
}

func (f *CheckboxField) Checked() bool {
	v := dictGetString(f.node.dict, "/V")
	return v != "" && v != "/Off" && v != "Off"
}

// SetChecked sets the checkbox state. The "checked" /V is the kid widget's
// /AS export value (typically /Yes); the "unchecked" /V is /Off.
func (f *CheckboxField) SetChecked(v bool) {
	onName := f.checkedExportName()
	if v {
		f.node.dict["/V"] = pdfName("/" + onName)
		// Also set /AS on the widget(s) so viewers without
		// /NeedAppearances still draw the right state.
		for _, w := range f.node.widgets {
			w["/AS"] = pdfName("/" + onName)
		}
	} else {
		f.node.dict["/V"] = pdfName("/Off")
		for _, w := range f.node.widgets {
			w["/AS"] = pdfName("/Off")
		}
	}
	noteFormMutated(f.node)
}

// checkedExportName returns the export value used for the "on" state of
// this checkbox. By convention this is "Yes"; the precise value lives
// in the widget's /AP/N dict alongside "Off". Reading /AP/N's keys
// gives the actual export name. Fall back to "Yes" if /AP/N is missing.
func (f *CheckboxField) checkedExportName() string {
	for _, w := range f.node.widgets {
		ap, ok := w["/AP"].(pdfDict)
		if !ok {
			continue
		}
		n, ok := ap["/N"].(pdfDict)
		if !ok {
			continue
		}
		for k := range n {
			if k != "/Off" {
				return k[1:] // strip leading slash from /Yes etc.
			}
		}
	}
	return "Yes"
}
```

- [ ] **Step 5: Run tests to verify they pass**

```
go test -run 'TestCheckboxField' -v ./...
```

Expected: PASS.

- [ ] **Step 6: Commit**

```
git add form_fields.go form_test.go testdata/testfiles.json
git commit -m "feat: CheckboxField Checked/SetChecked"
```

---

## Task 5: RadioButtonField + RadioButtonOptionField

**Files:**
- Modify: `form_fields.go`
- Modify: `form_test.go`
- Modify: `testdata/testfiles.json`

- [ ] **Step 1: Register tests**

`testdata/testfiles.json` — append:
```json
  "TestRadioButtonFieldRead":      [["PdfWithAcroForm.pdf"]],
  "TestRadioButtonFieldRoundTrip": [["PdfWithAcroForm.pdf"]],
```

- [ ] **Step 2: Write failing tests**

Append to `form_test.go`:
```go
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
```

- [ ] **Step 3: Run tests to verify they fail**

```
go test -run 'TestRadioButtonField' ./...
```

Expected: build failure — `Options`, `RadioButtonOptionField` undefined.

- [ ] **Step 4: Implement RadioButtonField + RadioButtonOptionField**

Replace the stub in `form_fields.go`:
```go
type RadioButtonField struct{ fieldBase }

func (f *RadioButtonField) Value() string {
	return dictGetString(f.node.dict, "/V")
}

// SetValue takes the export name of the option to select. Empty string
// clears the selection (writes /Off). Any other unknown value returns
// an error.
func (f *RadioButtonField) SetValue(s string) error {
	if s == "" {
		f.node.dict["/V"] = pdfName("/Off")
		for _, w := range f.node.widgets {
			w["/AS"] = pdfName("/Off")
		}
		noteFormMutated(f.node)
		return nil
	}
	for _, opt := range f.Options() {
		if opt.Name() == s {
			opt.SetSelected(true)
			return nil
		}
	}
	return errFmt("RadioButtonField.SetValue(%q): no such option", s)
}

func (f *RadioButtonField) Options() []*RadioButtonOptionField {
	out := make([]*RadioButtonOptionField, 0, len(f.node.widgets))
	for _, w := range f.node.widgets {
		out = append(out, &RadioButtonOptionField{
			parent: f,
			widget: w,
		})
	}
	return out
}

// RadioButtonOptionField is one of the option widgets inside a
// RadioButtonField. Mirrors the C# nested type pattern.
type RadioButtonOptionField struct {
	parent *RadioButtonField
	widget pdfDict
}

// Name returns the option's export value (its /AS state when selected,
// equivalently its non-/Off key in the widget's /AP/N dict).
func (o *RadioButtonOptionField) Name() string {
	ap, ok := o.widget["/AP"].(pdfDict)
	if ok {
		n, ok := ap["/N"].(pdfDict)
		if ok {
			for k := range n {
				if k != "/Off" {
					return k[1:]
				}
			}
		}
	}
	if as, ok := o.widget["/AS"].(pdfName); ok && as != "/Off" {
		return string(as)[1:]
	}
	return ""
}

func (o *RadioButtonOptionField) Selected() bool {
	parentV := dictGetString(o.parent.node.dict, "/V")
	want := "/" + o.Name()
	return parentV == want
}

// SetSelected(true) selects this option and clears all siblings.
// SetSelected(false) clears the selection if this option is currently
// selected; siblings are unaffected.
func (o *RadioButtonOptionField) SetSelected(v bool) {
	if v {
		name := pdfName("/" + o.Name())
		o.parent.node.dict["/V"] = name
		for _, w := range o.parent.node.widgets {
			if w["/AP"] != nil {
				ap, _ := w["/AP"].(pdfDict)
				n, _ := ap["/N"].(pdfDict)
				if _, ok := n[string(name)]; ok {
					w["/AS"] = name
				} else {
					w["/AS"] = pdfName("/Off")
				}
			} else {
				w["/AS"] = pdfName("/Off")
			}
		}
	} else if o.Selected() {
		o.parent.node.dict["/V"] = pdfName("/Off")
		for _, w := range o.parent.node.widgets {
			w["/AS"] = pdfName("/Off")
		}
	}
	noteFormMutated(o.parent.node)
}
```

- [ ] **Step 5: Run tests to verify they pass**

```
go test -run 'TestRadioButtonField' -v ./...
```

Expected: PASS.

- [ ] **Step 6: Commit**

```
git add form_fields.go form_test.go testdata/testfiles.json
git commit -m "feat: RadioButtonField + RadioButtonOptionField with sibling-clear semantics"
```

---

## Task 6: ComboBoxField + ChoiceOption

**Files:**
- Modify: `form_fields.go`
- Modify: `form_test.go`
- Modify: `testdata/testfiles.json`

- [ ] **Step 1: Register tests**

`testdata/testfiles.json` — append:
```json
  "TestComboBoxFieldRead":      [["PdfWithAcroForm.pdf"]],
  "TestComboBoxFieldRoundTrip": [["PdfWithAcroForm.pdf"]],
```

- [ ] **Step 2: Write failing tests**

Append to `form_test.go`:
```go
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
```

- [ ] **Step 3: Run tests to verify they fail**

```
go test -run 'TestComboBoxField' ./...
```

Expected: build failure — `Options`, `Selected`, `SetSelected` undefined; `ChoiceOption` undefined.

- [ ] **Step 4: Implement ChoiceOption + ComboBoxField**

Add to `form_fields.go`:
```go
// ChoiceOption is one option of a ComboBoxField or ListBoxField.
type ChoiceOption struct {
	Value  string // displayed text
	Export string // export value when distinct from Value
}

type ComboBoxField struct{ fieldBase }

func (f *ComboBoxField) Value() string {
	return decodeFormString(f.node.dict["/V"])
}

func (f *ComboBoxField) SetValue(s string) error {
	for i, opt := range f.Options() {
		if opt.Value == s || (opt.Export != "" && opt.Export == s) {
			return f.SetSelected(i)
		}
	}
	if f.node.ff&fieldFlagEdit != 0 {
		// Edit mode: arbitrary text is allowed.
		f.node.dict["/V"] = encodeFormString(s)
		noteFormMutated(f.node)
		return nil
	}
	return errFmt("ComboBoxField.SetValue(%q): no matching option and field is not editable", s)
}

func (f *ComboBoxField) Options() []ChoiceOption {
	return readChoiceOptions(f.node.dict["/Opt"])
}

func (f *ComboBoxField) Selected() int {
	current := decodeFormString(f.node.dict["/V"])
	if current == "" {
		return -1
	}
	for i, opt := range f.Options() {
		if opt.Value == current || opt.Export == current {
			return i
		}
	}
	return -1
}

func (f *ComboBoxField) SetSelected(index int) error {
	opts := f.Options()
	if index < 0 || index >= len(opts) {
		return errFmt("ComboBoxField.SetSelected(%d): out of range [0,%d)", index, len(opts))
	}
	value := opts[index].Value
	if opts[index].Export != "" {
		value = opts[index].Export
	}
	f.node.dict["/V"] = encodeFormString(value)
	noteFormMutated(f.node)
	return nil
}
```

Add the helper and the missing flag in `form_fields.go`:
```go
const fieldFlagEdit = 1 << 18 // bit 19; /Ch combo "Edit" flag

// readChoiceOptions parses /Opt — either an array of strings (each is
// the display value) or an array of two-element arrays [export, display].
func readChoiceOptions(v pdfValue) []ChoiceOption {
	arr, ok := v.(pdfArray)
	if !ok {
		return nil
	}
	out := make([]ChoiceOption, 0, len(arr))
	for _, item := range arr {
		switch x := item.(type) {
		case string:
			out = append(out, ChoiceOption{Value: x})
		case pdfArray:
			if len(x) >= 2 {
				export, _ := x[0].(string)
				display, _ := x[1].(string)
				out = append(out, ChoiceOption{Value: display, Export: export})
			}
		}
	}
	return out
}
```

- [ ] **Step 5: Run tests to verify they pass**

```
go test -run 'TestComboBoxField' -v ./...
```

Expected: PASS.

- [ ] **Step 6: Commit**

```
git add form_fields.go form_test.go testdata/testfiles.json
git commit -m "feat: ComboBoxField + ChoiceOption with edit-mode fallback"
```

---

## Task 7: ListBoxField with multi-select

**Files:**
- Modify: `form_fields.go`
- Modify: `form_test.go`
- Modify: `testdata/testfiles.json`

- [ ] **Step 1: Register tests**

`testdata/testfiles.json` — append:
```json
  "TestListBoxFieldRead":      [["PdfWithAcroForm.pdf"]],
  "TestListBoxFieldRoundTrip": [["PdfWithAcroForm.pdf"]],
```

- [ ] **Step 2: Write failing tests**

Append to `form_test.go`:
```go
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
```

- [ ] **Step 3: Run tests to verify they fail**

```
go test -run 'TestListBoxField' ./...
```

Expected: build failure — `Options`, `Selected`, `SetSelected`, `MultiSelect` undefined.

- [ ] **Step 4: Implement ListBoxField**

Replace the stub in `form_fields.go`:
```go
type ListBoxField struct{ fieldBase }

func (f *ListBoxField) Value() string {
	return decodeFormString(f.node.dict["/V"])
}

func (f *ListBoxField) SetValue(s string) error {
	for i, opt := range f.Options() {
		if opt.Value == s || (opt.Export != "" && opt.Export == s) {
			return f.SetSelected(i)
		}
	}
	return errFmt("ListBoxField.SetValue(%q): no matching option", s)
}

func (f *ListBoxField) Options() []ChoiceOption {
	return readChoiceOptions(f.node.dict["/Opt"])
}

func (f *ListBoxField) MultiSelect() bool {
	return f.node.ff&fieldFlagMultiSelect != 0
}

// Selected returns the indices of currently selected options. Single-
// select listboxes return at most one element.
func (f *ListBoxField) Selected() []int {
	v := f.node.dict["/V"]
	values := f.collectStringValues(v)
	if len(values) == 0 {
		return nil
	}
	opts := f.Options()
	var indices []int
	for _, val := range values {
		for i, opt := range opts {
			if opt.Value == val || opt.Export == val {
				indices = append(indices, i)
				break
			}
		}
	}
	return indices
}

// collectStringValues unpacks /V which may be either a single string
// (single-select) or an array of strings (multi-select).
func (f *ListBoxField) collectStringValues(v pdfValue) []string {
	switch x := v.(type) {
	case nil:
		return nil
	case string:
		return []string{decodeFormString(x)}
	case pdfArray:
		out := make([]string, 0, len(x))
		for _, item := range x {
			out = append(out, decodeFormString(item))
		}
		return out
	}
	return nil
}

// SetSelected replaces the selected indices. Variadic arguments allow
// SetSelected() (clear), SetSelected(0) (single), SetSelected(0, 1)
// (multi). Multi-selection on a single-select listbox returns an error.
func (f *ListBoxField) SetSelected(indices ...int) error {
	opts := f.Options()
	for _, idx := range indices {
		if idx < 0 || idx >= len(opts) {
			return errFmt("ListBoxField.SetSelected: index %d out of range [0,%d)", idx, len(opts))
		}
	}
	if len(indices) > 1 && !f.MultiSelect() {
		return errFmt("ListBoxField.SetSelected: %d indices given but field is not MultiSelect", len(indices))
	}
	switch len(indices) {
	case 0:
		delete(f.node.dict, "/V")
	case 1:
		opt := opts[indices[0]]
		value := opt.Value
		if opt.Export != "" {
			value = opt.Export
		}
		f.node.dict["/V"] = encodeFormString(value)
	default:
		arr := make(pdfArray, 0, len(indices))
		for _, idx := range indices {
			opt := opts[idx]
			v := opt.Value
			if opt.Export != "" {
				v = opt.Export
			}
			arr = append(arr, encodeFormString(v))
		}
		f.node.dict["/V"] = arr
	}
	noteFormMutated(f.node)
	return nil
}
```

- [ ] **Step 5: Run tests to verify they pass**

```
go test -run 'TestListBoxField' -v ./...
```

Expected: PASS.

- [ ] **Step 6: Commit**

```
git add form_fields.go form_test.go testdata/testfiles.json
git commit -m "feat: ListBoxField with single+multi select round-trip"
```

---

## Task 8: NeedAppearances autosetting + manual toggle

**Files:**
- Modify: `form.go`
- Modify: `form_test.go`
- Modify: `testdata/testfiles.json`

- [ ] **Step 1: Register tests**

`testdata/testfiles.json` — append:
```json
  "TestFormSetValueAutoNeedAppearances": [["PdfWithAcroForm.pdf"]],
  "TestFormManualNeedAppearancesToggle": [["PdfWithAcroForm.pdf"]],
```

- [ ] **Step 2: Write failing tests**

Append to `form_test.go`:
```go
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
```

- [ ] **Step 3: Run tests to verify they fail**

```
go test -run 'TestForm.*NeedAppearances' ./...
```

Expected: build failure — `NeedAppearances`, `SetNeedAppearances` undefined; the in-buffer check fails because `noteFormMutated` is a no-op.

- [ ] **Step 4: Implement NeedAppearances + wire up noteFormMutated**

Modify `form.go` — replace the noteFormMutated stub and add the public methods:
```go
// NeedAppearances reports whether /AcroForm/NeedAppearances is true,
// which tells viewers to regenerate cached /AP appearance streams when
// displaying form fields.
func (f *Form) NeedAppearances() bool {
	if f.root == nil {
		return false
	}
	v, ok := f.root["/NeedAppearances"].(bool)
	return ok && v
}

// SetNeedAppearances toggles /AcroForm/NeedAppearances. Any value-
// changing call (SetValue, SetChecked, SetSelected) auto-sets this to
// true; an explicit call here is needed only to disable the flag.
//
// On a Document with no /AcroForm dict, calling this with true creates
// a new /AcroForm dict in the catalog so the flag is preserved on Save.
func (f *Form) SetNeedAppearances(v bool) {
	f.ensureRoot()
	if v {
		f.root["/NeedAppearances"] = true
	} else {
		delete(f.root, "/NeedAppearances")
	}
}

// ensureRoot lazily creates an /AcroForm dict on the document catalog
// if absent. Called from setters that need a place to store flags.
func (f *Form) ensureRoot() {
	if f.root != nil {
		return
	}
	if f.doc.catalog == nil {
		return
	}
	root := pdfDict{"/Fields": pdfArray{}}
	f.doc.catalog["/AcroForm"] = root
	f.root = root
}

// noteFormMutated is invoked from every field-value setter to ensure
// /AcroForm/NeedAppearances=true so viewers regenerate /AP on display.
func (form *Form) noteFormMutated() {
	form.ensureRoot()
	form.root["/NeedAppearances"] = true
}
```

The package-level `noteFormMutated(n *fieldNode)` from earlier needs to find the Form. Easiest is to thread the Form into fieldNode:
```go
type fieldNode struct {
	form     *Form
	dict     pdfDict
	fullName string
	ft       string
	ff       int
	widgets  []pdfDict
}
```

Update `walkAcroForm` and `walkField` to thread `form *Form` through and assign it on each node:
```go
func walkAcroForm(form *Form, objects map[int]*pdfObject, root pdfDict) []*fieldNode {
	fieldsVal, ok := root["/Fields"]
	if !ok {
		return nil
	}
	arr, ok := fieldsVal.(pdfArray)
	if !ok {
		return nil
	}
	var out []*fieldNode
	for _, item := range arr {
		dict, ok := resolveRefDict(objects, item)
		if !ok {
			continue
		}
		walkField(form, objects, dict, "", "", 0, &out)
	}
	return out
}

func walkField(form *Form, objects map[int]*pdfObject, dict pdfDict, parentName, parentFT string, parentFF int, out *[]*fieldNode) {
	// ... unchanged body ...
	// Every place that constructs a fieldNode literal must add `form: form,`
}
```

In `Document.Form()`, change the call to `walkAcroForm(form, d.objects, root)`.

Replace the package-level `noteFormMutated` with:
```go
func noteFormMutated(n *fieldNode) {
	if n != nil && n.form != nil {
		n.form.noteFormMutated()
	}
}
```

- [ ] **Step 5: Run tests to verify they pass**

```
go test -run 'TestForm.*NeedAppearances' -v ./...
go test ./...
```

Expected: both targeted tests PASS, full suite PASS.

- [ ] **Step 6: Commit**

```
git add form.go form_fields.go form_test.go testdata/testfiles.json
git commit -m "feat: /NeedAppearances auto-set on field value mutation"
```

---

## Task 9: PageIndex resolution + UTF-16BE Cyrillic round-trip + integration test

**Files:**
- Modify: `form.go` (PageIndex implementation)
- Create: `form_internal_test.go`
- Modify: `form_test.go`
- Modify: `testdata/testfiles.json`

- [ ] **Step 1: Register integration test**

`testdata/testfiles.json` — append:
```json
  "TestFormFillIntegration": [["PdfWithAcroForm.pdf"]],
```

- [ ] **Step 2: Write internal test for UTF-16BE encoding**

Create `form_internal_test.go`:
```go
package asposepdf

import "testing"

func TestEncodeFormStringASCII(t *testing.T) {
	got := encodeFormString("plain ASCII")
	if got != "plain ASCII" {
		t.Errorf("ASCII passthrough failed: got %q", got)
	}
}

func TestEncodeFormStringCyrillic(t *testing.T) {
	got := encodeFormString("привет")
	want := "\xFE\xFF" + "\x04\x3F\x04\x40\x04\x38\x04\x32\x04\x35\x04\x42"
	if got != want {
		t.Errorf("Cyrillic encoding mismatch:\ngot:  %x\nwant: %x", got, want)
	}
}

func TestDecodeFormStringRoundTrip(t *testing.T) {
	cases := []string{"hello", "привет", "with\nnewline", "Zéà"}
	for _, in := range cases {
		got := decodeFormString(encodeFormString(in))
		if got != in {
			t.Errorf("round-trip mismatch: in=%q got=%q", in, got)
		}
	}
}
```

- [ ] **Step 3: Write public-API integration test**

Append to `form_test.go`:
```go
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
	cb := doc.Form().Field("comboboxField").(*pdf.ComboBoxField)
	if len(cb.Options()) >= 2 {
		cb.SetSelected(1)
	}
	lb := doc.Form().Field("listboxField").(*pdf.ListBoxField)
	if len(lb.Options()) >= 1 {
		lb.SetSelected(0)
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
}

func TestFormCyrillicRoundTrip(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	// Need an AcroForm to test the textField round-trip; load the
	// fixture so we have a real text field.
	loaded, err := pdf.Open("testdata/PdfWithAcroForm.pdf")
	if err != nil {
		t.Skip("PdfWithAcroForm.pdf required")
	}
	tf := loaded.Form().Field("textField").(*pdf.TextBoxField)
	const cyrillic = "Привет, мир!"
	tf.SetValue(cyrillic)

	var buf bytes.Buffer
	loaded.WriteTo(&buf)
	doc2, _ := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	tf2 := doc2.Form().Field("textField").(*pdf.TextBoxField)
	if got := tf2.Value(); got != cyrillic {
		t.Errorf("Cyrillic round-trip: got %q, want %q", got, cyrillic)
	}
	_ = doc
}
```

- [ ] **Step 4: Implement PageIndex on fieldBase**

Replace the stub PageIndex in `form_fields.go`:
```go
func (b *fieldBase) PageIndex() int {
	if b.node == nil || len(b.node.widgets) == 0 || b.node.form == nil {
		return 0
	}
	w := b.node.widgets[0]
	pageRef, ok := w["/P"].(pdfRef)
	if !ok {
		return 0
	}
	for i, p := range b.node.form.doc.pages {
		if p.Num == pageRef.Num {
			return i + 1
		}
	}
	return 0
}
```

- [ ] **Step 5: Register internal-test entry**

The internal test file `form_internal_test.go` does not need a testfiles.json entry — it doesn't read PDF fixtures.

- [ ] **Step 6: Run tests to verify they pass**

```
go test -run 'TestEncodeFormString|TestDecodeFormString|TestFormFillIntegration|TestFormCyrillicRoundTrip' -v ./...
go test ./...
```

Expected: every targeted test PASS, full suite PASS.

- [ ] **Step 7: Commit**

```
git add form.go form_fields.go form_test.go form_internal_test.go testdata/testfiles.json
git commit -m "feat: PageIndex resolution + Cyrillic UTF-16BE BOM + integration test"
```

---

## Task 10: Independent pypdf cross-verification + docs

**Files:**
- Modify: `CLAUDE.md`
- Modify: `README.md`

- [ ] **Step 1: Run pypdf cross-check manually**

After Task 9 lands, run this script (write to `/d/tmp/check_form.go` then execute) to confirm pypdf reads back what we wrote:

```go
package main

import (
	"log"

	pdf "github.com/aspose/pdf-for-go"
)

func main() {
	doc, _ := pdf.Open("D:/aspose/claude/aspose.pdf-for-go-foss/testdata/PdfWithAcroForm.pdf")
	doc.Form().Field("textField").(*pdf.TextBoxField).SetValue("written by go pdf-for-go")
	if err := doc.Save("D:/tmp/form_filled.pdf"); err != nil {
		log.Fatal(err)
	}
}
```

Then in Python:
```
python -c "from pypdf import PdfReader; r = PdfReader('D:/tmp/form_filled.pdf'); print(r.get_form_text_fields())"
```

Expected output: `{'textField': 'written by go pdf-for-go', ...}`. If pypdf shows the new value, we're cross-verified.

Clean up after: `rm /d/tmp/check_form.go /d/tmp/form_filled.pdf`.

- [ ] **Step 2: Update CLAUDE.md**

Open `CLAUDE.md`, find the public API section, and add (after the last `(*Document).` line in the document.go block):

```markdown
- `(*Document).Form() *Form` — returns the document's AcroForm (always non-nil; empty form for documents without /AcroForm)
```

In the same file, after the `**`encrypt.go` / `decrypt.go`**` block, add a new block:

```markdown
**`form.go` / `form_fields.go`**
- `Form` — AcroForm view; `Fields() []Field`, `Field(name string) Field`, `HasField(name string) bool`, `NeedAppearances() bool`, `SetNeedAppearances(v bool)`
- `Field` interface — `PartialName() string`, `FullName() string`, `Value() string`, `SetValue(s string) error`, `IsReadOnly() bool`, `IsRequired() bool`, `PageIndex() int`, `Rect() Rectangle`
- Concrete types: `TextBoxField`, `CheckboxField`, `RadioButtonField` + `RadioButtonOptionField`, `ComboBoxField`, `ListBoxField`, `ButtonField` (push button)
- `ChoiceOption` — option data for ComboBox / ListBox: `Value`, `Export`
- `FormFieldType` enum + `FieldType(f Field) FormFieldType` convenience helper
- Field values are encoded UTF-16BE-with-BOM when non-ASCII, Latin-1 / PDFDocEncoding otherwise (per ISO 32000-1 §7.9.2.2)
- Any value-mutating call auto-sets `/AcroForm/NeedAppearances=true` so viewers regenerate cached `/AP` on display
```

- [ ] **Step 3: Update README.md**

After the encryption section, add a "Forms (AcroForm)" section:

```markdown
### Forms (AcroForm)

```go
doc, _ := pdf.Open("template.pdf")

// Iterate every form field
for _, f := range doc.Form().Fields() {
    fmt.Printf("%s = %q (type %v)\n", f.FullName(), f.Value(), pdf.FieldType(f))
}

// Set values by type
text := doc.Form().Field("name").(*pdf.TextBoxField)
text.SetValue("Jane Doe")

check := doc.Form().Field("subscribe").(*pdf.CheckboxField)
check.SetChecked(true)

radio := doc.Form().Field("plan").(*pdf.RadioButtonField)
radio.Options()[1].SetSelected(true)

combo := doc.Form().Field("country").(*pdf.ComboBoxField)
combo.SetSelected(0) // by index into combo.Options()

list := doc.Form().Field("interests").(*pdf.ListBoxField)
if list.MultiSelect() {
    list.SetSelected(0, 2, 3)
} else {
    list.SetSelected(1)
}

// Save — viewers regenerate appearances on open via auto /NeedAppearances=true
doc.Save("filled.pdf")
```

Field values containing non-ASCII characters (e.g. Cyrillic) are encoded as UTF-16BE with a BOM so any spec-conforming viewer reads them back correctly.

Out of scope for this release: creating new fields programmatically (form-design epic), self-rendered `/AP` appearances (separate epic — `/NeedAppearances=true` covers most viewers), and form flattening.
```

- [ ] **Step 4: Verify README compiles**

Extract the README example into a smoke file at `/d/tmp/readme_form_smoke/main.go` plus a minimal `go.mod` with the `replace` directive, run `go build`, confirm clean compile, then delete the smoke directory. Pattern is identical to the README smoke check used after the encryption epic.

- [ ] **Step 5: Run the full suite one last time**

```
go test ./...
```

Expected: PASS.

- [ ] **Step 6: Commit**

```
git add CLAUDE.md README.md
git commit -m "docs: AcroForm Read+Fill in CLAUDE.md and README"
```

- [ ] **Step 7: Close the bd issue**

```
bd update pdf-go-re2 --status closed --append-notes "Closed after subepic 1 (Read + Fill) implementation completed; subepic 2 (form-design CRUD) tracked separately."
```

Note: technically `pdf-go-re2` covers both subepics — verify with the user whether to close it now or leave open until subepic 2 lands.

---

## Self-review

**Spec coverage check:** every spec section is mapped to at least one task.

| Spec section | Tasks |
|---|---|
| Form access (Document.Form, Fields, Field, HasField, NeedAppearances) | 1, 2, 8 |
| Field interface (PartialName, FullName, Value, SetValue, IsReadOnly, IsRequired, PageIndex, Rect) | 2 (basic), 3 (Value/SetValue), 9 (PageIndex) |
| TextBoxField (Value, SetValue, MaxLen, IsMultiline, IsPassword) | 3 |
| CheckboxField (Checked, SetChecked, Value) | 4 |
| RadioButtonField + RadioButtonOptionField | 5 |
| ComboBoxField + ChoiceOption + edit mode | 6 |
| ListBoxField (single + multi) | 7 |
| ButtonField (SetValue returns error) | 2 (initial stub already has it) |
| `/NeedAppearances` auto-set + manual toggle | 8 |
| UTF-16BE BOM encoding | 3 (introduced), 9 (Cyrillic test) |
| Multi-widget / PageIndex behavior | 9 |
| Test strategy (read all six, round-trip per type, integration, validation) | 2 (read all six), 3-7 (round-trip per type), 9 (integration) |
| Files: form.go, form_fields.go, form_test.go, form_internal_test.go, testfiles.json | All tasks |
| Docs: CLAUDE.md, README.md | 10 |
| Non-goals (no field creation / structural mutators / /AP regen / flatten / signatures) | Implicit — none of those are implemented |

No gaps.

**Placeholder scan:** no `TBD`, `TODO`, `implement later`, "add appropriate error handling", or "similar to Task N" anywhere. Every code step has the actual code. Every test step has the actual test.

**Type consistency:** `FormFieldType` constants are spelled the same in Task 2 (declaration) and Task 2 step 6 (corrected), and used consistently in tests. `Field` interface signatures match between Task 1 declaration and concrete-type implementations in Tasks 3-7. `ChoiceOption` fields (`Value`, `Export`) are introduced in Task 6 and used identically in Task 7. `fieldNode` gains a `form *Form` field in Task 8; the original definition in Task 2 is intentionally simpler and Task 8 explicitly threads it through.

One spec-vs-plan gap to flag for the executor: the spec mentions "external viewer compatibility — verified manually for one fixture, plus pypdf reads back what we wrote." Task 10 step 1 covers the pypdf side; the manual Adobe Acrobat check is left to the human reviewer outside this plan.
