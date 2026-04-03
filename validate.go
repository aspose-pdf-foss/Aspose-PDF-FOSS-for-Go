package asposepdf

import (
	"bytes"
	"fmt"
	"regexp"
)

// ValidationIssue describes a single problem found in a PDF file.
type ValidationIssue struct {
	Code    string // e.g. "INVALID_HEADER", "XREF_ERROR", "OBJECT_ERROR", "PAGE_TREE_ERROR", "ENCRYPTED"
	Message string
}

// ValidationReport is returned by Validate and summarises the structural integrity of a PDF.
type ValidationReport struct {
	// Valid is true when no issues were found.
	Valid  bool
	Issues []ValidationIssue
}

func (r *ValidationReport) add(code, msg string) {
	r.Valid = false
	r.Issues = append(r.Issues, ValidationIssue{Code: code, Message: msg})
}

// Validate checks a PDF file for structural integrity.
// It verifies the file header, cross-reference table, all indirect objects,
// and the page tree. Encrypted documents are flagged with an ENCRYPTED issue
// but are not treated as invalid.
//
// A non-nil error is returned only for I/O failures (file not found, etc.).
// PDF-level problems are reported inside ValidationReport.Issues.
//
// Example:
//
//	report, err := asposepdf.Validate("document.pdf")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	if !report.Valid {
//	    for _, issue := range report.Issues {
//	        fmt.Println(issue.Code, issue.Message)
//	    }
//	}
func Validate(inputPath string) (*ValidationReport, error) {
	data, err := readFile(inputPath)
	if err != nil {
		return nil, err
	}

	report := &ValidationReport{Valid: true}

	// 1. Check PDF header.
	if !bytes.HasPrefix(data, []byte("%PDF-")) {
		report.add("INVALID_HEADER", "file does not begin with %PDF-")
	}

	// 2. Parse xref and trailer (structural check).
	doc, err := openDocumentFromBytes(data)
	if err != nil {
		report.add("XREF_ERROR", err.Error())
		// Cannot proceed further without a valid document structure.
		return report, nil
	}

	// 3. Detect encryption.
	if _, ok := doc.trailer["/Encrypt"]; ok {
		report.add("ENCRYPTED", "document is password-protected")
	}

	// 4. Verify every non-free object in the xref is readable.
	for objNum, entry := range doc.xref.entries {
		if entry.Free {
			continue
		}
		if _, err := doc.getObject(objNum); err != nil {
			report.add("OBJECT_ERROR", fmt.Sprintf("object %d: %s", objNum, err))
		}
	}

	// 5. Validate the page tree.
	if _, err := doc.pages(); err != nil {
		report.add("PAGE_TREE_ERROR", err.Error())
	}

	// 6. Check for orphaned /Pages objects — objects with /Type /Pages that are
	// not reachable from the root page tree. This can happen when a splitter bug
	// copies the original page tree nodes into the output alongside the new /Pages node.
	if err := validateNoOrphanedPagesNodes(doc, report); err != nil {
		report.add("PAGE_TREE_ERROR", fmt.Sprintf("orphan check failed: %s", err))
	}

	// 7. Verify that every /Page object's /Parent resolves to a /Pages node.
	// A misremapped /Parent reference (e.g. pointing to a content stream) would
	// cause Acrobat to reject the file even though the page tree traversal succeeds.
	validatePageParentRefs(doc, report)

	// 8. Detect streams whose /Filter was stripped but data is still compressed.
	// This happens when a writer copies a stream it could not decode (e.g. DCTDecode/
	// JPEG) and then removes the /Filter entry, leaving opaque bytes with no encoding
	// declaration — Acrobat reports "Insufficient data for an image".
	validateStreamFilters(doc, report)

	return report, nil
}

