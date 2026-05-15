package asposepdf

// parseOutlines reads /Catalog/Outlines and builds the in-memory tree.
// Best-effort: malformed entries are silently skipped.
func parseOutlines(d *Document) *OutlineItemCollection {
	root := &OutlineItemCollection{doc: d, isExpanded: true}

	if d.catalog == nil {
		return root
	}
	outlineRef, ok := d.catalog["/Outlines"].(pdfRef)
	if !ok {
		return root
	}
	outlineObj, ok := d.objects[outlineRef.Num]
	if !ok {
		return root
	}
	outlineDict, ok := outlineObj.Value.(pdfDict)
	if !ok {
		return root
	}
	root.dict = outlineDict
	root.objNum = outlineRef.Num

	firstRef, ok := outlineDict["/First"].(pdfRef)
	if !ok {
		return root
	}
	walkOutlineSiblings(d, root, firstRef, map[int]bool{}, 0)
	return root
}

// walkOutlineSiblings reads the /Next-linked sibling chain at the given
// starting ref, parsing each into an OutlineItemCollection and recursing
// into /First for grandchildren. seen + depth defend against cycles.
func walkOutlineSiblings(d *Document, parent *OutlineItemCollection, ref pdfRef, seen map[int]bool, depth int) {
	if depth > 100 {
		return // hard cap
	}
	cur := ref
	for {
		if seen[cur.Num] {
			return
		}
		seen[cur.Num] = true
		obj, ok := d.objects[cur.Num]
		if !ok {
			return
		}
		dict, ok := obj.Value.(pdfDict)
		if !ok {
			return
		}
		item := &OutlineItemCollection{
			doc:    d,
			parent: parent,
			dict:   dict,
			objNum: cur.Num,
		}
		parent.children = append(parent.children, item)

		if firstRef, ok := dict["/First"].(pdfRef); ok {
			walkOutlineSiblings(d, item, firstRef, seen, depth+1)
		}
		nextRef, ok := dict["/Next"].(pdfRef)
		if !ok {
			return
		}
		cur = nextRef
	}
}

// parseDestination wraps parseDestinationArray to also handle indirect
// refs and named-destination strings.
func parseDestination(doc *Document, raw pdfValue) Destination {
	if raw == nil {
		return nil
	}
	switch v := raw.(type) {
	case pdfArray:
		return parseDestinationArray(doc, v)
	case pdfRef:
		if obj, ok := doc.objects[v.Num]; ok {
			return parseDestination(doc, obj.Value)
		}
	case string, pdfHexString:
		// Named destination — Subepic 2 will handle the /Names /Dests
		// tree. For Subepic 1, return nil (caller's Destination()
		// returns nil; viewers still navigate via /A if present).
		return nil
	}
	return nil
}

// parseDictAction resolves /A dict (possibly via indirect ref) and
// parses it via the existing parseAction helper from annotation_action.go.
func parseDictAction(doc *Document, raw pdfValue) Action {
	if raw == nil {
		return nil
	}
	dict, ok := raw.(pdfDict)
	if !ok {
		if ref, isRef := raw.(pdfRef); isRef {
			if obj, present := doc.objects[ref.Num]; present {
				dict, _ = obj.Value.(pdfDict)
			}
		}
	}
	if dict == nil {
		return nil
	}
	return parseAction(dict) // existing function from annotation_action.go
}

// outlineDictFlags returns the /F int value from an outline item dict, or 0.
func outlineDictFlags(d pdfDict) int {
	v, _ := d["/F"]
	return toInt(v)
}

// readDictColor returns the /C color, or nil if absent or malformed.
func readDictColor(d pdfDict) *Color {
	arr, ok := d["/C"].(pdfArray)
	if !ok || len(arr) != 3 {
		return nil
	}
	r, _ := toFloat(arr[0])
	g, _ := toFloat(arr[1])
	b, _ := toFloat(arr[2])
	return &Color{R: r, G: g, B: b, A: 1}
}

// readDictIsExpanded returns the boolean from /Count sign. Absent /Count
// means no children → effectively "expanded".
func readDictIsExpanded(d pdfDict) bool {
	count, ok := d["/Count"]
	if !ok {
		return true
	}
	return toInt(count) >= 0
}

// parseDestinationArray decodes a destination per ISO 32000-1 §12.3.2.2.
// Returns nil if the array is malformed or the referenced page is not
// in the in-memory document.
func parseDestinationArray(doc *Document, arr pdfArray) Destination {
	if len(arr) < 2 {
		return nil
	}
	page := resolvePageFromDestRef(doc, arr[0])
	if page == nil {
		return nil
	}
	fit, ok := arr[1].(pdfName)
	if !ok {
		return nil
	}
	switch fit {
	case "/XYZ":
		return parseDestXYZ(page, arr)
	case "/Fit":
		return &DestinationFit{page: page}
	case "/FitH":
		if len(arr) < 3 {
			return &DestinationFitH{page: page, useTop: false}
		}
		top, has := destFloat(arr[2])
		return &DestinationFitH{page: page, top: top, useTop: has}
	case "/FitV":
		if len(arr) < 3 {
			return &DestinationFitV{page: page, useLeft: false}
		}
		left, has := destFloat(arr[2])
		return &DestinationFitV{page: page, left: left, useLeft: has}
	case "/FitR":
		if len(arr) < 6 {
			return nil
		}
		l, _ := destFloat(arr[2])
		b, _ := destFloat(arr[3])
		r, _ := destFloat(arr[4])
		t, _ := destFloat(arr[5])
		return &DestinationFitR{page: page, left: l, bottom: b, right: r, top: t}
	case "/FitB":
		return &DestinationFitB{page: page}
	case "/FitBH":
		if len(arr) < 3 {
			return &DestinationFitBH{page: page, useTop: false}
		}
		top, has := destFloat(arr[2])
		return &DestinationFitBH{page: page, top: top, useTop: has}
	case "/FitBV":
		if len(arr) < 3 {
			return &DestinationFitBV{page: page, useLeft: false}
		}
		left, has := destFloat(arr[2])
		return &DestinationFitBV{page: page, left: left, useLeft: has}
	}
	return nil
}

func parseDestXYZ(page *Page, arr pdfArray) *DestinationXYZ {
	out := &DestinationXYZ{page: page}
	if len(arr) >= 3 {
		out.left, out.useLeft = destFloat(arr[2])
	}
	if len(arr) >= 4 {
		out.top, out.useTop = destFloat(arr[3])
	}
	if len(arr) >= 5 {
		out.zoom, out.useZoom = destFloat(arr[4])
	}
	return out
}

// destFloat returns (value, true) if v is a numeric value, or (0, false)
// if v is pdfNull (meaning "unchanged" in destination semantics).
func destFloat(v pdfValue) (float64, bool) {
	if _, ok := v.(pdfNull); ok {
		return 0, false
	}
	f, err := toFloat(v)
	if err != nil {
		return 0, false
	}
	return f, true
}

// resolvePageFromDestRef walks doc.pages looking for a page whose
// underlying object number matches the destination's first element.
// Handles both pdfRef and pdfDirectRef (the latter is used by
// encodeDestination before serialization). Returns nil if no match.
func resolvePageFromDestRef(doc *Document, v pdfValue) *Page {
	var num int
	switch r := v.(type) {
	case pdfRef:
		num = r.Num
	case pdfDirectRef:
		num = r.Num
	default:
		return nil
	}
	for i, po := range doc.pages {
		if po != nil && po.Num == num {
			// Return the cached page (1-based index).
			p, _ := doc.Page(i + 1)
			return p
		}
	}
	return nil
}
