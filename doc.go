// SPDX-License-Identifier: MIT

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
	raw := newRawDocument(data, xref, trailer)
	return parseAllObjectsFrom(raw)
}

// newRawDocument constructs a rawDocument with empty caches.
func newRawDocument(data []byte, xref *xrefTable, trailer pdfDict) *rawDocument {
	return &rawDocument{
		data:       data,
		xref:       xref,
		trailer:    trailer,
		cache:      make(map[int]*pdfObject),
		objStreams: make(map[int][]*pdfObject),
	}
}

// parseAllObjectsFrom walks the xref and resolves every non-free object,
// returning the populated objects map. Decryption (if enabled on raw) is
// applied per-object inside raw.getObject.
//
// An object that fails to parse (a corrupt stream, a bad offset, an
// unreadable dict) is skipped rather than failing the whole document, so a
// file with one damaged object still opens with everything else intact —
// the catalog and page tree downstream tolerate the resulting gaps. Valid
// files are unaffected (every object resolves, so nothing is skipped).
func parseAllObjectsFrom(raw *rawDocument) (map[int]*pdfObject, error) {
	objects := make(map[int]*pdfObject, len(raw.xref.entries))
	for num, entry := range raw.xref.entries {
		if entry.Free {
			continue
		}
		obj, err := raw.getObject(num)
		if err != nil {
			continue // skip the unreadable object, keep the rest
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
	if err := walkPageTree(objects, pagesVal, inheritedPageAttrs{}, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// inheritedPageAttrs tracks the inheritable page attributes per
// ISO 32000-1 §7.7.3.4 Table 30. Values propagate down the /Pages tree and
// are copied onto each leaf /Page dict so the page is self-sufficient once
// Open strips the intermediate /Pages nodes from d.objects.
type inheritedPageAttrs struct {
	resources pdfValue
	mediaBox  pdfValue
	cropBox   pdfValue
	rotate    pdfValue
}

func walkPageTree(objects map[int]*pdfObject, nodeVal pdfValue, inherited inheritedPageAttrs, result *[]*pdfObject) error {
	walkPageTreeRec(objects, nodeVal, inherited, result, map[int]bool{}, 0)
	return nil
}

// walkPageTreeRec traverses the /Pages tree tolerantly: structurally
// broken nodes are skipped rather than failing the whole document, so a
// partially-damaged file still yields the pages that are valid. It guards
// against cycles (a visited set) and pathological depth. A node carrying
// /Kids is treated as an intermediate node regardless of its /Type; a
// leaf with /Type /Page (or none) becomes a page. Missing objects,
// non-dict nodes, and non-ref kids are silently skipped.
func walkPageTreeRec(objects map[int]*pdfObject, nodeVal pdfValue, inherited inheritedPageAttrs, result *[]*pdfObject, visited map[int]bool, depth int) {
	if depth > 256 {
		return // runaway / cyclic tree backstop
	}
	ref, ok := nodeVal.(pdfRef)
	if !ok {
		return
	}
	if visited[ref.Num] {
		return // cycle
	}
	visited[ref.Num] = true
	obj, ok := objects[ref.Num]
	if !ok {
		return // dangling reference — skip
	}
	nodeDict, ok := obj.Value.(pdfDict)
	if !ok {
		return
	}

	// Any attribute present on this node overrides the value inherited from
	// ancestors for this subtree. Siblings are unaffected because inherited
	// is passed by value.
	if v, ok := nodeDict["/Resources"]; ok {
		inherited.resources = v
	}
	if v, ok := nodeDict["/MediaBox"]; ok {
		inherited.mediaBox = v
	}
	if v, ok := nodeDict["/CropBox"]; ok {
		inherited.cropBox = v
	}
	if v, ok := nodeDict["/Rotate"]; ok {
		inherited.rotate = v
	}

	// A node with a /Kids array is an intermediate node (even if /Type is
	// missing or wrong); recurse into each kid, skipping any that fail.
	if kids, ok := nodeDict["/Kids"].(pdfArray); ok {
		for _, kid := range kids {
			walkPageTreeRec(objects, kid, inherited, result, visited, depth+1)
		}
		return
	}

	// Leaf: treat /Page (or an untyped leaf) as a page; skip anything else.
	switch dictGetName(nodeDict, "/Type") {
	case "/Page", "":
		setIfMissing(nodeDict, "/Resources", inherited.resources)
		setIfMissing(nodeDict, "/MediaBox", inherited.mediaBox)
		setIfMissing(nodeDict, "/CropBox", inherited.cropBox)
		setIfMissing(nodeDict, "/Rotate", inherited.rotate)
		*result = append(*result, obj)
	}
}

func setIfMissing(d pdfDict, key string, v pdfValue) {
	if v == nil {
		return
	}
	if _, has := d[key]; has {
		return
	}
	d[key] = v
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

// collectReachableIDs returns the set of object IDs reachable from the given root objects.
// Used by RemoveUnusedObjects to identify orphaned objects.
func collectReachableIDs(objects map[int]*pdfObject, roots []*pdfObject) map[int]bool {
	visited := make(map[int]bool)
	for _, root := range roots {
		visited[root.Num] = true
		markReachable(objects, root.Value, visited)
	}
	return visited
}

func markReachable(objects map[int]*pdfObject, v pdfValue, visited map[int]bool) {
	switch val := v.(type) {
	case pdfRef:
		if visited[val.Num] {
			return
		}
		obj, ok := objects[val.Num]
		if !ok {
			return
		}
		visited[val.Num] = true
		markReachable(objects, obj.Value, visited)
	case pdfDict:
		for _, dv := range val {
			markReachable(objects, dv, visited)
		}
	case pdfArray:
		for _, av := range val {
			markReachable(objects, av, visited)
		}
	case *pdfStream:
		for _, dv := range val.Dict {
			markReachable(objects, dv, visited)
		}
		// Scan stream bytes for inline references (e.g. content streams).
		for _, m := range reRefDoc.FindAllSubmatch(val.Data, -1) {
			n := toIntBytes(m[1])
			if n > 0 && !visited[n] {
				if obj, ok := objects[n]; ok {
					visited[n] = true
					markReachable(objects, obj.Value, visited)
				}
			}
		}
	}
}

// cloneObjects returns a new objects map with every *pdfObject deep-copied.
// Dicts, arrays, streams, and byte-slice strings are recursively duplicated;
// mutations on the returned map do not affect the input and vice versa.
// Used by Split and Extract so that the returned documents are independent
// of the parent, as their API contracts claim.
func cloneObjects(in map[int]*pdfObject) map[int]*pdfObject {
	out := make(map[int]*pdfObject, len(in))
	for id, obj := range in {
		out[id] = &pdfObject{
			Num:   obj.Num,
			Gen:   obj.Gen,
			Value: deepCopyValue(obj.Value),
		}
	}
	return out
}

// deepCopyValue recursively copies v so the result shares no mutable state
// with the original. Immutable kinds (int, float, bool, pdfName, pdfRef,
// pdfNull, string) are returned unchanged.
func deepCopyValue(v pdfValue) pdfValue {
	switch val := v.(type) {
	case pdfDict:
		out := make(pdfDict, len(val))
		for k, vv := range val {
			out[k] = deepCopyValue(vv)
		}
		return out
	case pdfArray:
		out := make(pdfArray, len(val))
		for i, vv := range val {
			out[i] = deepCopyValue(vv)
		}
		return out
	case *pdfStream:
		nd := make(pdfDict, len(val.Dict))
		for k, dv := range val.Dict {
			nd[k] = deepCopyValue(dv)
		}
		data := append([]byte(nil), val.Data...)
		return &pdfStream{Dict: nd, Data: data, Decoded: val.Decoded}
	case pdfHexString:
		return append(pdfHexString(nil), val...)
	case pdfRef, pdfDirectRef, pdfName, pdfNull, int, float64, bool:
		// Value-typed kinds: returning by value is already a copy.
		return v
	}
	return v
}

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
