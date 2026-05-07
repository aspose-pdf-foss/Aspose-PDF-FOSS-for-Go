package asposepdf

// FileAttachmentIcon names per ISO 32000-1 §12.5.6.15 Table 178.
type FileAttachmentIcon int

const (
	FileAttachmentIconUnknown FileAttachmentIcon = iota
	FileAttachmentIconGraph
	FileAttachmentIconPaperclip // PDF default
	FileAttachmentIconPushPin
	FileAttachmentIconTag
)

// FileAttachmentAnnotation embeds a file in the document and shows an
// icon at the annotation's /Rect. Per ISO 32000-1 §12.5.6.15. No /AP
// is generated — viewers render the icon themselves based on /Name.
type FileAttachmentAnnotation struct {
	annotationBase
}

func (a *FileAttachmentAnnotation) AnnotationType() AnnotationType {
	return AnnotationTypeFileAttachment
}

// NewFileAttachmentAnnotation builds an unbound file-attachment
// annotation. Page must be non-nil. The /Rect is auto-computed as a
// 24×24 pt square anchored at position (Acrobat icon convention).
//
// Call SetFile or SetFileFromStream to embed file data; without that,
// the annotation has no /FS entry and viewers render an empty icon.
func NewFileAttachmentAnnotation(page *Page, position Point) *FileAttachmentAnnotation {
	if page == nil {
		panic("NewFileAttachmentAnnotation: nil page")
	}
	dict := pdfDict{
		"/Type":    pdfName("/Annot"),
		"/Subtype": pdfName("/FileAttachment"),
		"/Rect":    pdfArray{position.X, position.Y, position.X + 24, position.Y + 24},
		"/Name":    pdfName("/Paperclip"),
	}
	return &FileAttachmentAnnotation{annotationBase: annotationBase{
		dict: dict,
		doc:  page.doc,
		page: page,
	}}
}

// Icon returns the /Name entry mapped to a FileAttachmentIcon.
// Returns FileAttachmentIconPaperclip (the spec default) if absent.
func (a *FileAttachmentAnnotation) Icon() FileAttachmentIcon {
	n, ok := a.dict["/Name"].(pdfName)
	if !ok {
		return FileAttachmentIconPaperclip
	}
	switch n {
	case "/Graph":
		return FileAttachmentIconGraph
	case "/Paperclip":
		return FileAttachmentIconPaperclip
	case "/PushPin":
		return FileAttachmentIconPushPin
	case "/Tag":
		return FileAttachmentIconTag
	}
	return FileAttachmentIconUnknown
}

// SetIcon writes the /Name entry. Unknown is encoded as /Paperclip.
func (a *FileAttachmentAnnotation) SetIcon(i FileAttachmentIcon) {
	var name pdfName
	switch i {
	case FileAttachmentIconGraph:
		name = "/Graph"
	case FileAttachmentIconPushPin:
		name = "/PushPin"
	case FileAttachmentIconTag:
		name = "/Tag"
	default: // Paperclip + Unknown
		name = "/Paperclip"
	}
	a.dict["/Name"] = name
}

// HasFile returns true if SetFile or SetFileFromStream has been called
// successfully and not subsequently cleared. Stub for now — full
// implementation in Task 3.
func (a *FileAttachmentAnnotation) HasFile() bool {
	if a.dict["/FS"] == nil {
		return false
	}
	return true
}

// RegenerateAppearance is a no-op for FileAttachmentAnnotation (no /AP
// — viewers render the icon themselves).
func (a *FileAttachmentAnnotation) RegenerateAppearance() {}

// parseFileAttachmentAnnotation builds a FileAttachmentAnnotation from
// a parsed dict.
func parseFileAttachmentAnnotation(base annotationBase) *FileAttachmentAnnotation {
	return &FileAttachmentAnnotation{annotationBase: base}
}
