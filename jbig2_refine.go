// SPDX-License-Identifier: MIT

package asposepdf

// Generic refinement-region decoding (ITU-T T.88 §6.3): a bitmap is decoded
// against a reference bitmap (shifted by an offset) using a small template that
// mixes already-decoded pixels of the output with pixels of the reference. Used
// by refinement/aggregate symbol dictionaries (SDREFAGG) and refinement text
// regions. Templates 0 and 1 are supported; TPGRON typical prediction is not yet
// (refinement symbol dictionaries in the wild leave it off).

type jbig2RefTemplate struct {
	coding, reference []jbig2Point
}

var refinementTemplates = [2]jbig2RefTemplate{
	{
		coding:    []jbig2Point{{0, -1}, {1, -1}, {-1, 0}},
		reference: []jbig2Point{{0, -1}, {1, -1}, {-1, 0}, {0, 0}, {1, 0}, {-1, 1}, {0, 1}, {1, 1}},
	},
	{
		coding:    []jbig2Point{{-1, -1}, {0, -1}, {1, -1}, {-1, 0}},
		reference: []jbig2Point{{0, -1}, {-1, 0}, {0, 0}, {1, 0}, {0, 1}, {1, 1}},
	},
}

// decodeRefinement decodes a width×height bitmap refined from referenceBitmap
// (offset by offsetX/offsetY), using refinement template templateIndex with the
// adaptive pixels at (template 0 uses at[0] for coding and at[1] for reference).
func decodeRefinement(width, height, templateIndex int, referenceBitmap jbig2Bitmap, offsetX, offsetY int, at []jbig2Point, dec *mqDecoder, cx []byte) jbig2Bitmap {
	bitmap := newJBIG2Bitmap(width, height)
	if len(bitmap) != height {
		return bitmap // size rejected (degenerate or too large)
	}

	coding := append([]jbig2Point{}, refinementTemplates[templateIndex].coding...)
	reference := append([]jbig2Point{}, refinementTemplates[templateIndex].reference...)
	if templateIndex == 0 {
		if len(at) > 0 {
			coding = append(coding, at[0])
		}
		if len(at) > 1 {
			reference = append(reference, at[1])
		}
	}

	for i := 0; i < height; i++ {
		row := bitmap[i]
		for j := 0; j < width; j++ {
			ctx := 0
			for _, t := range coding {
				iy := i + t.y
				jx := j + t.x
				bit := 0
				if iy >= 0 && iy < height && jx >= 0 && jx < width {
					bit = int(bitmap[iy][jx])
				}
				ctx = (ctx << 1) | bit
			}
			for _, t := range reference {
				iy := i + t.y - offsetY
				jx := j + t.x - offsetX
				bit := 0
				if iy >= 0 && iy < len(referenceBitmap) && jx >= 0 && jx < len(referenceBitmap[iy]) {
					bit = int(referenceBitmap[iy][jx])
				}
				ctx = (ctx << 1) | bit
			}
			row[j] = byte(dec.readBit(cx, ctx))
		}
	}
	return bitmap
}
