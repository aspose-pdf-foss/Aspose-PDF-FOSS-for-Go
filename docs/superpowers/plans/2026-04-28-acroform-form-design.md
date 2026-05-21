# AcroForm Form-Design CRUD Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `Form.AddTextField/Checkbox/RadioGroup/ComboBox/ListBox/PushButton` constructors, `Form.RemoveField`, and per-type structural mutators so callers can build PDF forms programmatically and edit existing field structure.

**Architecture:** All new methods land in `form.go` (Form-level) and `form_fields.go` (per-type). Single-widget fields use the combined field+widget dict pattern; radio groups use parent + kid widgets. `/AcroForm` and `/AcroForm/DR/Font/Helv` are auto-created on first Add. Cache + fieldsList rebuild after every structural change so the live-handle invariant from `pdf-go-re2` holds.

**Tech Stack:** Go 1.24, standard library only. pypdf 6.x as external test oracle.

**Reference:** [docs/superpowers/specs/2026-04-28-acroform-form-design-design.md](../specs/2026-04-28-acroform-form-design-design.md)

---

## File Map

| File | Purpose |
|---|---|
| `form.go` (modify) | New `Form.AddTextField/Checkbox/ComboBox/ListBox/PushButton/RadioGroup`, `Form.RemoveField`, internal helpers `ensureFontHelv`, `appendWidgetToPage`, `removeWidgetFromPage`, `rebuildFieldCache` |
| `form_fields.go` (modify) | Per-type structural mutators (`SetReadOnly`, `SetRequired`, `SetMaxLen`, `SetMultiline`, `SetPassword`, `SetMultiSelect`, `SetEditable`); `AddOption`/`RemoveOption` on `ComboBoxField` and `ListBoxField`; `RadioItem` struct |
| `form_design_test.go` (new) | Public-API tests for all Add/Remove/structural-mutator paths |
| `testdata/testfiles.json` | Register tests that use existing fixtures (most use `NewDocument`, no fixture) |
| `CLAUDE.md` | New methods in public API list |
| `README.md` | "Forms — building from scratch" subsection with example |

---

## Task 1: `AddTextField` — auto-creates `/AcroForm`, `/DR/Font/Helv`, combined field+widget dict

**Files:**
- Modify: `form.go`
- Create: `form_design_test.go`

- [ ] **Step 1: Write the failing test**

Create `form_design_test.go`:
```go
package asposepdf_test

import (
	"bytes"
	"testing"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
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
```

- [ ] **Step 2: Run test to verify it fails**

```
go test -run TestFormAddTextFieldRoundTrip ./...
```

Expected: build failure — `AddTextField` undefined.

- [ ] **Step 3: Implement `AddTextField` and helpers**

Add to `form.go`:
```go
// AddTextField creates a single-line text input on pageNum with the
// given rectangle and field name, auto-creating /AcroForm and the
// default Helvetica font resource if needed. Returns the live
// *TextBoxField handle. Errors on duplicate name, invalid pageNum,
// or empty name.
func (f *Form) AddTextField(pageNum int, rect Rectangle, name string) (*TextBoxField, error) {
	if err := f.validateNewField(pageNum, name); err != nil {
		return nil, err
	}
	page, err := f.doc.Page(pageNum)
	if err != nil {
		return nil, err
	}

	helvName, err := f.ensureFontHelv()
	if err != nil {
		return nil, err
	}

	dict := pdfDict{
		"/Type":    pdfName("/Annot"),
		"/Subtype": pdfName("/Widget"),
		"/FT":      pdfName("/Tx"),
		"/T":       name,
		"/V":       "",
		"/DA":      "0 g /" + helvName + " 12 Tf",
		"/Rect":    rectToPDFArray(rect),
		"/P":       pdfRef{Num: page.pageObj().Num},
	}

	objID := f.doc.nextID
	f.doc.nextID++
	f.doc.objects[objID] = &pdfObject{Num: objID, Value: dict}
	ref := pdfRef{Num: objID}

	f.appendToFields(ref)
	appendWidgetToPage(page.pageObj(), ref)

	f.rebuildFieldCache()
	f.noteFormMutatedInForm()

	return f.cache[name].(*TextBoxField), nil
}

// validateNewField checks the common preconditions for any AddXxx call.
func (f *Form) validateNewField(pageNum int, name string) error {
	if name == "" {
		return fmt.Errorf("form field name is empty")
	}
	if pageNum < 1 || pageNum > f.doc.PageCount() {
		return fmt.Errorf("pageNum %d out of range [1,%d]", pageNum, f.doc.PageCount())
	}
	if f.HasField(name) {
		return fmt.Errorf("field with name %q already exists", name)
	}
	return nil
}

// appendToFields appends a ref to /AcroForm/Fields, creating the array
// if absent.
func (f *Form) appendToFields(ref pdfRef) {
	f.ensureRoot()
	arr, _ := f.root["/Fields"].(pdfArray)
	arr = append(arr, ref)
	f.root["/Fields"] = arr
}

// appendWidgetToPage appends a widget ref to a page's /Annots, creating
// the array if absent.
func appendWidgetToPage(pageObj *pdfObject, widgetRef pdfRef) {
	pageDict, _ := pageObj.Value.(pdfDict)
	if pageDict == nil {
		return
	}
	arr, _ := pageDict["/Annots"].(pdfArray)
	arr = append(arr, widgetRef)
	pageDict["/Annots"] = arr
}

// rebuildFieldCache regenerates Form.fieldsList and Form.cache from the
// current /AcroForm/Fields. Called after any structural change so live
// handles returned from prior calls remain canonical.
func (f *Form) rebuildFieldCache() {
	if f.root == nil {
		f.leaves = nil
		f.fieldsList = nil
		f.cache = nil
		return
	}
	f.leaves = walkAcroForm(f, f.doc.objects, f.root)
	f.fieldsList = make([]Field, len(f.leaves))
	f.cache = make(map[string]Field, len(f.leaves))
	for i, n := range f.leaves {
		field := fieldFromNode(n)
		f.fieldsList[i] = field
		f.cache[n.fullName] = field
	}
}

// ensureFontHelv registers a Helvetica font resource under /AcroForm/DR/
// Font/Helv and returns its resource name ("Helv"). Idempotent.
func (f *Form) ensureFontHelv() (string, error) {
	f.ensureRoot()
	dr, _ := f.root["/DR"].(pdfDict)
	if dr == nil {
		dr = pdfDict{}
		f.root["/DR"] = dr
	}
	fonts, _ := dr["/Font"].(pdfDict)
	if fonts == nil {
		fonts = pdfDict{}
		dr["/Font"] = fonts
	}
	if _, ok := fonts["Helv"]; ok {
		return "Helv", nil
	}
	fontDict := pdfDict{
		"/Type":     pdfName("/Font"),
		"/Subtype":  pdfName("/Type1"),
		"/BaseFont": pdfName("/Helvetica"),
		"/Encoding": pdfName("/WinAnsiEncoding"),
	}
	id := f.doc.nextID
	f.doc.nextID++
	f.doc.objects[id] = &pdfObject{Num: id, Value: fontDict}
	fonts["Helv"] = pdfRef{Num: id}
	return "Helv", nil
}

// rectToPDFArray converts a Rectangle to a /Rect pdfArray.
func rectToPDFArray(r Rectangle) pdfArray {
	return pdfArray{r.LLX, r.LLY, r.URX, r.URY}
}
```

