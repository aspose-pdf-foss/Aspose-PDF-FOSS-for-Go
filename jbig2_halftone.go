// SPDX-License-Identifier: MIT

package asposepdf

import "encoding/binary"

// Pattern dictionaries and halftone regions (ITU-T T.88 §6.6, §6.7). A pattern
// dictionary holds a set of equal-size pattern bitmaps (a collective bitmap split
// by index); a halftone region decodes a grayscale image (Gray-coded bitplanes)
// and stamps the indexed pattern at each grid position.

func (st *jbig2State) handlePatternDict(s jbig2Segment) {
	d := s.data
	if len(d) < 7 {
		return
	}
	flags := d[0]
	hdmmr := flags & 1
	hdTemplate := int(flags>>1) & 3
	hdpw := int(d[1])
	hdph := int(d[2])
	grayMax := int(binary.BigEndian.Uint32(d[3:]))
	if hdpw <= 0 || hdph <= 0 || grayMax < 0 || grayMax > 1<<16 {
		return
	}
	n := grayMax + 1
	collW := n * hdpw
	var collective jbig2Bitmap
	if hdmmr == 1 {
		collective = jbig2MMRDecode(d[7:], collW, hdph)
	} else {
		at := []jbig2Point{{-hdpw, 0}, {-3, -1}, {2, -2}, {-2, -2}}
		if hdTemplate != 0 {
			at = []jbig2Point{{-hdpw, 0}}
		}
		ctx := newJBIG2Ctx(1)
		dec := newMQDecoder(d, 7, len(d))
		collective = decodeGenericBitmap(collW, hdph, hdTemplate, false, nil, at, dec, ctx.gb)
	}
	patterns := make([]jbig2Bitmap, n)
	for i := 0; i < n; i++ {
		pat := newJBIG2Bitmap(hdpw, hdph)
		for y := 0; y < hdph && y < len(collective); y++ {
			for x := 0; x < hdpw; x++ {
				sx := i*hdpw + x
				if sx < len(collective[y]) {
					pat[y][x] = collective[y][sx]
				}
			}
		}
		patterns[i] = pat
	}
	st.patterns[s.number] = patterns
}

func (st *jbig2State) handleHalftoneRegion(s jbig2Segment) {
	d := s.data
	if len(d) < 17+1+16 {
		return
	}
	rw := int(binary.BigEndian.Uint32(d[0:]))
	rh := int(binary.BigEndian.Uint32(d[4:]))
	rx := int(binary.BigEndian.Uint32(d[8:]))
	ry := int(binary.BigEndian.Uint32(d[12:]))
	regionCombo := int(d[16]) & 7
	p := 17

	hflags := d[p]
	p++
	hmmr := hflags & 1
	hTemplate := int(hflags>>1) & 3
	hEnableSkip := (hflags>>3)&1 == 1
	hCombOp := int(hflags>>4) & 7
	hDefPixel := int(hflags>>7) & 1

	hgw := int(binary.BigEndian.Uint32(d[p:]))
	hgh := int(binary.BigEndian.Uint32(d[p+4:]))
	hgx := int(int32(binary.BigEndian.Uint32(d[p+8:])))
	hgy := int(int32(binary.BigEndian.Uint32(d[p+12:])))
	p += 16
	if p+4 > len(d) {
		return
	}
	hrx := int(binary.BigEndian.Uint16(d[p:]))
	hry := int(binary.BigEndian.Uint16(d[p+2:]))
	p += 4

	// Patterns come from the referred-to pattern dictionary.
	var patterns []jbig2Bitmap
	for _, r := range s.refs {
		if pats, ok := st.patterns[r]; ok {
			patterns = pats
			break
		}
	}
	if len(patterns) == 0 || rw <= 0 || rh <= 0 || hgw <= 0 || hgh <= 0 ||
		rw > 1<<20 || rh > 1<<20 || hgw > 1<<20 || hgh > 1<<20 {
		return
	}
	hpw := len(patterns[0][0])
	hph := len(patterns[0])

	// Optional skip bitmap: grid cells whose pattern lies entirely off-region.
	var skip jbig2Bitmap
	if hEnableSkip {
		skip = newJBIG2Bitmap(hgw, hgh)
		for m := 0; m < hgh; m++ {
			for nn := 0; nn < hgw; nn++ {
				x := (hgx + m*hry + nn*hrx) >> 8
				y := (hgy + m*hrx - nn*hry) >> 8
				if x+hpw <= 0 || x >= rw || y+hph <= 0 || y >= rh {
					skip[m][nn] = 1
				}
			}
		}
	}

	bpp := jbig2Log2(len(patterns))
	gray := st.decodeGrayscale(d, p, hgw, hgh, bpp, hmmr, hTemplate, skip)

	region := newJBIG2Bitmap(rw, rh)
	if hDefPixel != 0 {
		for i := range region {
			for j := range region[i] {
				region[i][j] = 1
			}
		}
	}
	for m := 0; m < hgh; m++ {
		for nn := 0; nn < hgw; nn++ {
			if skip != nil && skip[m][nn] != 0 {
				continue
			}
			gi := gray[m][nn]
			if gi < 0 || gi >= len(patterns) {
				continue
			}
			x := (hgx + m*hry + nn*hrx) >> 8
			y := (hgy + m*hrx - nn*hry) >> 8
			jbig2Blit(region, patterns[gi], x, y, hCombOp)
		}
	}
	st.composite(region, rx, ry, regionCombo)
}

