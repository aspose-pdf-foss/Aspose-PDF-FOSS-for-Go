package asposepdf

import (
	"bytes"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// writePage writes a single-page PDF for the given page to path.
func writePage(doc *rawDocument, page *pageInfo, path string) error {
	data, err := buildPagePDF(doc, page)
	if err != nil {
		return err
	}
	return writeFile(path, data)
}

// buildPagePDF constructs a minimal valid PDF containing just the given page.
func buildPagePDF(doc *rawDocument, page *pageInfo) ([]byte, error) {
	return buildMultiPagePDF(doc, []*pageInfo{page})
}

// buildMultiPagePDF constructs a minimal valid PDF containing the given pages in order.
// All pages must come from the same document.
func buildMultiPagePDF(doc *rawDocument, pages []*pageInfo) ([]byte, error) {
	return buildMultiPagePDFEx(doc, pages, nil)
}

// buildMultiPagePDFEx is like buildMultiPagePDF but accepts per-page dict patches.
// pagePatches maps original object numbers to pdfDict entries that are merged into
// the page dict at write time (overwriting any existing keys with the same name).
func buildMultiPagePDFEx(doc *rawDocument, pages []*pageInfo, pagePatches map[int]pdfDict) ([]byte, error) {
	entries := make([]mutablePage, len(pages))
	for i, p := range pages {
		entries[i] = mutablePage{src: doc, page: p}
	}
	patches := make(map[patchKey]pdfDict, len(pagePatches))
	for objNum, d := range pagePatches {
		patches[patchKey{doc, objNum}] = d
	}
	return buildDocumentPDF(entries, patches, nil)
}

// writeValue serialises a PDF value to buf, remapping object reference numbers via remap.
// encFn, if non-nil, is called to encrypt string and stream data for the current object.
func writeValue(buf *bytes.Buffer, v pdfValue, remap func(int) int, encFn func([]byte) []byte) {
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
			writeHexBytes(buf, encFn([]byte(val)))
		} else {
			buf.WriteString("(")
			buf.WriteString(escapeLiteral(val))
			buf.WriteString(")")
		}
	case pdfRef:
		buf.WriteString(strconv.Itoa(remap(val.Num)))
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
		// Sort keys for deterministic output.
		keys := make([]string, 0, len(val))
		for k := range val {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			buf.WriteString("\n")
			buf.WriteString(k)
			buf.WriteString(" ")
			writeValue(buf, val[k], remap, encFn)
		}
		buf.WriteString("\n>>")
	case pdfArray:
		buf.WriteString("[")
		for i, item := range val {
			if i > 0 {
				buf.WriteString(" ")
			}
			writeValue(buf, item, remap, encFn)
		}
		buf.WriteString("]")
	case *pdfStream:
		// Write uncompressed stream (simpler; avoids re-encoding).
		d := make(pdfDict, len(val.Dict))
		for k, dv := range val.Dict {
			// Remove encoding filters — we write raw (decoded) data.
			if k == "/Filter" || k == "/DecodeParms" || k == "/FFilter" || k == "/FDecodeParms" {
				continue
			}
			d[k] = dv
		}
		data := val.Data
		if encFn != nil {
			data = encFn(data)
		}
		d["/Length"] = len(data)
		writeValue(buf, d, remap, encFn)
		buf.WriteString("\nstream\n")
		buf.Write(data)
		buf.WriteString("\nendstream")
	}
}

// writeHexBytes writes data as a PDF hex string: <hex...>.
func writeHexBytes(buf *bytes.Buffer, data []byte) {
	const hexChars = "0123456789abcdef"
	buf.WriteByte('<')
	for _, b := range data {
		buf.WriteByte(hexChars[b>>4])
		buf.WriteByte(hexChars[b&0xf])
	}
	buf.WriteByte('>')
}

func escapeLiteral(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '(':
			b.WriteString(`\(`)
		case ')':
			b.WriteString(`\)`)
		case '\\':
			b.WriteString(`\\`)
		default:
			b.WriteByte(s[i])
		}
	}
	return b.String()
}