You'll need a small helper on Page to expose its underlying pdfObject (called `pageObj()` above). If it doesn't exist already, add to `page.go`:
```go
// pageObj returns the underlying *pdfObject for this page.
func (p *Page) pageObj() *pdfObject {
	return p.doc.pages[p.index]
}
```

If `pageDict` (lowercase) already exists in `page.go` for similar purpose, reuse it instead of `pageObj()`.

`fmt` import: add to form.go if not already.

- [ ] **Step 4: Run test to verify it passes**

```
go test -run TestFormAddTextFieldRoundTrip -v ./...
go test ./...
```

Both should PASS.

- [ ] **Step 5: Register the test (no fixture, but pattern requires entry only if testFile is used; this test uses NewDocument so no entry needed). Skip this step.**

- [ ] **Step 6: Commit**

```
git add form.go form_design_test.go page.go
git commit -m "feat: Form.AddTextField + ensureFontHelv + widget/page coordination"
```

---

## Task 2: `AddCheckbox`

**Files:**
- Modify: `form.go`
- Modify: `form_design_test.go`

- [ ] **Step 1: Write the failing test**

Append to `form_design_test.go`:
```go
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
	doc2, _ := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	cb2 := doc2.Form().Field("subscribe").(*pdf.CheckboxField)
	if !cb2.Checked() {
		t.Error("checkbox not checked after roundtrip")
	}
}
```

- [ ] **Step 2: Run to confirm failure**

```
go test -run TestFormAddCheckboxRoundTrip ./...
```

Expected: build failure — `AddCheckbox` undefined.

- [ ] **Step 3: Implement `AddCheckbox`**

Add to `form.go`:
```go
// AddCheckbox creates a checkbox widget. Default state is unchecked
// (/V = /Off). The widget's /AP/N has two states: "/Yes" (export name
// for checked) and "/Off". Caller can SetChecked(true) on the returned
// handle to flip state and ensure /V/AS sync.
func (f *Form) AddCheckbox(pageNum int, rect Rectangle, name string) (*CheckboxField, error) {
	if err := f.validateNewField(pageNum, name); err != nil {
		return nil, err
	}
	page, err := f.doc.Page(pageNum)
	if err != nil {
		return nil, err
	}
	helvName, err := f.ensureFontHelv()
	if err != nil {
		return nil, err
	}

	// Empty placeholder XObject refs for /Off and /Yes states. Viewers
	// regenerate visible appearances when /NeedAppearances=true.
	apN := pdfDict{
		"/Off": placeholderXObjectRef(f.doc),
		"/Yes": placeholderXObjectRef(f.doc),
	}

	dict := pdfDict{
		"/Type":    pdfName("/Annot"),
		"/Subtype": pdfName("/Widget"),
		"/FT":      pdfName("/Btn"),
		"/T":       name,
		"/V":       pdfName("/Off"),
		"/AS":      pdfName("/Off"),
		"/DA":      "0 g /" + helvName + " 12 Tf",
		"/Rect":    rectToPDFArray(rect),
		"/P":       pdfRef{Num: page.pageObj().Num},
		"/AP":      pdfDict{"/N": apN},
	}

	objID := f.doc.nextID
	f.doc.nextID++
	f.doc.objects[objID] = &pdfObject{Num: objID, Value: dict}
	ref := pdfRef{Num: objID}

	f.appendToFields(ref)
	appendWidgetToPage(page.pageObj(), ref)
	f.rebuildFieldCache()
	f.noteFormMutatedInForm()

	return f.cache[name].(*CheckboxField), nil
}

// placeholderXObjectRef creates an empty Form XObject and returns its
// reference. Used for widget /AP/N placeholder entries — viewers
// regenerate the actual visual at display time when /NeedAppearances
// is true.
func placeholderXObjectRef(doc *Document) pdfRef {
	stream := &pdfStream{
		Dict: pdfDict{
			"/Type":     pdfName("/XObject"),
			"/Subtype":  pdfName("/Form"),
			"/BBox":     pdfArray{0, 0, 0, 0},
			"/Resources": pdfDict{},
		},
		Data:    []byte{},
		Decoded: true,
	}
	id := doc.nextID
	doc.nextID++
	doc.objects[id] = &pdfObject{Num: id, Value: stream}
	return pdfRef{Num: id}
}
```

- [ ] **Step 4: Run tests**

```
go test -run TestFormAddCheckboxRoundTrip -v ./...
go test ./...
```

Both PASS.

- [ ] **Step 5: Commit**

```
git add form.go form_design_test.go
git commit -m "feat: Form.AddCheckbox with placeholder /AP states"
```

---

## Task 3: `AddComboBox`

**Files:**
- Modify: `form.go`
- Modify: `form_design_test.go`

- [ ] **Step 1: Write the failing test**

Append to `form_design_test.go`:
```go
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
	doc2, _ := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	cb2 := doc2.Form().Field("country").(*pdf.ComboBoxField)
	if got := len(cb2.Options()); got != 3 {
		t.Errorf("Options count = %d, want 3", got)
	}
	if got := cb2.Selected(); got != 1 {
		t.Errorf("Selected = %d, want 1", got)
	}
}
```

- [ ] **Step 2: Confirm failure**

```
go test -run TestFormAddComboBoxRoundTrip ./...
```

Expected: build failure.

- [ ] **Step 3: Implement `AddComboBox`**

