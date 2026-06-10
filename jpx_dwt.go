// SPDX-License-Identifier: MIT

package asposepdf

import "math"

// JPEG2000 tier-1 → coefficients (copyCoefficients), the inverse wavelet
// transforms (5/3 reversible and 9/7 irreversible) and the inverse RCT/ICT
// colour transform plus DC level shift. Faithful port of pdf.js.

// jpxCopyCoefficients decodes every code-block of a subband (tier-1) and writes
// its dequantized coefficients into the resolution-level coefficient grid.
func jpxCopyCoefficients(coefficients []float32, levelWidth, levelHeight int, sb *jpxSubband, delta float64, mb int, reversible, segmentationSymbolUsed, resetContextProbabilities bool) {
	x0 := sb.tbx0
	y0 := sb.tby0
	width := sb.tbx1 - sb.tbx0
	right := 0
	if sb.typ[0] == 'H' {
		right = 1
	}
	bottom := 0
	if sb.typ[1] == 'H' {
		bottom = levelWidth
	}
	interleave := sb.typ != "LL"
	for _, cb := range sb.codeblocks {
		blockWidth := cb.tbx1_ - cb.tbx0_
		blockHeight := cb.tby1_ - cb.tby0_
		if blockWidth == 0 || blockHeight == 0 || len(cb.data) == 0 {
			continue
		}
		bm := newJPXBitModel(blockWidth, blockHeight, cb.subbandType, cb.zeroBitPlanes)
		currentCodingpassType := 2
		totalLength := 0
		codingpasses := 0
		for _, ch := range cb.data {
			totalLength += ch.end - ch.start
			codingpasses += ch.codingpasses
		}
		encoded := make([]byte, totalLength)
		pos := 0
		for _, ch := range cb.data {
			copy(encoded[pos:], ch.data[ch.start:ch.end])
			pos += ch.end - ch.start
		}
		dec := newMQDecoder(encoded, 0, totalLength)
		bm.setDecoder(dec)
		for j := 0; j < codingpasses; j++ {
			switch currentCodingpassType {
			case 0:
				bm.runSignificancePropagationPass()
			case 1:
				bm.runMagnitudeRefinementPass()
			case 2:
				bm.runCleanupPass()
				if segmentationSymbolUsed {
					bm.checkSegmentationSymbol()
				}
			}
			if resetContextProbabilities {
				bm.reset()
			}
			currentCodingpassType = (currentCodingpassType + 1) % 3
		}
		offset := cb.tbx0_ - x0 + (cb.tby0_-y0)*width
		sign := bm.sign
		magnitude := bm.magnitude
		bitsDecoded := bm.bitsDecoded
		magnitudeCorrection := 0.5
		if reversible {
			magnitudeCorrection = 0
		}
		p := 0
		for j := 0; j < blockHeight; j++ {
			row := offset / width
			levelOffset := 2*row*(levelWidth-width) + right + bottom
			for k := 0; k < blockWidth; k++ {
				n := float64(magnitude[p])
				if n != 0 {
					n = (n + magnitudeCorrection) * delta
					if sign[p] != 0 {
						n = -n
					}
					nb := int(bitsDecoded[p])
					var coeffPos int
					if interleave {
						coeffPos = levelOffset + (offset << 1)
					} else {
						coeffPos = offset
					}
					if reversible && nb >= mb {
						coefficients[coeffPos] = float32(n)
					} else {
						coefficients[coeffPos] = float32(n * float64(int(1)<<uint(mb-nb)))
					}
				}
				offset++
				p++
			}
			offset += width - blockWidth
		}
	}
}

type jpxSubbandCoeffs struct {
	width, height int
	items         []float32
}

