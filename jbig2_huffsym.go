// SPDX-License-Identifier: MIT

package asposepdf

// Huffman-coded symbol dictionary decoding (ITU-T T.88 §6.5 with SDHUFF=1,
// SDREFAGG=0): symbols are grouped into height classes; each class records its
// symbols' widths via the DW table, then a single collective bitmap (MMR-coded,
// or uncompressed when BMSIZE=0) holds all the class's symbols side by side and
// is split back into individual symbols by width.

// customTables hands out the referred-to custom Huffman tables (type-53 segments)
// in order to each selector flagged "custom" (value 3).
type customTables struct {
	tables []*huffTable
	idx    int
}

func (c *customTables) next() *huffTable {
	if c == nil || c.idx >= len(c.tables) {
		return nil
	}
	t := c.tables[c.idx]
	c.idx++
	return t
}

// pickTable returns the standard table for sel, or the next custom table when
// sel selects "custom"; stdByFlag maps each non-custom flag value to a standard
// table number.
func pickTable(sel int, stdByFlag map[int]int, custom *customTables) *huffTable {
	if n, ok := stdByFlag[sel]; ok {
		return jbig2StdTables[n]
	}
	if t := custom.next(); t != nil {
		return t
	}
	return jbig2StdTables[1] // defensive fallback
}

func decodeHuffSymbolDict(r *jbig2Reader, dhTable, dwTable, bmTable, exTable *huffTable, input []jbig2Bitmap, numNew, numEx int) []jbig2Bitmap {
	newSyms := make([]jbig2Bitmap, 0, numNew)
	hcHeight := 0
	guard := 0
	for len(newSyms) < numNew {
		dh, _ := dhTable.decode(r)
		hcHeight += dh
		if hcHeight <= 0 || hcHeight > 1<<16 {
			break
		}
		symWidth, totWidth := 0, 0
		var classWidths []int
		for {
			dw, oob := dwTable.decode(r)
			if oob {
				break // end of height class
			}
			symWidth += dw
			if symWidth <= 0 || symWidth > 1<<16 || len(newSyms)+len(classWidths) >= numNew {
				break
			}
			totWidth += symWidth
			classWidths = append(classWidths, symWidth)
		}
		if len(classWidths) == 0 {
			if guard++; guard > numNew+8 {
				break
			}
			continue
		}

		bmsize, _ := bmTable.decode(r)
		r.align()
		var collective jbig2Bitmap
		if bmsize == 0 {
			collective = newJBIG2Bitmap(totWidth, hcHeight)
			for y := 0; y < hcHeight && y < len(collective); y++ {
				for x := 0; x < totWidth; x++ {
					collective[y][x] = byte(r.readBit())
				}
				r.align()
			}
		} else {
			end := r.bytePos + bmsize
			if end > len(r.data) {
				end = len(r.data)
			}
			collective = jbig2MMRDecode(r.data[r.bytePos:end], totWidth, hcHeight)
			r.bytePos = end
		}

		x := 0
		for _, w := range classWidths {
			sym := newJBIG2Bitmap(w, hcHeight)
			for yy := 0; yy < hcHeight && yy < len(sym) && yy < len(collective); yy++ {
				for xx := 0; xx < w && x+xx < totWidth; xx++ {
					sym[yy][xx] = collective[yy][x+xx]
				}
			}
			newSyms = append(newSyms, sym)
			x += w
		}
		if guard++; guard > numNew+8 {
			break
		}
	}

	// Exported-symbol flags: run-lengths decoded with the EX table (B.1).
	all := append(append([]jbig2Bitmap{}, input...), newSyms...)
	total := len(all)
	flags := make([]bool, 0, total)
	cur := false
	g := 0
	for len(flags) < total {
		run, oob := exTable.decode(r)
		if oob || run < 0 {
			break
		}
		for k := 0; k < run && len(flags) < total; k++ {
			flags = append(flags, cur)
		}
		cur = !cur
		if g++; g > total+8 {
			break
		}
	}
	exported := make([]jbig2Bitmap, 0, numEx)
	for i := 0; i < total && i < len(flags); i++ {
		if flags[i] {
			exported = append(exported, all[i])
		}
	}
	return exported
}
