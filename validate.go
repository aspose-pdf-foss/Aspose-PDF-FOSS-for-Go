package asposepdf

import (
	"bytes"
	"fmt"
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

	return report, nil
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
