package asposepdf

// StampName names per ISO 32000-1 §12.5.6.13 Table 184. Used in
// /Subtype /Stamp annotations' /Name entry. Unknown handles non-spec
// custom names (round-tripped via RawName).
type StampName int

const (
	StampNameUnknown StampName = iota
	StampNameApproved
	StampNameAsIs
	StampNameConfidential
	StampNameDepartmental
	StampNameDraft         // PDF default
	StampNameExperimental
	StampNameExpired
	StampNameFinal
	StampNameForComment
	StampNameForPublicRelease
	StampNameNotApproved
	StampNameNotForPublicRelease
	StampNameSold
	StampNameTopSecret
)

// StampAnnotation is a rubber-stamp annotation. Renders one of 14
// predefined visuals (Approved, Confidential, Draft, etc.) or a custom
// image. Per ISO 32000-1 §12.5.6.13.
type StampAnnotation struct {
	drawingAnnotationBase
	customImageObjID int // 0 = no custom image
}

func (a *StampAnnotation) AnnotationType() AnnotationType { return AnnotationTypeStamp }

// NewStampAnnotation builds an unbound stamp annotation. Page must be
// non-nil. /Name defaults to the supplied name (use StampNameDraft if
// uncertain).
func NewStampAnnotation(page *Page, rect Rectangle, name StampName) *StampAnnotation {
	if page == nil {
		panic("NewStampAnnotation: nil page")
	}
	dict := pdfDict{
		"/Type":    pdfName("/Annot"),
		"/Subtype": pdfName("/Stamp"),
		"/Rect":    pdfArray{rect.LLX, rect.LLY, rect.URX, rect.URY},
		"/Name":    stampNameToPDF(name),
	}
	a := &StampAnnotation{drawingAnnotationBase: drawingAnnotationBase{
		annotationBase: annotationBase{
			dict: dict,
			doc:  page.doc,
			page: page,
		},
	}}
	a.regenerate = a.regenerateAP
	a.regenerateAP()
	return a
}

// Name returns the StampName decoded from /Name. Returns
// StampNameUnknown for non-spec custom names.
func (a *StampAnnotation) Name() StampName {
	n, _ := a.dict["/Name"].(pdfName)
	return pdfNameToStampName(n)
}

// SetName writes the /Name entry from a typed StampName.
func (a *StampAnnotation) SetName(n StampName) {
	a.dict["/Name"] = stampNameToPDF(n)
	a.regenerateAP()
}

// RawName returns the /Name entry as a raw string ("/Approved", custom).
func (a *StampAnnotation) RawName() string {
	n, _ := a.dict["/Name"].(pdfName)
	return string(n)
}

// SetRawName writes the /Name entry from a raw string. Used for
// non-spec custom names. Calling SetRawName with a value not matching
// any spec name will cause Name() to return StampNameUnknown.
func (a *StampAnnotation) SetRawName(s string) {
	a.dict["/Name"] = pdfName(s)
	a.regenerateAP()
}

// HasCustomImage returns true if SetCustomImage / SetCustomImageFromStream
// has been called and not subsequently cleared. Stub for now — full
// custom-image support in Task 8.
func (a *StampAnnotation) HasCustomImage() bool {
	return a.customImageObjID != 0
}

// regenerateAP rebuilds /AP/N. Stub for now — full impl in Task 7.
func (a *StampAnnotation) regenerateAP() {
	setAppearanceN(&a.annotationBase, generateStampAppearance(a))
}

// RegenerateAppearance forces /AP/N to be rebuilt from current state.
func (a *StampAnnotation) RegenerateAppearance() {
	a.regenerateAP()
}

// parseStampAnnotation builds a StampAnnotation from a parsed dict.
func parseStampAnnotation(base annotationBase) *StampAnnotation {
	a := &StampAnnotation{drawingAnnotationBase: drawingAnnotationBase{annotationBase: base}}
	a.regenerate = a.regenerateAP
	return a
}