Add to `form.go`:
```go
// AddComboBox creates a single-select dropdown choice field. The
// caller can pre-populate options or pass an empty slice and call
// AddOption later. Field is non-editable by default; SetEditable(true)
// flips bit 19.
func (f *Form) AddComboBox(pageNum int, rect Rectangle, name string, options []ChoiceOption) (*ComboBoxField, error) {
	if err := f.validateNewField(pageNum, name); err != nil {
		return nil, err
	}
	page, err := f.doc.Page(pageNum)
	if err != nil {
		return nil, err
	}
	helvName, err := f.ensureFontHelv()
	if err != nil {
		return nil, err
	}

	dict := pdfDict{
		"/Type":    pdfName("/Annot"),
		"/Subtype": pdfName("/Widget"),
		"/FT":      pdfName("/Ch"),
		"/T":       name,
		"/V":       "",
		"/Ff":      fieldFlagCombo, // distinguishes ComboBox from ListBox
		"/Opt":     choiceOptionsToPDFArray(options),
		"/DA":      "0 g /" + helvName + " 12 Tf",
		"/Rect":    rectToPDFArray(rect),
		"/P":       pdfRef{Num: page.pageObj().Num},
	}

	objID := f.doc.nextID
	f.doc.nextID++
	f.doc.objects[objID] = &pdfObject{Num: objID, Value: dict}
	ref := pdfRef{Num: objID}

	f.appendToFields(ref)
	appendWidgetToPage(page.pageObj(), ref)
	f.rebuildFieldCache()
	f.noteFormMutatedInForm()

	return f.cache[name].(*ComboBoxField), nil
}

// choiceOptionsToPDFArray converts a slice of ChoiceOption to a /Opt
// array. Each element is either a single string (Value-only) or a
// two-element array [Export, Value] when Export is non-empty.
func choiceOptionsToPDFArray(options []ChoiceOption) pdfArray {
	arr := make(pdfArray, 0, len(options))
	for _, o := range options {
		if o.Export != "" {
			arr = append(arr, pdfArray{o.Export, o.Value})
		} else {
			arr = append(arr, o.Value)
		}
	}
	return arr
}
```

- [ ] **Step 4: Run tests**

```
go test -run TestFormAddComboBoxRoundTrip -v ./...
go test ./...
```

Both PASS.

- [ ] **Step 5: Commit**

```
git add form.go form_design_test.go
git commit -m "feat: Form.AddComboBox with /Opt array encoding"
```

---

## Task 4: `AddListBox`

**Files:**
- Modify: `form.go`
- Modify: `form_design_test.go`

- [ ] **Step 1: Write failing test**

Append:
```go
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
	doc2, _ := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	lb2 := doc2.Form().Field("color").(*pdf.ListBoxField)
	if got := len(lb2.Options()); got != 3 {
		t.Errorf("Options count = %d, want 3", got)
	}
	sel := lb2.Selected()
	if len(sel) != 1 || sel[0] != 0 {
		t.Errorf("Selected = %v, want [0]", sel)
	}
}
```

- [ ] **Step 2: Confirm failure**

- [ ] **Step 3: Implement `AddListBox`**

Add to `form.go`:
```go
// AddListBox creates a single-select list field. SetMultiSelect(true)
// on the returned handle enables multi-selection (bit 22).
func (f *Form) AddListBox(pageNum int, rect Rectangle, name string, options []ChoiceOption) (*ListBoxField, error) {
	if err := f.validateNewField(pageNum, name); err != nil {
		return nil, err
	}
	page, err := f.doc.Page(pageNum)
	if err != nil {
		return nil, err
	}
	helvName, err := f.ensureFontHelv()
	if err != nil {
		return nil, err
	}

	dict := pdfDict{
		"/Type":    pdfName("/Annot"),
		"/Subtype": pdfName("/Widget"),
		"/FT":      pdfName("/Ch"),
		"/T":       name,
		// /Ff is 0 — neither Combo (bit 18) nor MultiSelect (bit 22) set.
		"/Opt":  choiceOptionsToPDFArray(options),
		"/DA":   "0 g /" + helvName + " 12 Tf",
		"/Rect": rectToPDFArray(rect),
		"/P":    pdfRef{Num: page.pageObj().Num},
	}

	objID := f.doc.nextID
	f.doc.nextID++
	f.doc.objects[objID] = &pdfObject{Num: objID, Value: dict}
	ref := pdfRef{Num: objID}

	f.appendToFields(ref)
	appendWidgetToPage(page.pageObj(), ref)
	f.rebuildFieldCache()
	f.noteFormMutatedInForm()

	return f.cache[name].(*ListBoxField), nil
}
```

- [ ] **Step 4: Run tests**

- [ ] **Step 5: Commit**

```
git add form.go form_design_test.go
git commit -m "feat: Form.AddListBox"
```

---

## Task 5: `AddPushButton`

**Files:**
- Modify: `form.go`
- Modify: `form_design_test.go`

- [ ] **Step 1: Write failing test**

Append:
```go
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
	doc.WriteTo(&buf)
	doc2, _ := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	if pdf.FieldType(doc2.Form().Field("submit")) != pdf.FormFieldTypePushButton {
		t.Error("after roundtrip, type is not PushButton")
	}
}
```

- [ ] **Step 2: Confirm failure**

- [ ] **Step 3: Implement `AddPushButton`**

Add to `form.go`:
```go
// AddPushButton creates a non-toggling button. The caption is stored in
// /MK/CA and rendered by viewers as the button label. Push buttons have
// no value semantics — Value() returns "", SetValue returns an error.
func (f *Form) AddPushButton(pageNum int, rect Rectangle, name string, caption string) (*ButtonField, error) {
	if err := f.validateNewField(pageNum, name); err != nil {
		return nil, err
	}
	page, err := f.doc.Page(pageNum)
	if err != nil {
		return nil, err
	}
	helvName, err := f.ensureFontHelv()
	if err != nil {
		return nil, err
	}

	dict := pdfDict{
		"/Type":    pdfName("/Annot"),
		"/Subtype": pdfName("/Widget"),
		"/FT":      pdfName("/Btn"),
		"/T":       name,
		"/Ff":      fieldFlagPushbutton,
		"/DA":      "0 g /" + helvName + " 12 Tf",
		"/Rect":    rectToPDFArray(rect),
		"/P":       pdfRef{Num: page.pageObj().Num},
		"/MK":      pdfDict{"/CA": caption},
	}

	objID := f.doc.nextID
	f.doc.nextID++
	f.doc.objects[objID] = &pdfObject{Num: objID, Value: dict}
	ref := pdfRef{Num: objID}

	f.appendToFields(ref)
	appendWidgetToPage(page.pageObj(), ref)
	f.rebuildFieldCache()
	f.noteFormMutatedInForm()

	return f.cache[name].(*ButtonField), nil
}
```

