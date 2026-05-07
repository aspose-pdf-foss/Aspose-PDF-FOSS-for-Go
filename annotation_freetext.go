package asposepdf

// FreeTextIntent per ISO 32000-1 §12.5.6.6 /IT entry. Defaults to
// FreeTextIntentFreeText (plain text in a rectangle).
type FreeTextIntent int

const (
	FreeTextIntentFreeText  FreeTextIntent = iota // /FreeText
	FreeTextIntentCallout                          // /FreeTextCallout
	FreeTextIntentTypewriter                       // /FreeTextTypeWriter
)

// BorderEffect controls the /BE/S entry per ISO 32000-1 §12.5.4 Table 167.
type BorderEffect int

const (
	BorderEffectNone   BorderEffect = iota // /S = /S (default)
	BorderEffectCloudy                      // /S = /C — wavy "cloud" border
)
