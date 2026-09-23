// SPDX-License-Identifier: MIT

package asposepdf

// Associated files (ISO 32000-2 §14.13; required of every embedded file by
// PDF/A-3, ISO 19005-3 §6.8): a file specification states how the file relates
// to the document through /AFRelationship, and the catalog lists it in /AF.

// AFRelationship says how an embedded file relates to the document. Mirrors
// Aspose.PDF for .NET's AFRelationship.
type AFRelationship int

const (
	// AFUnspecified states no particular relationship. The zero value, and
	// what a file with no /AFRelationship reports.
	AFUnspecified AFRelationship = iota
	// AFSource is the original the document was produced from.
	AFSource
	// AFData is data the document's content is built on, such as the table
	// behind a chart.
	AFData
	// AFAlternative is an alternative representation of the content, such as
	// the XML of a hybrid e-invoice.
	AFAlternative
	// AFSupplement is a supplemental representation of the content.
	AFSupplement
)

var afRelationshipNames = [...]pdfName{
	AFUnspecified: "/Unspecified",
	AFSource:      "/Source",
	AFData:        "/Data",
	AFAlternative: "/Alternative",
	AFSupplement:  "/Supplement",
}

// String names the relationship as PDF writes it, without the slash.
func (r AFRelationship) String() string { return string(r.pdfName()[1:]) }

func (r AFRelationship) pdfName() pdfName {
	if r >= 0 && int(r) < len(afRelationshipNames) {
		return afRelationshipNames[r]
	}
	return afRelationshipNames[AFUnspecified]
}

func afRelationshipFromName(n pdfName) AFRelationship {
	for r, name := range afRelationshipNames {
		if name == n {
			return AFRelationship(r)
		}
	}
	return AFUnspecified
}

// AFRelationship reports how the file relates to the document; a file with no
// /AFRelationship reports AFUnspecified.
func (f *EmbeddedFile) AFRelationship() AFRelationship {
	n, _ := f.filespec["/AFRelationship"].(pdfName)
	return afRelationshipFromName(n)
}

// SetAFRelationship records how the file relates to the document and lists it
// in the catalog's /AF array — both of which PDF/A-3 requires of every
// embedded file.
func (f *EmbeddedFile) SetAFRelationship(r AFRelationship) {
	f.filespec["/AFRelationship"] = r.pdfName()
	if f.ref.Num == 0 {
		files := f.doc.EmbeddedFiles()
		// Another handle onto the same direct file-specification dictionary
		// may have already promoted it (e.g. a second Get() on a filespec
		// parsed as a direct dict in the name tree) — reuse that object
		// instead of promoting a duplicate and double-listing it in /AF.
		if existing, ok := files.raw()[f.name].(pdfRef); ok {
			f.ref = existing
		} else {
			f.ref = files.promote(f.name, f.filespec)
		}
	}
	f.doc.addAssociatedFile(f.ref)
}

// hasAFRelationship reports whether the file specification states a
// relationship at all.
func (f *EmbeddedFile) hasAFRelationship() bool {
	_, ok := f.filespec["/AFRelationship"].(pdfName)
	return ok
}

// isAssociatedFile reports whether the catalog's /AF lists ref.
func (d *Document) isAssociatedFile(ref pdfRef) bool {
	if ref.Num == 0 {
		return false
	}
	for _, v := range d.resolveArray(d.catalog["/AF"]) {
		if r, ok := v.(pdfRef); ok && r.Num == ref.Num {
			return true
		}
	}
	return false
}

// addAssociatedFile lists ref in the catalog's /AF, once.
func (d *Document) addAssociatedFile(ref pdfRef) {
	if ref.Num == 0 || d.isAssociatedFile(ref) {
		return
	}
	if d.catalog == nil {
		d.catalog = pdfDict{}
	}
	af := append(pdfArray{}, d.resolveArray(d.catalog["/AF"])...)
	d.catalog["/AF"] = append(af, ref)
}

// removeAssociatedFile drops ref from the catalog's /AF; an emptied array is
// removed.
func (d *Document) removeAssociatedFile(ref pdfRef) {
	if ref.Num == 0 || d.catalog == nil {
		return
	}
	var kept pdfArray
	for _, v := range d.resolveArray(d.catalog["/AF"]) {
		if r, ok := v.(pdfRef); ok && r.Num == ref.Num {
			continue
		}
		kept = append(kept, v)
	}
	if len(kept) == 0 {
		delete(d.catalog, "/AF")
		return
	}
	d.catalog["/AF"] = kept
}
