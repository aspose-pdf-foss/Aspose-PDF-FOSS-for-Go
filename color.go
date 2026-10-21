// SPDX-License-Identifier: MIT

package asposepdf

// Color represents an RGBA color with values in [0, 1].
type Color struct {
	R float64
	G float64
	B float64
	A float64
}

// HAlign specifies horizontal text alignment within a rectangle.
type HAlign int

const (
	HAlignLeft HAlign = iota // default
	HAlignCenter
	HAlignRight
)

// VAlign specifies vertical text alignment within a rectangle.
type VAlign int

const (
	VAlignTop VAlign = iota // default
	VAlignMiddle
	VAlignBottom
)

// TextStyle defines reusable text formatting properties.
type TextStyle struct {
	Font          Font    // nil defaults to FontHelvetica in AddText
	Size          float64 // in points; 0 treated as 12
	Color         *Color  // nil → black opaque {0,0,0,1}
	Background    *Color  // nil → no background
	HAlign        HAlign  // default: HAlignLeft
	VAlign        VAlign  // default: VAlignTop
	LineSpacing   float64 // multiplier of font size; 0 treated as 1.2
	Underline     bool
	Strikethrough bool
	Rotation      float64 // degrees counter-clockwise; pivot = lower-left corner of rect; default 0
	Behind        bool    // if true, text is drawn under existing page content; default false
	RTL           bool    // if true, the paragraph base direction is right-to-left (Hebrew/Arabic); default false (auto-detected from the text when it contains RTL characters)
	Skew          float64 // synthetic oblique: glyph slant in degrees (positive leans right, ~12 matches typical italics); applied per line about its baseline — faux italic for fonts without an italic face
	Invisible     bool    // if true, glyphs use text rendering mode 3 (ISO 32000-1 §9.3.6): nothing is painted, but the text remains selectable, searchable, and extractable. Underline/Strikethrough decorations are suppressed too. Used for hidden OCR text layers over scanned pages.

	// MissingGlyph controls what AddText/AddTextAt/MeasureText do when
	// the font has no glyph for a character in text. Zero value
	// (MissingGlyphAllow) keeps the long-standing default: the character
	// draws .notdef (glyph 0) — or, for a Type3 font, is silently
	// dropped — with no signal at all (GitHub issue #19,
	// shadowhunter1470-svg). A default-ignorable code point (a zero-width
	// joiner, a variation selector, …) is never flagged regardless of
	// this setting — drawing nothing for one of those is correct, not
	// missing.
	MissingGlyph MissingGlyphPolicy
	// ReplacementRune is substituted for a missing glyph when
	// MissingGlyph is MissingGlyphReplace. Zero value (rune 0) means
	// U+FFFD, the Unicode replacement character — the same substitution
	// this library's own text extraction already uses for a character
	// it can't map back from a glyph. If the font has no glyph for the
	// replacement rune either, that draws .notdef in turn; pick a
	// replacement the font actually covers if that matters.
	ReplacementRune rune
}

// MissingGlyphPolicy is TextStyle.MissingGlyph's type. See its doc
// comment for what each value does.
type MissingGlyphPolicy int

const (
	// MissingGlyphAllow draws .notdef (or, for Type3, nothing) for a
	// character the font has no glyph for — today's existing behavior,
	// and the zero value, so no caller is affected unless it opts in.
	MissingGlyphAllow MissingGlyphPolicy = iota
	// MissingGlyphError makes the draw/measure call fail, naming the
	// first character the font cannot show.
	MissingGlyphError
	// MissingGlyphReplace substitutes TextStyle.ReplacementRune for
	// every character the font cannot show, drawing that instead of
	// .notdef.
	MissingGlyphReplace
)
