// SPDX-License-Identifier: MIT

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
	doc         *Document
	index       int // 0-based index in doc.pages
	annotations *AnnotationCollection

	// obj, when non-nil, is a detached page dictionary used as a drawing
	// surface that is NOT part of doc.pages — the canvas behind an XForm. The
	// full *Page drawing API (AddText, Draw*, AddImage, …) then targets it,
	// and its content/resources are harvested into a Form XObject.
	obj *pdfObject
}

// Number returns the 1-based page number within the document.
func (p *Page) Number() int {
	return p.index + 1
}

// pageObj returns the underlying pdfObject for this page (the detached XForm
// canvas when set, otherwise the page at this index in the document).
func (p *Page) pageObj() *pdfObject {
	if p.obj != nil {
		return p.obj
	}
	return p.doc.pages[p.index]
}

// Annotations returns the page's annotation collection. Always non-nil;
// for a page with no /Annots array, the collection is empty.
func (p *Page) Annotations() *AnnotationCollection {
	if p.annotations == nil {
		p.annotations = &AnnotationCollection{page: p}
	}
	return p.annotations
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

// MediaBox returns the page's MediaBox as a Rectangle in PDF user space.
// If not set on the page itself, it is inherited from the page tree.
// Mirrors Aspose.PDF for .NET's Page.MediaBox.
func (p *Page) MediaBox() (Rectangle, error) {
	return mediaBoxRect(p.doc.objects, p.pageObj().Num)
}

// CropBox returns the crop box of the page as a Rectangle.
// Falls back to MediaBox if not set. Mirrors Aspose.PDF for .NET's Page.CropBox.
func (p *Page) CropBox() (Rectangle, error) {
	return pageBoxRect(p.doc.objects, p.pageObj().Num, "/CropBox")
}

// TrimBox returns the trim box of the page as a Rectangle.
// Falls back to CropBox, then MediaBox if not set.
func (p *Page) TrimBox() (Rectangle, error) {
	return pageBoxRect(p.doc.objects, p.pageObj().Num, "/TrimBox", "/CropBox")
}

// BleedBox returns the bleed box of the page as a Rectangle.
// Falls back to CropBox, then MediaBox if not set.
func (p *Page) BleedBox() (Rectangle, error) {
	return pageBoxRect(p.doc.objects, p.pageObj().Num, "/BleedBox", "/CropBox")
}

// ArtBox returns the art box of the page as a Rectangle.
// Falls back to CropBox, then MediaBox if not set.
func (p *Page) ArtBox() (Rectangle, error) {
	return pageBoxRect(p.doc.objects, p.pageObj().Num, "/ArtBox", "/CropBox")
}

// SetMediaBox sets the page's MediaBox (the full page rectangle) in PDF user
// space. The box is written directly on the page, overriding any inherited or
// referenced value. Mirrors Aspose.PDF for .NET's Page.MediaBox setter.
func (p *Page) SetMediaBox(rect Rectangle) error { return p.setBox("/MediaBox", rect) }

// SetCropBox sets the page's CropBox (the visible region). Mirrors Page.CropBox.
func (p *Page) SetCropBox(rect Rectangle) error { return p.setBox("/CropBox", rect) }

// SetTrimBox sets the page's TrimBox (intended finished dimensions).
func (p *Page) SetTrimBox(rect Rectangle) error { return p.setBox("/TrimBox", rect) }

// SetBleedBox sets the page's BleedBox (production bleed region).
func (p *Page) SetBleedBox(rect Rectangle) error { return p.setBox("/BleedBox", rect) }

// SetArtBox sets the page's ArtBox (meaningful content extent).
func (p *Page) SetArtBox(rect Rectangle) error { return p.setBox("/ArtBox", rect) }

// SetPageSize resizes the page by setting its MediaBox to [0 0 width height]
// (points). Existing content is not scaled or moved — only the page rectangle
// changes. Mirrors Aspose.PDF for .NET's Page.SetPageSize.
func (p *Page) SetPageSize(width, height float64) error {
	return p.SetMediaBox(Rectangle{LLX: 0, LLY: 0, URX: width, URY: height})
}

// setBox validates rect and writes it as a [llx lly urx ury] array directly on
// the page dict under the given box name.
func (p *Page) setBox(name string, rect Rectangle) error {
	if err := rect.validate(); err != nil {
		return err
	}
	d := p.pageDict()
	if d == nil {
		return fmt.Errorf("page %d has no dict", p.Number())
	}
	d[name] = pdfArray{rect.LLX, rect.LLY, rect.URX, rect.URY}
	return nil
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
		return decodedStreamData(cv), nil
	case pdfArray:
		var buf []byte
		for _, item := range cv {
			resolved := resolveRef(objects, item)
			if s, ok := resolved.(*pdfStream); ok {
				buf = append(buf, decodedStreamData(s)...)
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

// pageBoxRect reads the first named box that exists on the page dict, returning
// it as a Rectangle. If none of the requested boxes are found, it falls back to
// the MediaBox.
func pageBoxRect(objects map[int]*pdfObject, objNum int, boxes ...string) (Rectangle, error) {
	obj, ok := objects[objNum]
	if !ok {
		return Rectangle{}, fmt.Errorf("object %d not found", objNum)
	}
	d, ok := obj.Value.(pdfDict)
	if !ok {
		return Rectangle{}, fmt.Errorf("object %d is not a dict", objNum)
	}
	for _, name := range boxes {
		if v, ok := d[name]; ok {
			if arr, ok := resolveRefToArray(objects, v); ok {
				return rectFromArray(arr)
			}
		}
	}
	return mediaBoxRect(objects, objNum)
}

// letterMediaBox is the fallback page rectangle (US Letter, 612×792) used when
// neither the page nor any ancestor declares a /MediaBox — required by ISO
// 32000-1, but absent in the wild (e.g. XFA form shells whose /Page dict is an
// empty placeholder). Acrobat and MuPDF default to Letter in this case.
func letterMediaBox() Rectangle {
	return Rectangle{LLX: 0, LLY: 0, URX: 612, URY: 792}
}

// mediaBoxRect reads the /MediaBox of the page object at objNum as a Rectangle,
// walking up the /Parent chain if needed (inheritance). When the chain cannot
// produce a MediaBox (no /Parent, parent object missing — Pages nodes are
// dropped from doc.objects by design — or no /MediaBox at the root), it falls
// back to US Letter the way Acrobat/MuPDF do.
func mediaBoxRect(objects map[int]*pdfObject, objNum int) (Rectangle, error) {
	visited := make(map[int]bool)
	first := true
	for {
		if visited[objNum] {
			return Rectangle{}, fmt.Errorf("cycle in page tree at object %d", objNum)
		}
		visited[objNum] = true

		obj, ok := objects[objNum]
		if !ok {
			if first {
				return Rectangle{}, fmt.Errorf("object %d not found", objNum)
			}
			return letterMediaBox(), nil
		}
		first = false
		d, ok := obj.Value.(pdfDict)
		if !ok {
			return Rectangle{}, fmt.Errorf("object %d is not a dict", objNum)
		}

		if mb, ok := d["/MediaBox"]; ok {
			arr, ok := resolveRefToArray(objects, mb)
			if !ok {
				return Rectangle{}, fmt.Errorf("invalid /MediaBox")
			}
			return rectFromArray(arr)
		}

		parentVal, ok := d["/Parent"]
		if !ok {
			return letterMediaBox(), nil
		}
		parentRef, ok := parentVal.(pdfRef)
		if !ok {
			return letterMediaBox(), nil
		}
		objNum = parentRef.Num
	}
}

// mediaBoxSize reads the /MediaBox of the page object at objNum as width/height.
func mediaBoxSize(objects map[int]*pdfObject, objNum int) (PageSize, error) {
	r, err := mediaBoxRect(objects, objNum)
	if err != nil {
		return PageSize{}, err
	}
	return PageSize{Width: r.URX - r.LLX, Height: r.URY - r.LLY}, nil
}

// rectFromArray converts a [x1 y1 x2 y2] PDF array to a normalized Rectangle
// (lower-left / upper-right corners), tolerating arrays given in either corner
// order.
func rectFromArray(arr pdfArray) (Rectangle, error) {
	if len(arr) != 4 {
		return Rectangle{}, fmt.Errorf("box must have 4 elements, got %d", len(arr))
	}
	v := make([]float64, 4)
	for i, e := range arr {
		f, err := toFloat(e)
		if err != nil {
			return Rectangle{}, fmt.Errorf("box[%d]: %w", i, err)
		}
		v[i] = f
	}
	return Rectangle{
		LLX: minF(v[0], v[2]), LLY: minF(v[1], v[3]),
		URX: maxF(v[0], v[2]), URY: maxF(v[1], v[3]),
	}, nil
}
