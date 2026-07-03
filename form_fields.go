// SPDX-License-Identifier: MIT

package asposepdf

import (
	"fmt"
	"sort"
)

// widgetOnStateName returns the export name of a widget's "on" appearance
// state — the first key under /AP/N that is not /Off, with the leading
// slash stripped. Returns "" if no /AP/N dict or no non-/Off key. Map
// iteration order is unspecified; for widgets with multiple non-/Off
// keys (rare/malformed), the result is non-deterministic.
func widgetOnStateName(w pdfDict) string {
	ap, ok := w["/AP"].(pdfDict)
	if !ok {
		return ""
	}
	n, ok := ap["/N"].(pdfDict)
	if !ok {
		return ""
	}
	for k := range n {
		if k != "/Off" {
			return k[1:]
		}
	}
	return ""
}

// RadioItem describes one widget inside a radio group. PageNum is
// 1-based; Export is the unique export value for this option (becomes
// the /AS state name when selected).
type RadioItem struct {
	PageNum int
	Rect    Rectangle
	Export  string
}

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
	FormFieldTypePassword
	FormFieldTypeFileSelect
	FormFieldTypeRichText
	FormFieldTypeNumber
	FormFieldTypeDate
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
		return FormFieldTypeComboBox
	case *ButtonField:
		return FormFieldTypePushButton
	case *ListBoxField:
		return FormFieldTypeListBox
	case *PasswordBoxField:
		return FormFieldTypePassword
	case *FileSelectBoxField:
		return FormFieldTypeFileSelect
	case *RichTextBoxField:
		return FormFieldTypeRichText
	case *NumberField:
		return FormFieldTypeNumber
	case *DateField:
		return FormFieldTypeDate
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
	fieldFlagReadOnly    = 1 << 0  // bit 1
	fieldFlagRequired    = 1 << 1  // bit 2
	fieldFlagPushbutton  = 1 << 16 // bit 17
	fieldFlagRadio       = 1 << 15 // bit 16
	fieldFlagCombo       = 1 << 17 // bit 18
	fieldFlagEdit        = 1 << 18 // bit 19; /Ch combo "Edit" flag
	fieldFlagMultiSelect = 1 << 21 // bit 22
	fieldFlagMultiline   = 1 << 12 // bit 13
	fieldFlagPassword    = 1 << 13 // bit 14
)

// TextBoxField is a single- or multi-line text input.
type TextBoxField struct{ fieldBase }

func (f *TextBoxField) Value() string {
	return decodeFormString(f.node.dict["/V"])
}

func (f *TextBoxField) SetValue(s string) error {
	f.node.dict["/V"] = encodeFormString(s)
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

// CheckboxField is a checkbox with on/off state.
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
	return fmt.Errorf("CheckboxField.SetValue(%q): expected boolean string", s)
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
		if name := widgetOnStateName(w); name != "" {
			return name
		}
	}
	return "Yes"
}

// RadioButtonField is a group of mutually exclusive options.
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
	return fmt.Errorf("RadioButtonField.SetValue(%q): no such option", s)
}

// Options returns one RadioButtonOptionField per widget in the group.
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
	if name := widgetOnStateName(o.widget); name != "" {
		return name
	}
	if as, ok := o.widget["/AS"].(pdfName); ok && as != "/Off" {
		return string(as)[1:]
	}
	return ""
}

// Selected reports whether this option is the currently selected one.
func (o *RadioButtonOptionField) Selected() bool {
	parentV := dictGetString(o.parent.node.dict, "/V")
	want := "/" + o.Name()
	return parentV == want
}

// SetSelected selects this option and clears all siblings (true), or
// clears the selection if this option is currently
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

// ChoiceOption is one option of a ComboBoxField or ListBoxField.
type ChoiceOption struct {
	Value  string // displayed text
	Export string // export value when distinct from Value
}

// ComboBoxField is a single-select dropdown choice field.
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
		// Edit mode: arbitrary text is allowed. Clear any stale /I — the
		// typed value doesn't correspond to an /Opt index.
		f.node.dict["/V"] = encodeFormString(s)
		delete(f.node.dict, "/I")
		noteFormMutated(f.node)
		return nil
	}
	return fmt.Errorf("ComboBoxField.SetValue(%q): no matching option and field is not editable", s)
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
		return fmt.Errorf("ComboBoxField.SetSelected(%d): out of range [0,%d)", index, len(opts))
	}
	value := opts[index].Value
	if opts[index].Export != "" {
		value = opts[index].Export
	}
	f.node.dict["/V"] = encodeFormString(value)
	// /I records the selected /Opt index so interactive viewers
	// reconstruct selection state consistently with our pre-generated /AP.
	f.node.dict["/I"] = pdfArray{index}
	noteFormMutated(f.node)
	return nil
}

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