- [ ] **Step 4: Run tests**

- [ ] **Step 5: Commit**

```
git add form.go form_design_test.go
git commit -m "feat: Form.AddPushButton with /MK/CA caption"
```

---

## Task 6: `AddRadioGroup` — multi-widget parent + kids

**Files:**
- Modify: `form_fields.go` (add `RadioItem` struct)
- Modify: `form.go`
- Modify: `form_design_test.go`

- [ ] **Step 1: Write failing tests**

Append:
```go
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
	doc.WriteTo(&buf)
	doc2, _ := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
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
	doc.AddBlankPage(595, 842) // page 2
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
	doc.WriteTo(&buf)
	doc2, _ := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	rb2 := doc2.Form().Field("xpage").(*pdf.RadioButtonField)
	if !rb2.Options()[1].Selected() {
		t.Error("opt 1 should be selected after cross-page roundtrip")
	}
}
```

- [ ] **Step 2: Confirm failure**

- [ ] **Step 3: Add `RadioItem` to `form_fields.go`**

```go
// RadioItem describes one widget inside a radio group. PageNum is
// 1-based; Export is the unique export value for this option (becomes
// the /AS state name when selected).
type RadioItem struct {
	PageNum int
	Rect    Rectangle
	Export  string
}
```

- [ ] **Step 4: Implement `AddRadioGroup` in `form.go`**

```go
// AddRadioGroup creates a radio-button parent field plus one widget per
// item. Items may live on different pages. Export values must be
// unique within the group.
func (f *Form) AddRadioGroup(name string, items []RadioItem) (*RadioButtonField, error) {
	if name == "" {
		return nil, fmt.Errorf("form field name is empty")
	}
	if f.HasField(name) {
		return nil, fmt.Errorf("field with name %q already exists", name)
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("radio group %q has no items", name)
	}
	seen := map[string]bool{}
	for i, it := range items {
		if it.Export == "" {
			return nil, fmt.Errorf("radio item %d: empty Export", i)
		}
		if seen[it.Export] {
			return nil, fmt.Errorf("radio item %d: duplicate Export %q", i, it.Export)
		}
		seen[it.Export] = true
		if it.PageNum < 1 || it.PageNum > f.doc.PageCount() {
			return nil, fmt.Errorf("radio item %d: pageNum %d out of range", i, it.PageNum)
		}
	}

	// Allocate parent first.
	parentDict := pdfDict{
		"/FT":   pdfName("/Btn"),
		"/Ff":   fieldFlagRadio,
		"/T":    name,
		"/V":    pdfName("/Off"),
		"/Kids": pdfArray{},
	}
	parentID := f.doc.nextID
	f.doc.nextID++
	f.doc.objects[parentID] = &pdfObject{Num: parentID, Value: parentDict}
	parentRef := pdfRef{Num: parentID}

	for _, it := range items {
		page, err := f.doc.Page(it.PageNum)
		if err != nil {
			return nil, err
		}
		apN := pdfDict{
			"/Off":         placeholderXObjectRef(f.doc),
			"/" + it.Export: placeholderXObjectRef(f.doc),
		}
		widgetDict := pdfDict{
			"/Type":    pdfName("/Annot"),
			"/Subtype": pdfName("/Widget"),
			"/Parent":  parentRef,
			"/Rect":    rectToPDFArray(it.Rect),
			"/P":       pdfRef{Num: page.pageObj().Num},
			"/AS":      pdfName("/Off"),
			"/AP":      pdfDict{"/N": apN},
		}
		widgetID := f.doc.nextID
		f.doc.nextID++
		f.doc.objects[widgetID] = &pdfObject{Num: widgetID, Value: widgetDict}
		widgetRef := pdfRef{Num: widgetID}

		// Append widget ref to parent's /Kids.
		kids, _ := parentDict["/Kids"].(pdfArray)
		kids = append(kids, widgetRef)
		parentDict["/Kids"] = kids

		// Append widget ref to its page's /Annots.
		appendWidgetToPage(page.pageObj(), widgetRef)
	}

	f.appendToFields(parentRef)
	f.rebuildFieldCache()
	f.noteFormMutatedInForm()

	return f.cache[name].(*RadioButtonField), nil
}
```

- [ ] **Step 5: Run tests**

```
go test -run 'TestFormAddRadioGroup' -v ./...
go test ./...
```

Both PASS.

- [ ] **Step 6: Commit**

```
git add form.go form_fields.go form_design_test.go
git commit -m "feat: Form.AddRadioGroup with multi-widget cross-page support"
```

---

## Task 7: Validation tests for AddXxx error paths

**Files:**
- Modify: `form_design_test.go`

- [ ] **Step 1: Write tests**

Append:
```go
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
```

- [ ] **Step 2: Run tests**

```
go test -run 'TestFormAdd.*Error' -v ./...
```

All five PASS (validation is already implemented in tasks 1-6).

- [ ] **Step 3: Commit**

```
git add form_design_test.go
git commit -m "test: validation error paths for Form.AddXxx"
```

---

## Task 8: `Form.RemoveField`

**Files:**
- Modify: `form.go`
- Modify: `form_design_test.go`

- [ ] **Step 1: Write failing tests**

Append:
```go
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
	doc.AddBlankPage(595, 842)
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
	doc.WriteTo(&buf)
	doc2, _ := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	if doc2.Form().HasField("rg") {
		t.Error("HasField returned true after Remove + roundtrip")
	}
}
```

- [ ] **Step 2: Confirm failure**

- [ ] **Step 3: Implement `RemoveField`**

