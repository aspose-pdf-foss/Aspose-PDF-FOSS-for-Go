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
}
