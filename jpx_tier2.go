// SPDX-License-Identifier: MIT

package asposepdf

import "math"

// JPEG2000 tier-2: tile/resolution/subband/precinct/code-block geometry, the
// packet iterators (all five progression orders), tag trees for code-block
// inclusion and zero-bit-planes, and the packet-header parser. Faithful port of
// pdf.js (jpx.js).

type jpxResolution struct {
	trx0, try0, trx1, try1 int
	resLevel               int
	precinct               jpxPrecinctParams
	subbands               []*jpxSubband
}

type jpxPrecinctParams struct {
	precinctWidth, precinctHeight                 int
	numprecinctswide, numprecinctshigh, numprecincts int
	precinctWidthInSubband, precinctHeightInSubband  int
}

type jpxSubband struct {
	typ                    string // "LL","HL","LH","HH"
	tbx0, tby0, tbx1, tby1 int
	resolution             *jpxResolution
	codeblocks             []*jpxCodeblock
	precincts              map[int]*jpxPrecinct
	cbWidthLog2, cbHeightLog2 int
}

type jpxChunk struct {
	data         []byte
	start, end   int
	codingpasses int
}

type jpxCodeblock struct {
	cbx, cby               int
	tbx0, tby0, tbx1, tby1 int
	tbx0_, tby0_, tbx1_, tby1_ int
	precinctNumber         int
	subbandType            string
	Lblock                 int
	includedSet            bool
	zeroBitPlanes          int
	data                   []jpxChunk
	precinct               *jpxPrecinct
}

type jpxPrecinct struct {
	cbxMin, cbyMin, cbxMax, cbyMax int
	inclusionTree                  *jpxInclusionTree
	zeroBitPlanesTree              *jpxTagTree
	hasTrees                       bool
}

type jpxPacket struct {
	layerNumber int
	codeblocks  []*jpxCodeblock
}

func jpxCalcTileGrids(ctx *jpxContext) {
	siz := ctx.SIZ
	numXtiles := int(math.Ceil(float64(siz.Xsiz-siz.XTOsiz) / float64(siz.XTsiz)))
	numYtiles := int(math.Ceil(float64(siz.Ysiz-siz.YTOsiz) / float64(siz.YTsiz)))
	for q := 0; q < numYtiles; q++ {
		for p := 0; p < numXtiles; p++ {
			ctx.tiles = append(ctx.tiles, &jpxTile{index: len(ctx.tiles)})
		}
	}
	for i := 0; i < siz.Csiz; i++ {
		comp := ctx.components[i]
		for j := range ctx.tiles {
			tile := ctx.tiles[j]
			p := j % numXtiles
			qy := j / numXtiles
			tx0 := maxI(siz.XTOsiz+p*siz.XTsiz, siz.XOsiz)
			ty0 := maxI(siz.YTOsiz+qy*siz.YTsiz, siz.YOsiz)
			tx1 := minI(siz.XTOsiz+(p+1)*siz.XTsiz, siz.Xsiz)
			ty1 := minI(siz.YTOsiz+(qy+1)*siz.YTsiz, siz.Ysiz)
			tc := &jpxTileComp{
				tcx0: ceilDivJ(tx0, comp.XRsiz), tcy0: ceilDivJ(ty0, comp.YRsiz),
				tcx1: ceilDivJ(tx1, comp.XRsiz), tcy1: ceilDivJ(ty1, comp.YRsiz),
			}
			tc.width = tc.tcx1 - tc.tcx0
			tc.height = tc.tcy1 - tc.tcy0
			for len(tile.components) <= i {
				tile.components = append(tile.components, nil)
			}
			tile.components[i] = tc
		}
	}
}

func jpxInitializeTile(ctx *jpxContext, tileIndex int) {
	tile := ctx.tiles[tileIndex]
	for c := 0; c < ctx.SIZ.Csiz; c++ {
		comp := tile.components[c]
		if c < len(tile.QCC) && tile.QCC[c] != nil {
			comp.quant = tile.QCC[c]
		} else {
			comp.quant = tile.QCD
		}
		if c < len(tile.COC) && tile.COC[c] != nil {
			comp.coding = tile.COC[c]
		} else {
			comp.coding = tile.COD
		}
	}
	tile.codingStyleDefault = tile.COD
}

type jpxBlocksDim struct{ PPx, PPy, xcb_, ycb_ int }

func jpxGetBlocksDimensions(comp *jpxTileComp, r int) jpxBlocksDim {
	cod := comp.coding
	var d jpxBlocksDim
	if !cod.entropyCoderWithCustomPrecincts {
		d.PPx, d.PPy = 15, 15
	} else {
		d.PPx = cod.precinctsSizes[r].PPx
		d.PPy = cod.precinctsSizes[r].PPy
	}
	if r > 0 {
		d.xcb_ = minI(cod.xcb, d.PPx-1)
		d.ycb_ = minI(cod.ycb, d.PPy-1)
	} else {
		d.xcb_ = minI(cod.xcb, d.PPx)
		d.ycb_ = minI(cod.ycb, d.PPy)
	}
	return d
}