Add to `form.go`:
```go
// RemoveField removes the named field plus all its widget annotations
// from /AcroForm/Fields and from each affected page's /Annots. Returns
// true if the field was found and removed; false otherwise.
//
// After removal, any *Field handles previously returned for this field
// are dangling and must not be used.
func (f *Form) RemoveField(name string) bool {
	if f.cache == nil {
		return false
	}
	field, ok := f.cache[name]
	if !ok {
		return false
	}

	// Find the underlying node so we can collect widget refs.
	var target *fieldNode
	for _, n := range f.leaves {
		if n.fullName == name {
			target = n
			break
		}
	}
	if target == nil {
		return false
	}

	// Collect the set of object IDs to delete:
	//   - the field-object itself (parent for radio, combined dict otherwise)
	//   - every kid widget if separate from the field-object
	idsToRemove := map[int]bool{}
	objectsByDict := map[*pdfDict]int{}
	for id, obj := range f.doc.objects {
		if d, ok := obj.Value.(pdfDict); ok {
			objectsByDict[&d] = id
		}
	}

	// 1. Identify the field-object id by matching dict pointer.
	for id, obj := range f.doc.objects {
		if d, ok := obj.Value.(pdfDict); ok {
			if isSameDict(d, target.dict) {
				idsToRemove[id] = true
				break
			}
		}
	}

	// 2. Identify widget object IDs for each widget in target.widgets.
	for _, w := range target.widgets {
		for id, obj := range f.doc.objects {
			if d, ok := obj.Value.(pdfDict); ok {
				if isSameDict(d, w) {
					idsToRemove[id] = true
				}
			}
		}
	}

	// 3. Splice removed refs out of /AcroForm/Fields.
	if arr, ok := f.root["/Fields"].(pdfArray); ok {
		newArr := make(pdfArray, 0, len(arr))
		for _, item := range arr {
			if ref, ok := item.(pdfRef); ok && idsToRemove[ref.Num] {
				continue
			}
			newArr = append(newArr, item)
		}
		f.root["/Fields"] = newArr
	}

	// 4. Splice removed refs out of every page's /Annots.
	for _, p := range f.doc.pages {
		pageDict, _ := p.Value.(pdfDict)
		if pageDict == nil {
			continue
		}
		annots, _ := pageDict["/Annots"].(pdfArray)
		if len(annots) == 0 {
			continue
		}
		newAnnots := make(pdfArray, 0, len(annots))
		for _, a := range annots {
			if ref, ok := a.(pdfRef); ok && idsToRemove[ref.Num] {
				continue
			}
			newAnnots = append(newAnnots, a)
		}
		pageDict["/Annots"] = newAnnots
	}

	// 5. Delete the objects.
	for id := range idsToRemove {
		delete(f.doc.objects, id)
	}

	// 6. Rebuild caches.
	f.rebuildFieldCache()
	f.noteFormMutatedInForm()
	_ = field
	return true
}

// isSameDict reports whether two pdfDict references point to the same
// underlying map. Used during removal to map back from a known dict to
// its object id.
func isSameDict(a, b pdfDict) bool {
	if len(a) != len(b) {
		return false
	}
	// Quick identity probe: compare a sentinel pointer derived from the
	// map. In Go, two distinct maps never compare equal, but the same
	// map compared to itself does. We exploit this by checking address
	// via reflect.
	return fmt.Sprintf("%p", a) == fmt.Sprintf("%p", b)
}
```

- [ ] **Step 4: Run tests**

```
go test -run 'TestFormRemoveField' -v ./...
go test ./...
```

All PASS.

- [ ] **Step 5: Commit**

```
git add form.go form_design_test.go
git commit -m "feat: Form.RemoveField with multi-widget cascade"
```

---

## Task 9: TextBox structural mutators (`SetMaxLen`, `SetMultiline`, `SetPassword`, `SetReadOnly`, `SetRequired`)

**Files:**
- Modify: `form_fields.go`
- Modify: `form_design_test.go`

- [ ] **Step 1: Write failing tests**

Append:
```go
func TestTextBoxFieldSetMaxLenRoundTrip(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	tf, _ := doc.Form().AddTextField(1, pdf.Rectangle{LLX: 50, LLY: 700, URX: 545, URY: 730}, "x")
	tf.SetMaxLen(100)
	var buf bytes.Buffer
	doc.WriteTo(&buf)
	doc2, _ := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	tf2 := doc2.Form().Field("x").(*pdf.TextBoxField)
	if got := tf2.MaxLen(); got != 100 {
		t.Errorf("MaxLen = %d, want 100", got)
	}
}

func TestTextBoxFieldSetMultilineRoundTrip(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	tf, _ := doc.Form().AddTextField(1, pdf.Rectangle{LLX: 50, LLY: 700, URX: 545, URY: 730}, "x")
	tf.SetMultiline(true)
	var buf bytes.Buffer
	doc.WriteTo(&buf)
	doc2, _ := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	tf2 := doc2.Form().Field("x").(*pdf.TextBoxField)
	if !tf2.IsMultiline() {
		t.Error("IsMultiline = false after SetMultiline(true) + roundtrip")
	}
}

func TestTextBoxFieldSetPasswordRoundTrip(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	tf, _ := doc.Form().AddTextField(1, pdf.Rectangle{LLX: 50, LLY: 700, URX: 545, URY: 730}, "x")
	tf.SetPassword(true)
	var buf bytes.Buffer
	doc.WriteTo(&buf)
	doc2, _ := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	tf2 := doc2.Form().Field("x").(*pdf.TextBoxField)
	if !tf2.IsPassword() {
		t.Error("IsPassword = false after SetPassword(true)")
	}
}

func TestFieldSetReadOnlyRequiredRoundTrip(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	tf, _ := doc.Form().AddTextField(1, pdf.Rectangle{LLX: 50, LLY: 700, URX: 545, URY: 730}, "x")
	tf.SetReadOnly(true)
	tf.SetRequired(true)
	var buf bytes.Buffer
	doc.WriteTo(&buf)
	doc2, _ := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	tf2 := doc2.Form().Field("x").(*pdf.TextBoxField)
	if !tf2.IsReadOnly() {
		t.Error("IsReadOnly = false after SetReadOnly(true)")
	}
	if !tf2.IsRequired() {
		t.Error("IsRequired = false after SetRequired(true)")
	}
}
```

- [ ] **Step 2: Confirm failure**

- [ ] **Step 3: Implement structural mutators on `TextBoxField` and `fieldBase`**

In `form_fields.go`, add a helper for /Ff bit manipulation:
```go
// setFlag sets or clears a /Ff bit on the field's dict. Updates both
// node.ff (the cached value) and dict["/Ff"] (the persisted value).
func setFlag(node *fieldNode, bit int, on bool) {
	if on {
		node.ff |= bit
	} else {
		node.ff &^= bit
	}
	if node.ff == 0 {
		delete(node.dict, "/Ff")
	} else {
		node.dict["/Ff"] = node.ff
	}
	if node.form != nil {
		node.form.noteFormMutatedInForm()
	}
}
```

