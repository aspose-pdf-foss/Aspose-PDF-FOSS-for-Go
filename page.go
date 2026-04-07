package asposepdf

import "fmt"

// RotationAngle represents a valid PDF page rotation in clockwise degrees.
type RotationAngle int

const (
	Rotate0   RotationAngle = 0
	Rotate90  RotationAngle = 90
	Rotate180 RotationAngle = 180
	Rotate270 RotationAngle = 270
)

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

// pageObj returns the underlying pdfObject for this page.
func (p *Page) pageObj() *pdfObject {
	return p.doc.pages[p.index]
}

// pageDict returns the page's dictionary, or nil if not a dict.
func (p *Page) pageDict() pdfDict {
	if d, ok := p.pageObj().Value.(pdfDict); ok {
		return d
	}
	return nil
}

// Size returns the page dimensions from its MediaBox.
// If MediaBox is not set on the page itself, it is inherited from the page tree.
func (p *Page) Size() (PageSize, error) {
	return mediaBoxSize(p.doc.objects, p.pageObj().Num)
}

// Rotation returns the effective rotation of the page in degrees (0, 90, 180, 270).
func (p *Page) Rotation() RotationAngle {
	d := p.pageDict()
	if d == nil {
		return Rotate0
	}
	return RotationAngle(dictGetInt(d, "/Rotate"))
}

// CropBox returns the crop box of the page.
// Falls back to MediaBox if not set.
func (p *Page) CropBox() (PageSize, error) {
	return pageBoxWithFallback(p.doc.objects, p.pageObj().Num, "/CropBox")
}

// TrimBox returns the trim box of the page.
// Falls back to CropBox, then MediaBox if not set.
func (p *Page) TrimBox() (PageSize, error) {
	return pageBoxWithFallback(p.doc.objects, p.pageObj().Num, "/TrimBox", "/CropBox")
}

// BleedBox returns the bleed box of the page.
// Falls back to CropBox, then MediaBox if not set.
func (p *Page) BleedBox() (PageSize, error) {
	return pageBoxWithFallback(p.doc.objects, p.pageObj().Num, "/BleedBox", "/CropBox")
}

// ArtBox returns the art box of the page.
// Falls back to CropBox, then MediaBox if not set.
func (p *Page) ArtBox() (PageSize, error) {
	return pageBoxWithFallback(p.doc.objects, p.pageObj().Num, "/ArtBox", "/CropBox")
}

// contentStreams returns the concatenated decoded content stream bytes for this page.
// /Contents may be a single stream reference or an array of references.
func (p *Page) contentStreams() ([]byte, error) {
	d := p.pageDict()
	if d == nil {
		return nil, fmt.Errorf("page %d has no dict", p.Number())
	}
	contentsVal, ok := d["/Contents"]
	if !ok {
		return nil, nil // page with no content
	}

	objects := p.doc.objects
	contentsVal = resolveRef(objects, contentsVal)

	switch cv := contentsVal.(type) {
	case *pdfStream:
		return cv.Data, nil
	case pdfArray:
		var buf []byte
		for _, item := range cv {
			resolved := resolveRef(objects, item)
			if s, ok := resolved.(*pdfStream); ok {
				buf = append(buf, s.Data...)
				buf = append(buf, '\n')
			}
		}
		return buf, nil
	default:
		return nil, fmt.Errorf("unexpected /Contents type %T", contentsVal)
	}
}

// pageResources returns the /Resources dict for this page (may be inherited).
func (p *Page) pageResources() pdfDict {
	d := p.pageDict()
	if d == nil {
		return nil
	}
	resVal, ok := d["/Resources"]
	if !ok {
		return nil
	}
	res := resolveRef(p.doc.objects, resVal)
	if rd, ok := res.(pdfDict); ok {
		return rd
	}
	return nil
}

// PageSizes returns the dimensions of every page in the given PDF file.
func PageSizes(inputPath string) ([]PageSize, error) {
	doc, err := Open(inputPath)
	if err != nil {
		return nil, err
	}
	sizes := make([]PageSize, len(doc.pages))
	for i, pg := range doc.pages {
		sz, err := mediaBoxSize(doc.objects, pg.Num)
		if err != nil {
			return nil, fmt.Errorf("page %d: %w", i+1, err)
		}
		sizes[i] = sz
	}
	return sizes, nil
}

// pageBoxWithFallback reads the first named box that exists on the page dict.
// If none of the requested boxes are found, it falls back to the MediaBox.
func pageBoxWithFallback(objects map[int]*pdfObject, objNum int, boxes ...string) (PageSize, error) {
	obj, ok := objects[objNum]
	if !ok {
		return PageSize{}, fmt.Errorf("object %d not found", objNum)
	}
	d, ok := obj.Value.(pdfDict)
	if !ok {
		return PageSize{}, fmt.Errorf("object %d is not a dict", objNum)
	}
	for _, name := range boxes {
		if v, ok := d[name]; ok {
			arr, ok := resolveRefToArray(objects, v)
			if !ok {
				continue
			}
			return mediaBoxFromArray(arr)
		}
	}
	return mediaBoxSize(objects, objNum)
}

// mediaBoxSize reads the /MediaBox of the page object at objNum,
// walking up the /Parent chain if needed (inheritance).
func mediaBoxSize(objects map[int]*pdfObject, objNum int) (PageSize, error) {
	visited := make(map[int]bool)
	for {
		if visited[objNum] {
			return PageSize{}, fmt.Errorf("cycle in page tree at object %d", objNum)
		}
		visited[objNum] = true

		obj, ok := objects[objNum]
		if !ok {
			return PageSize{}, fmt.Errorf("object %d not found", objNum)
		}
		d, ok := obj.Value.(pdfDict)
		if !ok {
			return PageSize{}, fmt.Errorf("object %d is not a dict", objNum)
		}

		if mb, ok := d["/MediaBox"]; ok {
			arr, ok := resolveRefToArray(objects, mb)
			if !ok {
				return PageSize{}, fmt.Errorf("invalid /MediaBox")
			}
			return mediaBoxFromArray(arr)
		}

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
