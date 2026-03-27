package asposepdf

import (
	"bytes"
	"fmt"
	"io"
	"os"
)

// pageRef holds a page and its source document.
type pageRef struct {
	src  *rawDocument
	page *pageInfo
}

// patchKey identifies a page object within a specific source document.
type patchKey struct {
	src    *rawDocument
	objNum int
}

// Document is a PDF document. All operations return new Documents;
// the receiver is never modified.
type Document struct {
	pages         []pageRef
	patches       map[patchKey]pdfDict
	encryptConfig *encryptConfig // nil = no encryption
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
	doc, err := openDocumentFromBytes(data)
	if err != nil {
		return nil, fmt.Errorf("parse PDF: %w", err)
	}
	rawPages, err := doc.pages()
	if err != nil {
		return nil, fmt.Errorf("read pages: %w", err)
	}
	pages := make([]pageRef, len(rawPages))
	for i, p := range rawPages {
		pages[i] = pageRef{src: doc, page: p}
	}
	return &Document{
		pages:   pages,
		patches: make(map[patchKey]pdfDict),
	}, nil
}

// PageCount returns the number of pages in the document.
func (d *Document) PageCount() int {
	return len(d.pages)
}


// Reorder returns a new Document with pages rearranged according to order,
// a slice of 1-based page numbers. Pages may be repeated or omitted.
//
// Example — reverse a 4-page document:
//
//	doc, err = doc.Reorder([]int{4, 3, 2, 1})
func (d *Document) Reorder(order []int) (*Document, error) {
	result := make([]pageRef, len(order))
	for i, n := range order {
		if n < 1 || n > len(d.pages) {
			return nil, fmt.Errorf("page number %d out of range (1..%d)", n, len(d.pages))
		}
		result[i] = d.pages[n-1]
	}
	return &Document{pages: result, patches: copyPatches(d.patches)}, nil
}

// AppendFrom returns a new Document with all pages from other appended.
//
// Example:
//
//	doc1, _ := asposepdf.Open("part1.pdf")
//	doc2, _ := asposepdf.Open("part2.pdf")
//	combined := doc1.AppendFrom(doc2)
//	combined.Save("combined.pdf")
func (d *Document) AppendFrom(other *Document) *Document {
	newPages := append(append([]pageRef{}, d.pages...), other.pages...)
	newPatches := copyPatches(d.patches)
	for k, v := range other.patches {
		newPatches[k] = v
	}
	return &Document{pages: newPages, patches: newPatches}
}

// SetPassword returns a new Document configured to be encrypted when saved.
// userPassword is required to open the document; ownerPassword controls
// permission settings. If ownerPassword is empty, it defaults to userPassword.
//
// Example:
//
//	doc = doc.SetPassword("secret", "")
//	doc.Save("encrypted.pdf")
func (d *Document) SetPassword(userPassword, ownerPassword string) *Document {
	result := d.withCopiedPatches()
	result.encryptConfig = &encryptConfig{
		userPassword:  userPassword,
		ownerPassword: ownerPassword,
	}
	return result
}

// WriteTo writes the document to w. It implements io.WriterTo.
func (d *Document) WriteTo(w io.Writer) (int64, error) {
	if len(d.pages) == 0 {
		return 0, fmt.Errorf("document has no pages")
	}
	data, err := buildDocumentPDF(d.pages, d.patches, d.encryptConfig)
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

// validateRange validates from/to against [1, total] and returns an error for any invalid input.
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

// withCopiedPatches returns a shallow copy of d with an independent patches map.
func (d *Document) withCopiedPatches() *Document {
	return &Document{
		pages:         append([]pageRef{}, d.pages...),
		patches:       copyPatches(d.patches),
		encryptConfig: d.encryptConfig,
	}
}

// copyPatches returns a shallow copy of patches.
func copyPatches(src map[patchKey]pdfDict) map[patchKey]pdfDict {
	dst := make(map[patchKey]pdfDict, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

// patchedRotation returns the effective /Rotate for a page,
// considering already-applied patches first, then the source dict.
func (d *Document) patchedRotation(key patchKey, e pageRef) RotationAngle {
	if p, ok := d.patches[key]; ok {
		if r, ok := p["/Rotate"]; ok {
			if n, ok := r.(int); ok {
				return RotationAngle(n)
			}
		}
	}
	rot, _ := pageRotation(e.src, e.page)
	return rot
}

// setPatch sets a single key/value in the patch dict for key.
func (d *Document) setPatch(key patchKey, k string, v pdfValue) {
	if d.patches[key] == nil {
		d.patches[key] = make(pdfDict)
	}
	d.patches[key][k] = v
}

// resolvePageIndices converts 1-based page numbers to 0-based indices.
// If pageNums is empty, returns all indices.
func resolvePageIndices(total int, pageNums []int) ([]int, error) {
	if len(pageNums) == 0 {
		indices := make([]int, total)
		for i := range indices {
			indices[i] = i
		}
		return indices, nil
	}
	indices := make([]int, len(pageNums))
	for i, n := range pageNums {
		if n < 1 || n > total {
			return nil, fmt.Errorf("page number %d out of range (1..%d)", n, total)
		}
		indices[i] = n - 1
	}
	return indices, nil
}
