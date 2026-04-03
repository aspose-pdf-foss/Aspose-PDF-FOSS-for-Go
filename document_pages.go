package asposepdf

import "fmt"

// Rotate rotates selected pages clockwise by angle (Rotate90, Rotate180, or Rotate270).
// The rotation is added to any existing rotation.
// If no page numbers are given, all pages are rotated. Page numbers are 1-based.
//
// Example:
//
//	err = doc.Rotate(asposepdf.Rotate90)        // rotate all pages
//	err = doc.Rotate(asposepdf.Rotate180, 1, 3) // rotate pages 1 and 3
func (d *Document) Rotate(angle RotationAngle, pageNums ...int) error {
	if err := angle.validate(); err != nil {
		return err
	}
	if angle == Rotate0 {
		return nil
	}
	indices, err := resolvePageIndices(len(d.pages), pageNums)
	if err != nil {
		return err
	}
	for _, i := range indices {
		pg := &Page{doc: d, index: i}
		dict := pg.pageDict()
		if dict == nil {
			continue
		}
		current := pg.Rotation()
		dict["/Rotate"] = (int(current) + int(angle)) % 360
	}
	return nil
}

// SetRotation sets selected pages to exactly angle (Rotate0, Rotate90, Rotate180, or Rotate270),
// replacing any existing rotation.
// If no page numbers are given, all pages are affected. Page numbers are 1-based.
//
// Example:
//
//	err = doc.SetRotation(asposepdf.Rotate90)        // set all pages to 90°
//	err = doc.SetRotation(asposepdf.Rotate0, 1, 3)  // reset pages 1 and 3 to 0°
func (d *Document) SetRotation(angle RotationAngle, pageNums ...int) error {
	if err := angle.validate(); err != nil {
		return err
	}
	indices, err := resolvePageIndices(len(d.pages), pageNums)
	if err != nil {
		return err
	}
	for _, i := range indices {
		pg := &Page{doc: d, index: i}
		dict := pg.pageDict()
		if dict != nil {
			dict["/Rotate"] = int(angle)
		}
	}
	return nil
}

// Reorder rearranges pages according to order, a slice of 1-based page numbers.
// Pages may be repeated or omitted.
//
// Example — reverse a 4-page document:
//
//	err = doc.Reorder([]int{4, 3, 2, 1})
func (d *Document) Reorder(order []int) error {
	newPages := make([]*pdfObject, len(order))
	for i, n := range order {
		if n < 1 || n > len(d.pages) {
			return fmt.Errorf("page number %d out of range (1..%d)", n, len(d.pages))
		}
		newPages[i] = d.pages[n-1]
	}
	d.pages = newPages
	return nil
}

// Split returns each page of the document as a separate *Document.
//
// Example:
//
//	parts, err := doc.Split()
//	for i, p := range parts {
//	    p.Save(fmt.Sprintf("page%03d.pdf", i+1))
//	}
func (d *Document) Split() ([]*Document, error) {
	if len(d.pages) == 0 {
		return nil, fmt.Errorf("document has no pages")
	}
	result := make([]*Document, len(d.pages))
	for i, page := range d.pages {
		deps := collectPageDeps(d.objects, page)
		result[i] = &Document{
			objects: deps,
			pages:   []*pdfObject{page},
			nextID:  maxObjectID(deps) + 1,
		}
	}
	return result, nil
}

// Extract returns a new Document containing only the pages in the specified ranges.
// Ranges are 1-based and inclusive. Pages appear in the order the ranges are listed.
//
// Example:
//
//	extracted, err := doc.Extract(asposepdf.PageRange{1, 3}, asposepdf.PageRange{5, 5})
//	extracted.Save("output.pdf")
func (d *Document) Extract(ranges ...PageRange) (*Document, error) {
	if len(ranges) == 0 {
		return nil, fmt.Errorf("no page ranges specified")
	}
	var selected []*pdfObject
	for _, r := range ranges {
		from, to, err := validateRange(r.From, r.To, len(d.pages))
		if err != nil {
			return nil, err
		}
		selected = append(selected, d.pages[from-1:to]...)
	}
	// Collect deps for all selected pages.
	merged := make(map[int]*pdfObject)
	for _, page := range selected {
		for id, obj := range collectPageDeps(d.objects, page) {
			merged[id] = obj
		}
	}
	return &Document{
		objects: merged,
		pages:   selected,
		nextID:  maxObjectID(merged) + 1,
	}, nil
}
