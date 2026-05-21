// SPDX-License-Identifier: MIT

package asposepdf

// walkNameTree visits every (name, value) pair in a PDF name tree per
// ISO 32000-1 §7.9.6. Each node has either /Names (leaf) OR /Kids
// (intermediate). /Limits is advisory and ignored for walking.
//
// Defensive: cycle protection via seen[objNum], hard depth cap of 100.
func walkNameTree(d *Document, root pdfValue, visit func(name string, val pdfValue)) {
	seen := map[int]bool{}
	walkNameTreeNode(d, root, visit, seen, 0)
}

func walkNameTreeNode(d *Document, raw pdfValue, visit func(string, pdfValue), seen map[int]bool, depth int) {
	if depth > 100 {
		return
	}
	var nodeDict pdfDict
	switch v := raw.(type) {
	case pdfRef:
		if seen[v.Num] {
			return
		}
		seen[v.Num] = true
		obj, ok := d.objects[v.Num]
		if !ok {
			return
		}
		nodeDict, _ = obj.Value.(pdfDict)
	case pdfDict:
		nodeDict = v
	}
	if nodeDict == nil {
		return
	}

	// Leaf: /Names array of alternating name/value pairs.
	if namesArr, ok := nodeDict["/Names"].(pdfArray); ok {
		for i := 0; i+1 < len(namesArr); i += 2 {
			var name string
			switch s := namesArr[i].(type) {
			case string:
				name = s
			case pdfHexString:
				name = string(s)
			default:
				continue
			}
			visit(name, namesArr[i+1])
		}
		return
	}

	// Intermediate: /Kids array of child refs.
	if kids, ok := nodeDict["/Kids"].(pdfArray); ok {
		for _, kid := range kids {
			walkNameTreeNode(d, kid, visit, seen, depth+1)
		}
	}
}

// parseDestinationAny resolves a name's value into a Destination of
// one of the 8 explicit types. Per ISO 32000-1 §12.3.2.3 named
// destinations cannot themselves reference another name — so we
// silently ignore string values here.
func parseDestinationAny(d *Document, raw pdfValue) Destination {
	if raw == nil {
		return nil
	}
	switch v := raw.(type) {
	case pdfArray:
		return parseDestinationArray(d, v)
	case pdfDict:
		if dArr, ok := v["/D"].(pdfArray); ok {
			return parseDestinationArray(d, dArr)
		}
	case pdfRef:
		if obj, ok := d.objects[v.Num]; ok {
			return parseDestinationAny(d, obj.Value)
		}
	}
	return nil
}

// resolveNamedDest returns a *NamedDestination wrapper for the given
// name. Even unregistered names produce a wrapper — preserves the name
// for round-trip; callers detect unresolved names via wrapper.Resolve()
// returning nil. Empty name returns nil.
//
// Per ISO 32000-1 §12.3.2.3.
func resolveNamedDest(doc *Document, name string) Destination {
	if name == "" {
		return nil
	}
	return NewNamedDestination(doc, name)
}

// parseNamedDestinations reads /Catalog/Names/Dests (modern name tree)
// and merges /Catalog/Dests (legacy flat dict). On collision, the
// /Names/Dests entry wins (matches Adobe Acrobat / pypdf behavior).
// Always returns a non-nil collection.
//
// Per ISO 32000-1 §12.3.2.3.
func parseNamedDestinations(d *Document) *NamedDestinations {
	out := &NamedDestinations{doc: d, entries: map[string]Destination{}}

	if d.catalog == nil {
		return out
	}

	// 1. Legacy /Catalog/Dests (loaded first so /Names/Dests can override).
	if destsRaw, ok := d.catalog["/Dests"]; ok {
		if dict, ok := resolveRefToDict(d.objects, destsRaw); ok {
			for name, val := range dict {
				if dest := parseDestinationAny(d, val); dest != nil {
					out.entries[name] = dest
				}
			}
		}
	}

	// 2. Modern /Catalog/Names/Dests name tree.
	if namesRaw, ok := d.catalog["/Names"]; ok {
		if namesDict, ok := resolveRefToDict(d.objects, namesRaw); ok {
			if destsRaw, ok := namesDict["/Dests"]; ok {
				walkNameTree(d, destsRaw, func(name string, val pdfValue) {
					if dest := parseDestinationAny(d, val); dest != nil {
						out.entries[name] = dest
					}
				})
			}
		}
	}

	return out
}
