// SPDX-License-Identifier: MIT

package asposepdf

import (
	"math"
	"strings"
)

// ZapfDingbats is a Standard-14 font we deliberately do not bundle (see
// fallbackFontFor). Its dominant real-world use, though, is checkbox/radio
// widget appearances: the checked state draws a single dingbat — a check mark,
// cross, circle, etc. — and without glyph shapes those marks vanish, so a
// checked box renders empty. Rather than ship a font, we synthesize vector
// outlines for the handful of marks Acrobat uses as its field "check styles"
// (Check, Cross, Circle, Square, Diamond, Star) plus a few common variants.
//
// Shapes are hand-built in 1000-unit em space (y-up, baseline 0), sized to fill
// the glyph box roughly like the real dingbats so they sit correctly inside a
// widget's appearance rectangle. Advance widths are not needed here — they come
// from the resolved ZapfDingbats AFM metrics, so only the outline is supplied.

// isZapfDingbats reports whether a non-embedded font is ZapfDingbats and should
// use the synthesized marks.
func isZapfDingbats(fi fontInfo) bool {
	return strings.Contains(strings.ToLower(fi.name), "dingbat")
}

// zapfDingbatsContours returns synthesized outlines for the dingbat at the given
// (raw, ZapfDingbatsEncoding) code, or nil when the code is not one we draw.
func zapfDingbatsContours(code uint32) []glyphContour {
	switch code {
	case 0x34, 0x35: // '4','5' → check marks (a20/a21)
		return dbCheck()
	case 0x36, 0x37, 0x38: // '6','7','8' → crosses (a22/a23/a24)
		return dbCross()
	case 0x6C: // 'l' → filled circle (a108); also the usual radio-button dot
		return dbDisc(410, 360, 320, 44)
	case 0x6D, 0x6E: // 'm','n' → filled squares (a111/a112)
		return dbSquare()
	case 0x75: // 'u' → filled diamond (a117)
		return dbDiamond()
	case 0x48: // 'H' → filled five-point star (a72)
		return dbStar()
	}
	return nil
}

// dbThickSeg outlines a straight segment as a filled rectangle of half-width h,
// extended by ext past each end (so two segments sharing a vertex overlap and
// fill the corner cleanly under nonzero winding).
func dbThickSeg(ax, ay, bx, by, h, ext float64) glyphContour {
	dx, dy := bx-ax, by-ay
	l := math.Hypot(dx, dy)
	if l == 0 {
		return nil
	}
	ux, uy := dx/l, dy/l
	ax, ay = ax-ux*ext, ay-uy*ext
	bx, by = bx+ux*ext, by+uy*ext
	px, py := -uy*h, ux*h
	return glyphContour{
		{ax + px, ay + py, true},
		{bx + px, by + py, true},
		{bx - px, by - py, true},
		{ax - px, ay - py, true},
	}
}

// dbCheck builds a check mark from two thick strokes meeting at the low vertex.
func dbCheck() []glyphContour {
	const h, ext = 82, 78
	return []glyphContour{
		dbThickSeg(120, 360, 330, 150, h, ext),
		dbThickSeg(330, 150, 720, 710, h, ext),
	}
}

// dbCross builds an X from two crossing thick strokes.
func dbCross() []glyphContour {
	const h, ext = 80, 0
	return []glyphContour{
		dbThickSeg(150, 110, 690, 650, h, ext),
		dbThickSeg(150, 650, 690, 110, h, ext),
	}
}

// dbDisc returns a filled regular n-gon approximating a circle.
func dbDisc(cx, cy, r float64, n int) []glyphContour {
	if n < 8 {
		n = 8
	}
	c := make(glyphContour, n)
	for i := 0; i < n; i++ {
		a := 2 * math.Pi * float64(i) / float64(n)
		c[i] = glyphPoint{cx + r*math.Cos(a), cy + r*math.Sin(a), true}
	}
	return []glyphContour{c}
}

// dbSquare returns a filled square centered in the glyph box.
func dbSquare() []glyphContour {
	return []glyphContour{{
		{110, 60, true}, {710, 60, true}, {710, 660, true}, {110, 660, true},
	}}
}

// dbDiamond returns a filled diamond (rotated square).
func dbDiamond() []glyphContour {
	return []glyphContour{{
		{410, 40, true}, {740, 360, true}, {410, 680, true}, {80, 360, true},
	}}
}

// dbStar returns a filled five-point star.
func dbStar() []glyphContour {
	const cx, cy, rOut, rIn = 410, 372, 348, 142
	c := make(glyphContour, 10)
	for i := 0; i < 10; i++ {
		a := math.Pi/2 + float64(i)*math.Pi/5
		r := float64(rOut)
		if i%2 == 1 {
			r = rIn
		}
		c[i] = glyphPoint{cx + r*math.Cos(a), cy + r*math.Sin(a), true}
	}
	return []glyphContour{c}
}
