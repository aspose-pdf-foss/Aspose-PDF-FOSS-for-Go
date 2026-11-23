// SPDX-License-Identifier: MIT

package asposepdf

import "fmt"

// pageImportKey names a page of another document imported into this one.
type pageImportKey struct {
	src  *Document
	page int // 1-based
}

// importedPage is the Form XObject a page was imported as and the box it
// occupies once its /Matrix applies (lower-left at the origin).
type importedPage struct {
	formID int
	shown  Rectangle
}

// ImportPage imports page pageNum (1-based) of src as a Form XObject that
// can be placed on any page of d with Page.AddForm: the page's content and
// its whole resource graph are copied once, and every placement draws the
// same object. The form's box is the page as a viewer shows it (its CropBox,
// turned by its /Rotate), so Size reports the displayed dimensions. Importing
// the same page again returns a form for the same object, and pages of one
// source share the fonts and images they have in common. src may be d itself.
// Later changes to the source page are not picked up by a form imported
// before them. Mirrors Aspose.PDF for .NET's XForm.CreateNewForm(Page,
// Document).
func (d *Document) ImportPage(src *Document, pageNum int) (*XForm, error) {
	if src == nil {
		return nil, fmt.Errorf("ImportPage: nil source document")
	}
	if err := src.ApplyTextChanges(); err != nil {
		return nil, err
	}
	id, shown, err := d.importPageOnce(src, pageNum)
	if err != nil {
		return nil, fmt.Errorf("ImportPage: %w", err)
	}
	return d.trackForm(&XForm{doc: d, formID: id, bbox: shown}), nil
}

// importPageOnce returns the Form XObject page pageNum of src was imported
// as, importing it on first use (GitHub issue #26 point 1: PdfPageStamp
// copied the page and its fonts again for every page it was applied to).
func (d *Document) importPageOnce(src *Document, pageNum int) (int, Rectangle, error) {
	key := pageImportKey{src, pageNum}
	if ip, ok := d.importedPages[key]; ok {
		if _, live := d.objects[ip.formID]; live {
			return ip.formID, ip.shown, nil
		}
	}
	if pageNum < 1 || pageNum > src.PageCount() {
		return 0, Rectangle{}, fmt.Errorf("source page %d out of range [1,%d]", pageNum, src.PageCount())
	}
	srcPage, err := src.Page(pageNum)
	if err != nil {
		return 0, Rectangle{}, err
	}
	if d.importMaps == nil {
		d.importMaps = map[*Document]map[int]int{}
	}
	idMap := d.importMaps[src]
	if idMap == nil {
		idMap = map[int]int{}
		if src == d {
			// A page of this document already has its resources here.
			for id := range d.objects {
				idMap[id] = id
			}
		}
		d.importMaps[src] = idMap
	}
	id, shown, err := d.importPageAsXObject(src, srcPage, idMap)
	if err != nil {
		return 0, Rectangle{}, err
	}
	if d.importedPages == nil {
		d.importedPages = map[pageImportKey]importedPage{}
	}
	d.importedPages[key] = importedPage{formID: id, shown: shown}
	return id, shown, nil
}
