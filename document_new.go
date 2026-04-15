package asposepdf

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

// NewDocument creates a single-page blank document with the given dimensions in points.
func NewDocument(width, height float64) *Document {
	// Empty content stream.
	contentStream := &pdfStream{
		Dict:    pdfDict{},
		Data:    []byte{},
		Decoded: true,
	}
	contentObj := &pdfObject{Num: 1, Value: contentStream}

	// Page dict.
	pageDict := pdfDict{
		"/Type":      pdfName("/Page"),
		"/MediaBox":  pdfArray{0.0, 0.0, width, height},
		"/Resources": pdfDict{},
		"/Contents":  pdfRef{Num: 1},
	}
	pageObj := &pdfObject{Num: 2, Value: pageDict}

	return &Document{
		objects: map[int]*pdfObject{1: contentObj, 2: pageObj},
		pages:   []*pdfObject{pageObj},
		nextID:  3,
	}
}

// NewDocumentFromFormat creates a single-page blank document using a predefined page format.
func NewDocumentFromFormat(format PageFormat) *Document {
	return NewDocument(format.Width, format.Height)
}
