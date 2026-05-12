package asposepdf

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// buildDocumentPDF serializes d to a PDF byte slice.
func buildDocumentPDF(d *Document) ([]byte, error) {
	var encState *encryptState
	if d.preserved != nil {
		// Reuse the original /O, /U, /P, /ID bytes verbatim so that BOTH the
		// original user and owner passwords survive edit-in-place re-save.
		encState = d.preserved
	} else if d.encrypt != nil {
		var err error
		encState, err = newEncryptState(d.encrypt)
		if err != nil {
			return nil, fmt.Errorf("encrypt: %w", err)
		}
	}

	// Assign sequential output IDs to all content objects.
	// Reserve IDs for structural objects built by the writer.
	contentIDs := sortedObjectIDs(d.objects)
	remap := make(map[int]int, len(contentIDs))
	nextOut := 1
	for _, id := range contentIDs {
		remap[id] = nextOut
		nextOut++
	}
	pagesObjID := nextOut
	nextOut++
	catalogObjID := nextOut
	nextOut++
	var infoObjID int
	if d.info != nil {
		infoObjID = nextOut
		nextOut++
	}
	var encryptObjID int
	if encState != nil {
		encryptObjID = nextOut
		nextOut++
	}
	totalObjects := nextOut // exclusive upper bound

	remapFn := func(n int) int {
		if out, ok := remap[n]; ok {
			return out
		}
		return n
	}

	// Patch /Parent in every page dict to point to the new /Pages node.
	// Use pdfDirectRef so the writer outputs the ID as-is without remapping.
	for _, page := range d.pages {
		if dict, ok := page.Value.(pdfDict); ok {
			dict["/Parent"] = pdfDirectRef{Num: pagesObjID}
		}
	}

	var buf bytes.Buffer
	buf.WriteString("%PDF-1.4\n")
	buf.WriteString("%\xe2\xe3\xcf\xd3\n") // binary marker

	offsets := make(map[int]int64, totalObjects)

	// Write content objects.
	for _, oldID := range contentIDs {
		obj := d.objects[oldID]
		outID := remap[oldID]
		offsets[outID] = int64(buf.Len())
		var encFn func([]byte) ([]byte, error)
		if encState != nil {
			encFn = func(b []byte) ([]byte, error) { return encState.encryptBytes(outID, 0, b) }
		}
		if err := writeObject(&buf, outID, obj.Value, remapFn, encFn); err != nil {
			return nil, err
		}
	}

	// Write /Pages node.
	offsets[pagesObjID] = int64(buf.Len())
	writePageTreeNode(&buf, pagesObjID, d.pages, remapFn)

	// Write /Catalog. Preserve every field from the original catalog
	// (/Outlines, /AcroForm, /Names, /PageLabels, /Metadata, etc.) so
	// a Save+Reopen roundtrip is lossless. /Pages is replaced with the
	// writer-built node; other refs are remapped by writeValue. Deep-copy
	// so that building the output catalog never aliases d.catalog.
	offsets[catalogObjID] = int64(buf.Len())
	catOut := make(pdfDict, len(d.catalog)+2)
	for k, v := range d.catalog {
		if k == "/Pages" {
			continue
		}
		catOut[k] = deepCopyValue(v)
	}
	catOut["/Type"] = pdfName("/Catalog")
	catOut["/Pages"] = pdfDirectRef{Num: pagesObjID}
	var catalogEncFn func([]byte) ([]byte, error)
	if encState != nil {
		catalogEncFn = func(b []byte) ([]byte, error) { return encState.encryptBytes(catalogObjID, 0, b) }
	}
	if err := writeObject(&buf, catalogObjID, pdfValue(catOut), remapFn, catalogEncFn); err != nil {
		return nil, err
	}

	// Write /Info if present.
	if infoObjID != 0 {
		offsets[infoObjID] = int64(buf.Len())
		var encFn func([]byte) ([]byte, error)
		if encState != nil {
			encFn = func(b []byte) ([]byte, error) { return encState.encryptBytes(infoObjID, 0, b) }
		}
		if err := writeObject(&buf, infoObjID, pdfValue(d.info), remapFn, encFn); err != nil {
			return nil, err
		}
	}

	// Write /Encrypt if present.
	if encryptObjID != 0 {
		offsets[encryptObjID] = int64(buf.Len())
		encDict := buildEncryptDict(encState)
		writeObject(&buf, encryptObjID, pdfValue(encDict), func(n int) int { return n }, nil)
	}

	// Write xref table.
	xrefOffset := int64(buf.Len())
	fmt.Fprintf(&buf, "xref\n0 %d\n", totalObjects)
	fmt.Fprintf(&buf, "0000000000 65535 f \n")
	for i := 1; i < totalObjects; i++ {
		off, ok := offsets[i]
		if !ok {
			fmt.Fprintf(&buf, "0000000000 00000 f \n")
		} else {
			fmt.Fprintf(&buf, "%010d 00000 n \n", off)
		}
	}

	// Write trailer.
	buf.WriteString("trailer\n<<\n")
	fmt.Fprintf(&buf, "/Size %d\n", totalObjects)
	fmt.Fprintf(&buf, "/Root %d 0 R\n", catalogObjID)
	if infoObjID != 0 {
		fmt.Fprintf(&buf, "/Info %d 0 R\n", infoObjID)
	}
	if encState != nil {
		fmt.Fprintf(&buf, "/Encrypt %d 0 R\n", encryptObjID)
		buf.WriteString("/ID [")
		writeHexBytes(&buf, encState.fileID)
		buf.WriteString(" ")
		writeHexBytes(&buf, encState.fileID)
		buf.WriteString("]\n")
	}
	buf.WriteString(">>\n")
	fmt.Fprintf(&buf, "startxref\n%d\n%%%%EOF\n", xrefOffset)

	return buf.Bytes(), nil
}

