package asposepdf

import "fmt"

// RotationAngle represents a valid PDF page rotation in clockwise degrees.
// Only the values defined as constants (Rotate0, Rotate90, Rotate180, Rotate270) are valid.
type RotationAngle int

const (
	// Rotate0 is the default orientation (no rotation).
	Rotate0 RotationAngle = 0
	// Rotate90 rotates a page 90 degrees clockwise.
	Rotate90 RotationAngle = 90
	// Rotate180 rotates a page 180 degrees (upside down).
	Rotate180 RotationAngle = 180
	// Rotate270 rotates a page 270 degrees clockwise (90 degrees counter-clockwise).
	Rotate270 RotationAngle = 270
)

// validate returns an error if a is not a valid PDF rotation angle.
func (a RotationAngle) validate() error {
	if a != Rotate0 && a != Rotate90 && a != Rotate180 && a != Rotate270 {
		return fmt.Errorf("angle must be Rotate0, Rotate90, Rotate180, or Rotate270; got %d", a)
	}
	return nil
}

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

// CropBox returns the crop box of the page.
// The crop box defines the visible region. If not explicitly set on the page,
// it falls back to the MediaBox.
func (p *Page) CropBox() (PageSize, error) {
	e := p.doc.pages[p.index]
	return pageBoxWithFallback(e.src, e.page.objNum, "/CropBox")
}

// TrimBox returns the trim box of the page.
// The trim box defines the intended final dimensions after trimming.
// Falls back to CropBox, then MediaBox if not set.
func (p *Page) TrimBox() (PageSize, error) {
	e := p.doc.pages[p.index]
	return pageBoxWithFallback(e.src, e.page.objNum, "/TrimBox", "/CropBox")
}

// BleedBox returns the bleed box of the page.
// The bleed box defines the region to which content is clipped in production.
// Falls back to CropBox, then MediaBox if not set.
func (p *Page) BleedBox() (PageSize, error) {
	e := p.doc.pages[p.index]
	return pageBoxWithFallback(e.src, e.page.objNum, "/BleedBox", "/CropBox")
}

// ArtBox returns the art box of the page.
// The art box defines the extent of meaningful content.
// Falls back to CropBox, then MediaBox if not set.
func (p *Page) ArtBox() (PageSize, error) {
	e := p.doc.pages[p.index]
	return pageBoxWithFallback(e.src, e.page.objNum, "/ArtBox", "/CropBox")
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

// pageBoxWithFallback reads the first named box that exists on the page dict.
// If none of the requested boxes are found, it falls back to the MediaBox
// (walking the parent chain if needed).
// Note: only MediaBox is inherited per the PDF spec; the other boxes are looked
// up directly on the page object.
func pageBoxWithFallback(src *rawDocument, objNum int, boxes ...string) (PageSize, error) {
	obj, err := src.getObject(objNum)
	if err != nil {
		return PageSize{}, err
	}
	d, ok := obj.Value.(pdfDict)
	if !ok {
		return PageSize{}, fmt.Errorf("object %d is not a dict", objNum)
	}
	for _, name := range boxes {
		if v, ok := d[name]; ok {
			arr, err := resolveToArray(src, v)
			if err != nil {
				continue
			}
			return mediaBoxFromArray(arr)
		}
	}
	return mediaBoxSize(src, objNum)
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

