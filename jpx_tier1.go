// SPDX-License-Identifier: MIT

package asposepdf

// JPEG2000 tier-1 EBCOT bit-plane decoding (ISO/IEC 15444-1 Annex D). Faithful
// port of pdf.js BitModel: three coding passes (significance propagation,
// magnitude refinement, cleanup) over a code-block, driven by the shared MQ
// arithmetic decoder (jbig2_mq.go). Context state is a 19-entry array packed as
// (stateIndex<<1 | mps), exactly as the MQ decoder expects.

const (
	jpxUniformContext   = 17
	jpxRunlengthContext = 18
)

var jpxLLAndLHContextLabel = []byte{0, 5, 8, 0, 3, 7, 8, 0, 4, 7, 8, 0, 0, 0, 0, 0, 1, 6, 8, 0, 3, 7, 8, 0, 4, 7, 8, 0, 0, 0, 0, 0, 2, 6, 8, 0, 3, 7, 8, 0, 4, 7, 8, 0, 0, 0, 0, 0, 2, 6, 8, 0, 3, 7, 8, 0, 4, 7, 8, 0, 0, 0, 0, 0, 2, 6, 8, 0, 3, 7, 8, 0, 4, 7, 8}
var jpxHLContextLabel = []byte{0, 3, 4, 0, 5, 7, 7, 0, 8, 8, 8, 0, 0, 0, 0, 0, 1, 3, 4, 0, 6, 7, 7, 0, 8, 8, 8, 0, 0, 0, 0, 0, 2, 3, 4, 0, 6, 7, 7, 0, 8, 8, 8, 0, 0, 0, 0, 0, 2, 3, 4, 0, 6, 7, 7, 0, 8, 8, 8, 0, 0, 0, 0, 0, 2, 3, 4, 0, 6, 7, 7, 0, 8, 8, 8}
var jpxHHContextLabel = []byte{0, 1, 2, 0, 1, 2, 2, 0, 2, 2, 2, 0, 0, 0, 0, 0, 3, 4, 5, 0, 4, 5, 5, 0, 5, 5, 5, 0, 0, 0, 0, 0, 6, 7, 7, 0, 7, 7, 7, 0, 7, 7, 7, 0, 0, 0, 0, 0, 8, 8, 8, 0, 8, 8, 8, 0, 8, 8, 8, 0, 0, 0, 0, 0, 8, 8, 8, 0, 8, 8, 8, 0, 8, 8, 8}

type jpxBitModel struct {
	width, height         int
	labels                []byte
	neighborsSignificance []byte
	sign                  []byte
	magnitude             []uint32
	processingFlags       []byte
	bitsDecoded           []byte
	contexts              []byte
	decoder               *mqDecoder
}

func newJPXBitModel(width, height int, subbandType string, zeroBitPlanes int) *jpxBitModel {
	bm := &jpxBitModel{width: width, height: height}
	switch subbandType {
	case "HH":
		bm.labels = jpxHHContextLabel
	case "HL":
		bm.labels = jpxHLContextLabel
	default:
		bm.labels = jpxLLAndLHContextLabel
	}
	n := width * height
	bm.neighborsSignificance = make([]byte, n)
	bm.sign = make([]byte, n)
	bm.magnitude = make([]uint32, n)
	bm.processingFlags = make([]byte, n)
	bm.bitsDecoded = make([]byte, n)
	if zeroBitPlanes != 0 {
		for i := 0; i < n; i++ {
			bm.bitsDecoded[i] = byte(zeroBitPlanes)
		}
	}
	bm.reset()
	return bm
}

func (bm *jpxBitModel) setDecoder(d *mqDecoder) { bm.decoder = d }

func (bm *jpxBitModel) reset() {
	bm.contexts = make([]byte, 19)
	bm.contexts[0] = 4 << 1
	bm.contexts[jpxUniformContext] = 46 << 1
	bm.contexts[jpxRunlengthContext] = 3 << 1
}

func (bm *jpxBitModel) setNeighborsSignificance(row, column, index int) {
	ns := bm.neighborsSignificance
	width, height := bm.width, bm.height
	left := column > 0
	right := column+1 < width
	if row > 0 {
		i := index - width
		if left {
			ns[i-1] += 0x10
		}
		if right {
			ns[i+1] += 0x10
		}
		ns[i] += 0x04
	}
	if row+1 < height {
		i := index + width
		if left {
			ns[i-1] += 0x10
		}
		if right {
			ns[i+1] += 0x10
		}
		ns[i] += 0x04
	}
	if left {
		ns[index-1] += 0x01
	}
	if right {
		ns[index+1] += 0x01
	}
	ns[index] |= 0x80
}

func (bm *jpxBitModel) decodeSignBit(row, column, index int) int {
	width, height := bm.width, bm.height
	mag := bm.magnitude
	sgn := bm.sign
	var contribution, sign0, sign1 int
	significance1 := column > 0 && mag[index-1] != 0
	if column+1 < width && mag[index+1] != 0 {
		sign1 = int(sgn[index+1])
		if significance1 {
			sign0 = int(sgn[index-1])
			contribution = 1 - sign1 - sign0
		} else {
			contribution = 1 - sign1 - sign1
		}
	} else if significance1 {
		sign0 = int(sgn[index-1])
		contribution = 1 - sign0 - sign0
	} else {
		contribution = 0
	}
	horizontalContribution := 3 * contribution
	significance1 = row > 0 && mag[index-width] != 0
	if row+1 < height && mag[index+width] != 0 {
		sign1 = int(sgn[index+width])
		if significance1 {
			sign0 = int(sgn[index-width])
			contribution = 1 - sign1 - sign0 + horizontalContribution
		} else {
			contribution = 1 - sign1 - sign1 + horizontalContribution
		}
	} else if significance1 {
		sign0 = int(sgn[index-width])
		contribution = 1 - sign0 - sign0 + horizontalContribution
	} else {
		contribution = horizontalContribution
	}
	if contribution >= 0 {
		return bm.decoder.readBit(bm.contexts, 9+contribution)
	}
	return bm.decoder.readBit(bm.contexts, 9-contribution) ^ 1
}

