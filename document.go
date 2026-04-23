package asposepdf

import (
	"bytes"
	"fmt"
	"io"
	"os"
)

// Document is a PDF document. Operations directly mutate the receiver.
type Document struct {
	objects map[int]*pdfObject // all PDF objects by ID
	pages   []*pdfObject       // ordered /Page objects
	catalog pdfDict            // /Catalog dict
	info    pdfDict            // /Info dict; nil = no metadata
	encrypt *encryptConfig     // nil = no encryption
	nextID  int                // next available object ID
}

// Open opens a PDF file and returns a Document.
//
// Example:
//
//	doc, err := asposepdf.Open("input.pdf")
func Open(path string) (*Document, error) {
	data, err := readFile(path)
	if err != nil {
		return nil, fmt.Errorf("open PDF: %w", err)
	}
	return OpenStream(bytes.NewReader(data))
}

// OpenStream reads a PDF from r and returns a Document.
//
// Example:
//
//	doc, err := asposepdf.OpenStream(file)
func OpenStream(r io.Reader) (*Document, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("read PDF: %w", err)
	}

	startOff, err := findStartXRef(data)
	if err != nil {
		return nil, fmt.Errorf("parse PDF: %w", err)
	}
	xref, trailer, err := parseXRef(data, startOff)
	if err != nil {
		return nil, fmt.Errorf("parse PDF: %w", err)
	}
	if _, ok := trailer["/Encrypt"]; ok {
		return nil, fmt.Errorf("parse PDF: encrypted PDF is not supported")
	}

	objects, err := parseAllObjects(data, xref, trailer)
	if err != nil {
		return nil, fmt.Errorf("parse PDF: %w", err)
	}

	catalog, err := extractCatalog(objects, trailer)
	if err != nil {
		return nil, fmt.Errorf("parse PDF: %w", err)
	}

	pages, err := resolvePageTree(objects, catalog)
	if err != nil {
		return nil, fmt.Errorf("read pages: %w", err)
	}

	// Remove structural page-tree and catalog objects — the writer rebuilds them.
	// Keeping them would produce orphaned /Pages nodes that fail validation.
	for id, obj := range objects {
		if d, ok := obj.Value.(pdfDict); ok {
			switch dictGetName(d, "/Type") {
			case "/Pages", "/Catalog":
				delete(objects, id)
			}
		}
	}

	return &Document{
		objects: objects,
		pages:   pages,
		catalog: catalog,
		info:    extractInfo(objects, trailer),
		nextID:  maxObjectID(objects) + 1,
	}, nil
}

// PageCount returns the number of pages in the document.
func (d *Document) PageCount() int {
	return len(d.pages)
}

// Pages returns a live view of all pages in the document.
func (d *Document) Pages() []*Page {
	pages := make([]*Page, len(d.pages))
	for i := range d.pages {
		pages[i] = &Page{doc: d, index: i}
	}
	return pages
}

// Page returns a live view of the page at the given 1-based number.
func (d *Document) Page(n int) (*Page, error) {
	if n < 1 || n > len(d.pages) {
		return nil, fmt.Errorf("page number %d out of range (1..%d)", n, len(d.pages))
	}
	return &Page{doc: d, index: n - 1}, nil
}

// Append adds all pages from others to this document, merging their objects.
// Nil arguments are silently skipped.
//
// Example:
//
//	doc1, _ := asposepdf.Open("part1.pdf")
//	doc2, _ := asposepdf.Open("part2.pdf")
//	doc1.Append(doc2)
//	doc1.Save("combined.pdf")
func (d *Document) Append(others ...*Document) {
	for _, other := range others {
		if other == nil {
			continue
		}
		// Build ID mapping: other's object IDs → new IDs in d.
		idMap := make(map[int]int, len(other.objects))
		for oldID := range other.objects {
			idMap[oldID] = d.nextID
			d.nextID++
		}
		// Copy objects with rewritten refs.
		for oldID, obj := range other.objects {
			newID := idMap[oldID]
			d.objects[newID] = &pdfObject{
				Num:   newID,
				Gen:   obj.Gen,
				Value: rewriteRefs(obj.Value, idMap),
			}
		}
		// Add pages (using new IDs).
		for _, page := range other.pages {
			d.pages = append(d.pages, d.objects[idMap[page.Num]])
		}
	}
}

