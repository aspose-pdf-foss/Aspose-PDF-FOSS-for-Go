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
