// SPDX-License-Identifier: MIT

package asposepdf

import (
	"regexp"
	"sync"
)

// Regexes for the readable parts of a Type1 program.
var (
	reType1Dup        = regexp.MustCompile(`dup\s+(\d+)\s*/([^\s/{}()]+)\s+put`)
	reType1SubrsCount = regexp.MustCompile(`/Subrs\s+(\d+)`)
	reType1LenIV      = regexp.MustCompile(`/lenIV\s+(\d+)`)
	reType1FontMatrix = regexp.MustCompile(`/FontMatrix\s*\[([^\]]*)\]`)
)

// runeToGlyphName reverses the Adobe Glyph List (glyphToRune); the first name
// registered for a rune wins, which is the canonical one for the codes that
// matter (ASCII + Latin-1). Built once, lazily.
var (
	runeToGlyphNameOnce sync.Once
	runeToGlyphNameMap  map[rune]string
)

func runeToStdGlyphName(r rune) string {
	runeToGlyphNameOnce.Do(func() {
		runeToGlyphNameMap = make(map[rune]string, len(glyphToRune))
		for name, rr := range glyphToRune {
			if _, ok := runeToGlyphNameMap[rr]; !ok {
				runeToGlyphNameMap[rr] = name
			}
		}
	})
	return runeToGlyphNameMap[r]
}

// glyphContours runs the Type1 charstring for the glyph at gid and returns its
// outline contours in font units. Recovers from a malformed charstring by
// returning whatever was drawn so far.
func (f *type1Font) glyphContours(gid uint16) (contours []glyphContour) {
	defer func() {
		if recover() != nil {
			contours = nil
		}
	}()
	if int(gid) >= len(f.glyphNames) {
		return nil
	}
	cs := f.charstrings[f.glyphNames[gid]]
	if cs == nil {
		return nil
	}
	ip := &t1Interp{f: f}
	ip.run(cs, 0)
	ip.closeContour()
	return ip.contours
}

// t1Interp is the Type1 charstring interpreter state.
type t1Interp struct {
	f        *type1Font
	stack    []float64
	ps       []float64 // PostScript stack for callothersubr/pop
	x, y     float64
	sbx      float64
	contours []glyphContour
	cur      glyphContour
	open     bool
	flex     bool
	flexPts  [][2]float64
	width    float64
	done     bool
}

func (ip *t1Interp) push(v float64) { ip.stack = append(ip.stack, v) }
func (ip *t1Interp) clear()         { ip.stack = ip.stack[:0] }

func (ip *t1Interp) moveTo(x, y float64) {
	if ip.flex {
		ip.flexPts = append(ip.flexPts, [2]float64{x, y})
		ip.x, ip.y = x, y
		return
	}
	ip.closeContour()
	ip.x, ip.y = x, y
	ip.cur = glyphContour{{x: x, y: y, on: true}}
	ip.open = true
}

func (ip *t1Interp) lineTo(x, y float64) {
	ip.x, ip.y = x, y
	ip.cur = append(ip.cur, glyphPoint{x: x, y: y, on: true})
}

func (ip *t1Interp) curveTo(x1, y1, x2, y2, x3, y3 float64) {
	ip.cur = append(ip.cur,
		glyphPoint{x: x1, y: y1, on: false},
		glyphPoint{x: x2, y: y2, on: false},
		glyphPoint{x: x3, y: y3, on: true})
	ip.x, ip.y = x3, y3
}

func (ip *t1Interp) closeContour() {
	if ip.open && len(ip.cur) > 0 {
		ip.contours = append(ip.contours, ip.cur)
	}
	ip.cur = nil
	ip.open = false
}

// run executes a charstring (or subr), depth-limited against runaway recursion.
func (ip *t1Interp) run(cs []byte, depth int) {
	if depth > 30 || ip.done {
		return
	}
	for i := 0; i < len(cs); {
		b := cs[i]
		i++
		switch {
		case b >= 32:
			// Number operand.
			var v int
			switch {
			case b <= 246:
				v = int(b) - 139
			case b <= 250:
				v = (int(b)-247)*256 + int(cs[i]) + 108
				i++
			case b <= 254:
				v = -(int(b)-251)*256 - int(cs[i]) - 108
				i++
			default: // 255: 32-bit big-endian
				v = int(int32(uint32(cs[i])<<24 | uint32(cs[i+1])<<16 | uint32(cs[i+2])<<8 | uint32(cs[i+3])))
				i += 4
			}
			ip.push(float64(v))
			continue
		}
		// Operator.
		if b == 12 {
			ip.escape(cs[i], depth)
			i++
			if ip.done {
				return
			}
			continue
		}
		if ip.operator(b, depth) {
			return // endchar / return
		}
	}
}

