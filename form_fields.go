package asposepdf

import "fmt"

// FormFieldType identifies the kind of form field. Returned by FieldType().
type FormFieldType int

const (
	FormFieldTypeUnknown     FormFieldType = iota
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
		return FormFieldTypeComboBox
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
	fieldFlagReadOnly   = 1 << 0  // bit 1
	fieldFlagRequired   = 1 << 1  // bit 2
	fieldFlagPushbutton = 1 << 16 // bit 17
	fieldFlagRadio      = 1 << 15 // bit 16
	fieldFlagCombo      = 1 << 17 // bit 18
	fieldFlagMultiSelect = 1 << 21 // bit 22
	fieldFlagMultiline  = 1 << 12 // bit 13
	fieldFlagPassword   = 1 << 13 // bit 14
)

// TextBoxField is a single- or multi-line text input.
type TextBoxField struct{ fieldBase }

func (f *TextBoxField) Value() string           { return dictGetString(f.node.dict, "/V") }
func (f *TextBoxField) SetValue(s string) error { return notYetImpl("TextBoxField.SetValue") }

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

var errPushButtonHasNoValue = fmt.Errorf("push button field has no value")

func notYetImpl(name string) error {
	return fmt.Errorf("%s: not yet implemented", name)
}