// jpxTransformTile dequantizes and inverse-transforms one component of the tile,
// returning the spatial-domain samples.
func jpxTransformTile(ctx *jpxContext, tile *jpxTile, c int) *jpxSubbandCoeffs {
	comp := tile.components[c]
	cod := comp.coding
	q := comp.quant
	NL := cod.decompositionLevelsCount
	reversible := cod.reversibleTransformation
	precision := ctx.components[c].precision

	var subbandCoefficients []*jpxSubbandCoeffs
	b := 0
	for i := 0; i <= NL; i++ {
		res := comp.resolutions[i]
		width := res.trx1 - res.trx0
		height := res.try1 - res.try0
		coeffs := make([]float32, width*height)
		for _, sb := range res.subbands {
			var mu, epsilon int
			if !q.scalarExpounded {
				mu = q.SPqcds[0].mu
				epsilon = q.SPqcds[0].epsilon
				if i > 0 {
					epsilon += 1 - i
				}
			} else {
				mu = q.SPqcds[b].mu
				epsilon = q.SPqcds[b].epsilon
				b++
			}
			gainLog2 := jpxSubbandGainLog2[sb.typ]
			var delta float64
			if reversible {
				delta = 1
			} else {
				delta = math.Pow(2, float64(precision+gainLog2-epsilon)) * (1 + float64(mu)/2048)
			}
			mb := q.guardBits + epsilon - 1
			jpxCopyCoefficients(coeffs, width, height, sb, delta, mb, reversible, cod.segmentationSymbolUsed, cod.resetContextProbabilities)
		}
		subbandCoefficients = append(subbandCoefficients, &jpxSubbandCoeffs{width: width, height: height, items: coeffs})
	}
	if reversible {
		return jpxReversibleCalc(subbandCoefficients, comp.tcx0, comp.tcy0)
	}
	return jpxIrreversibleCalc(subbandCoefficients, comp.tcx0, comp.tcy0)
}

// jpxTransformComponents runs every tile/component and applies the colour
// transform + level shift, returning interleaved 8-bit samples.
func jpxTransformComponents(ctx *jpxContext) *jpxImageOut {
	ncomp := ctx.SIZ.Csiz
	tile := ctx.tiles[0]
	transformed := make([]*jpxSubbandCoeffs, ncomp)
	for c := 0; c < ncomp; c++ {
		transformed[c] = jpxTransformTile(ctx, tile, c)
	}
	t0 := transformed[0]
	out := make([]byte, len(t0.items)*ncomp)
	clamp := func(v float64, shift int) byte {
		iv := int32(v)
		if shift >= 0 {
			iv >>= uint(shift)
		}
		if iv < 0 {
			return 0
		}
		if iv > 255 {
			return 255
		}
		return byte(iv)
	}
	if tile.codingStyleDefault.multipleComponentTransform != 0 {
		y0i := transformed[0].items
		y1i := transformed[1].items
		y2i := transformed[2].items
		shift := ctx.components[0].precision - 8
		offset := float64(int(128)<<uint(maxI(shift, 0))) + 0.5
		reversible := tile.components[0].coding.reversibleTransformation
		pos := 0
		for j := 0; j < len(y0i); j++ {
			y0 := float64(y0i[j]) + offset
			y1 := float64(y1i[j])
			y2 := float64(y2i[j])
			if !reversible {
				out[pos] = clamp(y0+1.402*y2, shift)
				out[pos+1] = clamp(y0-0.34413*y1-0.71414*y2, shift)
				out[pos+2] = clamp(y0+1.772*y1, shift)
			} else {
				g := y0 - math.Floor((y2+y1)/4)
				out[pos] = clamp(g+y2, shift)
				out[pos+1] = clamp(g, shift)
				out[pos+2] = clamp(g+y1, shift)
			}
			pos += ncomp
		}
	} else {
		for c := 0; c < ncomp; c++ {
			items := transformed[c].items
			shift := ctx.components[c].precision - 8
			offset := float64(int(128)<<uint(maxI(shift, 0))) + 0.5
			pos := c
			for j := 0; j < len(items); j++ {
				out[pos] = clamp(float64(items[j])+offset, shift)
				pos += ncomp
			}
		}
	}
	return &jpxImageOut{width: t0.width, height: t0.height, components: ncomp, items: out}
}

// --- inverse wavelet transform ---

func jpxTransformExtend(buf []float32, offset, size int) {
	i1 := offset - 1
	j1 := offset + 1
	i2 := offset + size - 2
	j2 := offset + size
	buf[i1] = buf[j1]
	buf[j2] = buf[i2]
	i1--
	j1++
	i2--
	j2++
	buf[i1] = buf[j1]
	buf[j2] = buf[i2]
	i1--
	j1++
	i2--
	j2++
	buf[i1] = buf[j1]
	buf[j2] = buf[i2]
	i1--
	j1++
	i2--
	j2++
	buf[i1] = buf[j1]
	buf[j2] = buf[i2]
}

