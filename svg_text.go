// SPDX-License-Identifier: MIT

package asposepdf

import "strings"

type svgTextAnchor int

const (
	svgTextAnchorStart  svgTextAnchor = 0 // default (left of x)
	svgTextAnchorMiddle svgTextAnchor = 1
	svgTextAnchorEnd    svgTextAnchor = 2
)

// svgTextRun is a single contiguous text run at an absolute position.
// One <text> element produces one or more runs (one per <tspan> + leading/trailing
// CharData of the parent text element).
type svgTextRun struct {
	text  string
	x, y  float64
	style svgStyle // resolved style (font, fill, etc.)
}

// svgText is the IR node for an SVG <text> element.
type svgText struct {
	runs      []svgTextRun
	style     svgStyle // root-level style of the <text> element
	transform *svgMatrix
}

func (*svgText) svgNodeKind() string { return "text" }

// normalizeSVGTextWhitespace collapses any whitespace sequence to a single space
// and trims leading/trailing whitespace, per SVG xml:space="default" semantics.
func normalizeSVGTextWhitespace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// heuristicFont maps an SVG font-family + style to a Standard 14 PDF font.
// Recognizes common family keywords (Arial→Helvetica, Times→Times-Roman,
// Courier→Courier) plus the CSS generic families (sans-serif, serif, monospace).
// Unknown families fall back to Helvetica.
func heuristicFont(family string, bold, italic bool) Font {
	f := normalizeFontFamily(family)
	switch {
	case isMonospaceFamily(f):
		return chooseCourier(bold, italic)
	case isSerifFamily(f):
		return chooseTimes(bold, italic)
	}
	return chooseHelvetica(bold, italic)
}

// normalizeFontFamily strips quotes/whitespace and returns the first comma-separated entry, lowercased.
func normalizeFontFamily(family string) string {
	f := strings.TrimSpace(family)
	if comma := strings.IndexByte(f, ','); comma >= 0 {
		f = strings.TrimSpace(f[:comma])
	}
	f = strings.Trim(f, `"' `)
	return strings.ToLower(f)
}

func isMonospaceFamily(f string) bool {
	return strings.Contains(f, "courier") || strings.Contains(f, "monospace") ||
		strings.Contains(f, "mono")
}

func isSerifFamily(f string) bool {
	// Exclude sans-serif (contains "serif" as substring)
	if strings.Contains(f, "serif") && !strings.Contains(f, "sans") {
		return true
	}
	return strings.Contains(f, "times") || strings.Contains(f, "georgia") ||
		strings.Contains(f, "garamond")
}

func chooseHelvetica(bold, italic bool) Font {
	switch {
	case bold && italic:
		return FontHelveticaBoldOblique
	case bold:
		return FontHelveticaBold
	case italic:
		return FontHelveticaOblique
	}
	return FontHelvetica
}

func chooseTimes(bold, italic bool) Font {
	switch {
	case bold && italic:
		return FontTimesBoldItalic
	case bold:
		return FontTimesBold
	case italic:
		return FontTimesItalic
	}
	return FontTimesRoman
}

func chooseCourier(bold, italic bool) Font {
	switch {
	case bold && italic:
		return FontCourierBoldOblique
	case bold:
		return FontCourierBold
	case italic:
		return FontCourierOblique
	}
	return FontCourier
}