// validateStreamFilters reports a STREAM_ERROR for every stream that has no
// /Filter in its dict but whose data begins with a known compression magic
// sequence (zlib or JPEG), indicating that the filter was incorrectly stripped.
func validateStreamFilters(doc *rawDocument, report *ValidationReport) {
	for objNum, entry := range doc.xref.entries {
		if entry.Free {
			continue
		}
		obj, err := doc.getObject(objNum)
		if err != nil {
			continue
		}
		s, ok := obj.Value.(*pdfStream)
		if !ok {
			continue
		}
		// Only streams that declare no filter are candidates.
		if _, hasFilter := s.Dict["/Filter"]; hasFilter {
			continue
		}
		if looksCompressed(s.Data) {
			report.add("STREAM_ERROR", fmt.Sprintf(
				"object %d: stream has no /Filter but data appears compressed (filter may have been stripped)",
				objNum,
			))
		}
	}
}

// looksCompressed returns true when data begins with a well-known compression
// magic sequence that should never appear in raw (unencoded) PDF stream content.
func looksCompressed(data []byte) bool {
	if len(data) < 3 {
		return false
	}
	// JPEG: FF D8 FF
	if data[0] == 0xFF && data[1] == 0xD8 && data[2] == 0xFF {
		return true
	}
	// zlib (deflate): first byte 0x78, second byte one of 01 / 5E / 9C / DA
	if data[0] == 0x78 {
		switch data[1] {
		case 0x01, 0x5E, 0x9C, 0xDA:
			return true
		}
	}
	return false
}

// validatePageParentRefs checks that every /Type /Page object has a /Parent that
// resolves to an object with /Type /Pages.
func validatePageParentRefs(doc *rawDocument, report *ValidationReport) {
	for objNum, entry := range doc.xref.entries {
		if entry.Free {
			continue
		}
		obj, err := doc.getObject(objNum)
		if err != nil {
			continue
		}
		d, ok := obj.Value.(pdfDict)
		if !ok {
			continue
		}
		if dictGetName(d, "/Type") != "/Page" {
			continue
		}
		parentVal, ok := d["/Parent"]
		if !ok {
			report.add("PAGE_TREE_ERROR", fmt.Sprintf("page object %d has no /Parent", objNum))
			continue
		}
		parentDict, err := doc.resolveDict(parentVal)
		if err != nil {
			report.add("PAGE_TREE_ERROR", fmt.Sprintf("page object %d: /Parent cannot be resolved: %s", objNum, err))
			continue
		}
		if dictGetName(parentDict, "/Type") != "/Pages" {
			report.add("PAGE_TREE_ERROR", fmt.Sprintf("page object %d: /Parent does not point to a /Pages node", objNum))
		}
	}
}

// validateNoOrphanedPagesNodes reports a PAGE_TREE_ERROR for every /Pages object
// that exists in the xref but is not reachable from the root page tree.
func validateNoOrphanedPagesNodes(doc *rawDocument, report *ValidationReport) error {
	// Collect all /Pages node numbers reachable from the Catalog.
	reachable := make(map[int]bool)
	rootRef, ok := doc.trailer["/Root"]
	if !ok {
		return nil // already caught by page-tree check
	}
	catalog, err := doc.resolveDict(rootRef)
	if err != nil {
		return nil
	}
	pagesRef, ok := catalog["/Pages"]
	if !ok {
		return nil
	}
	collectPagesNodes(doc, pagesRef, reachable)

	// Scan every non-free object for /Type /Pages.
	orphans := 0
	for objNum, entry := range doc.xref.entries {
		if entry.Free {
			continue
		}
		obj, err := doc.getObject(objNum)
		if err != nil {
			continue
		}
		d, ok := obj.Value.(pdfDict)
		if !ok {
			continue
		}
		if dictGetName(d, "/Type") == "/Pages" && !reachable[objNum] {
			orphans++
		}
	}
	if orphans > 0 {
		report.add("PAGE_TREE_ERROR", fmt.Sprintf("%d orphaned /Pages object(s) not reachable from catalog", orphans))
	}
	return nil
}

// collectPagesNodes recursively collects object numbers of /Pages nodes reachable from ref.
func collectPagesNodes(doc *rawDocument, ref pdfValue, out map[int]bool) {
	r, ok := ref.(pdfRef)
	if !ok {
		return
	}
	if out[r.Num] {
		return
	}
	d, err := doc.resolveDict(ref)
	if err != nil {
		return
	}
	if dictGetName(d, "/Type") != "/Pages" {
		return
	}
	out[r.Num] = true
	kids, ok := d["/Kids"]
	if !ok {
		return
	}
	arr, ok := kids.(pdfArray)
	if !ok {
		return
	}
	for _, kid := range arr {
		collectPagesNodes(doc, kid, out)
	}
}

