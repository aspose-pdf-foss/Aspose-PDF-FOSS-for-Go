// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"strconv"
)

// Object streams and cross-reference streams (ISO 32000-1 §7.5.7, §7.5.8),
// written when OptimizationOptions.CompressObjects is set. Non-stream objects
// are packed a hundred at a time into Flate-compressed /ObjStm streams, which
// compress far better together than as separate indirect objects, and the
// classic xref table becomes a compressed /XRef stream.

// objStmCapacity is how many objects one object stream holds. Large enough to
// compress well, small enough that a reader resolving one object does not
// inflate a huge stream.
const objStmCapacity = 100

// packedObj is one object destined for an object stream: its output number
// and its serialized value, without the "obj"/"endobj" wrapper.
type packedObj struct {
	num  int
	body []byte
}

// buildObjectStream returns the decoded data of an object stream holding objs
// and the /First offset: a header of "number offset" pairs, then the bodies,
// each offset counted from /First.
func buildObjectStream(objs []packedObj) (data []byte, first int) {
	var header, bodies bytes.Buffer
	for i, o := range objs {
		if i > 0 {
			header.WriteByte(' ')
		}
		header.WriteString(strconv.Itoa(o.num))
		header.WriteByte(' ')
		header.WriteString(strconv.Itoa(bodies.Len()))
		bodies.Write(o.body)
		bodies.WriteByte('\n')
	}
	header.WriteByte('\n')
	first = header.Len()
	return append(header.Bytes(), bodies.Bytes()...), first
}

// xrefStreamEntry is one row of a cross-reference stream (ISO 32000-1 §7.5.8.3
// Table 18): type 0 free (f2 next free object, f3 generation), type 1 in use
// (f2 byte offset, f3 generation), type 2 compressed (f2 object-stream number,
// f3 index within it).
type xrefStreamEntry struct {
	typ int
	f2  int64
	f3  int
}

// encodeXRefEntries packs rows big-endian with widths [1 w2 w3], each the
// smallest that holds the largest value in its column, and returns the /W
// array alongside.
func encodeXRefEntries(rows []xrefStreamEntry) (data []byte, w pdfArray) {
	var max2 int64
	max3 := 0
	for _, r := range rows {
		if r.f2 > max2 {
			max2 = r.f2
		}
		if r.f3 > max3 {
			max3 = r.f3
		}
	}
	w2, w3 := byteWidth(max2), byteWidth(int64(max3))
	data = make([]byte, 0, len(rows)*(1+w2+w3))
	for _, r := range rows {
		data = append(data, byte(r.typ))
		data = appendBigEndian(data, r.f2, w2)
		data = appendBigEndian(data, int64(r.f3), w3)
	}
	return data, pdfArray{1, w2, w3}
}

// byteWidth is the number of bytes needed to hold v, at least one.
func byteWidth(v int64) int {
	w := 1
	for v > 0xFF {
		v >>= 8
		w++
	}
	return w
}

// appendBigEndian appends the low width bytes of v, most significant first.
func appendBigEndian(dst []byte, v int64, width int) []byte {
	for i := width - 1; i >= 0; i-- {
		dst = append(dst, byte(v>>(8*uint(i))))
	}
	return dst
}
