// SPDX-License-Identifier: MIT

package asposepdf

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"unicode"
)

// parseSVGPathData parses an SVG path data string into a normalized []svgPathOp.
// After normalization, only M, L, C, Q, Z kinds remain (A is further decomposed into C's),
// all with absolute coords.
func parseSVGPathData(d string) ([]svgPathOp, error) {
	tokens, err := tokenizeSVGPath(d)
	if err != nil {
		return nil, err
	}
	return normalizeSVGPath(tokens)
}

type svgPathToken struct {
	isCmd bool
	cmd   byte
	num   float64
}

func tokenizeSVGPath(d string) ([]svgPathToken, error) {
	out := make([]svgPathToken, 0, 32)
	i := 0
	for i < len(d) {
		c := d[i]
		if c == ' ' || c == ',' || c == '\t' || c == '\n' || c == '\r' {
			i++
			continue
		}
		if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') {
			out = append(out, svgPathToken{isCmd: true, cmd: c})
			i++
			continue
		}
		if c == '+' || c == '-' || c == '.' || (c >= '0' && c <= '9') {
			j := i + 1
			seenDot := c == '.'
			seenE := false
			for j < len(d) {
				ch := d[j]
				if ch >= '0' && ch <= '9' {
					j++
				} else if ch == '.' && !seenDot && !seenE {
					seenDot = true
					j++
				} else if (ch == 'e' || ch == 'E') && !seenE {
					seenE = true
					j++
					if j < len(d) && (d[j] == '+' || d[j] == '-') {
						j++
					}
				} else {
					break
				}
			}
			n, err := strconv.ParseFloat(d[i:j], 64)
			if err != nil {
				return nil, fmt.Errorf("invalid number %q in path data: %w", d[i:j], err)
			}
			out = append(out, svgPathToken{num: n})
			i = j
			continue
		}
		if unicode.IsSpace(rune(c)) {
			i++
			continue
		}
		return nil, fmt.Errorf("unexpected character %q in path data at %d", c, i)
	}
	return out, nil
}

