// SPDX-License-Identifier: MIT

package asposepdf

// jbig2Ctx holds the per-segment arithmetic coding contexts: one integer-decoder
// context per IAx procedure, the shared generic-region (GB) context, and the
// IAID symbol-ID context sized to the symbol code length.
type jbig2Ctx struct {
	iadh, iadw, iaex, iaai           *mqIntCtx
	iadt, iafs, iads, iait           *mqIntCtx
	iari, iardw, iardh, iardx, iardy *mqIntCtx
	gb                               []byte
	gr                               []byte
	iaid                             []byte
}

func newJBIG2Ctx(symCodeLen int) *jbig2Ctx {
	if symCodeLen < 1 {
		symCodeLen = 1
	}
	return &jbig2Ctx{
		iadh: newMQIntCtx(), iadw: newMQIntCtx(), iaex: newMQIntCtx(), iaai: newMQIntCtx(),
		iadt: newMQIntCtx(), iafs: newMQIntCtx(), iads: newMQIntCtx(), iait: newMQIntCtx(),
		iari: newMQIntCtx(), iardw: newMQIntCtx(), iardh: newMQIntCtx(),
		iardx: newMQIntCtx(), iardy: newMQIntCtx(),
		gb:   make([]byte, 1<<16),
		gr:   make([]byte, 1<<13),
		iaid: make([]byte, 1<<uint(symCodeLen+1)),
	}
}

// jbig2Log2 returns the number of bits needed to code values 0..x, matching the
// reference SBSYMCODELEN / symbol-code-length convention (min 1).
func jbig2Log2(x int) int {
	if x <= 0 {
		return 0
	}
	n := 1
	for x > (1 << uint(n)) {
		n++
	}
	return n
}

// symbolDictParams gathers the symbol-dictionary parameters for decoding.
type symbolDictParams struct {
	template   int
	at         []jbig2Point
	refagg     bool         // SDREFAGG: new symbols coded by refinement/aggregation
	rTemplate  int          // SDRTEMPLATE
	rat        []jbig2Point // SDRAT refinement adaptive pixels
	symCodeLen int          // SDSYMCODELEN, for IAID in the refagg path
	numNew     int
	numEx      int
}

// decodeSymbolDict decodes a symbol dictionary segment (ITU-T T.88 §6.5) in the
// arithmetic path: symbols are grouped into height classes. Each symbol bitmap is
// decoded either as a generic region (refagg off) or — when SDREFAGG is set — by
// refining a previously-decoded symbol (REFAGGNINST==1) or by an aggregate text
// region (REFAGGNINST>1). An export run-length list (IAEX) then selects which
// input+new symbols this dictionary exports.
func (dec *mqDecoder) decodeSymbolDict(ctx *jbig2Ctx, p symbolDictParams, inputSymbols []jbig2Bitmap) []jbig2Bitmap {
	numNew, numEx := p.numNew, p.numEx
	newSymbols := make([]jbig2Bitmap, 0, numNew)
	currentHeight := 0
	for len(newSymbols) < numNew {
		dh, _ := dec.decodeInt(ctx.iadh)
		currentHeight += dh
		if currentHeight <= 0 || currentHeight > 1<<20 {
			break
		}
		currentWidth := 0
		for {
			dw, oob := dec.decodeInt(ctx.iadw)
			if oob {
				break // end of this height class
			}
			currentWidth += dw
			if currentWidth <= 0 || currentWidth > 1<<20 || len(newSymbols) >= numNew {
				break
			}
			var bm jbig2Bitmap
			if p.refagg {
				bm = dec.decodeRefAggSymbol(ctx, p, currentWidth, currentHeight, inputSymbols, newSymbols)
			} else {
				bm = decodeGenericBitmap(currentWidth, currentHeight, p.template, false, nil, p.at, dec, ctx.gb)
			}
			newSymbols = append(newSymbols, bm)
		}
	}

	all := append(append([]jbig2Bitmap{}, inputSymbols...), newSymbols...)
	total := len(all)
	flags := make([]bool, 0, total)
	cur := false
	guard := 0
	for len(flags) < total {
		run, _ := dec.decodeInt(ctx.iaex)
		if run < 0 {
			break
		}
		for k := 0; k < run && len(flags) < total; k++ {
			flags = append(flags, cur)
		}
		cur = !cur
		if guard++; guard > total+8 {
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

// decodeRefAggSymbol decodes one symbol in a refinement/aggregate dictionary
// (SDREFAGG): REFAGGNINST==1 refines a single referenced symbol; >1 aggregates
// several via a refining text region (ITU-T T.88 §6.5.8.2).
func (dec *mqDecoder) decodeRefAggSymbol(ctx *jbig2Ctx, p symbolDictParams, width, height int, input, newSyms []jbig2Bitmap) jbig2Bitmap {
	nInst, _ := dec.decodeInt(ctx.iaai)
	if nInst == 1 {
		id := dec.decodeIAID(ctx.iaid, p.symCodeLen)
		rdx, _ := dec.decodeInt(ctx.iardx)
		rdy, _ := dec.decodeInt(ctx.iardy)
		var ref jbig2Bitmap
		if id >= 0 {
			if id < len(input) {
				ref = input[id]
			} else if id-len(input) < len(newSyms) {
				ref = newSyms[id-len(input)]
			}
		}
		return decodeRefinement(width, height, p.rTemplate, ref, rdx, rdy, p.rat, dec, ctx.gr)
	}
	// REFAGGNINST > 1: aggregate via a refining text region over all symbols
	// available so far.
	all := append(append([]jbig2Bitmap{}, input...), newSyms...)
	return dec.decodeTextRegion(ctx, all, textRegionParams{
		width: width, height: height, numInstances: nInst, symCodeLen: p.symCodeLen,
		logStrips: 0, refCorner: jbig2RefTopLeft, transposed: false,
		combOp: 0, dsOffset: 0, defPixel: 0, refine: true, rTemplate: p.rTemplate, rat: p.rat,
	})
}
