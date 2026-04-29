package asposepdf

import "fmt"

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
	if v {
		f.ensureRoot()
		if f.root != nil {
			f.root["/NeedAppearances"] = true
		}
	} else if f.root != nil {
		delete(f.root, "/NeedAppearances")
	}
}

// ensureRoot lazily creates an /AcroForm dict on the document catalog
// if absent. Also creates the catalog itself if the document is new
// (NewDocument doesn't initialise one). Called from setters that need
// a place to store flags.
func (f *Form) ensureRoot() {
	if f.root != nil {
		return
	}
	if f.doc.catalog == nil {
		f.doc.catalog = pdfDict{}
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
	if _, ok := fonts["/Helv"]; ok {
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
	fonts["/Helv"] = pdfRef{Num: id}
	return "Helv", nil
}

// rectToPDFArray converts a Rectangle to a /Rect pdfArray.
func rectToPDFArray(r Rectangle) pdfArray {
	return pdfArray{r.LLX, r.LLY, r.URX, r.URY}
}

// AddCheckbox creates a checkbox widget on pageNum with the given rectangle
// and field name. Default state is unchecked (/V = /Off). The widget's
// /AP/N has two appearance states: "/Yes" (export name for checked) and
// "/Off". Callers can call SetChecked(true) on the returned handle to flip
// state and ensure /V and /AS are in sync.
//
// Errors on duplicate name, invalid pageNum, or empty name.
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
			"/Type":      pdfName("/XObject"),
			"/Subtype":   pdfName("/Form"),
			"/BBox":      pdfArray{0, 0, 0, 0},
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