// normalizeSVGPath consumes raw tokens and emits normalized svgPathOps.
// Tracks current point (cx, cy), last C2 control (for S reflection), last Q control (for T),
// and subpath start (for Z).
func normalizeSVGPath(tokens []svgPathToken) ([]svgPathOp, error) {
	ops := make([]svgPathOp, 0, len(tokens)/4)
	var cx, cy float64
	var startX, startY float64
	var lastCubicC2X, lastCubicC2Y float64
	var hasLastCubic bool
	var lastQuadCX, lastQuadCY float64
	var hasLastQuad bool

	i := 0
	if len(tokens) == 0 {
		return nil, nil
	}
	if !tokens[0].isCmd {
		return nil, errors.New("path data must start with a command")
	}
	curCmd := tokens[0].cmd
	i++

	num := func() (float64, error) {
		if i >= len(tokens) || tokens[i].isCmd {
			return 0, fmt.Errorf("expected number for command %c at token %d", curCmd, i)
		}
		v := tokens[i].num
		i++
		return v, nil
	}

	for {
		if i < len(tokens) && tokens[i].isCmd {
			curCmd = tokens[i].cmd
			i++
		}
		switch curCmd {
		case 'M', 'm':
			x, err := num()
			if err != nil {
				return nil, err
			}
			y, err := num()
			if err != nil {
				return nil, err
			}
			if curCmd == 'm' {
				x += cx
				y += cy
			}
			cx, cy = x, y
			startX, startY = x, y
			ops = append(ops, svgPathOp{kind: 'M', args: [7]float64{x, y}})
			hasLastCubic, hasLastQuad = false, false
			// subsequent coordinates after M are implicit L/l
			if curCmd == 'M' {
				curCmd = 'L'
			} else {
				curCmd = 'l'
			}
		case 'L', 'l':
			x, err := num()
			if err != nil {
				return nil, err
			}
			y, err := num()
			if err != nil {
				return nil, err
			}
			if curCmd == 'l' {
				x += cx
				y += cy
			}
			cx, cy = x, y
			ops = append(ops, svgPathOp{kind: 'L', args: [7]float64{x, y}})
			hasLastCubic, hasLastQuad = false, false
		case 'H', 'h':
			x, err := num()
			if err != nil {
				return nil, err
			}
			if curCmd == 'h' {
				x += cx
			}
			ops = append(ops, svgPathOp{kind: 'L', args: [7]float64{x, cy}})
			cx = x
			hasLastCubic, hasLastQuad = false, false
		case 'V', 'v':
			y, err := num()
			if err != nil {
				return nil, err
			}
			if curCmd == 'v' {
				y += cy
			}
			ops = append(ops, svgPathOp{kind: 'L', args: [7]float64{cx, y}})
			cy = y
			hasLastCubic, hasLastQuad = false, false
		case 'C', 'c':
			x1, err := num()
			if err != nil {
				return nil, err
			}
			y1, err := num()
			if err != nil {
				return nil, err
			}
			x2, err := num()
			if err != nil {
				return nil, err
			}
			y2, err := num()
			if err != nil {
				return nil, err
			}
			x, err := num()
			if err != nil {
				return nil, err
			}
			y, err := num()
			if err != nil {
				return nil, err
			}
			if curCmd == 'c' {
				x1 += cx
				y1 += cy
				x2 += cx
				y2 += cy
				x += cx
				y += cy
			}
			ops = append(ops, svgPathOp{kind: 'C', args: [7]float64{x1, y1, x2, y2, x, y}})
			cx, cy = x, y
			lastCubicC2X, lastCubicC2Y = x2, y2
			hasLastCubic, hasLastQuad = true, false
		case 'S', 's':
			x2, err := num()
			if err != nil {
				return nil, err
			}
			y2, err := num()
			if err != nil {
				return nil, err
			}
			x, err := num()
			if err != nil {
				return nil, err
			}
			y, err := num()
			if err != nil {
				return nil, err
			}
			if curCmd == 's' {
				x2 += cx
				y2 += cy
				x += cx
				y += cy
			}
			var x1, y1 float64
			if hasLastCubic {
				x1 = 2*cx - lastCubicC2X
				y1 = 2*cy - lastCubicC2Y
			} else {
				x1, y1 = cx, cy
			}
			ops = append(ops, svgPathOp{kind: 'C', args: [7]float64{x1, y1, x2, y2, x, y}})
			cx, cy = x, y
			lastCubicC2X, lastCubicC2Y = x2, y2
			hasLastCubic, hasLastQuad = true, false
		case 'Q', 'q':
			x1, err := num()
			if err != nil {
				return nil, err
			}
			y1, err := num()
			if err != nil {
				return nil, err
			}
			x, err := num()
			if err != nil {
				return nil, err
			}
			y, err := num()
			if err != nil {
				return nil, err
			}
			if curCmd == 'q' {
				x1 += cx
				y1 += cy
				x += cx
				y += cy
			}
			ops = append(ops, svgPathOp{kind: 'Q', args: [7]float64{x1, y1, x, y}})
			cx, cy = x, y
			lastQuadCX, lastQuadCY = x1, y1
			hasLastQuad, hasLastCubic = true, false
		case 'T', 't':
			x, err := num()
			if err != nil {
				return nil, err
			}
			y, err := num()
			if err != nil {
				return nil, err
			}
			if curCmd == 't' {
				x += cx
				y += cy
			}
			var x1, y1 float64
			if hasLastQuad {
				x1 = 2*cx - lastQuadCX
				y1 = 2*cy - lastQuadCY
			} else {
				x1, y1 = cx, cy
			}
			ops = append(ops, svgPathOp{kind: 'Q', args: [7]float64{x1, y1, x, y}})
			cx, cy = x, y
			lastQuadCX, lastQuadCY = x1, y1
			hasLastQuad, hasLastCubic = true, false
		case 'A', 'a':
			rx, err := num()
			if err != nil {
				return nil, err
			}
			ry, err := num()
			if err != nil {
				return nil, err
			}
			xRot, err := num()
			if err != nil {
				return nil, err
			}
			large, err := num()
			if err != nil {
				return nil, err
			}
			sweep, err := num()
			if err != nil {
				return nil, err
			}
			x, err := num()
			if err != nil {
				return nil, err
			}
			y, err := num()
			if err != nil {
				return nil, err
			}
			if curCmd == 'a' {
				x += cx
				y += cy
			}
			beziers := decomposeArcToBeziers(cx, cy, x, y, rx, ry, xRot, large != 0, sweep != 0)
			ops = append(ops, beziers...)
			cx, cy = x, y
			hasLastCubic, hasLastQuad = false, false
		case 'Z', 'z':
			ops = append(ops, svgPathOp{kind: 'Z'})
			cx, cy = startX, startY
			hasLastCubic, hasLastQuad = false, false
		default:
			return nil, fmt.Errorf("unknown path command %c", curCmd)
		}
		if i >= len(tokens) {
			break
		}
	}
	return ops, nil
}

