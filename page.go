package asposepdf

import "fmt"

// PageSize holds the width and height of a PDF page in points (1/72 inch).
type PageSize struct {
	Width  float64
	Height float64
}

// Page is a live view of a single page within a Document.
// It reflects the current state of the document, including any mutations.
type Page struct {
	doc   *Document
	index int // 0-based index in doc.pages
}

// Number returns the 1-based page number within the document.
func (p *Page) Number() int {
	return p.index + 1
}

// Size returns the page dimensions from its MediaBox.
// If MediaBox is not set on the page itself, it is inherited from the page tree.
func (p *Page) Size() (PageSize, error) {
	e := p.doc.pages[p.index]
	return mediaBoxSize(e.src, e.page.objNum)
}

// Rotation returns the effective rotation of the page.
// It reflects any rotation applied via Document.Rotate as well as the original /Rotate
// value stored in the PDF.
func (p *Page) Rotation() RotationAngle {
	e := p.doc.pages[p.index]
	key := patchKey{e.src, e.page.objNum}
	return p.doc.patchedRotation(key, e)
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

// PageSizes returns the dimensions of every page in the given PDF file.
func PageSizes(inputPath string) ([]PageSize, error) {
	doc, err := Open(inputPath)
	if err != nil {
		return nil, err
	}
	sizes := make([]PageSize, len(doc.pages))
	for i, e := range doc.pages {
		sz, err := mediaBoxSize(e.src, e.page.objNum)
		if err != nil {
			return nil, fmt.Errorf("page %d: %w", i+1, err)
		}
		sizes[i] = sz
	}
	return sizes, nil
}

// mediaBoxSize reads the /MediaBox of the page object at objNum,
// walking up the /Parent chain if needed (inheritance).
func mediaBoxSize(src *rawDocument, objNum int) (PageSize, error) {
	visited := make(map[int]bool)
	for {
		if visited[objNum] {
			return PageSize{}, fmt.Errorf("cycle in page tree at object %d", objNum)
		}
		visited[objNum] = true

		obj, err := src.getObject(objNum)
		if err != nil {
			return PageSize{}, err
		}
		d, ok := obj.Value.(pdfDict)
		if !ok {
			return PageSize{}, fmt.Errorf("object %d is not a dict", objNum)
		}

		if mb, ok := d["/MediaBox"]; ok {
			arr, err := resolveToArray(src, mb)
			if err != nil {
				return PageSize{}, fmt.Errorf("invalid /MediaBox: %w", err)
			}
			return mediaBoxFromArray(arr)
		}

		// Not found on this node — walk up to /Parent.
		parentVal, ok := d["/Parent"]
		if !ok {
			return PageSize{}, fmt.Errorf("no /MediaBox found for object %d", objNum)
		}
		parentRef, ok := parentVal.(pdfRef)
		if !ok {
			return PageSize{}, fmt.Errorf("unexpected /Parent type %T", parentVal)
		}
		objNum = parentRef.Num
	}
}

// resolveToArray resolves v to a pdfArray, following one level of indirection if needed.
func resolveToArray(src *rawDocument, v pdfValue) (pdfArray, error) {
	rv, err := src.resolve(v)
	if err != nil {
		return nil, err
	}
	arr, ok := rv.(pdfArray)
	if !ok {
		return nil, fmt.Errorf("expected array, got %T", rv)
	}
	return arr, nil
}

// mediaBoxFromArray converts a [x1 y1 x2 y2] PDF array to PageSize.
func mediaBoxFromArray(arr pdfArray) (PageSize, error) {
	if len(arr) != 4 {
		return PageSize{}, fmt.Errorf("MediaBox must have 4 elements, got %d", len(arr))
	}
	vals := make([]float64, 4)
	for i, v := range arr {
		f, err := toFloat(v)
		if err != nil {
			return PageSize{}, fmt.Errorf("MediaBox[%d]: %w", i, err)
		}
		vals[i] = f
	}
	return PageSize{
		Width:  vals[2] - vals[0],
		Height: vals[3] - vals[1],
	}, nil
}

