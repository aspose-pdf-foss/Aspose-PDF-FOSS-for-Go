// SPDX-License-Identifier: MIT

package asposepdf

// buildNamedDestTree emits the /Names/Dests name tree as a flat
// single-root node containing all entries in lexicographic order.
// Returns: the tree root ref (value for /Catalog/Names → /Dests),
// the parent /Names dict ref (value for /Catalog/Names), and the
// slice of new pdfObjects to add to d.objects. Returns zero/zero/nil
// if the collection is empty.
//
// Per ISO 32000-1 §7.9.6 (name trees) and §12.3.2.3 (named destinations).
func buildNamedDestTree(d *Document) (pdfRef, pdfRef, []*pdfObject) {
	nd := d.namedDests
	if nd == nil || nd.Count() == 0 {
		return pdfRef{}, pdfRef{}, nil
	}

	names := nd.Names() // sorted snapshot per Names() contract

	var namesArr pdfArray
	for _, name := range names {
		dest := nd.entries[name]
		// Defensive: if a NamedDestination snuck in past Add validation,
		// try to resolve it; skip on failure.
		if inner, ok := dest.(*NamedDestination); ok {
			dest = inner.Resolve()
			if dest == nil {
				continue
			}
		}
		destArr := encodeDestination(dest)
		if destArr == nil {
			continue
		}
		namesArr = append(namesArr, name)
		namesArr = append(namesArr, destArr)
	}
	if len(namesArr) == 0 {
		return pdfRef{}, pdfRef{}, nil
	}

	// Tree root: single flat node with /Names and /Limits.
	treeRootDict := pdfDict{
		"/Names":  namesArr,
		"/Limits": pdfArray{names[0], names[len(names)-1]},
	}
	treeRootID := d.nextID
	d.nextID++

	// Parent /Names dict.
	namesDictID := d.nextID
	d.nextID++
	namesDict := pdfDict{
		"/Dests": pdfRef{Num: treeRootID},
	}

	return pdfRef{Num: treeRootID}, pdfRef{Num: namesDictID}, []*pdfObject{
		{Num: treeRootID, Value: treeRootDict},
		{Num: namesDictID, Value: namesDict},
	}
}