Then add the per-type methods:
```go
func (f *TextBoxField) SetReadOnly(v bool) { setFlag(f.node, fieldFlagReadOnly, v) }
func (f *TextBoxField) SetRequired(v bool) { setFlag(f.node, fieldFlagRequired, v) }
func (f *TextBoxField) SetMultiline(v bool) { setFlag(f.node, fieldFlagMultiline, v) }
func (f *TextBoxField) SetPassword(v bool)  { setFlag(f.node, fieldFlagPassword, v) }

// SetMaxLen sets the maximum number of characters. 0 removes the limit.
func (f *TextBoxField) SetMaxLen(n int) {
	if n <= 0 {
		delete(f.node.dict, "/MaxLen")
	} else {
		f.node.dict["/MaxLen"] = n
	}
	if f.node.form != nil {
		f.node.form.noteFormMutatedInForm()
	}
}

func (f *CheckboxField) SetReadOnly(v bool)    { setFlag(f.node, fieldFlagReadOnly, v) }
func (f *CheckboxField) SetRequired(v bool)    { setFlag(f.node, fieldFlagRequired, v) }
func (f *RadioButtonField) SetReadOnly(v bool) { setFlag(f.node, fieldFlagReadOnly, v) }
func (f *RadioButtonField) SetRequired(v bool) { setFlag(f.node, fieldFlagRequired, v) }
func (f *ButtonField) SetReadOnly(v bool)      { setFlag(f.node, fieldFlagReadOnly, v) }
```

- [ ] **Step 4: Run tests**

```
go test -run 'TestTextBoxFieldSet|TestFieldSet' -v ./...
go test ./...
```

All PASS.

- [ ] **Step 5: Commit**

```
git add form_fields.go form_design_test.go
git commit -m "feat: structural mutators on TextBox/Checkbox/Radio/Button (Set{ReadOnly,Required,MaxLen,Multiline,Password})"
```

---

## Task 10: Choice-type mutators (`SetEditable`, `SetMultiSelect`, `AddOption`, `RemoveOption`)

**Files:**
- Modify: `form_fields.go`
- Modify: `form_design_test.go`

- [ ] **Step 1: Write failing tests**

Append:
```go
func TestComboBoxFieldSetEditableRoundTrip(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	cb, _ := doc.Form().AddComboBox(1, pdf.Rectangle{LLX: 50, LLY: 600, URX: 250, URY: 625}, "x", []pdf.ChoiceOption{{Value: "a"}})
	cb.SetEditable(true)
	if err := cb.SetValue("free text"); err != nil {
		t.Errorf("editable combo SetValue failed: %v", err)
	}
}

func TestComboBoxFieldAddRemoveOptionRoundTrip(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	cb, _ := doc.Form().AddComboBox(1, pdf.Rectangle{LLX: 50, LLY: 600, URX: 250, URY: 625}, "x", []pdf.ChoiceOption{{Value: "a"}, {Value: "b"}})
	cb.AddOption(pdf.ChoiceOption{Value: "c"})
	if err := cb.RemoveOption(0); err != nil {
		t.Fatalf("RemoveOption: %v", err)
	}
	opts := cb.Options()
	if len(opts) != 2 || opts[0].Value != "b" || opts[1].Value != "c" {
		t.Errorf("Options after Add+Remove = %v, want [{b} {c}]", opts)
	}
}

func TestListBoxFieldSetMultiSelectRoundTrip(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	lb, _ := doc.Form().AddListBox(1, pdf.Rectangle{LLX: 50, LLY: 500, URX: 250, URY: 580}, "x", []pdf.ChoiceOption{{Value: "a"}, {Value: "b"}, {Value: "c"}})
	lb.SetMultiSelect(true)
	if err := lb.SetSelected(0, 2); err != nil {
		t.Fatalf("SetSelected multi after enabling multi: %v", err)
	}
	var buf bytes.Buffer
	doc.WriteTo(&buf)
	doc2, _ := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	lb2 := doc2.Form().Field("x").(*pdf.ListBoxField)
	if !lb2.MultiSelect() {
		t.Error("MultiSelect false after SetMultiSelect(true) + roundtrip")
	}
	sel := lb2.Selected()
	if len(sel) != 2 {
		t.Errorf("Selected count = %d, want 2", len(sel))
	}
}
```

- [ ] **Step 2: Confirm failure**

- [ ] **Step 3: Implement choice mutators**

Append to `form_fields.go`:
```go
// SetEditable toggles bit 19 (/Ff Edit). When true, the combo accepts
// arbitrary user-typed text instead of restricting to /Opt entries.
func (f *ComboBoxField) SetEditable(v bool) { setFlag(f.node, fieldFlagEdit, v) }

// AddOption appends a ChoiceOption to /Opt.
func (f *ComboBoxField) AddOption(o ChoiceOption) {
	arr := f.node.dict["/Opt"]
	pdfArr, _ := arr.(pdfArray)
	pdfArr = append(pdfArr, choiceOptionToPDFValue(o))
	f.node.dict["/Opt"] = pdfArr
	if f.node.form != nil {
		f.node.form.noteFormMutatedInForm()
	}
}

// RemoveOption removes the option at index. Errors on out-of-range.
func (f *ComboBoxField) RemoveOption(index int) error {
	pdfArr, _ := f.node.dict["/Opt"].(pdfArray)
	if index < 0 || index >= len(pdfArr) {
		return fmt.Errorf("ComboBoxField.RemoveOption(%d): out of range [0,%d)", index, len(pdfArr))
	}
	f.node.dict["/Opt"] = append(pdfArr[:index], pdfArr[index+1:]...)
	if f.node.form != nil {
		f.node.form.noteFormMutatedInForm()
	}
	return nil
}

// SetMultiSelect toggles bit 22 (/Ff MultiSelect) on a ListBoxField.
func (f *ListBoxField) SetMultiSelect(v bool) { setFlag(f.node, fieldFlagMultiSelect, v) }

func (f *ListBoxField) AddOption(o ChoiceOption) {
	arr := f.node.dict["/Opt"]
	pdfArr, _ := arr.(pdfArray)
	pdfArr = append(pdfArr, choiceOptionToPDFValue(o))
	f.node.dict["/Opt"] = pdfArr
	if f.node.form != nil {
		f.node.form.noteFormMutatedInForm()
	}
}

func (f *ListBoxField) RemoveOption(index int) error {
	pdfArr, _ := f.node.dict["/Opt"].(pdfArray)
	if index < 0 || index >= len(pdfArr) {
		return fmt.Errorf("ListBoxField.RemoveOption(%d): out of range [0,%d)", index, len(pdfArr))
	}
	f.node.dict["/Opt"] = append(pdfArr[:index], pdfArr[index+1:]...)
	if f.node.form != nil {
		f.node.form.noteFormMutatedInForm()
	}
	return nil
}

// choiceOptionToPDFValue is the per-option converter. Mirrors what
// choiceOptionsToPDFArray does for an entire slice.
func choiceOptionToPDFValue(o ChoiceOption) pdfValue {
	if o.Export != "" {
		return pdfArray{o.Export, o.Value}
	}
	return o.Value
}

// SetReadOnly/SetRequired on choice types.
func (f *ComboBoxField) SetReadOnly(v bool) { setFlag(f.node, fieldFlagReadOnly, v) }
func (f *ComboBoxField) SetRequired(v bool) { setFlag(f.node, fieldFlagRequired, v) }
func (f *ListBoxField) SetReadOnly(v bool)  { setFlag(f.node, fieldFlagReadOnly, v) }
func (f *ListBoxField) SetRequired(v bool)  { setFlag(f.node, fieldFlagRequired, v) }
```

