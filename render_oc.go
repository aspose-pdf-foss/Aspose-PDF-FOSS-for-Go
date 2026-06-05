// SPDX-License-Identifier: MIT

package asposepdf

// Optional Content (ISO 32000-1 §8.11) lets a document group content into
// layers (OCGs) that can be turned on or off. The renderer honors the default
// configuration: content tagged with an OCG/OCMD that is OFF — either via a
// /OC marked-content section (BDC … EMC) or an /OC entry on an XObject — is not
// drawn. Other configurations and interactive toggling are out of scope.

// ocOffSet returns the set of OCG object numbers switched off by the default
// configuration (/Catalog/OCProperties/D/OFF). nil when the document has no
// optional content.
func ocOffSet(objects map[int]*pdfObject, catalog pdfDict) map[int]bool {
	ocp, ok := resolveRefToDict(objects, catalog["/OCProperties"])
	if !ok {
		return nil
	}
	d, ok := resolveRefToDict(objects, ocp["/D"]) // default configuration
	if !ok {
		return nil
	}
	off := map[int]bool{}
	if arr, ok := resolveRefToArray(objects, d["/OFF"]); ok {
		for _, e := range arr {
			if ref, ok := e.(pdfRef); ok {
				off[ref.Num] = true
			}
		}
	}
	return off
}

// ocProperty resolves a BDC property operand: a name is looked up in the current
// /Resources/Properties; an inline dictionary is returned as-is.
func (rd *renderer) ocProperty(v pdfValue) pdfValue {
	if name, ok := v.(pdfName); ok {
		if props, ok := resolveRefToDict(rd.page.doc.objects, rd.res["/Properties"]); ok {
			return props[string(name)]
		}
		return nil
	}
	return v
}

// ocVisible reports whether content tagged with the given OCG/OCMD is visible
// under the default configuration. Unknown or unresolved tags are visible.
func (rd *renderer) ocVisible(prop pdfValue) bool {
	if len(rd.ocOff) == 0 {
		return true
	}
	num := -1
	if ref, ok := prop.(pdfRef); ok {
		num = ref.Num
	}
	d, ok := resolveRefToDict(rd.page.doc.objects, prop)
	if !ok {
		return true
	}
	switch dictGetName(d, "/Type") {
	case "/OCG":
		return num < 0 || !rd.ocOff[num]
	case "/OCMD":
		return rd.ocmdVisible(d)
	default:
		return true
	}
}

// ocmdVisible evaluates an OCMD membership dictionary: its member OCGs combined
// by the visibility policy /P (AnyOn default / AllOn / AnyOff / AllOff).
func (rd *renderer) ocmdVisible(d pdfDict) bool {
	var refs []pdfRef
	switch v := d["/OCGs"].(type) {
	case pdfRef:
		refs = []pdfRef{v}
	case pdfArray:
		for _, e := range v {
			if r, ok := e.(pdfRef); ok {
				refs = append(refs, r)
			}
		}
	}
	if len(refs) == 0 {
		return true
	}
	anyOn, allOn := false, true
	for _, r := range refs {
		on := !rd.ocOff[r.Num]
		anyOn = anyOn || on
		allOn = allOn && on
	}
	switch dictGetName(d, "/P") {
	case "/AllOn":
		return allOn
	case "/AnyOff":
		return !allOn
	case "/AllOff":
		return !anyOn
	default: // /AnyOn
		return anyOn
	}
}
