package asposepdf

import "fmt"

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
