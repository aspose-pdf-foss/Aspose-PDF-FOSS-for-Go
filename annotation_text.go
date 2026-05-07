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
