// SPDX-License-Identifier: MIT

package asposepdf

// outlineFlags returns the /F bit field per ISO 32000-1 §12.3.3 Table 153.
// bit 1 = italic, bit 2 = bold.
func outlineFlags(bold, italic bool) int {
	var f int
	if italic {
		f |= 1
	}
	if bold {
		f |= 2
	}
	return f
}

// visibleDescendantCount returns the magnitude used for the /Count entry
// per ISO 32000-1 §12.3.3:
//
//	Σ direct children + Σ (over expanded children) of count(child).
//
// The sign on the RESULT is applied by the caller — encodeOutlineItem
// negates if !node.IsExpanded() && node is not the root.
func visibleDescendantCount(node *OutlineItemCollection) int {
	total := len(node.children)
	for _, ch := range node.children {
		if ch.IsExpanded() {
			total += visibleDescendantCount(ch)
		}
	}
	return total
}

// encodeDestination produces the destination array per ISO 32000-1 §12.3.2.2.
// The page reference is a pdfDirectRef bypassing the writer's ID remap,
// so the array points at the page's actual emitted object number.
func encodeDestination(d Destination) pdfArray {
	if d == nil || d.Page() == nil {
		return nil
	}
	pageRef := pdfDirectRef{Num: d.Page().pageObj().Num}
	switch v := d.(type) {
	case *DestinationXYZ:
		return pdfArray{pageRef, pdfName("/XYZ"),
			optFloat(v.left, v.useLeft),
			optFloat(v.top, v.useTop),
			optFloat(v.zoom, v.useZoom),
		}
	case *DestinationFit:
		return pdfArray{pageRef, pdfName("/Fit")}
	case *DestinationFitH:
		return pdfArray{pageRef, pdfName("/FitH"), optFloat(v.top, v.useTop)}
	case *DestinationFitV:
		return pdfArray{pageRef, pdfName("/FitV"), optFloat(v.left, v.useLeft)}
	case *DestinationFitR:
		return pdfArray{pageRef, pdfName("/FitR"), v.left, v.bottom, v.right, v.top}
	case *DestinationFitB:
		return pdfArray{pageRef, pdfName("/FitB")}
	case *DestinationFitBH:
		return pdfArray{pageRef, pdfName("/FitBH"), optFloat(v.top, v.useTop)}
	case *DestinationFitBV:
		return pdfArray{pageRef, pdfName("/FitBV"), optFloat(v.left, v.useLeft)}
	}
	return nil
}

// optFloat returns v as float64 if use, else pdfNull{}.
func optFloat(v float64, use bool) pdfValue {
	if !use {
		return pdfNull{}
	}
	return v
}

// outlineEntry is a flat representation of an outline item used during
// the write pass. It carries the underlying *OutlineItemCollection plus
// the wiring (parent/prev/next/first/last) needed to emit a PDF dict.
type outlineEntry struct {
	item    *OutlineItemCollection
	objNum  int
	parent  int // 0 = root
	prev    int
	next    int
	firstCh int
	lastCh  int
}

// buildOutlineObjects flattens d.outlinesRoot into PDF objects. Returns
// the /Outlines root ref to wire into the catalog, and the slice of
// new pdfObjects to add to d.objects. Returns zero/nil if the tree
// has no children.
func buildOutlineObjects(d *Document) (pdfRef, []*pdfObject) {
	root := d.outlinesRoot
	if root == nil || root.Count() == 0 {
		return pdfRef{}, nil
	}

	// Allocate root /Outlines dict ID first.
	rootObjNum := d.nextID
	d.nextID++

	// Allocate IDs for items in DFS pre-order.
	var entries []*outlineEntry
	assignOutlineIDs(d, root, 0, &entries)

	// Wire prev/next/firstCh/lastCh.
	wireOutlineSiblings(entries)

	// Build item dicts.
	var objs []*pdfObject
	for _, e := range entries {
		dict := encodeOutlineItem(e, rootObjNum)
		objs = append(objs, &pdfObject{Num: e.objNum, Value: dict})
	}

	// Build root /Outlines dict.
	rootDict := pdfDict{"/Type": pdfName("/Outlines")}
	var firstTop, lastTop int
	for _, e := range entries {
		if e.parent == 0 {
			if firstTop == 0 {
				firstTop = e.objNum
			}
			lastTop = e.objNum
		}
	}
	if firstTop != 0 {
		rootDict["/First"] = pdfRef{Num: firstTop}
		rootDict["/Last"] = pdfRef{Num: lastTop}
		rootDict["/Count"] = visibleDescendantCount(root) // always positive for root
	}
	objs = append(objs, &pdfObject{Num: rootObjNum, Value: rootDict})

	return pdfRef{Num: rootObjNum}, objs
}