- [ ] **Step 4: Run tests**

```
go test -run 'TestComboBoxField|TestListBoxField' -v ./...
go test ./...
```

All PASS.

- [ ] **Step 5: Commit**

```
git add form_fields.go form_design_test.go
git commit -m "feat: choice-type mutators (SetEditable/SetMultiSelect/AddOption/RemoveOption)"
```

---

## Task 11: `/NeedAppearances` regression + multi-type integration

**Files:**
- Modify: `form_design_test.go`

- [ ] **Step 1: Write tests**

Append:
```go
func TestFormAddXxxAutoSetsNeedAppearances(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	if _, err := doc.Form().AddTextField(1, pdf.Rectangle{LLX: 50, LLY: 700, URX: 545, URY: 730}, "x"); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if !doc.Form().NeedAppearances() {
		t.Error("/NeedAppearances not auto-set after AddTextField")
	}
}

func TestFormBuildFromScratchIntegration(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	form := doc.Form()

	tf, _ := form.AddTextField(1, pdf.Rectangle{LLX: 50, LLY: 720, URX: 545, URY: 745}, "name")
	tf.SetValue("Jane Doe")
	tf.SetMaxLen(50)

	cb, _ := form.AddCheckbox(1, pdf.Rectangle{LLX: 50, LLY: 685, URX: 70, URY: 705}, "subscribe")
	cb.SetChecked(true)

	rb, _ := form.AddRadioGroup("plan", []pdf.RadioItem{
		{PageNum: 1, Rect: pdf.Rectangle{LLX: 50, LLY: 645, URX: 70, URY: 665}, Export: "basic"},
		{PageNum: 1, Rect: pdf.Rectangle{LLX: 50, LLY: 615, URX: 70, URY: 635}, Export: "premium"},
	})
	rb.Options()[1].SetSelected(true)

	combo, _ := form.AddComboBox(1, pdf.Rectangle{LLX: 50, LLY: 575, URX: 250, URY: 600}, "country",
		[]pdf.ChoiceOption{{Value: "USA"}, {Value: "Canada"}})
	combo.SetSelected(0)

	list, _ := form.AddListBox(1, pdf.Rectangle{LLX: 50, LLY: 480, URX: 250, URY: 565}, "color",
		[]pdf.ChoiceOption{{Value: "Red"}, {Value: "Green"}, {Value: "Blue"}})
	list.SetSelected(2)

	form.AddPushButton(1, pdf.Rectangle{LLX: 50, LLY: 430, URX: 200, URY: 460}, "submit", "Submit")

	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatalf("WriteTo: %v", err)
	}
	doc2, err := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("Reopen: %v", err)
	}

	form2 := doc2.Form()
	if got := len(form2.Fields()); got != 6 {
		t.Errorf("Fields count = %d, want 6", got)
	}
	if got := form2.Field("name").(*pdf.TextBoxField).Value(); got != "Jane Doe" {
		t.Errorf("name = %q, want 'Jane Doe'", got)
	}
	if !form2.Field("subscribe").(*pdf.CheckboxField).Checked() {
		t.Error("subscribe not checked")
	}
	if !form2.Field("plan").(*pdf.RadioButtonField).Options()[1].Selected() {
		t.Error("plan opt 1 not selected")
	}
	if got := form2.Field("country").(*pdf.ComboBoxField).Selected(); got != 0 {
		t.Errorf("country selected = %d, want 0", got)
	}
	sel := form2.Field("color").(*pdf.ListBoxField).Selected()
	if len(sel) != 1 || sel[0] != 2 {
		t.Errorf("color selected = %v, want [2]", sel)
	}
	if pdf.FieldType(form2.Field("submit")) != pdf.FormFieldTypePushButton {
		t.Error("submit not PushButton type")
	}
}
```

- [ ] **Step 2: Run tests**

```
go test -run 'TestFormAddXxxAutoSetsNeedAppearances|TestFormBuildFromScratchIntegration' -v ./...
go test ./...
```

Both PASS.

- [ ] **Step 3: Commit**

```
git add form_design_test.go
git commit -m "test: integration build-from-scratch + /NeedAppearances regression"
```

---

## Task 12: pypdf cross-verification + docs

**Files:**
- Modify: `CLAUDE.md`
- Modify: `README.md`

- [ ] **Step 1: pypdf cross-check (manual)**

Write `D:/tmp/check_form_design.go`:
```go
package main

import (
	"log"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

func main() {
	doc := pdf.NewDocument(595, 842)
	form := doc.Form()
	tf, _ := form.AddTextField(1, pdf.Rectangle{LLX: 50, LLY: 720, URX: 545, URY: 745}, "name")
	tf.SetValue("Jane Doe")
	cb, _ := form.AddCheckbox(1, pdf.Rectangle{LLX: 50, LLY: 685, URX: 70, URY: 705}, "subscribe")
	cb.SetChecked(true)
	if err := doc.Save("D:/tmp/form_built.pdf"); err != nil {
		log.Fatal(err)
	}
}
```