// RemoveUnusedObjects removes objects from the document that are not
// reachable from any page. Returns the number of objects removed.
func (d *Document) RemoveUnusedObjects() int {
	reachable := collectReachableIDs(d.objects, d.pages)

	removed := 0
	for id := range d.objects {
		if !reachable[id] {
			delete(d.objects, id)
			removed++
		}
	}
	return removed
}

// SetPassword configures the document to be encrypted when saved.
// userPassword is required to open; ownerPassword controls permissions.
// If ownerPassword is empty, it defaults to userPassword.
//
// Password encoding: The password bytes are passed unchanged to the PDF
// Standard Security Handler (RC4-128, V=2 R=3). For ASCII passwords this
// matches both the PDF specification and every major PDF viewer. For
// non-ASCII passwords the raw UTF-8 bytes are used, which is compatible
// with pypdf 6.x and Adobe Acrobat DC but may not be accepted by strictly
// legacy PDFDocEncoding-only viewers (e.g. Adobe Reader 9 and older).
// For international passwords with guaranteed interop, AES-256 (R=6) is
// the only complete solution — not yet supported by this library.
//
// Example:
//
//	doc.SetPassword("secret", "")
//	doc.Save("encrypted.pdf")
func (d *Document) SetPassword(userPassword, ownerPassword string) {
	if d.encrypt == nil {
		d.encrypt = &encryptConfig{}
	}
	d.encrypt.userPassword = userPassword
	d.encrypt.ownerPassword = ownerPassword
}

// SetPermissions configures what operations a viewer allows on the
// encrypted document (printing, copying, modifying, etc.). Permissions
// only take effect if the document is also encrypted — call SetPassword
// to set the user and owner passwords. If SetPermissions is never called,
// all operations are allowed by default, matching the historical behavior.
//
// Example:
//
//	doc.SetPassword("secret", "owner-secret")
//	doc.SetPermissions(asposepdf.Permissions{AllowPrint: true, AllowCopy: true})
//	doc.Save("restricted.pdf")
func (d *Document) SetPermissions(p Permissions) {
	if d.encrypt == nil {
		d.encrypt = &encryptConfig{}
	}
	d.encrypt.permissions = p.toPDFBits()
	d.encrypt.hasPermissions = true
}

// WriteTo writes the document to w. It implements io.WriterTo.
func (d *Document) WriteTo(w io.Writer) (int64, error) {
	if len(d.pages) == 0 {
		return 0, fmt.Errorf("document has no pages")
	}
	data, err := buildDocumentPDF(d)
	if err != nil {
		return 0, err
	}
	n, err := w.Write(data)
	return int64(n), err
}

// Save writes the document to outputPath.
func (d *Document) Save(outputPath string) error {
	f, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = d.WriteTo(f)
	return err
}

// validateRange validates from/to against [1, total].
func validateRange(from, to, total int) (int, int, error) {
	if from < 1 || from > total {
		return 0, 0, fmt.Errorf("page range from=%d out of bounds (1..%d)", from, total)
	}
	if to < 1 || to > total {
		return 0, 0, fmt.Errorf("page range to=%d out of bounds (1..%d)", to, total)
	}
	if from > to {
		return 0, 0, fmt.Errorf("invalid page range: from=%d > to=%d", from, to)
	}
	return from, to, nil
}

// resolvePageIndices converts 1-based page numbers to 0-based indices.
// If pageNums is empty, returns all indices. Duplicates are silently removed.
func resolvePageIndices(total int, pageNums []int) ([]int, error) {
	if len(pageNums) == 0 {
		indices := make([]int, total)
		for i := range indices {
			indices[i] = i
		}
		return indices, nil
	}
	seen := make(map[int]bool, len(pageNums))
	indices := make([]int, 0, len(pageNums))
	for _, n := range pageNums {
		if n < 1 || n > total {
			return nil, fmt.Errorf("page number %d out of range (1..%d)", n, total)
		}
		if !seen[n] {
			seen[n] = true
			indices = append(indices, n-1)
		}
	}
	return indices, nil
}