// openDocumentFromBytes is like openDocument but reuses already-read bytes.
func openDocumentFromBytes(data []byte) (*rawDocument, error) {
	startOff, err := findStartXRef(data)
	if err != nil {
		return nil, err
	}
	xref, trailer, err := parseXRef(data, startOff)
	if err != nil {
		return nil, err
	}
	return &rawDocument{
		data:      data,
		xref:      xref,
		trailer:   trailer,
		cache:     make(map[int]*pdfObject),
		objStreams: make(map[int][]*pdfObject),
	}, nil
}

// rawDocument is a parsed PDF used internally by Validate.
// It is distinct from the public Document type.
type rawDocument struct {
	data      []byte
	xref      *xrefTable
	trailer   pdfDict
	cache     map[int]*pdfObject
	objStreams map[int][]*pdfObject
}

func (d *rawDocument) getObject(num int) (*pdfObject, error) {
	if obj, ok := d.cache[num]; ok {
		return obj, nil
	}
	entry, ok := d.xref.entries[num]
	if !ok {
		return nil, fmt.Errorf("object %d not in xref", num)
	}
	if entry.Free {
		return nil, fmt.Errorf("object %d is free", num)
	}
	var obj *pdfObject
	var err error
	if entry.Compressed {
		obj, err = d.getFromObjStream(entry.StreamObjNum, num)
	} else {
		obj, err = parseIndirectObject(d.data, entry.Offset)
	}
	if err != nil {
		return nil, err
	}
	d.cache[num] = obj
	return obj, nil
}

func (d *rawDocument) getFromObjStream(streamObjNum, targetNum int) (*pdfObject, error) {
	if objs, ok := d.objStreams[streamObjNum]; ok {
		for _, o := range objs {
			if o.Num == targetNum {
				return o, nil
			}
		}
		return nil, fmt.Errorf("object %d not found in stream %d", targetNum, streamObjNum)
	}
	streamObj, err := d.getObject(streamObjNum)
	if err != nil {
		return nil, fmt.Errorf("object stream %d: %w", streamObjNum, err)
	}
	s, ok := streamObj.Value.(*pdfStream)
	if !ok {
		return nil, fmt.Errorf("object %d is not a stream", streamObjNum)
	}
	n := dictGetInt(s.Dict, "/N")
	first := dictGetInt(s.Dict, "/First")
	headerData := s.Data[:first]
	hl := newLexer(headerData)
	type objOffset struct {
		num    int
		offset int
	}
	offsets := make([]objOffset, 0, n)
	for i := 0; i < n; i++ {
		t1, _ := hl.Next()
		t2, _ := hl.Next()
		if t1.kind != tokInt || t2.kind != tokInt {
			break
		}
		oNum := toIntBytes(t1.raw)
		oOff := toIntBytes(t2.raw)
		offsets = append(offsets, objOffset{num: oNum, offset: first + oOff})
	}
	objs := make([]*pdfObject, 0, len(offsets))
	for _, oo := range offsets {
		l := newLexer(s.Data[oo.offset:])
		val, err := parseValue(l)
		if err != nil {
			continue
		}
		objs = append(objs, &pdfObject{Num: oo.num, Value: val})
	}
	d.objStreams[streamObjNum] = objs
	for _, o := range objs {
		if o.Num == targetNum {
			return o, nil
		}
	}
	return nil, fmt.Errorf("object %d not found in stream %d", targetNum, streamObjNum)
}

func (d *rawDocument) resolve(v pdfValue) (pdfValue, error) {
	ref, ok := v.(pdfRef)
	if !ok {
		return v, nil
	}
	obj, err := d.getObject(ref.Num)
	if err != nil {
		return nil, err
	}
	return obj.Value, nil
}

