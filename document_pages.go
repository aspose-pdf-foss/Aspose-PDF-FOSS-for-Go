package asposepdf

import "fmt"

// Rotate returns a new Document with selected pages rotated clockwise by angle
// (Rotate90, Rotate180, or Rotate270). The rotation is added to any existing
// rotation. If no page numbers are given, all pages are rotated. Page numbers are 1-based.
//
// Example:
//
//	doc, err = doc.Rotate(asposepdf.Rotate90)        // rotate all pages
//	doc, err = doc.Rotate(asposepdf.Rotate180, 1, 3) // rotate pages 1 and 3
func (d *Document) Rotate(angle RotationAngle, pageNums ...int) (*Document, error) {
	if err := angle.validate(); err != nil {
		return nil, err
	}
	if angle == Rotate0 {
		return d.withCopiedPatches(), nil
	}
	indices, err := resolvePageIndices(len(d.pages), pageNums)
	if err != nil {
		return nil, err
	}
	result := d.withCopiedPatches()
	for _, i := range indices {
		e := result.pages[i]
		key := patchKey{e.src, e.page.objNum}
		current := result.patchedRotation(key, e)
		result.setPatch(key, "/Rotate", (int(current)+int(angle))%360)
	}
	return result, nil
}

// SetRotation returns a new Document with selected pages set to exactly angle
// (Rotate0, Rotate90, Rotate180, or Rotate270), replacing any existing rotation.
// If no page numbers are given, all pages are affected. Page numbers are 1-based.
//
// Example:
//
//	doc, err = doc.SetRotation(asposepdf.Rotate90)        // set all pages to 90°
//	doc, err = doc.SetRotation(asposepdf.Rotate0, 1, 3)  // reset pages 1 and 3 to 0°
func (d *Document) SetRotation(angle RotationAngle, pageNums ...int) (*Document, error) {
	if err := angle.validate(); err != nil {
		return nil, err
	}
	indices, err := resolvePageIndices(len(d.pages), pageNums)
	if err != nil {
		return nil, err
	}
	result := d.withCopiedPatches()
	for _, i := range indices {
		e := result.pages[i]
		key := patchKey{e.src, e.page.objNum}
		result.setPatch(key, "/Rotate", int(angle))
	}
	return result, nil
}

// Split returns each page of the document as a separate *Document.
// The original document is not modified.
//
// Example:
//
//	doc, _ := asposepdf.Open("document.pdf")
//	pages, err := doc.Split()
//	for i, p := range pages {
//	    p.Save(fmt.Sprintf("page%03d.pdf", i+1))
//	}
func (d *Document) Split() ([]*Document, error) {
	if len(d.pages) == 0 {
		return nil, fmt.Errorf("document has no pages")
	}
	result := make([]*Document, len(d.pages))
	for i, p := range d.pages {
		key := patchKey{p.src, p.page.objNum}
		patches := make(map[patchKey]pdfDict)
		if patch, ok := d.patches[key]; ok {
			patches[key] = patch
		}
		result[i] = &Document{
			pages:   []pageRef{p},
			patches: patches,
		}
	}
	return result, nil
}

// Extract returns a new Document containing only the pages in the specified ranges.
// Ranges are 1-based and inclusive. Pages appear in the order the ranges are listed.
// The original document is not modified.
//
// Example:
//
//	doc, _ := asposepdf.Open("input.pdf")
//	extracted, err := doc.Extract(asposepdf.PageRange{1, 3}, asposepdf.PageRange{5, 5})
//	extracted.Save("output.pdf")
func (d *Document) Extract(ranges ...PageRange) (*Document, error) {
	if len(ranges) == 0 {
		return nil, fmt.Errorf("no page ranges specified")
	}
	var selected []pageRef
	for _, r := range ranges {
		from, to, err := validateRange(r.From, r.To, len(d.pages))
		if err != nil {
			return nil, err
		}
		selected = append(selected, d.pages[from-1:to]...)
	}
	return &Document{
		pages:   selected,
		patches: copyPatches(d.patches),
	}, nil
}
