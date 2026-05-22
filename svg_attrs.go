// SPDX-License-Identifier: MIT

package asposepdf

import (
	"strconv"
	"strings"
)

// parseSVGColor returns the parsed color and ok=true on success.
// For "none"/"transparent" returns (nil, true). For unrecognized input returns (nil, false).
// "currentColor" resolves to opaque black as a fallback (no parent context at parse time).
func parseSVGColor(s string) (*Color, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, false
	}
	switch strings.ToLower(s) {
	case "none", "transparent":
		return nil, true
	case "currentcolor":
		return &Color{R: 0, G: 0, B: 0, A: 1}, true
	}
	if strings.HasPrefix(s, "#") {
		return parseSVGColorHex(s[1:])
	}
	sl := strings.ToLower(s)
	if strings.HasPrefix(sl, "rgba(") {
		return parseSVGColorRGB(s)
	}
	if strings.HasPrefix(sl, "rgb(") {
		return parseSVGColorRGB(s)
	}
	if rgb, ok := svgNamedColors[sl]; ok {
		return &Color{
			R: float64(rgb[0]) / 255,
			G: float64(rgb[1]) / 255,
			B: float64(rgb[2]) / 255,
			A: 1,
		}, true
	}
	return nil, false
}

// parseSVGColorHex parses a hex color string without the leading '#'.
// Supports 3-digit (#RGB), 6-digit (#RRGGBB), and 8-digit (#RRGGBBAA) forms.
func parseSVGColorHex(h string) (*Color, bool) {
	switch len(h) {
	case 3:
		r, ok1 := hexNibble(h[0])
		g, ok2 := hexNibble(h[1])
		b, ok3 := hexNibble(h[2])
		if !ok1 || !ok2 || !ok3 {
			return nil, false
		}
		// Each nibble is duplicated: #RGB → #RRGGBB, so value = nibble * 17
		return &Color{R: float64(r*17) / 255, G: float64(g*17) / 255, B: float64(b*17) / 255, A: 1}, true
	case 6:
		r, ok1 := hexByte(h[0:2])
		g, ok2 := hexByte(h[2:4])
		b, ok3 := hexByte(h[4:6])
		if !ok1 || !ok2 || !ok3 {
			return nil, false
		}
		return &Color{R: float64(r) / 255, G: float64(g) / 255, B: float64(b) / 255, A: 1}, true
	case 8:
		r, ok1 := hexByte(h[0:2])
		g, ok2 := hexByte(h[2:4])
		b, ok3 := hexByte(h[4:6])
		a, ok4 := hexByte(h[6:8])
		if !ok1 || !ok2 || !ok3 || !ok4 {
			return nil, false
		}
		return &Color{R: float64(r) / 255, G: float64(g) / 255, B: float64(b) / 255, A: float64(a) / 255}, true
	}
	return nil, false
}

// hexNibble converts a single hex digit byte to its numeric value.
func hexNibble(b byte) (uint8, bool) {
	switch {
	case b >= '0' && b <= '9':
		return b - '0', true
	case b >= 'a' && b <= 'f':
		return b - 'a' + 10, true
	case b >= 'A' && b <= 'F':
		return b - 'A' + 10, true
	}
	return 0, false
}

// hexByte converts a two-character hex string to a byte value.
func hexByte(s string) (uint8, bool) {
	hi, ok1 := hexNibble(s[0])
	lo, ok2 := hexNibble(s[1])
	if !ok1 || !ok2 {
		return 0, false
	}
	return hi*16 + lo, true
}

// parseSVGColorRGB parses rgb(...) and rgba(...) color functions.
// Supports integer values (0-255) and percentage values (0%-100%) for RGB channels.
// Alpha in rgba() is a float in [0, 1] or a percentage.
func parseSVGColorRGB(s string) (*Color, bool) {
	sl := strings.ToLower(s)
	hasAlpha := strings.HasPrefix(sl, "rgba(")
	open := strings.IndexByte(s, '(')
	close := strings.IndexByte(s, ')')
	if open < 0 || close < 0 || close < open {
		return nil, false
	}
	body := s[open+1 : close]
	parts := strings.Split(body, ",")
	if hasAlpha && len(parts) != 4 {
		return nil, false
	}
	if !hasAlpha && len(parts) != 3 {
		return nil, false
	}

	// chanParse parses an RGB channel: integer 0-255 or percentage 0%-100%.
	chanParse := func(s string) (float64, bool) {
		s = strings.TrimSpace(s)
		if strings.HasSuffix(s, "%") {
			n, err := strconv.ParseFloat(s[:len(s)-1], 64)
			if err != nil {
				return 0, false
			}
			return n / 100, true
		}
		n, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return 0, false
		}
		return n / 255, true
	}

	r, ok1 := chanParse(parts[0])
	g, ok2 := chanParse(parts[1])
	b, ok3 := chanParse(parts[2])
	if !ok1 || !ok2 || !ok3 {
		return nil, false
	}

	a := 1.0
	if hasAlpha {
		as := strings.TrimSpace(parts[3])
		if strings.HasSuffix(as, "%") {
			n, err := strconv.ParseFloat(as[:len(as)-1], 64)
			if err != nil {
				return nil, false
			}
			a = n / 100
		} else {
			n, err := strconv.ParseFloat(as, 64)
			if err != nil {
				return nil, false
			}
			a = n
		}
	}
	return &Color{R: clamp01(r), G: clamp01(g), B: clamp01(b), A: clamp01(a)}, true
}

// clamp01 clamps x to the range [0, 1].
func clamp01(x float64) float64 {
	if x < 0 {
		return 0
	}
	if x > 1 {
		return 1
	}
	return x
}

// parseSVGLength parses an SVG length value into PDF points.
// Supports unitless (= px = pt = user units), pt, pc, in, mm, cm, px.
// Returns (0, false) for em/ex/% (Phase 3) and unrecognized input.
func parseSVGLength(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	// Find unit suffix (trailing alpha chars or '%')
	i := len(s)
	for i > 0 {
		c := s[i-1]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c == '%' {
			i--
		} else {
			break
		}
	}
	numStr, unit := s[:i], strings.ToLower(s[i:])
	n, err := strconv.ParseFloat(strings.TrimSpace(numStr), 64)
	if err != nil {
		return 0, false
	}
	switch unit {
	case "", "px", "pt":
		return n, true
	case "pc":
		return n * 12, true
	case "in":
		return n * 72, true
	case "mm":
		return n * 72 / 25.4, true
	case "cm":
		return n * 72 / 2.54, true
	case "em", "ex", "%":
		return 0, false
	}
	return 0, false
}

// parseSVGNumber parses a unitless float, used for opacity, stroke-width without units, etc.
func parseSVGNumber(s string) (float64, bool) {
	n, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return 0, false
	}
	return n, true
}
