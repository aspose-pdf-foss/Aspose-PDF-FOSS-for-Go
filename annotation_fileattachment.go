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