func (bm *jpxBitModel) runSignificancePropagationPass() {
	dec := bm.decoder
	width, height := bm.width, bm.height
	mag := bm.magnitude
	ns := bm.neighborsSignificance
	pf := bm.processingFlags
	labels := bm.labels
	const processedMask = 1
	const firstMagnitudeBitMask = 2
	for i0 := 0; i0 < height; i0 += 4 {
		for j := 0; j < width; j++ {
			index := i0*width + j
			for i1 := 0; i1 < 4; i1++ {
				i := i0 + i1
				if i >= height {
					break
				}
				pf[index] &^= processedMask
				if mag[index] != 0 || ns[index] == 0 {
					index += width
					continue
				}
				contextLabel := labels[ns[index]]
				decision := dec.readBit(bm.contexts, int(contextLabel))
				if decision != 0 {
					bm.sign[index] = byte(bm.decodeSignBit(i, j, index))
					mag[index] = 1
					bm.setNeighborsSignificance(i, j, index)
					pf[index] |= firstMagnitudeBitMask
				}
				bm.bitsDecoded[index]++
				pf[index] |= processedMask
				index += width
			}
		}
	}
}

func (bm *jpxBitModel) runMagnitudeRefinementPass() {
	dec := bm.decoder
	width, height := bm.width, bm.height
	mag := bm.magnitude
	ns := bm.neighborsSignificance
	pf := bm.processingFlags
	const processedMask = 1
	const firstMagnitudeBitMask = 2
	length := width * height
	width4 := width * 4
	for index0 := 0; index0 < length; index0 += width4 {
		indexNext := minI(length, index0+width4)
		for j := 0; j < width; j++ {
			for index := index0 + j; index < indexNext; index += width {
				if mag[index] == 0 || (pf[index]&processedMask) != 0 {
					continue
				}
				contextLabel := 16
				if (pf[index] & firstMagnitudeBitMask) != 0 {
					pf[index] ^= firstMagnitudeBitMask
					if (ns[index] & 127) == 0 {
						contextLabel = 15
					} else {
						contextLabel = 14
					}
				}
				bit := dec.readBit(bm.contexts, contextLabel)
				mag[index] = mag[index]<<1 | uint32(bit)
				bm.bitsDecoded[index]++
				pf[index] |= processedMask
			}
		}
	}
}

func (bm *jpxBitModel) runCleanupPass() {
	dec := bm.decoder
	width, height := bm.width, bm.height
	ns := bm.neighborsSignificance
	mag := bm.magnitude
	pf := bm.processingFlags
	labels := bm.labels
	bd := bm.bitsDecoded
	const processedMask = 1
	const firstMagnitudeBitMask = 2
	oneRowDown := width
	twoRowsDown := width * 2
	threeRowsDown := width * 3
	for i0 := 0; i0 < height; {
		iNext := minI(i0+4, height)
		indexBase := i0 * width
		checkAllEmpty := i0+3 < height
		for j := 0; j < width; j++ {
			index0 := indexBase + j
			allEmpty := checkAllEmpty &&
				pf[index0] == 0 && pf[index0+oneRowDown] == 0 &&
				pf[index0+twoRowsDown] == 0 && pf[index0+threeRowsDown] == 0 &&
				ns[index0] == 0 && ns[index0+oneRowDown] == 0 &&
				ns[index0+twoRowsDown] == 0 && ns[index0+threeRowsDown] == 0
			i1 := 0
			index := index0
			i := i0
			if allEmpty {
				if dec.readBit(bm.contexts, jpxRunlengthContext) == 0 {
					bd[index0]++
					bd[index0+oneRowDown]++
					bd[index0+twoRowsDown]++
					bd[index0+threeRowsDown]++
					continue
				}
				i1 = dec.readBit(bm.contexts, jpxUniformContext)<<1 | dec.readBit(bm.contexts, jpxUniformContext)
				if i1 != 0 {
					i = i0 + i1
					index += i1 * width
				}
				bm.sign[index] = byte(bm.decodeSignBit(i, j, index))
				mag[index] = 1
				bm.setNeighborsSignificance(i, j, index)
				pf[index] |= firstMagnitudeBitMask
				index = index0
				for i2 := i0; i2 <= i; i2++ {
					bd[index]++
					index += width
				}
				i1++
			}
			for i = i0 + i1; i < iNext; i++ {
				if mag[index] != 0 || (pf[index]&processedMask) != 0 {
					index += width
					continue
				}
				contextLabel := labels[ns[index]]
				decision := dec.readBit(bm.contexts, int(contextLabel))
				if decision == 1 {
					bm.sign[index] = byte(bm.decodeSignBit(i, j, index))
					mag[index] = 1
					bm.setNeighborsSignificance(i, j, index)
					pf[index] |= firstMagnitudeBitMask
				}
				bd[index]++
				index += width
			}
		}
		i0 = iNext
	}
}

func (bm *jpxBitModel) checkSegmentationSymbol() {
	dec := bm.decoder
	symbol := dec.readBit(bm.contexts, jpxUniformContext)<<3 |
		dec.readBit(bm.contexts, jpxUniformContext)<<2 |
		dec.readBit(bm.contexts, jpxUniformContext)<<1 |
		dec.readBit(bm.contexts, jpxUniformContext)
	_ = symbol // 0xa expected; ignore mismatch (best-effort)
}