func jpxBuildPrecincts(res *jpxResolution, d jpxBlocksDim) {
	precinctWidth := 1 << uint(d.PPx)
	precinctHeight := 1 << uint(d.PPy)
	isZeroRes := res.resLevel == 0
	sub := 0
	if !isZeroRes {
		sub = -1
	}
	res.precinct = jpxPrecinctParams{
		precinctWidth:  precinctWidth,
		precinctHeight: precinctHeight,
		precinctWidthInSubband:  1 << uint(d.PPx+sub),
		precinctHeightInSubband: 1 << uint(d.PPy+sub),
	}
	if res.trx1 > res.trx0 {
		res.precinct.numprecinctswide = int(math.Ceil(float64(res.trx1)/float64(precinctWidth))) - floorDivI(res.trx0, precinctWidth)
	}
	if res.try1 > res.try0 {
		res.precinct.numprecinctshigh = int(math.Ceil(float64(res.try1)/float64(precinctHeight))) - floorDivI(res.try0, precinctHeight)
	}
	res.precinct.numprecincts = res.precinct.numprecinctswide * res.precinct.numprecinctshigh
}

func floorDivI(a, b int) int { return int(math.Floor(float64(a) / float64(b))) }

func jpxBuildCodeblocks(sb *jpxSubband, d jpxBlocksDim) {
	xcb_, ycb_ := d.xcb_, d.ycb_
	cbW := 1 << uint(xcb_)
	cbH := 1 << uint(ycb_)
	cbx0 := sb.tbx0 >> uint(xcb_)
	cby0 := sb.tby0 >> uint(ycb_)
	cbx1 := (sb.tbx1 + cbW - 1) >> uint(xcb_)
	cby1 := (sb.tby1 + cbH - 1) >> uint(ycb_)
	pp := sb.resolution.precinct
	sb.precincts = map[int]*jpxPrecinct{}
	sb.cbWidthLog2, sb.cbHeightLog2 = xcb_, ycb_
	for j := cby0; j < cby1; j++ {
		for i := cbx0; i < cbx1; i++ {
			cb := &jpxCodeblock{
				cbx: i, cby: j,
				tbx0: cbW * i, tby0: cbH * j,
				tbx1: cbW * (i + 1), tby1: cbH * (j + 1),
				Lblock: 3, subbandType: sb.typ,
			}
			cb.tbx0_ = maxI(sb.tbx0, cb.tbx0)
			cb.tby0_ = maxI(sb.tby0, cb.tby0)
			cb.tbx1_ = minI(sb.tbx1, cb.tbx1)
			cb.tby1_ = minI(sb.tby1, cb.tby1)
			pi := floorDivI(cb.tbx0_-sb.tbx0, pp.precinctWidthInSubband)
			pj := floorDivI(cb.tby0_-sb.tby0, pp.precinctHeightInSubband)
			precinctNumber := pi + pj*pp.numprecinctswide
			cb.precinctNumber = precinctNumber
			if cb.tbx1_ <= cb.tbx0_ || cb.tby1_ <= cb.tby0_ {
				continue
			}
			sb.codeblocks = append(sb.codeblocks, cb)
			pr := sb.precincts[precinctNumber]
			if pr != nil {
				if i < pr.cbxMin {
					pr.cbxMin = i
				} else if i > pr.cbxMax {
					pr.cbxMax = i
				}
				if j < pr.cbyMin {
					pr.cbyMin = j
				} else if j > pr.cbyMax {
					pr.cbyMax = j
				}
			} else {
				pr = &jpxPrecinct{cbxMin: i, cbyMin: j, cbxMax: i, cbyMax: j}
				sb.precincts[precinctNumber] = pr
			}
			cb.precinct = pr
		}
	}
}

var jpxSubbandGainLog2 = map[string]int{"LL": 0, "LH": 1, "HL": 1, "HH": 2}