// decomposeArcToBeziers converts an SVG elliptical arc to 1-4 cubic Béziers
// per SVG implementation notes (endpoint-to-center conversion + de Casteljau formula).
func decomposeArcToBeziers(x1, y1, x2, y2, rx, ry, xRotDeg float64, large, sweep bool) []svgPathOp {
	if rx == 0 || ry == 0 {
		return []svgPathOp{{kind: 'L', args: [7]float64{x2, y2}}}
	}
	rx = math.Abs(rx)
	ry = math.Abs(ry)
	xRot := xRotDeg * math.Pi / 180
	cosR, sinR := math.Cos(xRot), math.Sin(xRot)

	dx := (x1 - x2) / 2
	dy := (y1 - y2) / 2
	x1p := cosR*dx + sinR*dy
	y1p := -sinR*dx + cosR*dy

	rxSq, rySq := rx*rx, ry*ry
	x1pSq, y1pSq := x1p*x1p, y1p*y1p
	lambda := x1pSq/rxSq + y1pSq/rySq
	if lambda > 1 {
		s := math.Sqrt(lambda)
		rx *= s
		ry *= s
		rxSq, rySq = rx*rx, ry*ry
	}

	sign := 1.0
	if large == sweep {
		sign = -1
	}
	numer := rxSq*rySq - rxSq*y1pSq - rySq*x1pSq
	if numer < 0 {
		numer = 0
	}
	denom := rxSq*y1pSq + rySq*x1pSq
	coef := sign * math.Sqrt(numer/denom)
	cxp := coef * (rx * y1p / ry)
	cyp := coef * (-ry * x1p / rx)

	cx := cosR*cxp - sinR*cyp + (x1+x2)/2
	cy := sinR*cxp + cosR*cyp + (y1+y2)/2

	angle := func(ux, uy, vx, vy float64) float64 {
		dot := ux*vx + uy*vy
		l := math.Hypot(ux, uy) * math.Hypot(vx, vy)
		a := math.Acos(math.Max(-1, math.Min(1, dot/l)))
		if ux*vy-uy*vx < 0 {
			a = -a
		}
		return a
	}
	startA := angle(1, 0, (x1p-cxp)/rx, (y1p-cyp)/ry)
	sweepA := angle((x1p-cxp)/rx, (y1p-cyp)/ry, (-x1p-cxp)/rx, (-y1p-cyp)/ry)
	if !sweep && sweepA > 0 {
		sweepA -= 2 * math.Pi
	}
	if sweep && sweepA < 0 {
		sweepA += 2 * math.Pi
	}

	segs := int(math.Ceil(math.Abs(sweepA) / (math.Pi / 2)))
	if segs == 0 {
		segs = 1
	}
	step := sweepA / float64(segs)
	out := make([]svgPathOp, 0, segs)
	a := startA
	for k := 0; k < segs; k++ {
		b := a + step
		t := math.Tan(step / 4)
		alpha := math.Sin(step) * (math.Sqrt(4+3*t*t) - 1) / 3
		ax, ay := math.Cos(a), math.Sin(a)
		bx, by := math.Cos(b), math.Sin(b)
		c1x := ax - alpha*ay
		c1y := ay + alpha*ax
		c2x := bx + alpha*by
		c2y := by - alpha*bx
		toPDF := func(px, py float64) (float64, float64) {
			px *= rx
			py *= ry
			rx2 := cosR*px - sinR*py
			ry2 := sinR*px + cosR*py
			return rx2 + cx, ry2 + cy
		}
		c1xt, c1yt := toPDF(c1x, c1y)
		c2xt, c2yt := toPDF(c2x, c2y)
		bxt, byt := toPDF(bx, by)
		out = append(out, svgPathOp{kind: 'C', args: [7]float64{c1xt, c1yt, c2xt, c2yt, bxt, byt}})
		a = b
	}
	return out
}
