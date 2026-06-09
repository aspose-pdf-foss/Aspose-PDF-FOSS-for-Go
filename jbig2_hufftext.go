// SPDX-License-Identifier: MIT

package asposepdf

// Huffman-coded text region decoding (ITU-T T.88 §6.4 with SBHUFF=1). The symbol
// instances are read with Huffman tables instead of arithmetic integer decoders,
// and the symbol ids use a per-segment table built by the run-code procedure
// (§7.4.3.1.7).

// huffTextParams carries the Huffman text-region parameters and its selected
// tables.
type huffTextParams struct {
	width, height int
	numInstances  int
	logStrips     int
	refCorner     int
	transposed    bool
	combOp        int
	dsOffset      int
	defPixel      int
	symIDTable    *huffTable
	fsTable       *huffTable
	dsTable       *huffTable
	dtTable       *huffTable
}

// buildSymbolIDTable reads the symbol-id Huffman table (ITU-T T.88 §7.4.3.1.7):
// 35 four-bit run-code lengths build a run-code table, which then decodes the
// per-symbol code lengths (with the 32/33/34 repeat codes); those lengths build
// the final symbol-id table.
func buildSymbolIDTable(r *jbig2Reader, numSyms int) *huffTable {
	runLines := make([]huffLine, 0, 35)
	for i := 0; i < 35; i++ {
		runLines = append(runLines, huffLine{prefLen: r.readBits(4), rangeLow: i})
	}
	runTable := newHuffTable(runLines)

	codeLens := make([]int, numSyms)
	prev := 0
	i := 0
	guard := 0
	for i < numSyms {
		rc, _ := runTable.decode(r)
		switch {
		case rc < 32:
			codeLens[i] = rc
			prev = rc
			i++
		case rc == 32:
			n := r.readBits(2) + 3
			for k := 0; k < n && i < numSyms; k++ {
				codeLens[i] = prev
				i++
			}
		case rc == 33:
			n := r.readBits(3) + 3
			for k := 0; k < n && i < numSyms; k++ {
				codeLens[i] = 0
				i++
			}
		case rc == 34:
			n := r.readBits(7) + 11
			for k := 0; k < n && i < numSyms; k++ {
				codeLens[i] = 0
				i++
			}
		default:
			i++
		}
		if guard++; guard > numSyms+64 {
			break
		}
	}

	lines := make([]huffLine, 0, numSyms)
	for idx, l := range codeLens {
		lines = append(lines, huffLine{prefLen: l, rangeLow: idx})
	}
	r.align()
	return newHuffTable(lines)
}

func decodeHuffTextRegion(r *jbig2Reader, symbols []jbig2Bitmap, p huffTextParams) jbig2Bitmap {
	bitmap := newJBIG2Bitmap(p.width, p.height)
	if len(bitmap) != p.height || p.width <= 0 {
		return bitmap
	}
	if p.defPixel != 0 {
		for i := range bitmap {
			for j := range bitmap[i] {
				bitmap[i][j] = 1
			}
		}
	}
	stripSize := 1 << uint(p.logStrips)

	dt0, _ := p.dtTable.decode(r)
	stripT := -dt0
	firstS := 0
	i := 0
	stripGuard := 0
	for i < p.numInstances {
		dt, _ := p.dtTable.decode(r)
		stripT += dt
		dfs, _ := p.fsTable.decode(r)
		firstS += dfs
		curS := firstS
		for {
			curT := 0
			if stripSize > 1 {
				curT = r.readBits(p.logStrips)
			}
			T := stripT*stripSize + curT

			id, _ := p.symIDTable.decode(r)
			var sym jbig2Bitmap
			if id >= 0 && id < len(symbols) {
				sym = symbols[id]
			}
			wi, hi := 0, 0
			if len(sym) > 0 {
				hi = len(sym)
				wi = len(sym[0])
			}

			if p.transposed {
				x0 := T
				y0 := curS
				if p.refCorner >= 2 {
					x0 = T - (wi - 1)
				}
				jbig2Blit(bitmap, sym, x0, y0, p.combOp)
				curS += hi - 1
			} else {
				x0 := curS
				y0 := T
				if p.refCorner&1 == 0 {
					y0 = T - (hi - 1)
				}
				jbig2Blit(bitmap, sym, x0, y0, p.combOp)
				curS += wi - 1
			}
			i++

			ids, oob := p.dsTable.decode(r)
			if oob {
				break
			}
			curS += ids + p.dsOffset
			if i >= p.numInstances {
				break
			}
		}
		if stripGuard++; stripGuard > p.numInstances+8 {
			break
		}
	}
	return bitmap
}
