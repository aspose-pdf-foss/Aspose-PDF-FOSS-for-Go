// SPDX-License-Identifier: MIT

package asposepdf

// TextIcon names per ISO 32000-1 §12.5.6.4 Table 172, used in
// /Subtype /Text annotations' /Name entry.
type TextIcon int

const (
	TextIconUnknown TextIcon = iota
	TextIconComment
	TextIconKey
	TextIconNote      // PDF default if /Name is absent
	TextIconHelp
	TextIconNewParagraph
	TextIconParagraph
	TextIconInsert
)

// TextAnnotation is a sticky-note annotation. Renders as an icon (no
// /AP/N — viewers draw their own icon for the /Name value). The
// associated /Contents is the note's body text shown in a popup when
// the icon is clicked.
type TextAnnotation struct {
	annotationBase
}

func (a *TextAnnotation) AnnotationType() AnnotationType { return AnnotationTypeText }

// NewTextAnnotation builds an unbound text-note annotation. Page must
// be non-nil. The /Rect is auto-computed as a 24×24 pt square anchored
// at position (Acrobat sticky-note convention).
func NewTextAnnotation(page *Page, position Point) *TextAnnotation {
	if page == nil {
		panic("NewTextAnnotation: nil page")
	}
	dict := pdfDict{
		"/Type":    pdfName("/Annot"),
		"/Subtype": pdfName("/Text"),
		"/Rect":    pdfArray{position.X, position.Y, position.X + 24, position.Y + 24},
	}
	return &TextAnnotation{annotationBase: annotationBase{
		dict: dict,
		doc:  page.doc,
		page: page,
	}}
}

// Icon returns the /Name value mapped to a TextIcon. Returns
// TextIconNote (the spec default) if /Name is absent.
func (a *TextAnnotation) Icon() TextIcon {
	n, ok := a.dict["/Name"].(pdfName)
	if !ok {
		return TextIconNote
	}
	switch n {
	case "/Comment":
		return TextIconComment
	case "/Key":
		return TextIconKey
	case "/Note":
		return TextIconNote
	case "/Help":
		return TextIconHelp
	case "/NewParagraph":
		return TextIconNewParagraph
	case "/Paragraph":
		return TextIconParagraph
	case "/Insert":
		return TextIconInsert
	}
	return TextIconUnknown
}

// SetIcon writes the /Name entry. Unknown is encoded as /Note (default)
// to avoid producing an empty name.
func (a *TextAnnotation) SetIcon(t TextIcon) {
	var name pdfName
	switch t {
	case TextIconComment:
		name = "/Comment"
	case TextIconKey:
		name = "/Key"
	case TextIconHelp:
		name = "/Help"
	case TextIconNewParagraph:
		name = "/NewParagraph"
	case TextIconParagraph:
		name = "/Paragraph"
	case TextIconInsert:
		name = "/Insert"
	default: // TextIconNote and TextIconUnknown
		name = "/Note"
	}
	a.dict["/Name"] = name
}

// Open returns the /Open flag (whether the popup is initially shown).
func (a *TextAnnotation) Open() bool {
	v, _ := a.dict["/Open"].(bool)
	return v
}

// SetOpen writes the /Open flag.
func (a *TextAnnotation) SetOpen(open bool) {
	if open {
		a.dict["/Open"] = true
	} else {
		delete(a.dict, "/Open")
	}
}

// RegenerateAppearance is a no-op for TextAnnotation (no /AP — viewers
// render the icon themselves). Present for API symmetry across all
// annotation types.
func (a *TextAnnotation) RegenerateAppearance() {}

// parseTextAnnotation builds a TextAnnotation from a parsed dict.
func parseTextAnnotation(base annotationBase) *TextAnnotation {
	return &TextAnnotation{annotationBase: base}
}