// writePageTreeNode writes the /Pages node with kids pointing to pages.
func writePageTreeNode(buf *bytes.Buffer, pagesObjID int, pages []*pdfObject, remapFn func(int) int) {
	fmt.Fprintf(buf, "%d 0 obj\n<<\n/Type /Pages\n/Count %d\n/Kids [", pagesObjID, len(pages))
	for i, page := range pages {
		if i > 0 {
			buf.WriteString(" ")
		}
		fmt.Fprintf(buf, "%d 0 R", remapFn(page.Num))
	}
	buf.WriteString("]\n>>\nendobj\n")
}

// writeObject writes "N 0 obj\n...\nendobj\n" for the given value.
func writeObject(buf *bytes.Buffer, id int, v pdfValue, remapFn func(int) int, encFn func([]byte) ([]byte, error)) error {
	fmt.Fprintf(buf, "%d 0 obj\n", id)
	if err := writeValue(buf, v, remapFn, encFn); err != nil {
		return err
	}
	buf.WriteString("\nendobj\n")
	return nil
}

// sortedObjectIDs returns the object IDs from objects in ascending order.
func sortedObjectIDs(objects map[int]*pdfObject) []int {
	ids := make([]int, 0, len(objects))
	for id := range objects {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	return ids
}

// writeValue serialises a PDF value to buf, remapping object reference numbers via remapFn.
// encFn, if non-nil, is called to encrypt string and stream data for the current object.
func writeValue(buf *bytes.Buffer, v pdfValue, remapFn func(int) int, encFn func([]byte) ([]byte, error)) error {
	switch val := v.(type) {
	case pdfNull:
		buf.WriteString("null")
	case bool:
		if val {
			buf.WriteString("true")
		} else {
			buf.WriteString("false")
		}
	case int:
		buf.WriteString(strconv.Itoa(val))
	case float64:
		buf.WriteString(strconv.FormatFloat(val, 'g', -1, 64))
	case pdfName:
		buf.WriteString(string(val))
	case string:
		if encFn != nil {
			enc, err := encFn([]byte(val))
			if err != nil {
				return err
			}
			writeHexBytes(buf, enc)
		} else {
			buf.WriteString("(")
			buf.WriteString(escapeLiteral(val))
			buf.WriteString(")")
		}
	case pdfHexString:
		if encFn != nil {
			enc, err := encFn([]byte(val))
			if err != nil {
				return err
			}
			writeHexBytes(buf, enc)
		} else {
			writeHexBytes(buf, []byte(val))
		}
	case pdfRef:
		buf.WriteString(strconv.Itoa(remapFn(val.Num)))
		buf.WriteByte(' ')
		buf.WriteString(strconv.Itoa(val.Gen))
		buf.WriteString(" R")
	case pdfDirectRef:
		// Already in new object space — write as-is, no remapping.
		buf.WriteString(strconv.Itoa(val.Num))
		buf.WriteByte(' ')
		buf.WriteString(strconv.Itoa(val.Gen))
		buf.WriteString(" R")
	case pdfDict:
		buf.WriteString("<<")
		keys := make([]string, 0, len(val))
		for k := range val {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			buf.WriteString("\n")
			buf.WriteString(k)
			buf.WriteString(" ")
			if err := writeValue(buf, val[k], remapFn, encFn); err != nil {
				return err
			}
		}
		buf.WriteString("\n>>")
	case pdfArray:
		buf.WriteString("[")
		for i, item := range val {
			if i > 0 {
				buf.WriteString(" ")
			}
			if err := writeValue(buf, item, remapFn, encFn); err != nil {
				return err
			}
		}
		buf.WriteString("]")
	case *pdfStream:
		d := make(pdfDict, len(val.Dict))
		for k, dv := range val.Dict {
			// Remove encoding filters — we re-compress decoded data below.
			// If Decoded==false the raw compressed bytes are preserved as-is,
			// so the original /Filter must stay in the dict.
			if val.Decoded && (k == "/Filter" || k == "/DecodeParms" || k == "/FFilter" || k == "/FDecodeParms") {
				continue
			}
			d[k] = dv
		}
		data := val.Data
		if val.Decoded {
			var zbuf bytes.Buffer
			w := zlib.NewWriter(&zbuf)
			w.Write(data)
			w.Close()
			data = zbuf.Bytes()
			d["/Filter"] = pdfName("/FlateDecode")
		}
		if encFn != nil {
			enc, err := encFn(data)
			if err != nil {
				return err
			}
			data = enc
		}
		d["/Length"] = len(data)
		if err := writeValue(buf, d, remapFn, nil); err != nil { // dict keys are not encrypted
			return err
		}
		buf.WriteString("\nstream\n")
		buf.Write(data)
		buf.WriteString("\nendstream")
	}
	return nil
}

// writeHexBytes writes b as a PDF hex string <AABB...>.
func writeHexBytes(buf *bytes.Buffer, b []byte) {
	const hex = "0123456789abcdef"
	buf.WriteByte('<')
	for _, c := range b {
		buf.WriteByte(hex[c>>4])
		buf.WriteByte(hex[c&0xf])
	}
	buf.WriteByte('>')
}

// escapeLiteral escapes special characters in a PDF literal string.
func escapeLiteral(s string) string {
	var b strings.Builder
	for _, c := range []byte(s) {
		switch c {
		case '(', ')', '\\':
			b.WriteByte('\\')
			b.WriteByte(c)
		case '\r':
			b.WriteString(`\r`)
		case '\n':
			b.WriteString(`\n`)
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}

// buildEncryptDict builds the /Encrypt dictionary for the standard
// security handler. Branches on s.algorithm:
//   - EncryptionAlgRC4_128 → V=2 R=3, no /CF (default crypt filter is
//     implicit RC4 for V<4).
//   - EncryptionAlgAES128 → V=4 R=4 with /CF/StdCF/CFM /AESV2 and
//     /StmF and /StrF pointing to /StdCF. ISO 32000-1 §7.6.3.2.
//
// /O and /U are raw 32-byte binary values emitted as hex strings to
// avoid literal-string parsing ambiguities (embedded NULs, etc.).
func buildEncryptDict(s *encryptState) pdfDict {
	dict := pdfDict{
		"/Filter": pdfName("/Standard"),
		"/Length": 128,
		// /P is a 32-bit bitfield. Emit as unsigned so it reads as a positive
		// integer (e.g. 4294967292 for grant-all), matching Adobe/pypdf
		// convention and avoiding signed-int interop pitfalls.
		"/P": int(uint32(s.permissions)),
		"/O": pdfHexString(s.ownerEntry),
		"/U": pdfHexString(s.userEntry),
	}
	switch s.algorithm {
	case EncryptionAlgAES128:
		dict["/V"] = 4
		dict["/R"] = 4
		dict["/CF"] = pdfDict{
			"/StdCF": pdfDict{
				"/Type":      pdfName("/CryptFilter"),
				"/CFM":       pdfName("/AESV2"),
				"/AuthEvent": pdfName("/DocOpen"),
				"/Length":    16,
			},
		}
		dict["/StmF"] = pdfName("/StdCF")
		dict["/StrF"] = pdfName("/StdCF")
	default: // EncryptionAlgRC4_128
		dict["/V"] = 2
		dict["/R"] = 3
	}
	return dict
}