// decodeGrayscale decodes the bpp Gray-coded bitplanes of a halftone grayscale
// image (ITU-T T.88 Annex C.5) and returns the per-cell pattern indices.
func (st *jbig2State) decodeGrayscale(d []byte, off, hgw, hgh, bpp int, hmmr byte, hTemplate int, skip jbig2Bitmap) [][]int {
	planes := make([]jbig2Bitmap, bpp)
	at := []jbig2Point{{3, -1}, {-3, -1}, {2, -2}, {-2, -2}}
	if hTemplate >= 2 {
		at = []jbig2Point{{2, -1}}
	} else if hTemplate == 1 {
		at = []jbig2Point{{3, -1}}
	}

	if hmmr == 1 {
		// All planes are MMR-coded consecutively (each terminated by EOFB); decode
		// from a single advancing reader over the data.
		pos := off
		for j := bpp - 1; j >= 0; j-- {
			planes[j] = jbig2MMRDecodeAdvance(d, &pos, hgw, hgh)
		}
	} else {
		ctx := newJBIG2Ctx(1)
		dec := newMQDecoder(d, off, len(d))
		for j := bpp - 1; j >= 0; j-- {
			planes[j] = decodeGenericBitmap(hgw, hgh, hTemplate, false, skip, at, dec, ctx.gb)
		}
	}

	gray := make([][]int, hgh)
	for m := 0; m < hgh; m++ {
		gray[m] = make([]int, hgw)
		for n := 0; n < hgw; n++ {
			bit := 0
			if bpp > 0 && m < len(planes[bpp-1]) && n < len(planes[bpp-1][m]) {
				bit = int(planes[bpp-1][m][n])
			}
			val := bit
			for j := bpp - 2; j >= 0; j-- {
				pb := 0
				if m < len(planes[j]) && n < len(planes[j][m]) {
					pb = int(planes[j][m][n])
				}
				bit ^= pb
				val = val<<1 | bit
			}
			gray[m][n] = val
		}
	}
	return gray
}

// jbig2MMRDecodeAdvance decodes one MMR (Group 4) bitplane starting at *pos and
// advances *pos past the bytes consumed (byte-aligned, skipping a trailing EOFB),
// so consecutive halftone bitplanes can be decoded from one buffer.
func jbig2MMRDecodeAdvance(d []byte, pos *int, width, height int) jbig2Bitmap {
	bm := newJBIG2Bitmap(width, height)
	if len(bm) != height || width <= 0 {
		return bm
	}
	br := &ccittBits{data: d[*pos:]}
	ref := []int{width, width}
	for y := 0; y < height; y++ {
		cur, ok := ccittDecodeRow(br, ref, width, -1)
		if !ok {
			break
		}
		row := bm[y]
		color := 0
		p := 0
		for _, c := range cur {
			if c > width {
				c = width
			}
			if color == 1 {
				for x := p; x < c; x++ {
					row[x] = 1
				}
			}
			p = c
			color ^= 1
			if p >= width {
				break
			}
		}
		ref = append(cur[:len(cur):len(cur)], width, width)
	}
	// Skip an EOFB (two EOL codes: 24 zero/one bits) if present, then byte-align.
	br.align()
	*pos += br.pos
	return bm
}