// jpxIterate performs one resolution-level inverse DWT combining ll with the
// hl/lh/hh detail band, using the supplied 1-D filter.
func jpxIterate(ll, band *jpxSubbandCoeffs, u0, v0 int, filter func(x []float32, offset, length int)) *jpxSubbandCoeffs {
	llWidth, llHeight := ll.width, ll.height
	llItems := ll.items
	width := band.width
	height := band.height
	items := band.items

	for i, k := 0, 0; i < llHeight; i++ {
		l := i * 2 * width
		for j := 0; j < llWidth; j++ {
			items[l] = llItems[k]
			k++
			l += 2
		}
	}
	const bufferPadding = 4
	if width == 1 {
		if u0&1 != 0 {
			for v, k := 0, 0; v < height; v++ {
				items[k] *= 0.5
				k += width
			}
		}
	} else {
		rowBuffer := make([]float32, width+2*bufferPadding)
		for v, k := 0, 0; v < height; v++ {
			copy(rowBuffer[bufferPadding:bufferPadding+width], items[k:k+width])
			jpxTransformExtend(rowBuffer, bufferPadding, width)
			filter(rowBuffer, bufferPadding, width)
			copy(items[k:k+width], rowBuffer[bufferPadding:bufferPadding+width])
			k += width
		}
	}
	if height == 1 {
		if v0&1 != 0 {
			for u := 0; u < width; u++ {
				items[u] *= 0.5
			}
		}
	} else {
		colBuffer := make([]float32, height+2*bufferPadding)
		for u := 0; u < width; u++ {
			for k, l := u, bufferPadding; l < bufferPadding+height; l++ {
				colBuffer[l] = items[k]
				k += width
			}
			jpxTransformExtend(colBuffer, bufferPadding, height)
			filter(colBuffer, bufferPadding, height)
			for k, l := u, bufferPadding; l < bufferPadding+height; l++ {
				items[k] = colBuffer[l]
				k += width
			}
		}
	}
	return &jpxSubbandCoeffs{width: width, height: height, items: items}
}

func jpxIrreversibleFilter(x []float32, offset, length int) {
	ln := length >> 1
	const alpha = -1.586134342059924
	const beta = -0.052980118572961
	const gamma = 0.882911075530934
	const delta = 0.443506852043971
	const K = 1.230174104914001
	const K_ = 1 / K
	j := offset - 3
	for n := ln + 4; n > 0; n-- {
		x[j] *= float32(K_)
		j += 2
	}
	j = offset - 2
	current := delta * float64(x[j-1])
	for n := ln + 3; n > 0; n-- {
		next := delta * float64(x[j+1])
		x[j] = float32(K*float64(x[j]) - current - next)
		j += 2
		n--
		if n <= 0 {
			break
		}
		current = delta * float64(x[j+1])
		x[j] = float32(K*float64(x[j]) - current - next)
		j += 2
	}
	j = offset - 1
	current = gamma * float64(x[j-1])
	for n := ln + 2; n > 0; n-- {
		next := gamma * float64(x[j+1])
		x[j] -= float32(current + next)
		j += 2
		n--
		if n <= 0 {
			break
		}
		current = gamma * float64(x[j+1])
		x[j] -= float32(current + next)
		j += 2
	}
	j = offset
	current = beta * float64(x[j-1])
	for n := ln + 1; n > 0; n-- {
		next := beta * float64(x[j+1])
		x[j] -= float32(current + next)
		j += 2
		n--
		if n <= 0 {
			break
		}
		current = beta * float64(x[j+1])
		x[j] -= float32(current + next)
		j += 2
	}
	if ln != 0 {
		j = offset + 1
		current = alpha * float64(x[j-1])
		for n := ln; n > 0; n-- {
			next := alpha * float64(x[j+1])
			x[j] -= float32(current + next)
			j += 2
			n--
			if n <= 0 {
				break
			}
			current = alpha * float64(x[j+1])
			x[j] -= float32(current + next)
			j += 2
		}
	}
}

func jpxReversibleFilter(x []float32, offset, length int) {
	ln := length >> 1
	j := offset
	for n := ln + 1; n > 0; n-- {
		x[j] -= float32((int(x[j-1]) + int(x[j+1]) + 2) >> 2)
		j += 2
	}
	j = offset + 1
	for n := ln; n > 0; n-- {
		x[j] += float32((int(x[j-1]) + int(x[j+1])) >> 1)
		j += 2
	}
}

func jpxIrreversibleCalc(subbands []*jpxSubbandCoeffs, u0, v0 int) *jpxSubbandCoeffs {
	ll := subbands[0]
	for i := 1; i < len(subbands); i++ {
		ll = jpxIterate(ll, subbands[i], u0, v0, jpxIrreversibleFilter)
	}
	return ll
}

func jpxReversibleCalc(subbands []*jpxSubbandCoeffs, u0, v0 int) *jpxSubbandCoeffs {
	ll := subbands[0]
	for i := 1; i < len(subbands); i++ {
		ll = jpxIterate(ll, subbands[i], u0, v0, jpxReversibleFilter)
	}
	return ll
}
