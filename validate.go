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

	return report, nil
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
