package asposepdf

import (
	"fmt"
	"regexp"
)

// toIntBytes converts a byte slice of ASCII digits to int.
func toIntBytes(raw []byte) int {
	n := 0
	for _, b := range raw {
		if b >= '0' && b <= '9' {
			n = n*10 + int(b-'0')
		}
	}
	return n
}

// parseAllObjects iterates every non-free entry in xref and parses the
// corresponding object, decoding streams eagerly. Returns a map of objNum → *pdfObject.
// trailer is required to initialise the rawDocument used for object-stream parsing.
func parseAllObjects(data []byte, xref *xrefTable, trailer pdfDict) (map[int]*pdfObject, error) {
	// Re-use the rawDocument parsing logic (private to validate.go) to handle
	// object streams (PDF 1.5+ compressed objects). Both files are in the same package.
	raw := &rawDocument{
		data:      data,
		xref:      xref,
		trailer:   trailer,
		cache:     make(map[int]*pdfObject),
		objStreams: make(map[int][]*pdfObject),
	}

	objects := make(map[int]*pdfObject, len(xref.entries))
	for num, entry := range xref.entries {
		if entry.Free {
			continue
		}
		obj, err := raw.getObject(num)
		if err != nil {
			return nil, fmt.Errorf("parse object %d: %w", num, err)
		}
		objects[num] = obj
	}
	return objects, nil
}

