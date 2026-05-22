// SPDX-License-Identifier: MIT

package asposepdf

import (
	"math"
	"strings"
)

func matrixIdentity() svgMatrix {
	return svgMatrix{1, 0, 0, 1, 0, 0}
}

// matrixMul returns A × B (column-vector convention).
// If point p = (x, y, 1), then (A × B) p means "first apply B, then A".
// For SVG composite transforms left-to-right, we accumulate result = result × M for each new M.
func matrixMul(a, b svgMatrix) svgMatrix {
	return svgMatrix{
		a[0]*b[0] + a[2]*b[1],
		a[1]*b[0] + a[3]*b[1],
		a[0]*b[2] + a[2]*b[3],
		a[1]*b[2] + a[3]*b[3],
		a[0]*b[4] + a[2]*b[5] + a[4],
		a[1]*b[4] + a[3]*b[5] + a[5],
	}
}

func matrixTranslate(tx, ty float64) svgMatrix {
	return svgMatrix{1, 0, 0, 1, tx, ty}
}

func matrixScale(sx, sy float64) svgMatrix {
	return svgMatrix{sx, 0, 0, sy, 0, 0}
}

func matrixRotate(deg float64) svgMatrix {
	r := deg * math.Pi / 180
	c, s := math.Cos(r), math.Sin(r)
	return svgMatrix{c, s, -s, c, 0, 0}
}

func matrixSkewX(deg float64) svgMatrix {
	return svgMatrix{1, 0, math.Tan(deg * math.Pi / 180), 1, 0, 0}
}

func matrixSkewY(deg float64) svgMatrix {
	return svgMatrix{1, math.Tan(deg * math.Pi / 180), 0, 1, 0, 0}
}

// parseSVGTransform parses one or more SVG transform functions joined by whitespace/commas.
// Returns identity for empty input. Returns ok=false if any function is malformed.
func parseSVGTransform(s string) (svgMatrix, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return matrixIdentity(), true
	}
	result := matrixIdentity()
	for len(s) > 0 {
		s = strings.TrimLeft(s, " \t\n\r,")
		if len(s) == 0 {
			break
		}
		openIdx := strings.IndexByte(s, '(')
		if openIdx < 0 {
			return matrixIdentity(), false
		}
		name := strings.TrimSpace(s[:openIdx])
		closeIdx := strings.IndexByte(s[openIdx:], ')')
		if closeIdx < 0 {
			return matrixIdentity(), false
		}
		body := s[openIdx+1 : openIdx+closeIdx]
		args, ok := parseSVGNumberList(body)
		if !ok {
			return matrixIdentity(), false
		}
		var m svgMatrix
		switch name {
		case "translate":
			switch len(args) {
			case 1:
				m = matrixTranslate(args[0], 0)
			case 2:
				m = matrixTranslate(args[0], args[1])
			default:
				return matrixIdentity(), false
			}
		case "scale":
			switch len(args) {
			case 1:
				m = matrixScale(args[0], args[0])
			case 2:
				m = matrixScale(args[0], args[1])
			default:
				return matrixIdentity(), false
			}
		case "rotate":
			switch len(args) {
			case 1:
				m = matrixRotate(args[0])
			case 3:
				m = matrixMul(matrixTranslate(args[1], args[2]),
					matrixMul(matrixRotate(args[0]), matrixTranslate(-args[1], -args[2])))
			default:
				return matrixIdentity(), false
			}
		case "matrix":
			if len(args) != 6 {
				return matrixIdentity(), false
			}
			m = svgMatrix{args[0], args[1], args[2], args[3], args[4], args[5]}
		case "skewX":
			if len(args) != 1 {
				return matrixIdentity(), false
			}
			m = matrixSkewX(args[0])
		case "skewY":
			if len(args) != 1 {
				return matrixIdentity(), false
			}
			m = matrixSkewY(args[0])
		default:
			return matrixIdentity(), false
		}
		result = matrixMul(result, m)
		s = s[openIdx+closeIdx+1:]
	}
	return result, true
}

// parseSVGNumberList parses a comma/space-separated list of floats.
func parseSVGNumberList(s string) ([]float64, bool) {
	s = strings.ReplaceAll(s, ",", " ")
	fields := strings.Fields(s)
	out := make([]float64, 0, len(fields))
	for _, f := range fields {
		n, ok := parseSVGNumber(f)
		if !ok {
			return nil, false
		}
		out = append(out, n)
	}
	return out, true
}
