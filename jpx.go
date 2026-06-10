// SPDX-License-Identifier: MIT

package asposepdf

import (
	"encoding/binary"
	"fmt"
	"math"
)

// JPEG2000 (ISO/IEC 15444-1) decoding for the PDF /JPXDecode filter. This is a
// faithful pure-Go port of the pdf.js JpxImage decoder: codestream parsing,
// tier-2 packet parsing with tag trees, tier-1 EBCOT (sharing the MQ arithmetic
// decoder with JBIG2, jbig2_mq.go), inverse 5/3 / 9/7 wavelet transforms and the
// RCT/ICT colour transform. Epic pdf-go-8ju. Supported subset: scalar
// quantization, standard (non-bypass) code-blocks, all five progression orders;
// not supported: arithmetic-coding bypass / per-pass termination / vertically
// causal / predictable termination (these set jpxErr).

// --- parsed codestream structures (mirroring pdf.js context) ---

type jpxSIZ struct {
	Xsiz, Ysiz, XOsiz, YOsiz     int
	XTsiz, YTsiz, XTOsiz, YTOsiz int
	Csiz                         int
}

type jpxComp struct {
	precision    int
	isSigned     bool
	XRsiz, YRsiz int
	x0, x1, y0, y1, width, height int
}

type jpxCOD struct {
	entropyCoderWithCustomPrecincts bool
	sopMarkerUsed, ephMarkerUsed    bool
	progressionOrder                int
	layersCount                     int
	multipleComponentTransform      int
	decompositionLevelsCount        int
	xcb, ycb                        int
	resetContextProbabilities       bool
	segmentationSymbolUsed          bool
	bypass, termEachPass            bool
	vertStripe, predictableTerm     bool
	reversibleTransformation        bool
	precinctsSizes                  []jpxPP
}

type jpxPP struct{ PPx, PPy int }

type jpxSPqcd struct{ epsilon, mu int }

type jpxQuant struct {
	noQuantization  bool
	scalarExpounded bool
	guardBits       int
	SPqcds          []jpxSPqcd
}

type jpxContext struct {
	SIZ         jpxSIZ
	COD         *jpxCOD
	QCD         *jpxQuant
	COC         []*jpxCOD
	QCC         []*jpxQuant
	components  []*jpxComp
	tiles       []*jpxTile
	currentTile *jpxTile
	mainHeader  bool
}

type jpxTile struct {
	index      int
	dataEnd    int
	partIndex  int
	partsCount int
	COD        *jpxCOD
	COC        []*jpxCOD
	QCD        *jpxQuant
	QCC        []*jpxQuant
	components []*jpxTileComp

	codingStyleDefault *jpxCOD
	packets            *jpxPacketIterator
}

type jpxTileComp struct {
	tcx0, tcy0, tcx1, tcy1 int
	width, height          int
	resolutions            []*jpxResolution
	subbands               []*jpxSubband
	coding                 *jpxCOD
	quant                  *jpxQuant
}

// jpxImageOut is the final transformed tile (RGB-ish interleaved bytes).
type jpxImageOut struct {
	width, height, components int
	items                     []byte
}

var jpxErr = fmt.Errorf("jpx: unsupported codestream feature")

// jpxDecode decodes a /JPXDecode stream into interleaved 8-bit samples and the
// component count.
func jpxDecode(data []byte) (pixels []byte, comps int, w int, h int, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("jpx: decode panic: %v", r)
		}
	}()
	cs := jpxParseBoxes(data)
	if cs == nil {
		return nil, 0, 0, 0, fmt.Errorf("jpx: no codestream")
	}
	ctx, out, e := jpxParseCodestream(cs)
	if e != nil {
		return nil, 0, 0, 0, e
	}
	width := ctx.SIZ.Xsiz - ctx.SIZ.XOsiz
	height := ctx.SIZ.Ysiz - ctx.SIZ.YOsiz
	return out.items, out.components, width, height, nil
}

