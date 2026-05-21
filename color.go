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
	HAlignLeft   HAlign = iota // default
	HAlignCenter
	HAlignRight
)

// VAlign specifies vertical text alignment within a rectangle.
type VAlign int

const (
	VAlignTop    VAlign = iota // default
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
}
