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
	form     *Form
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
	form.leaves = walkAcroForm(form, d.objects, root)
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
		dict, ok := resolveRefToDict(objects, item)
		if !ok {
			continue
		}
		walkField(form, objects, dict, "", "", 0, &out)
	}
	return out
}

func walkField(form *Form, objects map[int]*pdfObject, dict pdfDict, parentName, parentFT string, parentFF int, out *[]*fieldNode) {
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
		*out = append(*out, &fieldNode{form: form, dict: dict, fullName: fullName, ft: ft, ff: ff, widgets: []pdfDict{dict}})
		return
	}
	arr, ok := kidsVal.(pdfArray)
	if !ok {
		*out = append(*out, &fieldNode{form: form, dict: dict, fullName: fullName, ft: ft, ff: ff})
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
		*out = append(*out, &fieldNode{form: form, dict: dict, fullName: fullName, ft: ft, ff: ff, widgets: widgets})
		return
	}
	// Recurse into sub-fields.
	for _, item := range arr {
		k, ok := resolveRefToDict(objects, item)
		if !ok {
			continue
		}
		walkField(form, objects, k, fullName, ft, ff, out)
	}
}

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
					r = 0x10000 + ((r-0xD800)<<10) + (lo - 0xDC00)
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
// stream on display.
func noteFormMutated(n *fieldNode) {
	if n != nil && n.form != nil {
		n.form.noteFormMutatedInForm()
	}
}

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
	} else if f.root != nil {
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

// noteFormMutatedInForm sets /NeedAppearances=true on the form's root.
// Different name from the package-level noteFormMutated to avoid name
// collision; the package-level function remains as the call site for
// concrete-type setters.
func (f *Form) noteFormMutatedInForm() {
	f.ensureRoot()
	if f.root != nil {
		f.root["/NeedAppearances"] = true
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
