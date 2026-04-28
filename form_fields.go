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