Plus minimal `D:/tmp/check_form_design/go.mod` with replace directive (same pattern as Read+Fill epic Task 10).

Run:
```
cd D:/tmp/check_form_design && go run main.go
python -c "from pypdf import PdfReader; r = PdfReader('D:/tmp/form_built.pdf'); print(r.get_form_text_fields()); print(r.get_fields().keys())"
```

Expected: `{'name': 'Jane Doe'}` and the keys list should include both `'name'` and `'subscribe'`.

If pypdf doesn't see the fields, STOP — report BLOCKED.

Cleanup: `rm -rf D:/tmp/check_form_design D:/tmp/form_built.pdf`.

- [ ] **Step 2: Update CLAUDE.md**

Find the `**`form.go` / `form_fields.go`**` section. Append after the existing bullet about `/NeedAppearances`:

```markdown
- `(*Form).AddTextField/AddCheckbox/AddRadioGroup/AddComboBox/AddListBox/AddPushButton` — programmatic field creation; auto-creates /AcroForm and /AcroForm/DR/Font/Helv on first call; combined field+widget dict for single-widget fields, parent + kids for radio groups
- `(*Form).RemoveField(name) bool` — removes field plus all its widgets from /AcroForm/Fields and per-page /Annots
- Per-type structural mutators: `SetReadOnly`, `SetRequired` on every type; `TextBoxField.{SetMaxLen,SetMultiline,SetPassword}`; `ComboBoxField.{SetEditable,AddOption,RemoveOption}`; `ListBoxField.{SetMultiSelect,AddOption,RemoveOption}`
- `RadioItem` struct — `PageNum`, `Rect`, `Export` for cross-page radio groups
```

- [ ] **Step 3: Update README.md**

In the "Forms (AcroForm)" section, add a "Building forms from scratch" subsection after the existing fill example:

````markdown
#### Building forms from scratch

```go
doc := pdf.NewDocument(595, 842)
form := doc.Form()

// Single-widget fields
tf, _ := form.AddTextField(1, pdf.Rectangle{LLX: 50, LLY: 700, URX: 545, URY: 725}, "name")
tf.SetMaxLen(50)
tf.SetValue("Jane Doe")

cb, _ := form.AddCheckbox(1, pdf.Rectangle{LLX: 50, LLY: 660, URX: 70, URY: 680}, "subscribe")
cb.SetChecked(true)

combo, _ := form.AddComboBox(1, pdf.Rectangle{LLX: 50, LLY: 600, URX: 250, URY: 625}, "country",
    []pdf.ChoiceOption{{Value: "USA"}, {Value: "Canada"}})
combo.SetSelected(0)

// Radio group: widgets can span multiple pages
rb, _ := form.AddRadioGroup("plan", []pdf.RadioItem{
    {PageNum: 1, Rect: pdf.Rectangle{LLX: 50, LLY: 540, URX: 70, URY: 560}, Export: "basic"},
    {PageNum: 1, Rect: pdf.Rectangle{LLX: 50, LLY: 510, URX: 70, URY: 530}, Export: "premium"},
})
rb.Options()[0].SetSelected(true)

form.AddPushButton(1, pdf.Rectangle{LLX: 50, LLY: 460, URX: 200, URY: 490}, "submit", "Submit")

// Remove a field by name
form.RemoveField("subscribe")

doc.Save("form.pdf")
```

`/AcroForm/NeedAppearances` is auto-set on every Add or structural mutation, so any standards-compliant viewer regenerates the field appearances at display time.
````

- [ ] **Step 4: Verify README compiles**

Extract the new code block into `D:/tmp/readme_form_design_smoke/main.go` (wrap with package main + imports), plus `go.mod` with replace. Run `go build ./...`. Cleanup.

- [ ] **Step 5: Run full suite**

```
go test ./...
```

PASS.

- [ ] **Step 6: Commit**

```
git add CLAUDE.md README.md
git commit -m "docs: AcroForm form-design CRUD in CLAUDE.md and README"
```

- [ ] **Step 7: Close the bd issue**

```
bd update pdf-go-4df --status closed --append-notes "Closed after form-design CRUD implementation: 12-task plan executed via subagent-driven-development. AddTextField/Checkbox/RadioGroup/ComboBox/ListBox/PushButton, RemoveField, structural mutators per type, AddOption/RemoveOption on choice types. pypdf cross-verified."
```

---

## Self-review

**Spec coverage:** every section of the spec has at least one task.

| Spec section | Tasks |
|---|---|
| Form.AddTextField | 1 |
| Form.AddCheckbox | 2 |
| Form.AddComboBox | 3 |
| Form.AddListBox | 4 |
| Form.AddPushButton | 5 |
| Form.AddRadioGroup + RadioItem | 6 |
| Validation (duplicate name, invalid pageNum, empty name, empty radio items, duplicate export) | 7 |
| Form.RemoveField | 8 |
| ensureFontHelv, default /DA, widget+page coordination | 1 (introduced) |
| TextBox structural mutators | 9 |
| Checkbox/Radio/Button SetReadOnly/SetRequired | 9 |
| ComboBox/ListBox structural mutators + AddOption/RemoveOption | 10 |
| /NeedAppearances auto-set | 11 |
| Build-from-scratch integration | 11 |
| Docs (CLAUDE.md, README.md) | 12 |
| pypdf cross-check | 12 |

No gaps.

**Placeholder scan:** searched for "TBD", "TODO", "implement later", "appropriate error handling" — none. Every task has full code blocks. Tests are spelled out. Validation rules are explicit.

**Type consistency:** `RadioItem` introduced in Task 6 with fields `PageNum int`, `Rect Rectangle`, `Export string` — used identically in Tasks 7 (validation), 8 (radio cascade), 11 (integration), 12 (README). `ChoiceOption.Value`/`.Export` from `pdf-go-re2` carries through. Method signatures `SetReadOnly(bool)`, `SetMaxLen(int)`, `AddOption(ChoiceOption)`, `RemoveOption(int) error` are consistent across types and tasks.

**One known plan-vs-reality unknown:** the `isSameDict` helper at the end of Task 8 uses `fmt.Sprintf("%p", a)` to compare map identities. This is stylistically ugly but functionally correct in Go (maps are reference types; the same map literal compared to itself produces the same `%p`). The implementer may choose a cleaner approach (e.g. tracking `(node, objID)` pairs at parse time so we don't need to map dict→id at remove time). If the implementer goes that route, document the change in their report.
