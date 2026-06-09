// SPDX-License-Identifier: MIT

package asposepdf

import "sort"

// jbig2Bitmap is a height×width bilevel image, one byte per pixel (0 or 1).
type jbig2Bitmap [][]byte

func newJBIG2Bitmap(w, h int) jbig2Bitmap {
	if w <= 0 || h <= 0 || w > 1<<16 || h > 1<<16 {
		return jbig2Bitmap{}
	}
	b := make(jbig2Bitmap, h)
	for i := range b {
		b[i] = make([]byte, w)
	}
	return b
}

// jbig2Point is a template pixel offset relative to the pixel being decoded.
type jbig2Point struct{ x, y int }

// codingTemplates are the fixed neighbour pixels for generic-region templates
// 0–3 (ITU-T T.88 §6.2.5.3, Figures 4–7), excluding the adaptive (AT) pixels
// which the segment header supplies separately.
var codingTemplates = [4][]jbig2Point{
	{{-1, -2}, {0, -2}, {1, -2}, {-2, -1}, {-1, -1}, {0, -1}, {1, -1}, {2, -1}, {-4, 0}, {-3, 0}, {-2, 0}, {-1, 0}},
	{{-1, -2}, {0, -2}, {1, -2}, {2, -2}, {-2, -1}, {-1, -1}, {0, -1}, {1, -1}, {2, -1}, {-3, 0}, {-2, 0}, {-1, 0}},
	{{-1, -2}, {0, -2}, {1, -2}, {-2, -1}, {-1, -1}, {0, -1}, {1, -1}, {-2, 0}, {-1, 0}},
	{{-3, -1}, {-2, -1}, {-1, -1}, {0, -1}, {1, -1}, {-4, 0}, {-3, 0}, {-2, 0}, {-1, 0}},
}

// reusedContexts are the context labels used to decode the SLTP "typical
// prediction" bit per template when TPGDON is on (ITU-T T.88 §6.2.5.7).
var reusedContexts = [4]int{0x9b25, 0x0795, 0x00e5, 0x0195}

// decodeGenericBitmap decodes an arithmetically-coded generic region (ITU-T T.88
// §6.2) into a bilevel bitmap. at holds the adaptive template pixels; prediction
// enables TPGDON typical-prediction; skip, when non-nil, marks pixels forced to
// 0. cx is the shared GB context array (size 1<<16).
func decodeGenericBitmap(width, height, templateIndex int, prediction bool, skip jbig2Bitmap, at []jbig2Point, dec *mqDecoder, cx []byte) jbig2Bitmap {
	bitmap := newJBIG2Bitmap(width, height)
	if len(bitmap) != height {
		return bitmap // size rejected (degenerate or too large)
	}

	template := append([]jbig2Point{}, codingTemplates[templateIndex]...)
	template = append(template, at...)
	sort.SliceStable(template, func(i, j int) bool {
		if template[i].y != template[j].y {
			return template[i].y < template[j].y
		}
		return template[i].x < template[j].x
	})
	tl := len(template)

	ltp := 0
	for i := 0; i < height; i++ {
		if prediction {
			ltp ^= dec.readBit(cx, reusedContexts[templateIndex])
			if ltp == 1 {
				if i > 0 {
					copy(bitmap[i], bitmap[i-1])
				}
				continue
			}
		}
		row := bitmap[i]
		for j := 0; j < width; j++ {
			if skip != nil && skip[i][j] != 0 {
				row[j] = 0
				continue
			}
			ctx := 0
			for k := 0; k < tl; k++ {
				iy := i + template[k].y
				jx := j + template[k].x
				bit := 0
				if iy >= 0 && jx >= 0 && jx < width {
					bit = int(bitmap[iy][jx])
				}
				ctx = (ctx << 1) | bit
			}
			row[j] = byte(dec.readBit(cx, ctx))
		}
	}
	return bitmap
}
