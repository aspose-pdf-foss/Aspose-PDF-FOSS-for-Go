// SPDX-License-Identifier: MIT

package asposepdf

// OpenAction returns the action the viewer runs when the document is
// opened (/Catalog/OpenAction per ISO 32000-1 §12.3.2 / §12.6.1), or nil
// when none is set. A GoTo open-action has its destination page resolved
// to a 1-based PageNum. When /OpenAction is a bare destination array (a
// "go to page on open" without an explicit action dict) this returns nil
// — read the destination via NamedDestinations / the page tree instead.
//
// Mirrors Aspose.PDF for .NET's Document.OpenAction.
func (d *Document) OpenAction() Action {
	if d.catalog == nil {
		return nil
	}
	raw, ok := d.catalog["/OpenAction"]
	if !ok {
		return nil
	}
	dict, ok := resolveRefToDict(d.objects, raw)
	if !ok {
		return nil // bare destination array, or malformed
	}
	act := parseAction(dict)
	resolveGoToPage(d, act, dict)
	return act
}

// SetOpenAction sets the action run when the document opens. A nil action
// clears /OpenAction. A GoTo action is bound to this document so its
// destination is written as a proper page reference.
//
// Mirrors Aspose.PDF for .NET's Document.OpenAction setter.
func (d *Document) SetOpenAction(act Action) {
	if d.catalog == nil {
		d.catalog = pdfDict{}
	}
	if act == nil {
		delete(d.catalog, "/OpenAction")
		return
	}
	if gt, ok := act.(*GoToAction); ok {
		gt.doc = d
	}
	d.catalog["/OpenAction"] = act.encode()
}

// RemoveOpenAction clears any /Catalog/OpenAction. Equivalent to
// SetOpenAction(nil).
func (d *Document) RemoveOpenAction() {
	if d.catalog != nil {
		delete(d.catalog, "/OpenAction")
	}
}

// resolveGoToPage binds a parsed GoTo action to its document and resolves
// a page-reference destination to a 1-based PageNum. No-op for other
// action types. Shared by OpenAction and LinkAnnotation.Action.
func resolveGoToPage(doc *Document, act Action, d pdfDict) {
	gt, ok := act.(*GoToAction)
	if !ok {
		return
	}
	gt.doc = doc
	if gt.pageNum != 0 {
		return
	}
	dest, ok := d["/D"].(pdfArray)
	if !ok || len(dest) == 0 {
		return
	}
	ref, ok := dest[0].(pdfRef)
	if !ok {
		return
	}
	for i, p := range doc.pages {
		if p.Num == ref.Num {
			gt.pageNum = i + 1
			return
		}
	}
}
