// SPDX-License-Identifier: MIT

package asposepdf

// Reference-corner codes (ITU-T T.88 §6.4.5 / Table 31).
const (
	jbig2RefBottomLeft  = 0
	jbig2RefTopLeft     = 1
	jbig2RefBottomRight = 2
	jbig2RefTopRight    = 3
)

// textRegionParams gathers the text-region segment parameters needed by the
// arithmetic decoder (Huffman/refinement variants are out of Phase 1 scope).
type textRegionParams struct {
	width, height int
	numInstances  int
	symCodeLen    int
	logStrips     int
	refCorner     int
	transposed    bool
	combOp        int
	dsOffset      int
	defPixel      int
	refine        bool         // SBREFINE: instances may be refined before placing
	rTemplate     int          // SBRTEMPLATE
	rat           []jbig2Point // refinement adaptive pixels
}

// decodeTextRegion decodes a text-region segment (ITU-T T.88 §6.4) in the
// arithmetic, non-refinement path: it walks strips of symbol instances, reading
// each instance's strip-T offset (IADT/IAIT), S position (IAFS/IADS) and symbol
// id (IAID), and blits the referenced symbol bitmap onto the region.
func (dec *mqDecoder) decodeTextRegion(ctx *jbig2Ctx, symbols []jbig2Bitmap, p textRegionParams) jbig2Bitmap {
	bitmap := newJBIG2Bitmap(p.width, p.height)
	if len(bitmap) != p.height || p.width <= 0 {
		return bitmap // size rejected
	}
	if p.defPixel != 0 {
		for i := range bitmap {
			for j := range bitmap[i] {
				bitmap[i][j] = 1
			}
		}
	}
	if p.width <= 0 || p.height <= 0 {
		return bitmap
	}
	stripSize := 1 << uint(p.logStrips)

	dt0, _ := dec.decodeInt(ctx.iadt)
	stripT := -dt0
	firstS := 0
	i := 0
	stripGuard := 0
	for i < p.numInstances {
		dt, _ := dec.decodeInt(ctx.iadt)
		stripT += dt
		dfs, _ := dec.decodeInt(ctx.iafs)
		firstS += dfs
		curS := firstS

		// Decode each symbol instance in the strip until IADS returns OOB. The
		// first instance uses curS = firstS; every instance reads IADS *after*
		// it (so the terminating OOB that ends the strip is always consumed).
		for {
			curT := 0
			if stripSize > 1 {
				curT, _ = dec.decodeInt(ctx.iait)
			}
			T := stripT*stripSize + curT

			id := dec.decodeIAID(ctx.iaid, p.symCodeLen)
			var sym jbig2Bitmap
			if id >= 0 && id < len(symbols) {
				sym = symbols[id]
			}
			wi, hi := 0, 0
			if len(sym) > 0 {
				hi = len(sym)
				wi = len(sym[0])
			}

			if p.refine {
				ri, _ := dec.decodeInt(ctx.iari)
				if ri != 0 {
					rdw, _ := dec.decodeInt(ctx.iardw)
					rdh, _ := dec.decodeInt(ctx.iardh)
					rdx, _ := dec.decodeInt(ctx.iardx)
					rdy, _ := dec.decodeInt(ctx.iardy)
					nw, nh := wi+rdw, hi+rdh
					sym = decodeRefinement(nw, nh, p.rTemplate, sym, (rdw>>1)+rdx, (rdh>>1)+rdy, p.rat, dec, ctx.gr)
					wi, hi = nw, nh
				}
			}

			if p.transposed {
				x0 := T
				y0 := curS
				if p.refCorner >= 2 { // right corner: TOPRIGHT/BOTTOMRIGHT
					x0 = T - (wi - 1)
				}
				jbig2Blit(bitmap, sym, x0, y0, p.combOp)
				curS += hi - 1
			} else {
				x0 := curS
				y0 := T
				if p.refCorner&1 == 0 { // bottom corner: BOTTOMLEFT/BOTTOMRIGHT
					y0 = T - (hi - 1)
				}
				jbig2Blit(bitmap, sym, x0, y0, p.combOp)
				curS += wi - 1
			}
			i++

			ids, oob := dec.decodeInt(ctx.iads)
			if oob {
				break // end of strip
			}
			curS += ids + p.dsOffset
			if i >= p.numInstances {
				break // all instances placed (malformed stream guard)
			}
		}
		if stripGuard++; stripGuard > p.numInstances+8 {
			break
		}
	}
	return bitmap
}

// jbig2Blit draws src into dst with its top-left at (x0,y0) using the JBIG2
// combination operator (0 OR, 1 AND, 2 XOR, 3 XNOR, 4 replace). Pixels outside
// dst are clipped.
func jbig2Blit(dst, src jbig2Bitmap, x0, y0, op int) {
	if len(src) == 0 {
		return
	}
	h := len(dst)
	if h == 0 {
		return
	}
	w := len(dst[0])
	for sy := 0; sy < len(src); sy++ {
		dy := y0 + sy
		if dy < 0 || dy >= h {
			continue
		}
		srow := src[sy]
		drow := dst[dy]
		for sx := 0; sx < len(srow); sx++ {
			dx := x0 + sx
			if dx < 0 || dx >= w {
				continue
			}
			s := srow[sx]
			switch op {
			case 1:
				drow[dx] &= s
			case 2:
				drow[dx] ^= s
			case 3:
				drow[dx] = 1 ^ (drow[dx] ^ s)
			case 4:
				drow[dx] = s
			default:
				drow[dx] |= s
			}
		}
	}
}