func sortedKeys(m map[int]bool) []int {
	keys := make([]int, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Ints(keys)
	return keys
}

// buildDocumentPDF constructs a PDF from a mutable page list with optional per-page patches.
// Pages may come from multiple source documents in any order.
// encCfg, if non-nil, enables RC4-128 encryption of the output.
func buildDocumentPDF(entries []mutablePage, patches map[patchKey]pdfDict, encCfg *encryptConfig) ([]byte, error) {
	if len(entries) == 0 {
		return nil, fmt.Errorf("no pages to write")
	}

	type objKey struct {
		src    *rawDocument
		objNum int
	}

	// Assign globally unique new object numbers to every dependency object.
	// 1 = Catalog, 2 = Pages, 3..N = content objects.
	globalMap := make(map[objKey]int)
	newNum := 3
	for _, e := range entries {
		for num := range e.page.deps {
			key := objKey{e.src, num}
			if _, ok := globalMap[key]; !ok {
				globalMap[key] = newNum
				newNum++
			}
		}
	}

	// If encryption is requested, compute keys and reserve an object number for
	// the /Encrypt dictionary (it is NOT itself encrypted).
	var encState *encryptState
	var encryptObjNum int
	if encCfg != nil {
		var err error
		encState, err = newEncryptState(encCfg)
		if err != nil {
			return nil, fmt.Errorf("init encryption: %w", err)
		}
		encryptObjNum = newNum
		newNum++
	}

	isPageObj := make(map[objKey]bool)
	for _, e := range entries {
		isPageObj[objKey{e.src, e.page.objNum}] = true
	}

	const catalogNum = 1
	const pagesNum = 2

	var buf bytes.Buffer
	buf.Grow(128 * 1024)
	offsets := make(map[int]int64)

	buf.WriteString("%PDF-1.4\n")
	buf.WriteString("%\xe2\xe3\xcf\xd3\n")

	// Collect all unique (src, objNum) pairs in deterministic order
	// (sorted by their assigned new object number).
	type srcDep struct {
		src    *rawDocument
		objNum int
	}
	allDeps := make([]srcDep, 0, len(globalMap))
	seen := make(map[objKey]bool)
	for _, e := range entries {
		for num := range e.page.deps {
			key := objKey{e.src, num}
			if !seen[key] {
				seen[key] = true
				allDeps = append(allDeps, srcDep{e.src, num})
			}
		}
	}
	sort.Slice(allDeps, func(i, j int) bool {
		return globalMap[objKey{allDeps[i].src, allDeps[i].objNum}] <
			globalMap[objKey{allDeps[j].src, allDeps[j].objNum}]
	})

	for _, dep := range allDeps {
		key := objKey{dep.src, dep.objNum}
		nn := globalMap[key]
		offsets[nn] = int64(buf.Len())

		srcDoc := dep.src
		remap := func(oldNum int) int {
			if nn2, ok := globalMap[objKey{srcDoc, oldNum}]; ok {
				return nn2
			}
			return oldNum
		}

		obj, err := dep.src.getObject(dep.objNum)
		if err != nil {
			fmt.Fprintf(&buf, "%d 0 obj\nnull\nendobj\n", nn)
			continue
		}

		val := obj.Value
		if isPageObj[key] {
			if d, ok := val.(pdfDict); ok {
				patched := make(pdfDict, len(d))
				for k, v := range d {
					patched[k] = v
				}
				patched["/Parent"] = pdfDirectRef{Num: pagesNum, Gen: 0}
				pk := patchKey{dep.src, dep.objNum}
				for k, v := range patches[pk] {
					patched[k] = v
				}
				val = patched
			}
		}

		// Build a per-object encrypt function (nil when no encryption).
		var encFn func([]byte) []byte
		if encState != nil {
			objNum := nn // capture for closure
			encFn = func(data []byte) []byte {
				return encState.encryptBytes(objNum, data)
			}
		}

		fmt.Fprintf(&buf, "%d 0 obj\n", nn)
		writeValue(&buf, val, remap, encFn)
		buf.WriteString("\nendobj\n")
	}

	// Write Pages object (contains only names and refs — no encryption needed).
	offsets[pagesNum] = int64(buf.Len())
	fmt.Fprintf(&buf, "%d 0 obj\n<< /Type /Pages /Kids [", pagesNum)
	for i, e := range entries {
		if i > 0 {
			buf.WriteByte(' ')
		}
		fmt.Fprintf(&buf, "%d 0 R", globalMap[objKey{e.src, e.page.objNum}])
	}
	fmt.Fprintf(&buf, "] /Count %d >>\nendobj\n", len(entries))

	// Write Catalog (contains only names and refs — no encryption needed).
	offsets[catalogNum] = int64(buf.Len())
	fmt.Fprintf(&buf, "%d 0 obj\n<< /Type /Catalog /Pages %d 0 R >>\nendobj\n",
		catalogNum, pagesNum)

	// Write /Encrypt dictionary (if encryption is active).
	// Per PDF spec, the /Encrypt object itself is never encrypted.
	if encState != nil {
		offsets[encryptObjNum] = int64(buf.Len())
		fmt.Fprintf(&buf, "%d 0 obj\n", encryptObjNum)
		fmt.Fprintf(&buf, "<< /Filter /Standard /V 2 /R 3 /Length 128 /P %d\n/O ", int(encryptPermissions))
		writeHexBytes(&buf, encState.ownerEntry)
		buf.WriteString("\n/U ")
		writeHexBytes(&buf, encState.userEntry)
		buf.WriteString("\n>>\nendobj\n")
	}

	// Write xref table.
	xrefOffset := int64(buf.Len())
	fmt.Fprintf(&buf, "xref\n0 %d\n", newNum)
	buf.WriteString("0000000000 65535 f\r\n")
	for i := 1; i < newNum; i++ {
		off, ok := offsets[i]
		if !ok {
			buf.WriteString("0000000000 65535 f\r\n")
		} else {
			fmt.Fprintf(&buf, "%010d 00000 n\r\n", off)
		}
	}

	// Write trailer.
	fmt.Fprintf(&buf, "trailer\n<< /Size %d /Root %d 0 R", newNum, catalogNum)
	if encState != nil {
		fmt.Fprintf(&buf, " /Encrypt %d 0 R /ID [", encryptObjNum)
		writeHexBytes(&buf, encState.fileID)
		buf.WriteByte(' ')
		writeHexBytes(&buf, encState.fileID)
		buf.WriteByte(']')
	}
	fmt.Fprintf(&buf, " >>\nstartxref\n%d\n%%%%EOF\n", xrefOffset)

	return buf.Bytes(), nil
}
