package asposepdf

import "fmt"

// PageFormat describes a page size in points (1/72 inch).
type PageFormat struct {
	Width  float64
	Height float64
}

// Predefined page formats (portrait orientation).
var (
	PageFormatA3     = PageFormat{Width: 842, Height: 1191}
	PageFormatA4     = PageFormat{Width: 595, Height: 842}
	PageFormatLetter = PageFormat{Width: 612, Height: 792}
	PageFormatLegal  = PageFormat{Width: 612, Height: 1008}
)

// Landscape returns the format with width and height swapped.
func (f PageFormat) Landscape() PageFormat {
	return PageFormat{Width: f.Height, Height: f.Width}
}

// createBlankPage creates a blank page with associated content stream and registers
// both objects in the document. Returns the page object.
func (d *Document) createBlankPage(width, height float64) *pdfObject {
	contentID := d.nextID
	d.nextID++
	contentStream := &pdfStream{
		Dict:    pdfDict{},
		Data:    []byte{},
		Decoded: true,
	}
	d.objects[contentID] = &pdfObject{Num: contentID, Value: contentStream}

	pageID := d.nextID
	d.nextID++
	pageDict := pdfDict{
		"/Type":      pdfName("/Page"),
		"/MediaBox":  pdfArray{0.0, 0.0, width, height},
		"/Resources": pdfDict{},
		"/Contents":  pdfRef{Num: contentID},
	}
	pageObj := &pdfObject{Num: pageID, Value: pageDict}
	d.objects[pageID] = pageObj

	return pageObj
}

// NewDocument creates a single-page blank document with the given dimensions in points.
func NewDocument(width, height float64) *Document {
	doc := &Document{
		objects: make(map[int]*pdfObject),
		nextID:  1,
	}
	pageObj := doc.createBlankPage(width, height)
	doc.pages = []*pdfObject{pageObj}
	return doc
}

// NewDocumentFromFormat creates a single-page blank document using a predefined page format.
func NewDocumentFromFormat(format PageFormat) *Document {
	return NewDocument(format.Width, format.Height)
}

// AddBlankPage appends a blank page to the end of the document.
func (d *Document) AddBlankPage(width, height float64) error {
	pageObj := d.createBlankPage(width, height)
	d.pages = append(d.pages, pageObj)
	return nil
}

// AddBlankPageFromFormat appends a blank page using a predefined page format.
func (d *Document) AddBlankPageFromFormat(format PageFormat) error {
	return d.AddBlankPage(format.Width, format.Height)
}

// InsertBlankPage inserts a blank page at the given 1-based position.
// Existing pages at and after that position shift by one.
func (d *Document) InsertBlankPage(position int, width, height float64) error {
	if position < 1 || position > len(d.pages)+1 {
		return fmt.Errorf("insert blank page: position %d out of range [1, %d]", position, len(d.pages)+1)
	}
	pageObj := d.createBlankPage(width, height)
	idx := position - 1
	d.pages = append(d.pages, nil)
	copy(d.pages[idx+1:], d.pages[idx:])
	d.pages[idx] = pageObj
	return nil
}

// InsertBlankPageFromFormat inserts a blank page at the given position using a predefined page format.
func (d *Document) InsertBlankPageFromFormat(position int, format PageFormat) error {
	return d.InsertBlankPage(position, format.Width, format.Height)
}
