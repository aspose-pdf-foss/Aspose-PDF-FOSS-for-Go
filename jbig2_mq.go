// SPDX-License-Identifier: MIT

package asposepdf

// MQ arithmetic decoder (ITU-T T.88 Annex E) in the JBIG2 software conventions:
// the code register C is kept as a 16-bit high half (chigh) and 16-bit low half
// (clow); A is the interval register and CT the bit counter. This mirrors the
// reference decoder used by jbig2dec / pdf.js. Probabilities come from mqStates.
type mqDecoder struct {
	data        []byte
	bp, end     int
	chigh, clow int
	a, ct       int
}

func newMQDecoder(data []byte, start, end int) *mqDecoder {
	if end > len(data) {
		end = len(data)
	}
	d := &mqDecoder{data: data, bp: start, end: end}
	if start < end {
		d.chigh = int(data[start])
	} else {
		d.chigh = 0xff
	}
	d.byteIn()
	d.chigh = ((d.chigh << 7) & 0xffff) | ((d.clow >> 9) & 0x7f)
	d.clow = (d.clow << 7) & 0xffff
	d.ct -= 7
	d.a = 0x8000
	return d
}

// at returns the data byte at i, or the 0xff padding the decoder assumes past
// the end of the stream.
func (d *mqDecoder) at(i int) int {
	if i < d.end {
		return int(d.data[i])
	}
	return 0xff
}

func (d *mqDecoder) byteIn() {
	if d.at(d.bp) == 0xff {
		if d.at(d.bp+1) > 0x8f {
			d.clow += 0xff00
			d.ct = 8
		} else {
			d.bp++
			d.clow += d.at(d.bp) << 9
			d.ct = 7
		}
	} else {
		d.bp++
		d.clow += d.at(d.bp) << 8
		d.ct = 8
	}
	if d.clow > 0xffff {
		d.chigh += d.clow >> 16
		d.clow &= 0xffff
	}
}

// readBit decodes one binary decision using the context stored at cx[label],
// updating that context's state in place. cx packs each context as
// (stateIndex<<1 | mps).
func (d *mqDecoder) readBit(cx []byte, label int) int {
	idx := int(cx[label] >> 1)
	mps := int(cx[label] & 1)
	st := mqStates[idx]
	qe := int(st.qe)
	d.a -= qe
	var bit int
	if d.chigh < qe {
		// LPS exchange.
		if d.a < qe {
			d.a = qe
			bit = mps
			idx = int(st.nmps)
		} else {
			d.a = qe
			bit = 1 ^ mps
			if st.sw == 1 {
				mps = bit
			}
			idx = int(st.nlps)
		}
	} else {
		d.chigh -= qe
		if d.a&0x8000 != 0 {
			cx[label] = byte(idx<<1 | mps)
			return mps
		}
		// MPS exchange.
		if d.a < qe {
			bit = 1 ^ mps
			if st.sw == 1 {
				mps = bit
			}
			idx = int(st.nlps)
		} else {
			bit = mps
			idx = int(st.nmps)
		}
	}
	// Renormalize.
	for {
		if d.ct == 0 {
			d.byteIn()
		}
		d.a <<= 1
		d.chigh = ((d.chigh << 1) & 0xffff) | ((d.clow >> 15) & 1)
		d.clow = (d.clow << 1) & 0xffff
		d.ct--
		if d.a&0x8000 != 0 {
			break
		}
	}
	cx[label] = byte(idx<<1 | mps)
	return bit
}

// mqIntCtx is the 512-entry context used by one arithmetic integer decoding
// procedure (IADH, IADW, IADT, …); each procedure owns its own instance.
type mqIntCtx struct{ cx []byte }

func newMQIntCtx() *mqIntCtx { return &mqIntCtx{cx: make([]byte, 512)} }

// decodeInt runs the arithmetic integer decoding procedure (ITU-T T.88 Annex
// A.3). It returns the decoded value, or oob=true for the out-of-band symbol.
func (d *mqDecoder) decodeInt(c *mqIntCtx) (value int, oob bool) {
	prev := 1
	bit := func() int {
		b := d.readBit(c.cx, prev)
		if prev < 256 {
			prev = (prev << 1) | b
		} else {
			prev = (((prev<<1 | b) & 511) | 256)
		}
		return b
	}
	s := bit()
	var n, offset int
	switch {
	case bit() == 0:
		n, offset = 2, 0
	case bit() == 0:
		n, offset = 4, 4
	case bit() == 0:
		n, offset = 6, 20
	case bit() == 0:
		n, offset = 8, 84
	case bit() == 0:
		n, offset = 12, 340
	default:
		n, offset = 32, 4436
	}
	v := 0
	for i := 0; i < n; i++ {
		v = (v << 1) | bit()
	}
	v += offset
	if s == 0 {
		return v, false
	}
	if v > 0 {
		return -v, false
	}
	return 0, true // OOB
}

// decodeIAID decodes a symbol ID of codeLen bits (ITU-T T.88 Annex A.3, IAID).
// cx must have at least 1<<(codeLen+1) entries.
func (d *mqDecoder) decodeIAID(cx []byte, codeLen int) int {
	prev := 1
	for i := 0; i < codeLen; i++ {
		b := d.readBit(cx, prev)
		prev = (prev << 1) | b
	}
	if codeLen < 31 {
		return prev & ((1 << uint(codeLen)) - 1)
	}
	return prev & 0x7fffffff
}