// assignOutlineIDs walks the tree in DFS pre-order, allocating IDs.
func assignOutlineIDs(d *Document, node *OutlineItemCollection, parentNum int, out *[]*outlineEntry) {
	for _, ch := range node.children {
		objNum := d.nextID
		d.nextID++
		e := &outlineEntry{item: ch, objNum: objNum, parent: parentNum}
		*out = append(*out, e)
		assignOutlineIDs(d, ch, objNum, out)
	}
}

// wireOutlineSiblings sets prev/next/firstCh/lastCh on each entry.
func wireOutlineSiblings(entries []*outlineEntry) {
	byParent := map[int][]int{}
	for _, e := range entries {
		byParent[e.parent] = append(byParent[e.parent], e.objNum)
	}
	byNum := map[int]*outlineEntry{}
	for _, e := range entries {
		byNum[e.objNum] = e
	}
	for _, siblings := range byParent {
		for i, num := range siblings {
			e := byNum[num]
			if i > 0 {
				e.prev = siblings[i-1]
			}
			if i < len(siblings)-1 {
				e.next = siblings[i+1]
			}
		}
	}
	for _, e := range entries {
		ch := byParent[e.objNum]
		if len(ch) > 0 {
			e.firstCh = ch[0]
			e.lastCh = ch[len(ch)-1]
		}
	}
}

// encodeOutlineItem produces the pdfDict for a single outline item.
func encodeOutlineItem(e *outlineEntry, rootObjNum int) pdfDict {
	o := e.item
	dict := pdfDict{
		"/Title": encodeFormString(o.Title()),
	}
	if e.parent == 0 {
		dict["/Parent"] = pdfRef{Num: rootObjNum}
	} else {
		dict["/Parent"] = pdfRef{Num: e.parent}
	}
	if e.prev != 0 {
		dict["/Prev"] = pdfRef{Num: e.prev}
	}
	if e.next != 0 {
		dict["/Next"] = pdfRef{Num: e.next}
	}
	if e.firstCh != 0 {
		dict["/First"] = pdfRef{Num: e.firstCh}
		dict["/Last"] = pdfRef{Num: e.lastCh}
		count := visibleDescendantCount(o)
		if !o.IsExpanded() {
			count = -count
		}
		dict["/Count"] = count
	}
	if flags := outlineFlags(o.Bold(), o.Italic()); flags != 0 {
		dict["/F"] = flags
	}
	if c := o.Color(); c != nil {
		dict["/C"] = pdfArray{c.R, c.G, c.B}
	}
	if d := o.Destination(); d != nil {
		if nd, ok := d.(*NamedDestination); ok {
			// Per ISO 32000-1 §12.3.2.3, /Dest may hold either an
			// explicit-dest array or a name-string reference into
			// /Names/Dests. The writer's case string: branch emits
			// Go strings as PDF string literals "(name)".
			dict["/Dest"] = nd.Name()
		} else if arr := encodeDestination(d); arr != nil {
			dict["/Dest"] = arr
		}
	}
	if a := o.Action(); a != nil {
		dict["/A"] = a.encode()
	}
	return dict
}