// ListBoxField is a single- or multi-select list choice field.
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
	return fmt.Errorf("ListBoxField.SetValue(%q): no matching option", s)
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
			return fmt.Errorf("ListBoxField.SetSelected: index %d out of range [0,%d)", idx, len(opts))
		}
	}
	if len(indices) > 1 && !f.MultiSelect() {
		return fmt.Errorf("ListBoxField.SetSelected: %d indices given but field is not MultiSelect", len(indices))
	}
	// Work on a sorted ascending copy so both /V (array form) and /I are
	// emitted in index order, as required for /I by ISO 32000-1 §12.7.4.4.
	sorted := append([]int(nil), indices...)
	sort.Ints(sorted)

	switch len(sorted) {
	case 0:
		delete(f.node.dict, "/V")
	case 1:
		opt := opts[sorted[0]]
		value := opt.Value
		if opt.Export != "" {
			value = opt.Export
		}
		f.node.dict["/V"] = encodeFormString(value)
	default:
		arr := make(pdfArray, 0, len(sorted))
		for _, idx := range sorted {
			opt := opts[idx]
			v := opt.Value
			if opt.Export != "" {
				v = opt.Export
			}
			arr = append(arr, encodeFormString(v))
		}
		f.node.dict["/V"] = arr
	}

	// /I (selected indices) — required for multi-select list boxes and
	// honoured by interactive viewers to reconstruct the exact selection
	// and scroll state. Without it a viewer in edit mode regenerates its
	// own list layout, which doesn't line up with our pre-generated /AP
	// and ghosts the option text at a second set of Y positions. Sorted
	// ascending per spec; cleared when there is no selection.
	if len(sorted) == 0 {
		delete(f.node.dict, "/I")
	} else {
		iArr := make(pdfArray, len(sorted))
		for i, idx := range sorted {
			iArr[i] = idx
		}
		f.node.dict["/I"] = iArr
	}

	noteFormMutated(f.node)
	return nil
}

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

func (f *TextBoxField) SetReadOnly(v bool)  { setFlag(f.node, fieldFlagReadOnly, v) }
func (f *TextBoxField) SetRequired(v bool)  { setFlag(f.node, fieldFlagRequired, v) }
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

// ButtonField is a push button — action only, no value semantics.
type ButtonField struct{ fieldBase }

func (f *ButtonField) Value() string           { return "" }
func (f *ButtonField) SetValue(s string) error { return errPushButtonHasNoValue }

var errPushButtonHasNoValue = fmt.Errorf("push button field has no value")

// SetEditable toggles bit 19 (/Ff Edit). When true, the combo accepts
// arbitrary user-typed text instead of restricting to /Opt entries.
func (f *ComboBoxField) SetEditable(v bool) { setFlag(f.node, fieldFlagEdit, v) }

// addChoiceOption appends an option to /Opt on the given field node.
func addChoiceOption(node *fieldNode, o ChoiceOption) {
	arr, _ := node.dict["/Opt"].(pdfArray)
	arr = append(arr, choiceOptionToPDFValue(o))
	node.dict["/Opt"] = arr
	if node.form != nil {
		node.form.noteFormMutatedInForm()
	}
}

// removeChoiceOption removes the option at index. Errors on out-of-range.
func removeChoiceOption(typeName string, node *fieldNode, index int) error {
	arr, _ := node.dict["/Opt"].(pdfArray)
	if index < 0 || index >= len(arr) {
		return fmt.Errorf("%s.RemoveOption(%d): out of range [0,%d)", typeName, index, len(arr))
	}
	node.dict["/Opt"] = append(arr[:index], arr[index+1:]...)
	if node.form != nil {
		node.form.noteFormMutatedInForm()
	}
	return nil
}

// AddOption appends a ChoiceOption to /Opt.
func (f *ComboBoxField) AddOption(o ChoiceOption) { addChoiceOption(f.node, o) }

// RemoveOption removes the option at index. Errors on out-of-range.
func (f *ComboBoxField) RemoveOption(index int) error {
	return removeChoiceOption("ComboBoxField", f.node, index)
}

// SetMultiSelect toggles bit 22 (/Ff MultiSelect) on a ListBoxField.
func (f *ListBoxField) SetMultiSelect(v bool) { setFlag(f.node, fieldFlagMultiSelect, v) }

// AddOption appends a ChoiceOption to /Opt.
func (f *ListBoxField) AddOption(o ChoiceOption) { addChoiceOption(f.node, o) }

// RemoveOption removes the option at index. Errors on out-of-range.
func (f *ListBoxField) RemoveOption(index int) error {
	return removeChoiceOption("ListBoxField", f.node, index)
}

// choiceOptionToPDFValue is the per-option converter. Mirrors what
// choiceOptionsToPDFArray does for an entire slice.
func choiceOptionToPDFValue(o ChoiceOption) pdfValue {
	if o.Export != "" {
		return pdfArray{o.Export, o.Value}
	}
	return o.Value
}

// SetReadOnly sets the read-only flag; the choice types share the same
// setFlag-based mutators as the text field.
func (f *ComboBoxField) SetReadOnly(v bool) { setFlag(f.node, fieldFlagReadOnly, v) }
func (f *ComboBoxField) SetRequired(v bool) { setFlag(f.node, fieldFlagRequired, v) }
func (f *ListBoxField) SetReadOnly(v bool)  { setFlag(f.node, fieldFlagReadOnly, v) }
func (f *ListBoxField) SetRequired(v bool)  { setFlag(f.node, fieldFlagRequired, v) }
