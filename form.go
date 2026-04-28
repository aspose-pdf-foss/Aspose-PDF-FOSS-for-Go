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