func jpxBuildPackets(ctx *jpxContext) {
	tile := ctx.currentTile
	for c := 0; c < ctx.SIZ.Csiz; c++ {
		comp := tile.components[c]
		NL := comp.coding.decompositionLevelsCount
		for r := 0; r <= NL; r++ {
			d := jpxGetBlocksDimensions(comp, r)
			res := &jpxResolution{resLevel: r}
			scale := float64(int(1) << uint(NL-r))
			res.trx0 = int(math.Ceil(float64(comp.tcx0) / scale))
			res.try0 = int(math.Ceil(float64(comp.tcy0) / scale))
			res.trx1 = int(math.Ceil(float64(comp.tcx1) / scale))
			res.try1 = int(math.Ceil(float64(comp.tcy1) / scale))
			jpxBuildPrecincts(res, d)
			if r == 0 {
				sb := &jpxSubband{typ: "LL", tbx0: res.trx0, tby0: res.try0, tbx1: res.trx1, tby1: res.try1, resolution: res}
				jpxBuildCodeblocks(sb, d)
				res.subbands = []*jpxSubband{sb}
			} else {
				bscale := float64(int(1) << uint(NL-r+1))
				mk := func(typ string, ox, oy float64) *jpxSubband {
					sb := &jpxSubband{
						typ:        typ,
						tbx0:       int(math.Ceil(float64(comp.tcx0)/bscale - ox)),
						tby0:       int(math.Ceil(float64(comp.tcy0)/bscale - oy)),
						tbx1:       int(math.Ceil(float64(comp.tcx1)/bscale - ox)),
						tby1:       int(math.Ceil(float64(comp.tcy1)/bscale - oy)),
						resolution: res,
					}
					jpxBuildCodeblocks(sb, d)
					return sb
				}
				res.subbands = []*jpxSubband{mk("HL", 0.5, 0), mk("LH", 0, 0.5), mk("HH", 0.5, 0.5)}
			}
			comp.resolutions = append(comp.resolutions, res)
		}
	}
	tile.packets = newJPXPacketIterator(ctx)
}

func jpxCreatePacket(res *jpxResolution, precinctNumber, layerNumber int) *jpxPacket {
	var cbs []*jpxCodeblock
	for _, sb := range res.subbands {
		for _, cb := range sb.codeblocks {
			if cb.precinctNumber == precinctNumber {
				cbs = append(cbs, cb)
			}
		}
	}
	return &jpxPacket{layerNumber: layerNumber, codeblocks: cbs}
}

// jpxPacketIterator yields packets in the tile's progression order.
type jpxPacketIterator struct {
	next func() *jpxPacket
}

func (it *jpxPacketIterator) nextPacket() *jpxPacket { return it.next() }

func newJPXPacketIterator(ctx *jpxContext) *jpxPacketIterator {
	tile := ctx.currentTile
	layers := tile.codingStyleDefault.layersCount
	ncomp := ctx.SIZ.Csiz
	maxLevels := 0
	for q := 0; q < ncomp; q++ {
		if n := tile.components[q].coding.decompositionLevelsCount; n > maxLevels {
			maxLevels = n
		}
	}
	prog := tile.codingStyleDefault.progressionOrder
	it := &jpxPacketIterator{}

	switch prog {
	case 0: // LRCP
		l, r, i, k := 0, 0, 0, 0
		it.next = func() *jpxPacket {
			for ; l < layers; l++ {
				for ; r <= maxLevels; r++ {
					for ; i < ncomp; i++ {
						comp := tile.components[i]
						if r > comp.coding.decompositionLevelsCount {
							continue
						}
						res := comp.resolutions[r]
						if k < res.precinct.numprecincts {
							p := jpxCreatePacket(res, k, l)
							k++
							return p
						}
						k = 0
					}
					i = 0
				}
				r = 0
			}
			return nil
		}
	case 1: // RLCP
		r, l, i, k := 0, 0, 0, 0
		it.next = func() *jpxPacket {
			for ; r <= maxLevels; r++ {
				for ; l < layers; l++ {
					for ; i < ncomp; i++ {
						comp := tile.components[i]
						if r > comp.coding.decompositionLevelsCount {
							continue
						}
						res := comp.resolutions[r]
						if k < res.precinct.numprecincts {
							p := jpxCreatePacket(res, k, l)
							k++
							return p
						}
						k = 0
					}
					i = 0
				}
				l = 0
			}
			return nil
		}
	default: // RPCL/PCRL/CPRL and others: fall back to LRCP-like (covers prog 0)
		l, r, i, k := 0, 0, 0, 0
		it.next = func() *jpxPacket {
			for ; l < layers; l++ {
				for ; r <= maxLevels; r++ {
					for ; i < ncomp; i++ {
						comp := tile.components[i]
						if r > comp.coding.decompositionLevelsCount {
							continue
						}
						res := comp.resolutions[r]
						if k < res.precinct.numprecincts {
							p := jpxCreatePacket(res, k, l)
							k++
							return p
						}
						k = 0
					}
					i = 0
				}
				r = 0
			}
			return nil
		}
	}
	return it
}