// jpxParseBoxes extracts the contiguous codestream (jp2c) from a JP2-boxed file,
// or returns the input unchanged when it is already a raw codestream (SOC).
func jpxParseBoxes(data []byte) []byte {
	if len(data) >= 2 && data[0] == 0xFF && data[1] == 0x4F {
		return data
	}
	p := 0
	for p+8 <= len(data) {
		length := int(binary.BigEndian.Uint32(data[p:]))
		typ := string(data[p+4 : p+8])
		hdr := 8
		if length == 1 {
			if p+16 > len(data) {
				break
			}
			length = int(binary.BigEndian.Uint64(data[p+8:]))
			hdr = 16
		}
		end := p + length
		if length == 0 || end > len(data) || end <= p {
			end = len(data)
		}
		if typ == "jp2c" {
			return data[p+hdr : end]
		}
		p = end
	}
	return nil
}

func maxI(a, b int) int {
	if a > b {
		return a
	}
	return b
}
func minI(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func ru16(b []byte, p int) int { return int(binary.BigEndian.Uint16(b[p:])) }
func ru32(b []byte, p int) int { return int(int32(binary.BigEndian.Uint32(b[p:]))) }

// jpxParseCodestream walks the codestream markers, builds the context, decodes
// the tile packets and returns the colour-transformed result.
func jpxParseCodestream(data []byte) (*jpxContext, *jpxImageOut, error) {
	ctx := &jpxContext{}
	pos := 0
	end := len(data)
	for pos+1 < end {
		code := ru16(data, pos)
		pos += 2
		length := 0
		switch code {
		case 0xff4f: // SOC
			ctx.mainHeader = true
		case 0xffd9: // EOC
		case 0xff51: // SIZ
			length = ru16(data, pos)
			siz := jpxSIZ{
				Xsiz: ru32(data, pos+4), Ysiz: ru32(data, pos+8),
				XOsiz: ru32(data, pos+12), YOsiz: ru32(data, pos+16),
				XTsiz: ru32(data, pos+20), YTsiz: ru32(data, pos+24),
				XTOsiz: ru32(data, pos+28), YTOsiz: ru32(data, pos+32),
				Csiz: ru16(data, pos+36),
			}
			ctx.SIZ = siz
			j := pos + 38
			for i := 0; i < siz.Csiz; i++ {
				c := &jpxComp{
					precision: int(data[j]&0x7f) + 1,
					isSigned:  data[j]&0x80 != 0,
					XRsiz:     int(data[j+1]), YRsiz: int(data[j+2]),
				}
				j += 3
				jpxCalcCompDims(c, &siz)
				ctx.components = append(ctx.components, c)
			}
			jpxCalcTileGrids(ctx)
			ctx.QCC = nil
			ctx.COC = nil
		case 0xff5c: // QCD
			length = ru16(data, pos)
			q := jpxReadQuant(data, pos+2, pos+length)
			if ctx.mainHeader {
				ctx.QCD = q
			} else {
				ctx.currentTile.QCD = q
			}
		case 0xff5d: // QCC
			length = ru16(data, pos)
			j := pos + 2
			var cqcc int
			if ctx.SIZ.Csiz < 257 {
				cqcc = int(data[j])
				j++
			} else {
				cqcc = ru16(data, j)
				j += 2
			}
			q := jpxReadQuant(data, j, pos+length)
			if ctx.mainHeader {
				ctx.QCC = jpxSetAt(ctx.QCC, cqcc, q)
			} else {
				ctx.currentTile.QCC = jpxSetAt(ctx.currentTile.QCC, cqcc, q)
			}
		case 0xff52: // COD
			length = ru16(data, pos)
			cod, e := jpxReadCOD(data, pos+2, pos+length)
			if e != nil {
				return nil, nil, e
			}
			if ctx.mainHeader {
				ctx.COD = cod
			} else {
				ctx.currentTile.COD = cod
			}
		case 0xff90: // SOT
			length = ru16(data, pos)
			t := &jpxTile{
				index:      ru16(data, pos+2),
				dataEnd:    ru32(data, pos+4) + pos - 2,
				partIndex:  int(data[pos+8]),
				partsCount: int(data[pos+9]),
			}
			ctx.mainHeader = false
			if t.partIndex == 0 {
				ctx.tiles[t.index].COD = ctx.COD
				ctx.tiles[t.index].COC = append([]*jpxCOD(nil), ctx.COC...)
				ctx.tiles[t.index].QCD = ctx.QCD
				ctx.tiles[t.index].QCC = append([]*jpxQuant(nil), ctx.QCC...)
			}
			ctx.currentTile = ctx.tiles[t.index]
			ctx.currentTile.dataEnd = t.dataEnd
			ctx.currentTile.partIndex = t.partIndex
		case 0xff93: // SOD
			tile := ctx.currentTile
			if tile.partIndex == 0 {
				jpxInitializeTile(ctx, tile.index)
				jpxBuildPackets(ctx)
			}
			length = tile.dataEnd - pos
			jpxParseTilePackets(ctx, data, pos, length)
		case 0xff53, 0xff55, 0xff57, 0xff58, 0xff64, 0xff5e, 0xff5f, 0xff60, 0xff61, 0xff63: // COC/TLM/PLM/PPM/COM/RGN/POC/PPT/PLT/CRG: skip
			length = ru16(data, pos)
		default:
			return nil, nil, fmt.Errorf("jpx: unknown marker %04x at %d", code, pos-2)
		}
		pos += length
	}
	out := jpxTransformComponents(ctx)
	return ctx, out, nil
}

func jpxSetAt[T any](s []T, i int, v T) []T {
	for len(s) <= i {
		var z T
		s = append(s, z)
	}
	s[i] = v
	return s
}

func jpxCalcCompDims(c *jpxComp, siz *jpxSIZ) {
	c.x0 = ceilDivJ(siz.XOsiz, c.XRsiz)
	c.x1 = ceilDivJ(siz.Xsiz, c.XRsiz)
	c.y0 = ceilDivJ(siz.YOsiz, c.YRsiz)
	c.y1 = ceilDivJ(siz.Ysiz, c.YRsiz)
	c.width = c.x1 - c.x0
	c.height = c.y1 - c.y0
}

func ceilDivJ(a, b int) int {
	return int(math.Ceil(float64(a) / float64(b)))
}

func jpxReadQuant(data []byte, j, end int) *jpxQuant {
	sqcd := int(data[j])
	j++
	q := &jpxQuant{guardBits: sqcd >> 5}
	var spqcdSize int
	switch sqcd & 0x1f {
	case 0:
		spqcdSize, q.scalarExpounded, q.noQuantization = 8, true, true
	case 1:
		spqcdSize, q.scalarExpounded = 16, false
	case 2:
		spqcdSize, q.scalarExpounded = 16, true
	default:
		spqcdSize = 16
	}
	for j < end {
		var sp jpxSPqcd
		if spqcdSize == 8 {
			sp.epsilon = int(data[j]) >> 3
			j++
		} else {
			sp.epsilon = int(data[j]) >> 3
			sp.mu = (int(data[j]&0x7) << 8) | int(data[j+1])
			j += 2
		}
		q.SPqcds = append(q.SPqcds, sp)
	}
	return q
}

func jpxReadCOD(data []byte, j, end int) (*jpxCOD, error) {
	cod := &jpxCOD{}
	scod := int(data[j])
	j++
	cod.entropyCoderWithCustomPrecincts = scod&1 != 0
	cod.sopMarkerUsed = scod&2 != 0
	cod.ephMarkerUsed = scod&4 != 0
	cod.progressionOrder = int(data[j])
	j++
	cod.layersCount = ru16(data, j)
	j += 2
	cod.multipleComponentTransform = int(data[j])
	j++
	cod.decompositionLevelsCount = int(data[j])
	j++
	cod.xcb = int(data[j]&0xf) + 2
	j++
	cod.ycb = int(data[j]&0xf) + 2
	j++
	bs := int(data[j])
	j++
	cod.bypass = bs&1 != 0
	cod.resetContextProbabilities = bs&2 != 0
	cod.termEachPass = bs&4 != 0
	cod.vertStripe = bs&8 != 0
	cod.predictableTerm = bs&16 != 0
	cod.segmentationSymbolUsed = bs&32 != 0
	cod.reversibleTransformation = data[j] != 0
	j++
	if cod.entropyCoderWithCustomPrecincts {
		for j < end {
			ps := int(data[j])
			j++
			cod.precinctsSizes = append(cod.precinctsSizes, jpxPP{PPx: ps & 0xf, PPy: ps >> 4})
		}
	}
	if cod.bypass || cod.termEachPass || cod.vertStripe || cod.predictableTerm {
		return nil, jpxErr
	}
	return cod, nil
}
