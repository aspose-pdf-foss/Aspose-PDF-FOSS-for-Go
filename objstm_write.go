// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"fmt"
	"sort"
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

// objectPackable reports whether an object may live inside an object stream:
// not a stream, not the encryption dictionary, and not a signature dictionary,
// whose /Contents and /ByteRange are patched in place by byte offset.
func objectPackable(id int, v pdfValue, encryptObjID int) bool {
	if id == encryptObjID && encryptObjID != 0 {
		return false
	}
	switch t := v.(type) {
	case *pdfStream:
		return false
	case pdfDict:
		return !isSignatureDict(t)
	}
	return true
}

// countPackableObjects counts the document's objects eligible for packing.
func (d *Document) countPackableObjects() int {
	n := 0
	for num, obj := range d.objects {
		if objectPackable(num, obj.Value, 0) {
			n++
		}
	}
	return n
}

// buildObjectStreamPDF lays out an assembled document with eligible objects
// packed into object streams and a cross-reference stream in place of the
// classic table. Numbering comes from assemble; object streams and the
// cross-reference stream take the numbers after it.
func buildObjectStreamPDF(d *Document, asm *assembled) ([]byte, error) {
	encState := asm.encState
	remapFn := asm.remapFn()
	identity := func(n int) int { return n }

	type item struct {
		id      int
		val     pdfValue
		remap   func(int) int
		encrypt bool
	}
	items := make([]item, 0, len(asm.contentIDs)+4)
	for _, oldID := range asm.contentIDs {
		items = append(items, item{asm.remap[oldID], d.objects[oldID].Value, remapFn, true})
	}
	kids := make(pdfArray, len(d.pages))
	for i, p := range d.pages {
		kids[i] = pdfDirectRef{Num: remapFn(p.Num)}
	}
	items = append(items, item{asm.pagesObjID,
		pdfDict{"/Type": pdfName("/Pages"), "/Count": len(d.pages), "/Kids": kids}, identity, false})
	items = append(items, item{asm.catalogObjID, pdfValue(asm.catalog), remapFn, true})
	if asm.infoObjID != 0 {
		items = append(items, item{asm.infoObjID, pdfValue(d.info), remapFn, true})
	}
	if asm.encryptObjID != 0 {
		items = append(items, item{asm.encryptObjID, pdfValue(buildEncryptDict(encState)), identity, false})
	}

	header := asm.header
	if header == "%PDF-1.4\n" {
		header = "%PDF-1.5\n"
	}
	var buf bytes.Buffer
	buf.WriteString(header)
	buf.WriteString("%\xe2\xe3\xcf\xd3\n")

	encFor := func(num int) func([]byte) ([]byte, error) {
		if encState == nil {
			return nil
		}
		return func(b []byte) ([]byte, error) { return encState.encryptBytes(num, 0, b) }
	}

	offsets := make(map[int]int64, len(items))
	var packed []packedObj
	for _, it := range items {
		if objectPackable(it.id, it.val, asm.encryptObjID) {
			// Written without per-object encryption: the object stream that
			// holds it is encrypted as a whole.
			var body bytes.Buffer
			if err := writeValue(&body, it.val, it.remap, nil); err != nil {
				return nil, err
			}
			packed = append(packed, packedObj{num: it.id, body: body.Bytes()})
			continue
		}
		offsets[it.id] = int64(buf.Len())
		var encFn func([]byte) ([]byte, error)
		if it.encrypt {
			encFn = encFor(it.id)
		}
		if err := writeObject(&buf, it.id, it.val, it.remap, encFn); err != nil {
			return nil, err
		}
	}
	sort.Slice(packed, func(i, j int) bool { return packed[i].num < packed[j].num })

	type location struct{ stream, index int }
	where := make(map[int]location, len(packed))
	nextID := asm.totalObjects
	for start := 0; start < len(packed); start += objStmCapacity {
		end := start + objStmCapacity
		if end > len(packed) {
			end = len(packed)
		}
		batch := packed[start:end]
		data, first := buildObjectStream(batch)
		num := nextID
		nextID++
		for i, o := range batch {
			where[o.num] = location{num, i}
		}
		offsets[num] = int64(buf.Len())
		st := &pdfStream{
			Dict:    pdfDict{"/Type": pdfName("/ObjStm"), "/N": len(batch), "/First": first},
			Data:    data,
			Decoded: true,
		}
		if err := writeObject(&buf, num, st, identity, encFor(num)); err != nil {
			return nil, err
		}
	}

	xrefNum := nextID
	size := xrefNum + 1
	xrefOff := int64(buf.Len())
	rows := make([]xrefStreamEntry, size)
	rows[0] = xrefStreamEntry{typ: 0, f2: 0, f3: 65535}
	for n := 1; n < size; n++ {
		if l, ok := where[n]; ok {
			rows[n] = xrefStreamEntry{typ: 2, f2: int64(l.stream), f3: l.index}
		} else if off, ok := offsets[n]; ok {
			rows[n] = xrefStreamEntry{typ: 1, f2: off}
		}
	}
	rows[xrefNum] = xrefStreamEntry{typ: 1, f2: xrefOff}
	data, w := encodeXRefEntries(rows)

	dict := pdfDict{
		"/Type": pdfName("/XRef"),
		"/Size": size,
		"/W":    w,
		"/Root": pdfDirectRef{Num: asm.catalogObjID},
	}
	if asm.infoObjID != 0 {
		dict["/Info"] = pdfDirectRef{Num: asm.infoObjID}
	}
	if encState != nil {
		dict["/Encrypt"] = pdfDirectRef{Num: asm.encryptObjID}
		dict["/ID"] = pdfArray{pdfHexString(encState.fileID), pdfHexString(encState.fileID)}
	}
	// The cross-reference stream is never encrypted (ISO 32000-1 §7.5.8.2).
	if err := writeObject(&buf, xrefNum, &pdfStream{Dict: dict, Data: data, Decoded: true}, identity, nil); err != nil {
		return nil, err
	}
	fmt.Fprintf(&buf, "startxref\n%d\n%%%%EOF\n", xrefOff)
	return buf.Bytes(), nil
}
