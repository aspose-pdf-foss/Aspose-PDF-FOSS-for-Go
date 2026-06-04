// SPDX-License-Identifier: MIT

package asposepdf

import "encoding/binary"

// glyphPoint is a control point of a glyph outline in font design units, with
// on/off-curve flag (TrueType quadratic outlines).
type glyphPoint struct {
	x, y float64
	on   bool
}

// glyphContour is one closed contour of a glyph outline.
type glyphContour []glyphPoint

// tableDir re-parses the sfnt table directory of a font program.
func tableDir(data []byte) map[string]tableRecord {
	if len(data) < 12 {
		return nil
	}
	n := int(binary.BigEndian.Uint16(data[4:6]))
	if len(data) < 12+n*16 {
		return nil
	}
	t := make(map[string]tableRecord, n)
	for i := 0; i < n; i++ {
		off := 12 + i*16
		t[string(data[off:off+4])] = tableRecord{
			offset: binary.BigEndian.Uint32(data[off+8 : off+12]),
			length: binary.BigEndian.Uint32(data[off+12 : off+16]),
		}
	}
	return t
}

// glyphContours returns the outline of glyph gid as contours in font design
// units. Returns nil for an empty glyph (e.g. space), a font without glyf/loca,
// or on any malformed data (recovered).
func (f *ttfFont) glyphContours(gid uint16) (contours []glyphContour) {
	defer func() {
		if recover() != nil {
			contours = nil
		}
	}()
	tables := tableDir(f.data)
	if tables == nil {
		return nil
	}
	loca, err := readLoca(f, tables)
	if err != nil {
		return nil
	}
	glyf := tableSlice(f.data, tables, "glyf")
	if glyf == nil {
		return nil
	}
	return f.glyphContoursRec(gid, loca, glyf, 0)
}

func (f *ttfFont) glyphContoursRec(gid uint16, loca []uint32, glyf []byte, depth int) []glyphContour {
	if depth > 8 || int(gid)+1 >= len(loca) {
		return nil
	}
	start, end := loca[gid], loca[gid+1]
	if start >= end || end > uint32(len(glyf)) {
		return nil // empty glyph
	}
	b := glyf[start:end]
	if len(b) < 10 {
		return nil
	}
	numContours := int(int16(binary.BigEndian.Uint16(b[0:2])))
	if numContours < 0 {
		return f.parseCompositeGlyph(b, loca, glyf, depth)
	}
	return parseSimpleGlyph(b, numContours)
}

// parseSimpleGlyph decodes a simple (non-composite) glyph's contours.
func parseSimpleGlyph(b []byte, numContours int) []glyphContour {
	pos := 10 // skip numberOfContours(2) + xMin/yMin/xMax/yMax(8)
	endPts := make([]int, numContours)
	for i := 0; i < numContours; i++ {
		endPts[i] = int(binary.BigEndian.Uint16(b[pos:]))
		pos += 2
	}
	if numContours == 0 {
		return nil
	}
	numPoints := endPts[numContours-1] + 1
	instrLen := int(binary.BigEndian.Uint16(b[pos:]))
	pos += 2 + instrLen

	const (
		onCurve  = 0x01
		xShort   = 0x02
		yShort   = 0x04
		repeat   = 0x08
		xSamePos = 0x10
		ySamePos = 0x20
	)

	flags := make([]byte, numPoints)
	for i := 0; i < numPoints; {
		fl := b[pos]
		pos++
		flags[i] = fl
		i++
		if fl&repeat != 0 {
			rep := int(b[pos])
			pos++
			for r := 0; r < rep && i < numPoints; r++ {
				flags[i] = fl
				i++
			}
		}
	}

	xs := make([]int, numPoints)
	x := 0
	for i := 0; i < numPoints; i++ {
		fl := flags[i]
		switch {
		case fl&xShort != 0:
			dx := int(b[pos])
			pos++
			if fl&xSamePos == 0 {
				dx = -dx
			}
			x += dx
		case fl&xSamePos == 0:
			x += int(int16(binary.BigEndian.Uint16(b[pos:])))
			pos += 2
		}
		xs[i] = x
	}
	ys := make([]int, numPoints)
	y := 0
	for i := 0; i < numPoints; i++ {
		fl := flags[i]
		switch {
		case fl&yShort != 0:
			dy := int(b[pos])
			pos++
			if fl&ySamePos == 0 {
				dy = -dy
			}
			y += dy
		case fl&ySamePos == 0:
			y += int(int16(binary.BigEndian.Uint16(b[pos:])))
			pos += 2
		}
		ys[i] = y
	}

	contours := make([]glyphContour, 0, numContours)
	start := 0
	for c := 0; c < numContours; c++ {
		end := endPts[c]
		ct := make(glyphContour, 0, end-start+1)
		for i := start; i <= end && i < numPoints; i++ {
			ct = append(ct, glyphPoint{x: float64(xs[i]), y: float64(ys[i]), on: flags[i]&onCurve != 0})
		}
		contours = append(contours, ct)
		start = end + 1
	}
	return contours
}

// parseCompositeGlyph decodes a composite glyph by transforming its components.
func (f *ttfFont) parseCompositeGlyph(b []byte, loca []uint32, glyf []byte, depth int) []glyphContour {
	const (
		argsAreWords = 0x0001
		argsAreXY    = 0x0002
		haveScale    = 0x0008
		moreComps    = 0x0020
		haveXYScale  = 0x0040
		have2x2      = 0x0080
	)
	pos := 10
	var out []glyphContour
	for {
		flags := binary.BigEndian.Uint16(b[pos:])
		pos += 2
		compGID := binary.BigEndian.Uint16(b[pos:])
		pos += 2

		var dx, dy float64
		if flags&argsAreWords != 0 {
			dx = float64(int16(binary.BigEndian.Uint16(b[pos:])))
			dy = float64(int16(binary.BigEndian.Uint16(b[pos+2:])))
			pos += 4
		} else {
			dx = float64(int8(b[pos]))
			dy = float64(int8(b[pos+1]))
			pos += 2
		}
		a, bb, cc, d := 1.0, 0.0, 0.0, 1.0
		switch {
		case flags&haveScale != 0:
			a = f2dot14(b[pos:])
			d = a
			pos += 2
		case flags&haveXYScale != 0:
			a = f2dot14(b[pos:])
			d = f2dot14(b[pos+2:])
			pos += 4
		case flags&have2x2 != 0:
			a = f2dot14(b[pos:])
			bb = f2dot14(b[pos+2:])
			cc = f2dot14(b[pos+4:])
			d = f2dot14(b[pos+6:])
			pos += 8
		}

		if flags&argsAreXY != 0 { // point-matching args are not supported
			for _, ct := range f.glyphContoursRec(compGID, loca, glyf, depth+1) {
				tc := make(glyphContour, len(ct))
				for i, p := range ct {
					tc[i] = glyphPoint{x: a*p.x + cc*p.y + dx, y: bb*p.x + d*p.y + dy, on: p.on}
				}
				out = append(out, tc)
			}
		}
		if flags&moreComps == 0 {
			break
		}
	}
	return out
}

func f2dot14(b []byte) float64 {
	return float64(int16(binary.BigEndian.Uint16(b))) / 16384.0
}