// operator executes a single-byte Type1 operator. Returns true to stop the
// current charstring (endchar) or subr (return).
func (ip *t1Interp) operator(b byte, depth int) bool {
	s := ip.stack
	switch b {
	case 13: // hsbw: sbx wx
		if len(s) >= 2 {
			ip.sbx = s[0]
			ip.width = s[1]
			ip.x = ip.sbx
		}
		ip.clear()
	case 9: // closepath
		ip.clear()
	case 21: // rmoveto
		if len(s) >= 2 {
			ip.moveTo(ip.x+s[len(s)-2], ip.y+s[len(s)-1])
		}
		ip.clear()
	case 22: // hmoveto
		if len(s) >= 1 {
			ip.moveTo(ip.x+s[len(s)-1], ip.y)
		}
		ip.clear()
	case 4: // vmoveto
		if len(s) >= 1 {
			ip.moveTo(ip.x, ip.y+s[len(s)-1])
		}
		ip.clear()
	case 5: // rlineto
		if len(s) >= 2 {
			ip.lineTo(ip.x+s[len(s)-2], ip.y+s[len(s)-1])
		}
		ip.clear()
	case 6: // hlineto
		if len(s) >= 1 {
			ip.lineTo(ip.x+s[len(s)-1], ip.y)
		}
		ip.clear()
	case 7: // vlineto
		if len(s) >= 1 {
			ip.lineTo(ip.x, ip.y+s[len(s)-1])
		}
		ip.clear()
	case 8: // rrcurveto
		if len(s) >= 6 {
			ip.rrcurve(s[len(s)-6], s[len(s)-5], s[len(s)-4], s[len(s)-3], s[len(s)-2], s[len(s)-1])
		}
		ip.clear()
	case 30: // vhcurveto: dy1 dx2 dy2 dx3
		if len(s) >= 4 {
			ip.rrcurve(0, s[len(s)-4], s[len(s)-3], s[len(s)-2], s[len(s)-1], 0)
		}
		ip.clear()
	case 31: // hvcurveto: dx1 dx2 dy2 dy3
		if len(s) >= 4 {
			ip.rrcurve(s[len(s)-4], 0, s[len(s)-3], s[len(s)-2], 0, s[len(s)-1])
		}
		ip.clear()
	case 1, 3: // hstem, vstem (hints — ignore)
		ip.clear()
	case 10: // callsubr
		if len(ip.stack) >= 1 {
			idx := int(ip.stack[len(ip.stack)-1])
			ip.stack = ip.stack[:len(ip.stack)-1]
			if idx >= 0 && idx < len(ip.f.subrs) && ip.f.subrs[idx] != nil {
				ip.run(ip.f.subrs[idx], depth+1)
			}
		}
	case 11: // return
		return true
	case 14: // endchar
		ip.done = true
		return true
	default:
		ip.clear()
	}
	return false
}

// rrcurve appends a cubic Bézier given the six relative deltas (the Type1
// convention: each pair is relative to the running point).
func (ip *t1Interp) rrcurve(dx1, dy1, dx2, dy2, dx3, dy3 float64) {
	x1 := ip.x + dx1
	y1 := ip.y + dy1
	x2 := x1 + dx2
	y2 := y1 + dy2
	x3 := x2 + dx3
	y3 := y2 + dy3
	ip.curveTo(x1, y1, x2, y2, x3, y3)
}