// resolvePageTree walks the /Pages tree in catalog and returns the ordered list
// of /Page objects.
func resolvePageTree(objects map[int]*pdfObject, catalog pdfDict) ([]*pdfObject, error) {
	pagesVal, ok := catalog["/Pages"]
	if !ok {
		return nil, fmt.Errorf("catalog missing /Pages")
	}
	var result []*pdfObject
	if err := walkPageTree(objects, pagesVal, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func walkPageTree(objects map[int]*pdfObject, nodeVal pdfValue, result *[]*pdfObject) error {
	ref, ok := nodeVal.(pdfRef)
	if !ok {
		return fmt.Errorf("page tree node is not a ref: %T", nodeVal)
	}
	obj, ok := objects[ref.Num]
	if !ok {
		return fmt.Errorf("object %d not found", ref.Num)
	}
	nodeDict, ok := obj.Value.(pdfDict)
	if !ok {
		return fmt.Errorf("page tree object %d is not a dict", ref.Num)
	}
	switch dictGetName(nodeDict, "/Type") {
	case "/Pages":
		kidsVal, ok := nodeDict["/Kids"]
		if !ok {
			return fmt.Errorf("Pages node %d missing /Kids", ref.Num)
		}
		arr, ok := kidsVal.(pdfArray)
		if !ok {
			return fmt.Errorf("/Kids is not an array")
		}
		for _, kid := range arr {
			if err := walkPageTree(objects, kid, result); err != nil {
				return err
			}
		}
	case "/Page", "": // empty /Type is tolerated for compatibility with some malformed PDFs
		*result = append(*result, obj)
	default:
		return fmt.Errorf("unknown page tree node type: %s at object %d",
			dictGetName(nodeDict, "/Type"), ref.Num)
	}
	return nil
}

// extractInfo reads the /Info dictionary from the trailer, resolving the reference.
// Returns nil if no /Info entry is present.
func extractInfo(objects map[int]*pdfObject, trailer pdfDict) pdfDict {
	infoVal, ok := trailer["/Info"]
	if !ok {
		return nil
	}
	ref, ok := infoVal.(pdfRef)
	if !ok {
		return nil
	}
	obj, ok := objects[ref.Num]
	if !ok {
		return nil
	}
	d, ok := obj.Value.(pdfDict)
	if !ok {
		return nil
	}
	return d
}

// extractCatalog resolves the /Root reference from the trailer.
func extractCatalog(objects map[int]*pdfObject, trailer pdfDict) (pdfDict, error) {
	rootVal, ok := trailer["/Root"]
	if !ok {
		return nil, fmt.Errorf("trailer missing /Root")
	}
	ref, ok := rootVal.(pdfRef)
	if !ok {
		return nil, fmt.Errorf("/Root is not a ref")
	}
	obj, ok := objects[ref.Num]
	if !ok {
		return nil, fmt.Errorf("/Root object %d not found", ref.Num)
	}
	d, ok := obj.Value.(pdfDict)
	if !ok {
		return nil, fmt.Errorf("/Root object %d is not a dict", ref.Num)
	}
	return d, nil
}

// maxObjectID returns the largest object ID in the map.
func maxObjectID(objects map[int]*pdfObject) int {
	max := 0
	for id := range objects {
		if id > max {
			max = id
		}
	}
	return max
}

// collectPageDeps returns a map of all objects needed to render page,
// including the page object itself. Skips /Pages, /Catalog, and /Page nodes
// reached transitively (e.g. via link annotations).
func collectPageDeps(objects map[int]*pdfObject, page *pdfObject) map[int]*pdfObject {
	deps := make(map[int]*pdfObject)
	visited := make(map[int]bool)
	// Add the page itself directly (skip the type guard in collectObjDeps).
	deps[page.Num] = page
	visited[page.Num] = true
	if d, ok := page.Value.(pdfDict); ok {
		collectDictDeps(objects, d, deps, visited)
	}
	return deps
}

func collectObjDeps(objects map[int]*pdfObject, num int, deps map[int]*pdfObject, visited map[int]bool) {
	if visited[num] {
		return
	}
	obj, ok := objects[num]
	if !ok {
		return
	}
	// Skip page-tree structural nodes — they are rebuilt by the writer.
	if d, ok := obj.Value.(pdfDict); ok {
		switch dictGetName(d, "/Type") {
		case "/Pages", "/Catalog", "/Page":
			return
		}
	}
	visited[num] = true
	deps[num] = obj
	collectValueDepsDoc(objects, obj.Value, deps, visited)
}

func collectValueDepsDoc(objects map[int]*pdfObject, v pdfValue, deps map[int]*pdfObject, visited map[int]bool) {
	switch val := v.(type) {
	case pdfRef:
		collectObjDeps(objects, val.Num, deps, visited)
	case pdfDict:
		collectDictDeps(objects, val, deps, visited)
	case pdfArray:
		for _, av := range val {
			collectValueDepsDoc(objects, av, deps, visited)
		}
	case *pdfStream:
		collectDictDeps(objects, val.Dict, deps, visited)
		// Scan stream bytes for inline references (content streams).
		for _, m := range reRefDoc.FindAllSubmatch(val.Data, -1) {
			n := toIntBytes(m[1])
			if n > 0 {
				collectObjDeps(objects, n, deps, visited)
			}
		}
	}
}

func collectDictDeps(objects map[int]*pdfObject, d pdfDict, deps map[int]*pdfObject, visited map[int]bool) {
	for _, dv := range d {
		collectValueDepsDoc(objects, dv, deps, visited)
	}
}

var reRefDoc = regexp.MustCompile(`\b(\d+)\s+\d+\s+R\b`)

// rewriteRefs returns a deep copy of v with all pdfRef IDs translated through idMap.
// Objects whose IDs are not in idMap are left as-is.
func rewriteRefs(v pdfValue, idMap map[int]int) pdfValue {
	switch val := v.(type) {
	case pdfRef:
		if newID, ok := idMap[val.Num]; ok {
			return pdfRef{Num: newID, Gen: val.Gen}
		}
		return val
	case pdfDict:
		nd := make(pdfDict, len(val))
		for k, dv := range val {
			nd[k] = rewriteRefs(dv, idMap)
		}
		return nd
	case pdfArray:
		na := make(pdfArray, len(val))
		for i, av := range val {
			na[i] = rewriteRefs(av, idMap)
		}
		return na
	case *pdfStream:
		nd := make(pdfDict, len(val.Dict))
		for k, dv := range val.Dict {
			nd[k] = rewriteRefs(dv, idMap)
		}
		newData := reRefDoc.ReplaceAllFunc(val.Data, func(match []byte) []byte {
			sub := reRefDoc.FindSubmatch(match)
			if sub == nil {
				return match
			}
			n := toIntBytes(sub[1])
			if newID, ok := idMap[n]; ok {
				return []byte(fmt.Sprintf("%d 0 R", newID))
			}
			return match
		})
		return &pdfStream{Dict: nd, Data: newData, Decoded: val.Decoded}
	}
	return v
}

// resolveRef follows one level of indirection if v is a pdfRef.
func resolveRef(objects map[int]*pdfObject, v pdfValue) pdfValue {
	ref, ok := v.(pdfRef)
	if !ok {
		return v
	}
	obj, ok := objects[ref.Num]
	if !ok {
		return v
	}
	return obj.Value
}

// resolveRefToDict resolves v to a pdfDict, following a ref if needed.
func resolveRefToDict(objects map[int]*pdfObject, v pdfValue) (pdfDict, bool) {
	rv := resolveRef(objects, v)
	d, ok := rv.(pdfDict)
	return d, ok
}

// resolveRefToArray resolves v to a pdfArray, following a ref if needed.
func resolveRefToArray(objects map[int]*pdfObject, v pdfValue) (pdfArray, bool) {
	rv := resolveRef(objects, v)
	a, ok := rv.(pdfArray)
	return a, ok
}