func (d *rawDocument) resolveDict(v pdfValue) (pdfDict, error) {
	rv, err := d.resolve(v)
	if err != nil {
		return nil, err
	}
	switch rd := rv.(type) {
	case pdfDict:
		return rd, nil
	case *pdfStream:
		return rd.Dict, nil
	}
	return nil, fmt.Errorf("expected dict, got %T", rv)
}

func (d *rawDocument) pages() ([]*pageInfo, error) {
	rootRef, ok := d.trailer["/Root"]
	if !ok {
		return nil, fmt.Errorf("trailer missing /Root")
	}
	catalog, err := d.resolveDict(rootRef)
	if err != nil {
		return nil, fmt.Errorf("catalog: %w", err)
	}
	pagesRef, ok := catalog["/Pages"]
	if !ok {
		return nil, fmt.Errorf("catalog missing /Pages")
	}
	var result []*pageInfo
	if err := d.walkPageTree(pagesRef, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// pageInfo describes a single PDF page and all objects it needs.
type pageInfo struct {
	objNum int
	deps   map[int]bool
}

func (d *rawDocument) walkPageTree(nodeRef pdfValue, result *[]*pageInfo) error {
	ref, ok := nodeRef.(pdfRef)
	if !ok {
		return fmt.Errorf("page tree node is not a ref")
	}
	nodeDict, err := d.resolveDict(nodeRef)
	if err != nil {
		return err
	}
	nodeType := dictGetName(nodeDict, "/Type")
	switch nodeType {
	case "/Pages":
		kids, ok := nodeDict["/Kids"]
		if !ok {
			return fmt.Errorf("Pages node missing /Kids")
		}
		arr, ok := kids.(pdfArray)
		if !ok {
			return fmt.Errorf("/Kids is not an array")
		}
		for _, kid := range arr {
			if err := d.walkPageTree(kid, result); err != nil {
				return err
			}
		}
	case "/Page", "":
		deps := make(map[int]bool)
		deps[ref.Num] = true
		d.collectValueDeps(nodeDict, deps)
		if err := d.collectInheritedDeps(nodeRef, deps); err != nil {
			return err
		}
		*result = append(*result, &pageInfo{objNum: ref.Num, deps: deps})
	default:
		return fmt.Errorf("unknown page tree node type: %s", nodeType)
	}
	return nil
}

func (d *rawDocument) collectInheritedDeps(pageRef pdfValue, deps map[int]bool) error {
	nodeDict, err := d.resolveDict(pageRef)
	if err != nil {
		return err
	}
	parentRef, ok := nodeDict["/Parent"]
	if !ok {
		return nil
	}
	parentDict, err := d.resolveDict(parentRef)
	if err != nil {
		return err
	}
	if res, ok := parentDict["/Resources"]; ok {
		d.collectValueDeps(res, deps)
	}
	return d.collectInheritedDeps(parentRef, deps)
}

var reRef = regexp.MustCompile(`\b(\d+)\s+\d+\s+R\b`)

func (d *rawDocument) collectDeps(objNum int, deps map[int]bool) error {
	if deps[objNum] {
		return nil
	}
	obj, err := d.getObject(objNum)
	if err != nil {
		return nil
	}
	if dict, ok := obj.Value.(pdfDict); ok {
		switch dictGetName(dict, "/Type") {
		case "/Pages", "/Catalog", "/Page":
			return nil
		}
	}
	deps[objNum] = true
	d.collectValueDeps(obj.Value, deps)
	return nil
}

func (d *rawDocument) collectValueDeps(v pdfValue, deps map[int]bool) {
	switch val := v.(type) {
	case pdfRef:
		d.collectDeps(val.Num, deps)
	case pdfDict:
		for _, dv := range val {
			d.collectValueDeps(dv, deps)
		}
	case pdfArray:
		for _, av := range val {
			d.collectValueDeps(av, deps)
		}
	case *pdfStream:
		for _, dv := range val.Dict {
			d.collectValueDeps(dv, deps)
		}
		refs := reRef.FindAllSubmatch(val.Data, -1)
		for _, m := range refs {
			n := toIntBytes(m[1])
			if n > 0 {
				d.collectDeps(n, deps)
			}
		}
	}
}