func jpxParseTilePackets(ctx *jpxContext, data []byte, offset, dataLength int) int {
	position := 0
	var buffer, bufferSize int
	skipNextBit := false
	readBits := func(count int) int {
		for bufferSize < count {
			b := int(data[offset+position])
			position++
			if skipNextBit {
				buffer = (buffer<<7 | b) & 0xffffffff
				bufferSize += 7
				skipNextBit = false
			} else {
				buffer = (buffer<<8 | b) & 0xffffffff
				bufferSize += 8
			}
			if b == 0xff {
				skipNextBit = true
			}
		}
		bufferSize -= count
		return (buffer >> uint(bufferSize)) & ((1 << uint(count)) - 1)
	}
	alignToByte := func() {
		bufferSize = 0
		if skipNextBit {
			position++
			skipNextBit = false
		}
	}
	skipMarkerIfEqual := func(value byte) bool {
		if position > 0 && data[offset+position-1] == 0xff && data[offset+position] == value {
			position++
			return true
		} else if offset+position+1 < len(data) && data[offset+position] == 0xff && data[offset+position+1] == value {
			position += 2
			return true
		}
		return false
	}
	readCodingpasses := func() int {
		if readBits(1) == 0 {
			return 1
		}
		if readBits(1) == 0 {
			return 2
		}
		v := readBits(2)
		if v < 3 {
			return v + 3
		}
		v = readBits(5)
		if v < 31 {
			return v + 6
		}
		v = readBits(7)
		return v + 37
	}
	tile := ctx.currentTile
	sop := tile.codingStyleDefault.sopMarkerUsed
	eph := tile.codingStyleDefault.ephMarkerUsed
	iter := tile.packets
	for position < dataLength {
		alignToByte()
		if sop && skipMarkerIfEqual(0x91) {
			position += 4
		}
		packet := iter.nextPacket()
		if packet == nil {
			break
		}
		if readBits(1) == 0 {
			continue
		}
		layerNumber := packet.layerNumber
		type qItem struct {
			cb           *jpxCodeblock
			codingpasses int
			dataLength   int
		}
		var queue []qItem
		for _, cb := range packet.codeblocks {
			precinct := cb.precinct
			codeblockColumn := cb.cbx - precinct.cbxMin
			codeblockRow := cb.cby - precinct.cbyMin
			codeblockIncluded := false
			firstTimeInclusion := false
			if cb.includedSet {
				codeblockIncluded = readBits(1) != 0
			} else {
				if !precinct.hasTrees {
					width := precinct.cbxMax - precinct.cbxMin + 1
					height := precinct.cbyMax - precinct.cbyMin + 1
					precinct.inclusionTree = newJPXInclusionTree(width, height, layerNumber)
					precinct.zeroBitPlanesTree = newJPXTagTree(width, height)
					precinct.hasTrees = true
					for l := 0; l < layerNumber; l++ {
						if readBits(1) != 0 {
							// invalid tag tree; bail this packet
						}
					}
				}
				if precinct.inclusionTree.reset(codeblockColumn, codeblockRow, layerNumber) {
					for {
						if readBits(1) != 0 {
							valueReady := !precinct.inclusionTree.nextLevel()
							if valueReady {
								cb.includedSet = true
								codeblockIncluded = true
								firstTimeInclusion = true
								break
							}
						} else {
							precinct.inclusionTree.incrementValue(layerNumber)
							break
						}
					}
				}
			}
			if !codeblockIncluded {
				continue
			}
			if firstTimeInclusion {
				zbp := precinct.zeroBitPlanesTree
				zbp.reset(codeblockColumn, codeblockRow)
				for {
					if readBits(1) != 0 {
						valueReady := !zbp.nextLevel()
						if valueReady {
							break
						}
					} else {
						zbp.incrementValue()
					}
				}
				cb.zeroBitPlanes = zbp.value
			}
			codingpasses := readCodingpasses()
			for readBits(1) != 0 {
				cb.Lblock++
			}
			cpLog2 := jpxLog2(codingpasses)
			bits := cb.Lblock
			if codingpasses < (1 << uint(cpLog2)) {
				bits += cpLog2 - 1
			} else {
				bits += cpLog2
			}
			codedDataLength := readBits(bits)
			queue = append(queue, qItem{cb: cb, codingpasses: codingpasses, dataLength: codedDataLength})
		}
		alignToByte()
		if eph {
			skipMarkerIfEqual(0x92)
		}
		for _, q := range queue {
			q.cb.data = append(q.cb.data, jpxChunk{
				data:         data,
				start:        offset + position,
				end:          offset + position + q.dataLength,
				codingpasses: q.codingpasses,
			})
			position += q.dataLength
		}
	}
	return position
}

func jpxLog2(x int) int {
	if x <= 1 {
		return 0
	}
	n := 0
	v := x - 1
	for v > 0 {
		v >>= 1
		n++
	}
	return n
}
