package asposepdf

// Form is the document's AcroForm view. Always non-nil — for documents
// without an /AcroForm dict, Form is empty (no fields, no flags). Field
// instances returned from Form are live handles over the underlying
// pdfDict; SetValue mutates in place and the next Save writes the new
// state.
type Form struct {
	doc        *Document
	root       pdfDict // resolved /AcroForm dict; nil if document has none
	leaves     []*fieldNode
	cache      map[string]Field
	fieldsList []Field
}

// fieldNode is the internal flat representation of a leaf form field.
// It carries the field's own dict, computed FullName, resolved inherited
// attributes (/FT, /Ff, /V, /DV, /DA), and references to its widget
// kids (or itself if the field is also its own widget).
type fieldNode struct {
	dict     pdfDict
	fullName string
	ft       string    // resolved /FT
	ff       int       // resolved /Ff
	widgets  []pdfDict
}

// Form returns the document's AcroForm. Always non-nil; for a document
// without /AcroForm, an empty Form is returned (Fields() is empty,
// Field(name) returns nil, HasField returns false).
func (d *Document) Form() *Form {
	form := &Form{doc: d}
	if d.catalog == nil {
		return form
	}
	root, ok := resolveRefToDict(d.objects, d.catalog["/AcroForm"])
	if !ok {
		return form
	}
	form.root = root
	form.leaves = walkAcroForm(d.objects, root)
	// Build canonical Field instances once so Field(), Fields(), and
	// HasField() all share the same pointers. SetValue in later tasks
	// mutates node.dict in place, so callers must see the same instance.
	form.cache = make(map[string]Field, len(form.leaves))
	form.fieldsList = make([]Field, 0, len(form.leaves))
	for _, n := range form.leaves {
		f := fieldFromNode(n)
		if f == nil {
			continue
		}
		form.fieldsList = append(form.fieldsList, f)
		form.cache[n.fullName] = f
	}
	return form
}

// Fields returns all leaf form fields as a flat slice. Field tree
// hierarchy is resolved internally; callers see only the leaves whose
// FullName carries the dotted path.
func (f *Form) Fields() []Field {
	return f.fieldsList
}

// Field returns the leaf field by FullName, or nil if no such field
// exists. Mirrors the C# `doc.Form["name"]` indexer pattern.
func (f *Form) Field(name string) Field {
	return f.cache[name]
}

// HasField reports whether a leaf field with the given FullName exists.
func (f *Form) HasField(name string) bool {
	_, ok := f.cache[name]
	return ok
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
		dict, ok := resolveRefToDict(objects, item)
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
		k, ok := resolveRefToDict(objects, item)
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
		k, ok := resolveRefToDict(objects, item)
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
