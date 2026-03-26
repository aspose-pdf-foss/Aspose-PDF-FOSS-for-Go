package asposepdf

import (
	"bytes"
	"fmt"
	"io"
	"os"
)

// mutablePage holds a page and its source document.
type mutablePage struct {
	src  *rawDocument
	page *pageInfo
}

// patchKey identifies a page object within a specific source document.
type patchKey struct {
	src    *rawDocument
	objNum int
}

// Document is a mutable PDF document. Pages can be reordered, rotated,
// extracted, and merged from multiple sources before saving.
type Document struct {
	pages         []mutablePage
	patches       map[patchKey]pdfDict
	encryptConfig *encryptConfig // nil = no encryption
}

// Open opens a PDF file and returns a mutable Document.
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

// OpenStream reads a PDF from r and returns a mutable Document.
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
	pages := make([]mutablePage, len(rawPages))
	for i, p := range rawPages {
		pages[i] = mutablePage{src: doc, page: p}
	}
	return &Document{
		pages:   pages,
		patches: make(map[patchKey]pdfDict),
	}, nil
}

// PageCount returns the current number of pages in the document.
func (d *Document) PageCount() int {
	return len(d.pages)
}

// Rotate rotates selected pages clockwise by the given angle (Rotate90, Rotate180, or Rotate270).
// The rotation is added to any existing rotation (including previously applied rotations).
// If no page numbers are given, all pages are rotated. Page numbers are 1-based.
//
// Example:
//
//	doc.Rotate(asposepdf.Rotate90)        // rotate all pages
//	doc.Rotate(asposepdf.Rotate180, 1, 3) // rotate pages 1 and 3
func (d *Document) Rotate(angle RotationAngle, pageNums ...int) error {
	if err := angle.validate(); err != nil {
		return err
	}
	indices, err := resolvePageIndices(len(d.pages), pageNums)
	if err != nil {
		return err
	}
	for _, i := range indices {
		e := d.pages[i]
		key := patchKey{e.src, e.page.objNum}
		current := d.patchedRotation(key, e)
		d.setPatch(key, "/Rotate", (int(current)+int(angle))%360)
	}
	return nil
}

// ExtractPages keeps only the specified page ranges, discarding all other pages.
// Ranges are 1-based and inclusive.
//
// Example:
//
//	doc.ExtractPages(asposepdf.PageRange{1, 3}, asposepdf.PageRange{5, 5})
func (d *Document) ExtractPages(ranges ...PageRange) error {
	if len(ranges) == 0 {
		return fmt.Errorf("no page ranges specified")
	}
	var selected []mutablePage
	for _, r := range ranges {
		from, to, err := normalizeRange(r.From, r.To, len(d.pages))
		if err != nil {
			return err
		}
		selected = append(selected, d.pages[from-1:to]...)
	}
	d.pages = selected
	return nil
}

// Reorder rearranges pages according to order, a slice of 1-based page numbers.
// Pages may be repeated or omitted. The result will have len(order) pages.
//
// Example — reverse a 4-page document:
//
//	doc.Reorder([]int{4, 3, 2, 1})
func (d *Document) Reorder(order []int) error {
	result := make([]mutablePage, len(order))
	for i, n := range order {
		if n < 1 || n > len(d.pages) {
			return fmt.Errorf("page number %d out of range (1..%d)", n, len(d.pages))
		}
		result[i] = d.pages[n-1]
	}
	d.pages = result
	return nil
}

// AppendFrom appends all pages from other at the end of this document.
// Patches applied to other are preserved.
//
// Example:
//
//	doc1, _ := asposepdf.Open("part1.pdf")
//	doc2, _ := asposepdf.Open("part2.pdf")
//	doc1.AppendFrom(doc2)
//	doc1.Save("combined.pdf")
func (d *Document) AppendFrom(other *Document) {
	d.pages = append(d.pages, other.pages...)
	for key, patch := range other.patches {
		d.patches[key] = patch
	}
}

// SetPassword configures the document to be encrypted when saved.
// userPassword is required to open the document; ownerPassword controls permission settings.
// If ownerPassword is empty, it defaults to userPassword.
// The document is encrypted using RC4-128 (PDF 1.4 Standard Security Handler).
//
// Example:
//
//	doc.SetPassword("secret", "")
//	doc.Save("encrypted.pdf")
func (d *Document) SetPassword(userPassword, ownerPassword string) {
	d.encryptConfig = &encryptConfig{
		userPassword:  userPassword,
		ownerPassword: ownerPassword,
	}
}

// WriteTo writes the current document state to w.
// It implements io.WriterTo.
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

// Save writes the current document state to outputPath.
func (d *Document) Save(outputPath string) error {
	f, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = d.WriteTo(f)
	return err
}

// normalizeRange clamps from/to to valid bounds [1, total] and validates ordering.
func normalizeRange(from, to, total int) (int, int, error) {
	if from < 1 {
		from = 1
	}
	if to < 1 || to > total {
		to = total
	}
	if from > to {
		return 0, 0, fmt.Errorf("invalid range: from=%d > to=%d", from, to)
	}
	return from, to, nil
}

// patchedRotation returns the effective /Rotate for a page,
// considering already-applied patches first, then the source dict.
func (d *Document) patchedRotation(key patchKey, e mutablePage) RotationAngle {
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