// escape handles the 12-prefixed (two-byte) operators.
func (ip *t1Interp) escape(b byte, depth int) {
	s := ip.stack
	switch b {
	case 0: // dotsection
		ip.clear()
	case 1, 2: // vstem3, hstem3 (hints)
		ip.clear()
	case 6: // seac: asb adx ady bchar achar — accent composition
		if len(s) >= 5 {
			ip.seac(s[len(s)-5], s[len(s)-4], s[len(s)-3], int(s[len(s)-2]), int(s[len(s)-1]), depth)
		}
		ip.clear()
		ip.done = true
	case 7: // sbw: sbx sby wx wy
		if len(s) >= 4 {
			ip.sbx = s[0]
			ip.x, ip.y = s[0], s[1]
			ip.width = s[2]
		}
		ip.clear()
	case 12: // div: a b → a/b
		if len(s) >= 2 {
			a, bb := s[len(s)-2], s[len(s)-1]
			ip.stack = s[:len(s)-2]
			if bb != 0 {
				ip.push(a / bb)
			} else {
				ip.push(0)
			}
		}
	case 16: // callothersubr
		ip.callOtherSubr()
	case 17: // pop: move a value from the PS stack to the operand stack
		if len(ip.ps) > 0 {
			ip.push(ip.ps[len(ip.ps)-1])
			ip.ps = ip.ps[:len(ip.ps)-1]
		} else {
			ip.push(0)
		}
	case 33: // setcurrentpoint
		if len(s) >= 2 {
			ip.x, ip.y = s[len(s)-2], s[len(s)-1]
		}
		ip.clear()
	default:
		ip.clear()
	}
}

// callOtherSubr handles the standard OtherSubrs: 0 flex end, 1 flex start,
// 2 flex point, 3 hint replacement; unknown subrs pass their arguments through
// to the PS stack so the following pops retrieve them.
func (ip *t1Interp) callOtherSubr() {
	s := ip.stack
	if len(s) < 2 {
		ip.clear()
		return
	}
	othersubr := int(s[len(s)-1])
	n := int(s[len(s)-2])
	ip.stack = s[:len(s)-2]
	if n < 0 || n > len(ip.stack) {
		n = 0
	}
	args := append([]float64(nil), ip.stack[len(ip.stack)-n:]...)
	ip.stack = ip.stack[:len(ip.stack)-n]

	switch othersubr {
	case 1: // start flex: collect the 7 reference points via rmoveto
		ip.flex = true
		ip.flexPts = ip.flexPts[:0]
	case 2: // flex point: the point was added by the preceding rmoveto
	case 0: // end flex: draw two curves through the 7 collected points
		ip.flex = false
		if len(ip.flexPts) >= 7 {
			p := ip.flexPts
			// p[0] is the reference point; p[1..6] are two cubic segments.
			ip.curveTo(p[1][0], p[1][1], p[2][0], p[2][1], p[3][0], p[3][1])
			ip.curveTo(p[4][0], p[4][1], p[5][0], p[5][1], p[6][0], p[6][1])
		}
		// Leave end x,y for the two following pops (setcurrentpoint).
		if len(args) >= 3 {
			ip.ps = append(ip.ps, args[2], args[1]) // pops give x then y
		}
	case 3: // hint replacement: push the subr number back for the pop+callsubr
		ip.ps = append(ip.ps, 3)
	default: // unknown: pass args through (reverse, so pops come out in order)
		for i := len(args) - 1; i >= 0; i-- {
			ip.ps = append(ip.ps, args[i])
		}
	}
}

// seac composes an accented glyph from a base (bchar) and accent (achar),
// both StandardEncoding codes, offset by (adx − asb + sbx, ady).
func (ip *t1Interp) seac(asb, adx, ady float64, bchar, achar, depth int) {
	baseName := stdEncodingName(bchar)
	accName := stdEncodingName(achar)
	if baseName != "" {
		if cs := ip.f.charstrings[baseName]; cs != nil {
			sub := &t1Interp{f: ip.f}
			sub.run(cs, depth+1)
			sub.closeContour()
			ip.contours = append(ip.contours, sub.contours...)
		}
	}
	if accName != "" {
		if cs := ip.f.charstrings[accName]; cs != nil {
			sub := &t1Interp{f: ip.f}
			sub.run(cs, depth+1)
			sub.closeContour()
			dx := adx - asb + ip.sbx
			for _, c := range sub.contours {
				shifted := make(glyphContour, len(c))
				for i, p := range c {
					shifted[i] = glyphPoint{x: p.x + dx, y: p.y + ady, on: p.on}
				}
				ip.contours = append(ip.contours, shifted)
			}
		}
	}
}

// stdEncodingName returns the StandardEncoding glyph name for a code.
func stdEncodingName(code int) string {
	if code < 0 || code >= 256 {
		return ""
	}
	r := standardEncoding[code]
	if r == 0 || r == 0xFFFD {
		return ""
	}
	return runeToStdGlyphName(r)
}
