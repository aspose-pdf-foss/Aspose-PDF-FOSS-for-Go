# Phase 1: In-Memory Object Model Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the `rawDocument`+patches architecture with a full in-memory PDF object model where all operations read and write the same live data structures — no save/reload required to observe mutations.

**Architecture:** Parse entire PDF eagerly into `map[int]*pdfObject` on `Open`. All mutation operations (`Rotate`, `SetMetadata`, etc.) write directly into these in-memory structures. `Save`/`WriteTo` serializes the live tree to bytes.

**Tech Stack:** Pure Go, no external dependencies. Reuses `lexer.go`, `parser.go`, `xref.go` unchanged.

---

## File Map

| File | Change |
|------|--------|
| `types.go` | keep as-is (`pdfDirectRef` still needed by writer) |
| `doc.go` | **rewrite**: remove `rawDocument`; add `parseAllObjects`, `resolvePageTree`, `extractInfo`, `collectPageDeps`, `rewriteRefs` |
| `document.go` | **rewrite**: new `Document` struct; rewrite `OpenStream`, `Append`, `SetPassword`, `WriteTo`, `Save` |
| `document_pages.go` | **rewrite**: mutable `Rotate`, `SetRotation`, `Reorder`, `Split`, `Extract` |
| `page.go` | **update**: `Page` reads/writes from `doc.pages[index]` dict directly; all box methods read from object dict |
| `page_labels.go` | **update**: replace `*rawDocument` parameter with `*Document` |
| `metadata.go` | **update**: `Metadata()` from `doc.info`; `SetMetadata`/`ClearMetadata` no return value |
| `writer.go` | **rewrite**: serialize `doc.objects` + `doc.pages` directly |
| `encrypt.go` | **update**: `SetPassword` no return value |
| `validate.go` | **update**: inline `rawDocument` as private implementation detail (move from doc.go) |
| `*_test.go` | **update**: remove `doc = doc.Method()` assignments; update signatures |
| `lexer.go`, `parser.go`, `xref.go`, `io.go`, `page_range.go` | **unchanged** |

---

## Task 1: Move rawDocument into validate.go

`validate.go` is the only file that will keep using `rawDocument` after the refactor (it needs low-level xref/object-iteration access for its structural checks). Move it there before touching anything else.

**Files:**
- Modify: `validate.go`
- Modify: `doc.go`

- [ ] **Step 1: Copy rawDocument and all its methods from doc.go into validate.go**

Add to the bottom of `validate.go` (after the existing `openDocumentFromBytes` function):

```go
// rawDocument is a parsed PDF used internally by Validate.
// It is distinct from the public Document type.
type rawDocument struct {
	data      []byte
	xref      *xrefTable
	trailer   pdfDict
	cache     map[int]*pdfObject
	objStreams map[int][]*pdfObject
}

func (d *rawDocument) getObject(num int) (*pdfObject, error) {
	if obj, ok := d.cache[num]; ok {
		return obj, nil
	}
	entry, ok := d.xref.entries[num]
	if !ok {
		return nil, fmt.Errorf("object %d not in xref", num)
	}
	if entry.Free {
		return nil, fmt.Errorf("object %d is free", num)
	}
	var obj *pdfObject
	var err error
	if entry.Compressed {
		obj, err = d.getFromObjStream(entry.StreamObjNum, num)
	} else {
		obj, err = parseIndirectObject(d.data, entry.Offset)
	}
	if err != nil {
		return nil, err
	}
	d.cache[num] = obj
	return obj, nil
}

func (d *rawDocument) getFromObjStream(streamObjNum, targetNum int) (*pdfObject, error) {
	if objs, ok := d.objStreams[streamObjNum]; ok {
		for _, o := range objs {
			if o.Num == targetNum {
				return o, nil
			}
		}
		return nil, fmt.Errorf("object %d not found in stream %d", targetNum, streamObjNum)
	}
	streamObj, err := d.getObject(streamObjNum)
	if err != nil {
		return nil, fmt.Errorf("object stream %d: %w", streamObjNum, err)
	}
	s, ok := streamObj.Value.(*pdfStream)
	if !ok {
		return nil, fmt.Errorf("object %d is not a stream", streamObjNum)
	}
	n := dictGetInt(s.Dict, "/N")
	first := dictGetInt(s.Dict, "/First")
	headerData := s.Data[:first]
	hl := newLexer(headerData)
	type objOffset struct {
		num    int
		offset int
	}
	offsets := make([]objOffset, 0, n)
	for i := 0; i < n; i++ {
		t1, _ := hl.Next()
		t2, _ := hl.Next()
		if t1.kind != tokInt || t2.kind != tokInt {
			break
		}
		oNum := toIntBytes(t1.raw)
		oOff := toIntBytes(t2.raw)
		offsets = append(offsets, objOffset{num: oNum, offset: first + oOff})
	}
	objs := make([]*pdfObject, 0, len(offsets))
	for _, oo := range offsets {
		p := newParser(s.Data[oo.offset:])
		val, err := p.parseValue()
		if err != nil {
			continue
		}
		objs = append(objs, &pdfObject{Num: oo.num, Value: val})
	}
	d.objStreams[streamObjNum] = objs
	for _, o := range objs {
		if o.Num == targetNum {
			return o, nil
		}
	}
	return nil, fmt.Errorf("object %d not found in stream %d", targetNum, streamObjNum)
}

func (d *rawDocument) resolve(v pdfValue) (pdfValue, error) {
	ref, ok := v.(pdfRef)
	if !ok {
		return v, nil
	}
	obj, err := d.getObject(ref.Num)
	if err != nil {
		return nil, err
	}
	return obj.Value, nil
}

func (d *rawDocument) resolveDict(v pdfValue) (pdfDict, error) {
	rv, err := d.resolve(v)
	if err != nil {
		return nil, err
	}
	d2, ok := rv.(pdfDict)
	if !ok {
		return nil, fmt.Errorf("expected dict, got %T", rv)
	}
	return d2, nil
}

func (d *rawDocument) pages() ([]*pageInfoV, error) {
	rootRef, ok := d.trailer["/Root"]
	if !ok {
		return nil, fmt.Errorf("trailer missing /Root")
	}
	catalog, err := d.resolveDict(rootRef)
	if err != nil {
		return nil, fmt.Errorf("catalog: %w", err)
	}
	pagesRef, ok := catalog["/Pages"]
	if !ok {
		return nil, fmt.Errorf("catalog missing /Pages")
	}
	var result []*pageInfoV
	if err := d.walkPageTree(pagesRef, &result); err != nil {
		return nil, err
	}
	return result, nil
}

type pageInfoV struct {
	objNum int
	deps   map[int]bool
}

func (d *rawDocument) walkPageTree(nodeRef pdfValue, result *[]*pageInfoV) error {
	ref, ok := nodeRef.(pdfRef)
	if !ok {
		return fmt.Errorf("page tree node is not a ref")
	}
	nodeDict, err := d.resolveDict(nodeRef)
	if err != nil {
		return err
	}
	nodeType := dictGetName(nodeDict, "/Type")
	switch nodeType {
	case "/Pages":
		kids, ok := nodeDict["/Kids"]
		if !ok {
			return fmt.Errorf("Pages node missing /Kids")
		}
		arr, ok := kids.(pdfArray)
		if !ok {
			return fmt.Errorf("/Kids is not an array")
		}
		for _, kid := range arr {
			if err := d.walkPageTree(kid, result); err != nil {
				return err
			}
		}
	case "/Page", "":
		deps := make(map[int]bool)
		deps[ref.Num] = true
		d.collectValueDeps(nodeDict, deps)
		if err := d.collectInheritedDeps(nodeRef, deps); err != nil {
			return err
		}
		*result = append(*result, &pageInfoV{objNum: ref.Num, deps: deps})
	default:
		return fmt.Errorf("unknown page tree node type: %s", nodeType)
	}
	return nil
}

func (d *rawDocument) collectInheritedDeps(pageRef pdfValue, deps map[int]bool) error {
	nodeDict, err := d.resolveDict(pageRef)
	if err != nil {
		return err
	}
	parentRef, ok := nodeDict["/Parent"]
	if !ok {
		return nil
	}
	parentDict, err := d.resolveDict(parentRef)
	if err != nil {
		return err
	}
	if res, ok := parentDict["/Resources"]; ok {
		d.collectValueDeps(res, deps)
	}
	return d.collectInheritedDeps(parentRef, deps)
}

var reRefV = regexp.MustCompile(`\b(\d+)\s+\d+\s+R\b`)

func (d *rawDocument) collectDeps(objNum int, deps map[int]bool) error {
	if deps[objNum] {
		return nil
	}
	obj, err := d.getObject(objNum)
	if err != nil {
		return nil
	}
	if dict, ok := obj.Value.(pdfDict); ok {
		switch dictGetName(dict, "/Type") {
		case "/Pages", "/Catalog", "/Page":
			return nil
		}
	}
	deps[objNum] = true
	d.collectValueDeps(obj.Value, deps)
	return nil
}

func (d *rawDocument) collectValueDeps(v pdfValue, deps map[int]bool) {
	switch val := v.(type) {
	case pdfRef:
		d.collectDeps(val.Num, deps)
	case pdfDict:
		for _, dv := range val {
			d.collectValueDeps(dv, deps)
		}
	case pdfArray:
		for _, av := range val {
			d.collectValueDeps(av, deps)
		}
	case *pdfStream:
		for _, dv := range val.Dict {
			d.collectValueDeps(dv, deps)
		}
		refs := reRefV.FindAllSubmatch(val.Data, -1)
		for _, m := range refs {
			n := toIntBytes(m[1])
			if n > 0 {
				d.collectDeps(n, deps)
			}
		}
	}
}
```

- [ ] **Step 2: Update openDocumentFromBytes in validate.go to return the local rawDocument**

The existing `openDocumentFromBytes` at line ~264 of `validate.go` already returns `*rawDocument`. Since `rawDocument` will now be defined in `validate.go` instead of `doc.go`, this is unchanged — no code edit needed for this function.

- [ ] **Step 3: Delete rawDocument and all its methods from doc.go**

Remove from `doc.go`:
- The `rawDocument` struct definition and all its methods (`getObject`, `getFromObjStream`, `resolve`, `resolveDict`, `pages`, `walkPageTree`, `collectInheritedDeps`, `collectDeps`, `collectValueDeps`)
- The `pageInfo` struct
- The `reRef` variable
- The `openDocument` function
- The `openDocumentFromBytes` function (it now lives in validate.go)

Keep in `doc.go` only: package declaration, imports, `dictGetInt`, `dictGetName`, `toInt`, `toIntBytes`, `toFloat`, `dictGet` helpers (anything used by both old and new code).

- [ ] **Step 4: Verify build passes**

```bash
go build ./...
```

Expected: no errors.

- [ ] **Step 5: Run tests**

```bash
go test ./...
```

Expected: all tests pass (behaviour unchanged).

- [ ] **Step 6: Commit**

```bash
git add doc.go validate.go
git commit -m "Move rawDocument to validate.go; isolate from public Document"
```

---

## Task 2: New parser functions in doc.go

Replace `doc.go` content with functions that eagerly parse all objects from raw bytes into memory.

**Files:**
- Rewrite: `doc.go`

- [ ] **Step 1: Write new doc.go**

Replace the entire content of `doc.go` with:

```go
package asposepdf

import (
	"fmt"
	"regexp"
)

// parseAllObjects iterates every non-free entry in xref and parses the
// corresponding object, decoding streams eagerly. Returns a map of objNum → *pdfObject.
// trailer is required to initialise the rawDocument used for object-stream parsing.
func parseAllObjects(data []byte, xref *xrefTable, trailer pdfDict) (map[int]*pdfObject, error) {
	// Re-use the rawDocument parsing logic (now private to validate.go) to handle
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
	case "/Page", "":
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

// collectPageDeps returns a map of all object IDs needed to render page,
// including the page object itself. Skips /Pages, /Catalog, and /Page nodes
// reached transitively (e.g. via link annotations).
func collectPageDeps(objects map[int]*pdfObject, page *pdfObject) map[int]*pdfObject {
	deps := make(map[int]*pdfObject)
	visited := make(map[int]bool)
	// Add the page itself directly (skip the type guard below).
	deps[page.Num] = page
	visited[page.Num] = true
	// Collect its value-level dependencies.
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
	// Skip page-tree structural nodes.
	if d, ok := obj.Value.(pdfDict); ok {
		switch dictGetName(d, "/Type") {
		case "/Pages", "/Catalog", "/Page":
			return
		}
	}
	visited[num] = true
	deps[num] = obj
	collectValueDeps(objects, obj.Value, deps, visited)
}

func collectValueDeps(objects map[int]*pdfObject, v pdfValue, deps map[int]*pdfObject, visited map[int]bool) {
	switch val := v.(type) {
	case pdfRef:
		collectObjDeps(objects, val.Num, deps, visited)
	case pdfDict:
		collectDictDeps(objects, val, deps, visited)
	case pdfArray:
		for _, av := range val {
			collectValueDeps(objects, av, deps, visited)
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
		collectValueDeps(objects, dv, deps, visited)
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
```

Note: `rawDocument` is now referenced in `doc.go` only inside `parseAllObjects` (to reuse the existing object-stream parsing logic from `validate.go`). Both files are in the same package, so this compiles.

- [ ] **Step 2: Verify build passes**

```bash
go build ./...
```

Expected: no errors (doc.go has new functions; old callers in document.go still reference old rawDocument which is now in validate.go).

- [ ] **Step 3: Commit**

```bash
git add doc.go
git commit -m "doc.go: add parseAllObjects, resolvePageTree, extractInfo, collectPageDeps, rewriteRefs"
```

---

## Task 3: New Document struct + OpenStream

Replace the `Document` struct and the open pipeline. This task touches the most files and is the largest single change.

**Files:**
- Rewrite: `document.go`
- Update: `document_pages.go` (stubs — full implementation in Task 6)
- Update: `page.go` (Page struct change)
- Update: `metadata.go` (remove metadataConfig)
- Update: `writer.go` (update signature only — full rewrite in Task 4)
- Update: `encrypt.go` (SetPassword no return)

- [ ] **Step 1: Rewrite document.go**

```go
package asposepdf

import (
	"bytes"
	"fmt"
	"io"
	"os"
)

// Document is a PDF document. Operations directly mutate the receiver.
type Document struct {
	objects map[int]*pdfObject // all PDF objects by ID
	pages   []*pdfObject       // ordered /Page objects
	catalog pdfDict            // /Catalog dict
	info    pdfDict            // /Info dict; nil = no metadata
	encrypt *encryptConfig     // nil = no encryption
	nextID  int                // next available object ID
}

// Open opens a PDF file and returns a Document.
func Open(path string) (*Document, error) {
	data, err := readFile(path)
	if err != nil {
		return nil, fmt.Errorf("open PDF: %w", err)
	}
	return OpenStream(bytes.NewReader(data))
}

// OpenStream reads a PDF from r and returns a Document.
func OpenStream(r io.Reader) (*Document, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("read PDF: %w", err)
	}

	startOff, err := findStartXRef(data)
	if err != nil {
		return nil, fmt.Errorf("parse PDF: %w", err)
	}
	xref, trailer, err := parseXRef(data, startOff)
	if err != nil {
		return nil, fmt.Errorf("parse PDF: %w", err)
	}
	if _, ok := trailer["/Encrypt"]; ok {
		return nil, fmt.Errorf("parse PDF: encrypted PDF is not supported")
	}

	objects, err := parseAllObjects(data, xref, trailer)
	if err != nil {
		return nil, fmt.Errorf("parse PDF: %w", err)
	}

	catalog, err := extractCatalog(objects, trailer)
	if err != nil {
		return nil, fmt.Errorf("parse PDF: %w", err)
	}

	pages, err := resolvePageTree(objects, catalog)
	if err != nil {
		return nil, fmt.Errorf("read pages: %w", err)
	}

	return &Document{
		objects: objects,
		pages:   pages,
		catalog: catalog,
		info:    extractInfo(objects, trailer),
		nextID:  maxObjectID(objects) + 1,
	}, nil
}

// PageCount returns the number of pages in the document.
func (d *Document) PageCount() int {
	return len(d.pages)
}

// Pages returns a live view of all pages in the document.
func (d *Document) Pages() []*Page {
	pages := make([]*Page, len(d.pages))
	for i := range d.pages {
		pages[i] = &Page{doc: d, index: i}
	}
	return pages
}

// Page returns a live view of the page at the given 1-based number.
func (d *Document) Page(n int) (*Page, error) {
	if n < 1 || n > len(d.pages) {
		return nil, fmt.Errorf("page number %d out of range (1..%d)", n, len(d.pages))
	}
	return &Page{doc: d, index: n - 1}, nil
}

// Append adds all pages from others to this document, merging their objects.
// Nil arguments are silently skipped.
func (d *Document) Append(others ...*Document) {
	for _, other := range others {
		if other == nil {
			continue
		}
		// Build ID mapping: other's object IDs → new IDs in d.
		idMap := make(map[int]int, len(other.objects))
		for oldID := range other.objects {
			idMap[oldID] = d.nextID
			d.nextID++
		}
		// Copy objects with rewritten refs.
		for oldID, obj := range other.objects {
			newID := idMap[oldID]
			d.objects[newID] = &pdfObject{
				Num:   newID,
				Gen:   obj.Gen,
				Value: rewriteRefs(obj.Value, idMap),
			}
		}
		// Add pages (using new IDs).
		for _, page := range other.pages {
			d.pages = append(d.pages, d.objects[idMap[page.Num]])
		}
	}
}

// SetPassword configures the document to be encrypted when saved.
// userPassword is required to open; ownerPassword controls permissions.
// If ownerPassword is empty, it defaults to userPassword.
func (d *Document) SetPassword(userPassword, ownerPassword string) {
	d.encrypt = &encryptConfig{
		userPassword:  userPassword,
		ownerPassword: ownerPassword,
	}
}

// WriteTo writes the document to w. It implements io.WriterTo.
func (d *Document) WriteTo(w io.Writer) (int64, error) {
	if len(d.pages) == 0 {
		return 0, fmt.Errorf("document has no pages")
	}
	data, err := buildDocumentPDF(d)
	if err != nil {
		return 0, err
	}
	n, err := w.Write(data)
	return int64(n), err
}

// Save writes the document to outputPath.
func (d *Document) Save(outputPath string) error {
	f, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = d.WriteTo(f)
	return err
}

// validateRange validates from/to against [1, total].
func validateRange(from, to, total int) (int, int, error) {
	if from < 1 || from > total {
		return 0, 0, fmt.Errorf("page range from=%d out of bounds (1..%d)", from, total)
	}
	if to < 1 || to > total {
		return 0, 0, fmt.Errorf("page range to=%d out of bounds (1..%d)", to, total)
	}
	if from > to {
		return 0, 0, fmt.Errorf("invalid page range: from=%d > to=%d", from, to)
	}
	return from, to, nil
}

// resolvePageIndices converts 1-based page numbers to 0-based indices.
// If pageNums is empty, returns all indices.
func resolvePageIndices(total int, pageNums []int) ([]int, error) {
	if len(pageNums) == 0 {
		indices := make([]int, total)
		for i := range indices {
			indices[i] = i
		}
		return indices, nil
	}
	seen := make(map[int]bool, len(pageNums))
	indices := make([]int, 0, len(pageNums))
	for _, n := range pageNums {
		if n < 1 || n > total {
			return nil, fmt.Errorf("page number %d out of range (1..%d)", n, total)
		}
		if !seen[n] {
			seen[n] = true
			indices = append(indices, n-1)
		}
	}
	return indices, nil
}
```

- [ ] **Step 2: Update Page struct in page.go**

Replace the `Page` struct and its `Number()`, `Size()`, `Rotation()` methods (leave box methods and PageSizes for Task 5):

```go
// Page is a live view of a single page within a Document.
type Page struct {
	doc   *Document
	index int // 0-based index in doc.pages
}

// Number returns the 1-based page number within the document.
func (p *Page) Number() int {
	return p.index + 1
}

// pageObj returns the underlying pdfObject for this page.
func (p *Page) pageObj() *pdfObject {
	return p.doc.pages[p.index]
}

// pageDict returns the page's dictionary.
func (p *Page) pageDict() pdfDict {
	if d, ok := p.pageObj().Value.(pdfDict); ok {
		return d
	}
	return nil
}
```

The existing `Size()`, `Rotation()`, `CropBox()` etc. methods will be updated in Task 5. For now they will be compilation errors — fix them by temporarily stubbing:

```go
// Size returns the page dimensions from its MediaBox.
func (p *Page) Size() (PageSize, error) {
	return mediaBoxSize(p.doc.objects, p.pageObj().Num)
}

// Rotation returns the effective rotation of the page in degrees.
func (p *Page) Rotation() RotationAngle {
	d := p.pageDict()
	if d == nil {
		return Rotate0
	}
	return RotationAngle(dictGetInt(d, "/Rotate"))
}
```

Also update `Pages()` and `Page()` — these are now in `document.go` (already done in Step 1 above). Remove them from `page.go`.

Also remove `PageSizes` function temporarily (add it back in Task 5):

```go
// PageSizes returns the dimensions of every page in the given PDF file.
func PageSizes(inputPath string) ([]PageSize, error) {
	doc, err := Open(inputPath)
	if err != nil {
		return nil, err
	}
	sizes := make([]PageSize, len(doc.pages))
	for i, pg := range doc.pages {
		sz, err := mediaBoxSize(doc.objects, pg.Num)
		if err != nil {
			return nil, fmt.Errorf("page %d: %w", i+1, err)
		}
		sizes[i] = sz
	}
	return sizes, nil
}
```

- [ ] **Step 3: Update metadata.go — remove metadataConfig, use doc.info**

Replace `SetMetadata`, `ClearMetadata`, and `Metadata()`:

```go
// Metadata returns the Info metadata from this document.
func (d *Document) Metadata() (Metadata, error) {
	if d.info == nil {
		return Metadata{}, nil
	}
	return readMetadataFromDict(d.info), nil
}

// readMetadataFromDict extracts a Metadata value from a pdfDict.
func readMetadataFromDict(infoDict pdfDict) Metadata {
	standardKeys := map[string]bool{
		"/Title": true, "/Author": true, "/Subject": true, "/Keywords": true,
		"/Creator": true, "/Producer": true, "/CreationDate": true, "/ModDate": true,
	}
	var custom map[string]string
	for k, v := range infoDict {
		if standardKeys[k] {
			continue
		}
		if s, ok := v.(string); ok && s != "" {
			if custom == nil {
				custom = make(map[string]string)
			}
			custom[strings.TrimPrefix(k, "/")] = s
		}
	}
	return Metadata{
		Title:        infoString(infoDict, "/Title"),
		Author:       infoString(infoDict, "/Author"),
		Subject:      infoString(infoDict, "/Subject"),
		Keywords:     infoString(infoDict, "/Keywords"),
		Creator:      infoString(infoDict, "/Creator"),
		Producer:     infoString(infoDict, "/Producer"),
		CreationDate: infoString(infoDict, "/CreationDate"),
		ModDate:      infoString(infoDict, "/ModDate"),
		Custom:       custom,
	}
}

// SetMetadata replaces the document's Info dictionary with the given metadata.
// Empty string fields are omitted. This is a full replacement.
func (d *Document) SetMetadata(meta Metadata) {
	d.info = buildInfoDict(meta)
}

// ClearMetadata removes the Info dictionary entirely.
func (d *Document) ClearMetadata() {
	d.info = nil
}
```

Remove `readMetadata(doc *rawDocument)` and `metadataConfig` type.

- [ ] **Step 4: Update encrypt.go — SetPassword no return**

The `SetPassword` method is now in `document.go`. Remove it from `encrypt.go` if it exists there. The `encryptConfig` struct stays in `encrypt.go` unchanged.

- [ ] **Step 5: Stub buildDocumentPDF in writer.go**

The `WriteTo` in `document.go` calls `buildDocumentPDF(d *Document)`. Add a temporary stub to `writer.go` so it compiles:

```go
// buildDocumentPDF serializes the document to PDF bytes.
// Full implementation in Task 4.
func buildDocumentPDF(d *Document) ([]byte, error) {
	return nil, fmt.Errorf("buildDocumentPDF: not yet implemented")
}
```

Keep all existing writer functions intact for now — they will be replaced in Task 4.

- [ ] **Step 6: Fix document_pages.go compilation**

The methods in `document_pages.go` (`Rotate`, `SetRotation`, `Reorder`, `Split`, `Extract`) reference `pageRef`, `patchKey`, `withCopiedPatches`, etc. which no longer exist. Replace the file with stubs that compile:

```go
package asposepdf

import "fmt"

// Rotate rotates selected pages clockwise by angle. Full implementation in Task 6.
func (d *Document) Rotate(angle RotationAngle, pageNums ...int) error {
	if err := angle.validate(); err != nil {
		return err
	}
	indices, err := resolvePageIndices(len(d.pages), pageNums)
	if err != nil {
		return err
	}
	for _, i := range indices {
		pg := &Page{doc: d, index: i}
		dict := pg.pageDict()
		if dict == nil {
			continue
		}
		current := pg.Rotation()
		dict["/Rotate"] = (int(current) + int(angle)) % 360
	}
	return nil
}

// SetRotation sets selected pages to exactly angle. Full implementation in Task 6.
func (d *Document) SetRotation(angle RotationAngle, pageNums ...int) error {
	if err := angle.validate(); err != nil {
		return err
	}
	indices, err := resolvePageIndices(len(d.pages), pageNums)
	if err != nil {
		return err
	}
	for _, i := range indices {
		pg := &Page{doc: d, index: i}
		dict := pg.pageDict()
		if dict != nil {
			dict["/Rotate"] = int(angle)
		}
	}
	return nil
}

// Reorder rearranges pages according to order (1-based). Pages may be repeated or omitted.
func (d *Document) Reorder(order []int) error {
	newPages := make([]*pdfObject, len(order))
	for i, n := range order {
		if n < 1 || n > len(d.pages) {
			return fmt.Errorf("page number %d out of range (1..%d)", n, len(d.pages))
		}
		newPages[i] = d.pages[n-1]
	}
	d.pages = newPages
	return nil
}

// Split returns each page as a separate Document.
func (d *Document) Split() ([]*Document, error) {
	if len(d.pages) == 0 {
		return nil, fmt.Errorf("document has no pages")
	}
	result := make([]*Document, len(d.pages))
	for i, page := range d.pages {
		deps := collectPageDeps(d.objects, page)
		result[i] = &Document{
			objects: deps,
			pages:   []*pdfObject{page},
			nextID:  maxObjectID(deps) + 1,
		}
	}
	return result, nil
}

// Extract returns a new Document with only the pages in the specified ranges.
func (d *Document) Extract(ranges ...PageRange) (*Document, error) {
	if len(ranges) == 0 {
		return nil, fmt.Errorf("no page ranges specified")
	}
	var selected []*pdfObject
	for _, r := range ranges {
		from, to, err := validateRange(r.From, r.To, len(d.pages))
		if err != nil {
			return nil, err
		}
		selected = append(selected, d.pages[from-1:to]...)
	}
	// Collect deps for all selected pages.
	merged := make(map[int]*pdfObject)
	for _, page := range selected {
		for id, obj := range collectPageDeps(d.objects, page) {
			merged[id] = obj
		}
	}
	return &Document{
		objects: merged,
		pages:   selected,
		nextID:  maxObjectID(merged) + 1,
	}, nil
}
```

- [ ] **Step 7: Update page_labels.go**

`computePageLabel` currently takes `*rawDocument`. Change its signature to use `*Document` and update its body to use `d.catalog` and `d.objects`:

```go
func (p *Page) Label() string {
	label, err := computePageLabel(p.doc, p.index)
	if err != nil {
		return fmt.Sprintf("%d", p.index+1)
	}
	return label
}

func computePageLabel(doc *Document, pageIndex int) (string, error) {
	labelsVal, ok := doc.catalog["/PageLabels"]
	if !ok {
		return fmt.Sprintf("%d", pageIndex+1), nil
	}
	pairs, err := flattenNumberTree(doc.objects, labelsVal)
	if err != nil || len(pairs) == 0 {
		return fmt.Sprintf("%d", pageIndex+1), nil
	}
	rangeStart := 0
	var labelDict pdfDict
	for _, pair := range pairs {
		if pair.key <= pageIndex {
			rangeStart = pair.key
			labelDict = pair.dict
		}
	}
	return formatPageLabel(labelDict, pageIndex-rangeStart), nil
}

func flattenNumberTree(objects map[int]*pdfObject, nodeVal pdfValue) ([]numberTreeEntry, error) {
	node, ok := resolveRefToDict(objects, nodeVal)
	if !ok {
		return nil, fmt.Errorf("number tree node is not a dict")
	}
	if numsVal, ok := node["/Nums"]; ok {
		arr, ok := resolveRefToArray(objects, numsVal)
		if !ok {
			return nil, fmt.Errorf("/Nums is not an array")
		}
		var entries []numberTreeEntry
		for i := 0; i+1 < len(arr); i += 2 {
			key := toInt(arr[i])
			d, ok := resolveRefToDict(objects, arr[i+1])
			if !ok {
				continue
			}
			entries = append(entries, numberTreeEntry{key: key, dict: d})
		}
		return entries, nil
	}
	if kidsVal, ok := node["/Kids"]; ok {
		arr, ok := resolveRefToArray(objects, kidsVal)
		if !ok {
			return nil, fmt.Errorf("/Kids is not an array")
		}
		var entries []numberTreeEntry
		for _, kid := range arr {
			sub, err := flattenNumberTree(objects, kid)
			if err != nil {
				continue
			}
			entries = append(entries, sub...)
		}
		return entries, nil
	}
	return nil, nil
}
```

- [ ] **Step 8: Update page box methods in page.go**

Replace `CropBox`, `TrimBox`, `BleedBox`, `ArtBox`, `pageBoxWithFallback`, `mediaBoxSize` to use `doc.objects`:

```go
func (p *Page) CropBox() (PageSize, error) {
	return pageBoxWithFallback(p.doc.objects, p.pageObj().Num, "/CropBox")
}

func (p *Page) TrimBox() (PageSize, error) {
	return pageBoxWithFallback(p.doc.objects, p.pageObj().Num, "/TrimBox", "/CropBox")
}

func (p *Page) BleedBox() (PageSize, error) {
	return pageBoxWithFallback(p.doc.objects, p.pageObj().Num, "/BleedBox", "/CropBox")
}

func (p *Page) ArtBox() (PageSize, error) {
	return pageBoxWithFallback(p.doc.objects, p.pageObj().Num, "/ArtBox", "/CropBox")
}

func pageBoxWithFallback(objects map[int]*pdfObject, objNum int, boxes ...string) (PageSize, error) {
	obj, ok := objects[objNum]
	if !ok {
		return PageSize{}, fmt.Errorf("object %d not found", objNum)
	}
	d, ok := obj.Value.(pdfDict)
	if !ok {
		return PageSize{}, fmt.Errorf("object %d is not a dict", objNum)
	}
	for _, name := range boxes {
		if v, ok := d[name]; ok {
			arr, ok := resolveRefToArray(objects, v)
			if !ok {
				continue
			}
			return mediaBoxFromArray(arr)
		}
	}
	return mediaBoxSize(objects, objNum)
}

func mediaBoxSize(objects map[int]*pdfObject, objNum int) (PageSize, error) {
	visited := make(map[int]bool)
	for {
		if visited[objNum] {
			return PageSize{}, fmt.Errorf("cycle in page tree at object %d", objNum)
		}
		visited[objNum] = true
		obj, ok := objects[objNum]
		if !ok {
			return PageSize{}, fmt.Errorf("object %d not found", objNum)
		}
		d, ok := obj.Value.(pdfDict)
		if !ok {
			return PageSize{}, fmt.Errorf("object %d is not a dict", objNum)
		}
		if mb, ok := d["/MediaBox"]; ok {
			arr, ok2 := resolveRefToArray(objects, mb)
			if !ok2 {
				return PageSize{}, fmt.Errorf("invalid /MediaBox")
			}
			return mediaBoxFromArray(arr)
		}
		parentVal, ok := d["/Parent"]
		if !ok {
			return PageSize{}, fmt.Errorf("no /MediaBox found for object %d", objNum)
		}
		parentRef, ok := parentVal.(pdfRef)
		if !ok {
			return PageSize{}, fmt.Errorf("unexpected /Parent type %T", parentVal)
		}
		objNum = parentRef.Num
	}
}
```

Remove the old `resolveToArray` function (replaced by `resolveRefToArray` from doc.go).

- [ ] **Step 9: Verify build passes**

```bash
go build ./...
```

Expected: no errors.

- [ ] **Step 10: Run tests (some will fail — WriteTo is stubbed)**

```bash
go test ./... 2>&1 | head -40
```

Expected: tests that don't call Save/WriteTo pass; tests that write PDFs fail with "not yet implemented".

- [ ] **Step 11: Commit**

```bash
git add document.go document_pages.go page.go page_labels.go metadata.go encrypt.go writer.go
git commit -m "New Document struct: in-memory object model, mutable API"
```

---

## Task 4: New writer

Replace `buildDocumentPDF` with a serializer that walks `doc.objects` directly.

**Files:**
- Rewrite: `writer.go`

- [ ] **Step 1: Rewrite writer.go**

```go
package asposepdf

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// buildDocumentPDF serializes d to a PDF byte slice.
func buildDocumentPDF(d *Document) ([]byte, error) {
	var encState *encryptState
	if d.encrypt != nil {
		var err error
		encState, err = newEncryptState(d.encrypt)
		if err != nil {
			return nil, fmt.Errorf("encrypt: %w", err)
		}
	}

	// Assign sequential output IDs to all content objects.
	// Reserve IDs for structural objects built by the writer.
	contentIDs := sortedObjectIDs(d.objects)
	remap := make(map[int]int, len(contentIDs))
	nextOut := 1
	for _, id := range contentIDs {
		remap[id] = nextOut
		nextOut++
	}
	pagesObjID := nextOut; nextOut++
	catalogObjID := nextOut; nextOut++
	var infoObjID int
	if d.info != nil {
		infoObjID = nextOut; nextOut++
	}
	var encryptObjID int
	if encState != nil {
		encryptObjID = nextOut; nextOut++
	}
	totalObjects := nextOut // exclusive upper bound

	remapFn := func(n int) int {
		if out, ok := remap[n]; ok {
			return out
		}
		return n
	}

	var buf bytes.Buffer
	buf.WriteString("%PDF-1.4\n")
	buf.WriteString("%\xe2\xe3\xcf\xd3\n") // binary marker

	offsets := make(map[int]int64, totalObjects)

	// Write content objects.
	for _, oldID := range contentIDs {
		obj := d.objects[oldID]
		outID := remap[oldID]
		offsets[outID] = int64(buf.Len())
		var encFn func([]byte) []byte
		if encState != nil {
			encFn = func(b []byte) []byte { return encState.encryptBytes(outID, b) }
		}
		writeObject(&buf, outID, obj.Value, remapFn, encFn)
	}

	// Write /Pages node.
	offsets[pagesObjID] = int64(buf.Len())
	writePageTreeNode(&buf, pagesObjID, catalogObjID, d.pages, remapFn)

	// Write /Catalog.
	offsets[catalogObjID] = int64(buf.Len())
	fmt.Fprintf(&buf, "%d 0 obj\n<<\n/Type /Catalog\n/Pages %d 0 R\n>>\nendobj\n",
		catalogObjID, pagesObjID)

	// Write /Info if present.
	if infoObjID != 0 {
		offsets[infoObjID] = int64(buf.Len())
		var encFn func([]byte) []byte
		if encState != nil {
			encFn = func(b []byte) []byte { return encState.encryptBytes(infoObjID, b) }
		}
		writeObject(&buf, infoObjID, pdfValue(d.info), remapFn, encFn)
	}

	// Write /Encrypt if present.
	if encryptObjID != 0 {
		offsets[encryptObjID] = int64(buf.Len())
		encDict := buildEncryptDict(encState)
		writeObject(&buf, encryptObjID, pdfValue(encDict), func(n int) int { return n }, nil)
	}

	// Write xref table.
	xrefOffset := int64(buf.Len())
	fmt.Fprintf(&buf, "xref\n0 %d\n", totalObjects)
	fmt.Fprintf(&buf, "0000000000 65535 f \n")
	for i := 1; i < totalObjects; i++ {
		off, ok := offsets[i]
		if !ok {
			fmt.Fprintf(&buf, "0000000000 00000 f \n")
		} else {
			fmt.Fprintf(&buf, "%010d 00000 n \n", off)
		}
	}

	// Write trailer.
	buf.WriteString("trailer\n<<\n")
	fmt.Fprintf(&buf, "/Size %d\n", totalObjects)
	fmt.Fprintf(&buf, "/Root %d 0 R\n", catalogObjID)
	if infoObjID != 0 {
		fmt.Fprintf(&buf, "/Info %d 0 R\n", infoObjID)
	}
	if encState != nil {
		fmt.Fprintf(&buf, "/Encrypt %d 0 R\n", encryptObjID)
		buf.WriteString("/ID [")
		writeHexBytes(&buf, encState.fileID)
		buf.WriteString(" ")
		writeHexBytes(&buf, encState.fileID)
		buf.WriteString("]\n")
	}
	buf.WriteString(">>\n")
	fmt.Fprintf(&buf, "startxref\n%d\n%%%%EOF\n", xrefOffset)

	return buf.Bytes(), nil
}

// writePageTreeNode writes the /Pages node with kids pointing to d.pages.
// Each page dict gets a /Parent patch pointing to pagesObjID.
func writePageTreeNode(buf *bytes.Buffer, pagesObjID, catalogObjID int, pages []*pdfObject, remapFn func(int) int) {
	fmt.Fprintf(buf, "%d 0 obj\n<<\n/Type /Pages\n/Count %d\n/Kids [", pagesObjID, len(pages))
	for i, page := range pages {
		if i > 0 {
			buf.WriteString(" ")
		}
		fmt.Fprintf(buf, "%d 0 R", remapFn(page.Num))
	}
	buf.WriteString("]\n>>\nendobj\n")
}

// writeObject writes "N 0 obj\n...\nendobj\n" for the given value.
func writeObject(buf *bytes.Buffer, id int, v pdfValue, remapFn func(int) int, encFn func([]byte) []byte) {
	fmt.Fprintf(buf, "%d 0 obj\n", id)
	// For page dicts, patch /Parent to point to the new /Pages node.
	// The caller must set /Parent before calling writeObject, or handle it here.
	writeValue(buf, v, remapFn, encFn)
	buf.WriteString("\nendobj\n")
}

// sortedObjectIDs returns the object IDs from objects in ascending order.
func sortedObjectIDs(objects map[int]*pdfObject) []int {
	ids := make([]int, 0, len(objects))
	for id := range objects {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	return ids
}

// writeValue serialises a PDF value to buf, remapping object reference numbers via remapFn.
func writeValue(buf *bytes.Buffer, v pdfValue, remapFn func(int) int, encFn func([]byte) []byte) {
	switch val := v.(type) {
	case pdfNull:
		buf.WriteString("null")
	case bool:
		if val {
			buf.WriteString("true")
		} else {
			buf.WriteString("false")
		}
	case int:
		buf.WriteString(strconv.Itoa(val))
	case float64:
		buf.WriteString(strconv.FormatFloat(val, 'g', -1, 64))
	case pdfName:
		buf.WriteString(string(val))
	case string:
		if encFn != nil {
			writeHexBytes(buf, encFn([]byte(val)))
		} else {
			buf.WriteString("(")
			buf.WriteString(escapeLiteral(val))
			buf.WriteString(")")
		}
	case pdfRef:
		buf.WriteString(strconv.Itoa(remapFn(val.Num)))
		buf.WriteByte(' ')
		buf.WriteString(strconv.Itoa(val.Gen))
		buf.WriteString(" R")
	case pdfDirectRef:
		buf.WriteString(strconv.Itoa(val.Num))
		buf.WriteByte(' ')
		buf.WriteString(strconv.Itoa(val.Gen))
		buf.WriteString(" R")
	case pdfDict:
		buf.WriteString("<<")
		keys := make([]string, 0, len(val))
		for k := range val {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			buf.WriteString("\n")
			buf.WriteString(k)
			buf.WriteString(" ")
			writeValue(buf, val[k], remapFn, encFn)
		}
		buf.WriteString("\n>>")
	case pdfArray:
		buf.WriteString("[")
		for i, item := range val {
			if i > 0 {
				buf.WriteString(" ")
			}
			writeValue(buf, item, remapFn, encFn)
		}
		buf.WriteString("]")
	case *pdfStream:
		d := make(pdfDict, len(val.Dict))
		for k, dv := range val.Dict {
			if val.Decoded && (k == "/Filter" || k == "/DecodeParms" || k == "/FFilter" || k == "/FDecodeParms") {
				continue
			}
			d[k] = dv
		}
		data := val.Data
		if val.Decoded {
			var zbuf bytes.Buffer
			w := zlib.NewWriter(&zbuf)
			w.Write(data)
			w.Close()
			data = zbuf.Bytes()
			d["/Filter"] = pdfName("/FlateDecode")
		}
		if encFn != nil {
			data = encFn(data)
		}
		d["/Length"] = len(data)
		writeValue(buf, d, remapFn, nil) // dict keys are not encrypted
		buf.WriteString("\nstream\n")
		buf.Write(data)
		buf.WriteString("\nendstream")
	}
}

// writeHexBytes writes b as a PDF hex string <AABB...>.
func writeHexBytes(buf *bytes.Buffer, b []byte) {
	const hex = "0123456789abcdef"
	buf.WriteByte('<')
	for _, c := range b {
		buf.WriteByte(hex[c>>4])
		buf.WriteByte(hex[c&0xf])
	}
	buf.WriteByte('>')
}

// escapeLiteral escapes special characters in a PDF literal string.
func escapeLiteral(s string) string {
	var b strings.Builder
	for _, c := range []byte(s) {
		switch c {
		case '(', ')', '\\':
			b.WriteByte('\\')
			b.WriteByte(c)
		case '\r':
			b.WriteString(`\r`)
		case '\n':
			b.WriteString(`\n`)
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}

// buildEncryptDict builds the /Encrypt dictionary for standard RC4-128 security.
func buildEncryptDict(s *encryptState) pdfDict {
	return pdfDict{
		"/Filter":   pdfName("/Standard"),
		"/V":        2,
		"/R":        3,
		"/Length":   128,
		"/P":        int(encryptPermissions),
		"/O":        string(s.ownerEntry),
		"/U":        string(s.userEntry),
		"/StmF":     pdfName("/StdCF"),
		"/StrF":     pdfName("/StdCF"),
		"/CF": pdfDict{
			"/StdCF": pdfDict{
				"/AuthEvent": pdfName("/DocOpen"),
				"/CFM":       pdfName("/RC4"),
				"/Length":    16,
			},
		},
	}
}
```

**Important:** The writer needs to patch each page dict's `/Parent` to point to `pagesObjID`. Update `writePageTreeNode` and add the patch before writing page objects:

Actually, the cleanest approach is to patch `/Parent` in the page dicts in memory before serializing. Add this to `buildDocumentPDF` after computing `pagesObjID`:

```go
// Patch /Parent in every page dict to point to the new /Pages node.
// Use pdfDirectRef so the writer outputs the ID as-is without remapping.
for _, page := range d.pages {
	if dict, ok := page.Value.(pdfDict); ok {
		dict["/Parent"] = pdfDirectRef{Num: pagesObjID}
	}
}
```

This mutates the page dicts in place. That is intentional — each `Save`/`WriteTo` call refreshes these refs.

- [ ] **Step 2: Verify build passes**

```bash
go build ./...
```

Expected: no errors.

- [ ] **Step 3: Run tests**

```bash
go test ./...
```

Expected: most tests pass. Tests checking saved PDF structure or round-trip metadata will now pass.

- [ ] **Step 4: Commit**

```bash
git add writer.go
git commit -m "writer: serialize live object tree; remove rawDocument dependency"
```

---

## Task 5: Port page box methods + validate.go cleanup

Update `page.go` box methods to use the new `resolveRefToArray`/`resolveRefToDict` functions and verify `validate.go` still compiles standalone.

**Files:**
- Verify: `page.go` (should already be correct after Task 3)
- Verify: `validate.go`

- [ ] **Step 1: Remove stale helpers from page.go**

In Task 3 we replaced `resolveToArray` with `resolveRefToArray` from `doc.go`. Make sure no reference to the old `resolveToArray` remains in `page.go`. Search and confirm:

```bash
grep -n "resolveToArray" page.go
```

Expected: no matches.

- [ ] **Step 2: Run full test suite**

```bash
go test -v ./... 2>&1 | grep -E "^(ok|FAIL|---)"
```

Expected: all pass.

- [ ] **Step 3: Commit if any changes were needed**

```bash
git add page.go validate.go
git commit -m "port page boxes and labels to use in-memory object model"
```

---

## Task 6: Full test coverage for new API

Run the existing tests and fix failures due to changed signatures (`doc = doc.Method()` → `doc.Method()`).

**Files:**
- Update: `document_test.go`
- Update: `append_test.go`
- Update: `splitter_test.go`
- Update: `metadata_test.go`
- Update: `encrypt_test.go`
- Update: `page_test.go`
- Update: `page_labels_test.go`

- [ ] **Step 1: Run tests to see all failures**

```bash
go test ./... 2>&1
```

Collect the list of compilation errors and test failures.

- [ ] **Step 2: Fix document_test.go**

Pattern: change `doc, err = doc.Rotate(...)` → `err = doc.Rotate(...)`; change `doc = doc.SetPassword(...)` → `doc.SetPassword(...)`. Example:

```go
// Before:
doc, err = doc.Rotate(asposepdf.Rotate90)
// After:
err = doc.Rotate(asposepdf.Rotate90)

// Before:
doc = doc.SetPassword("pass", "")
// After:
doc.SetPassword("pass", "")

// Before:
doc, err = doc.SetRotation(asposepdf.Rotate0, 1)
// After:
err = doc.SetRotation(asposepdf.Rotate0, 1)
```

- [ ] **Step 3: Fix metadata_test.go**

```go
// Before:
doc = doc.SetMetadata(asposepdf.Metadata{Author: "test"})
// After:
doc.SetMetadata(asposepdf.Metadata{Author: "test"})

// Before:
doc = doc.ClearMetadata()
// After:
doc.ClearMetadata()

// TestSetMetadataRoundTrip: remove save/reload round-trip — Metadata() now reads live:
doc.SetMetadata(asposepdf.Metadata{Author: "test"})
meta, err := doc.Metadata()
// meta.Author is already "test" — no save needed
```

- [ ] **Step 4: Fix append_test.go**

```go
// Before:
combined := doc1.Append(doc2, doc3)
// After:
doc1.Append(doc2, doc3)
// use doc1 from here on
```

- [ ] **Step 5: Fix splitter_test.go**

```go
// Before:
doc, err = doc.Reorder([]int{2, 1})
// After:
err = doc.Reorder([]int{2, 1})
```

- [ ] **Step 6: Fix encrypt_test.go**

```go
// Before:
doc = doc.SetPassword("user", "owner")
// After:
doc.SetPassword("user", "owner")
```

- [ ] **Step 7: Run tests — all should pass**

```bash
go test ./...
```

Expected: `ok github.com/aspose-pdf-foss/aspose-pdf-foss-for-go`.

- [ ] **Step 8: Commit**

```bash
git add *_test.go
git commit -m "Update tests to mutable API: remove doc = doc.Method() patterns"
```

---

## Task 7: Remove dead code

Delete all code that is no longer used after the refactor.

**Files:**
- Update: `document.go` (remove `withCopiedPatches`, `copyPatches`, `patchedRotation`, `setPatch`, `pageRotation` if still present)
- Update: `types.go` (remove `pdfDirectRef` only if writer no longer uses it — check first)
- Update: `metadata.go` (remove `metadataConfig`, `readMetadata` if still present)
- Update: `doc.go` (remove any leftover rawDocument references if doc.go still imports it)

- [ ] **Step 1: Check for unused symbols**

```bash
go vet ./...
```

Also run:

```bash
grep -rn "pageRef\|patchKey\|withCopiedPatches\|copyPatches\|metadataConfig\|rawDocument\|pageInfo\b" --include="*.go" .
```

Expected: only matches in `validate.go` (for `rawDocument`) and possibly test files.

- [ ] **Step 2: Remove unused symbols**

Delete each symbol that has no remaining callers:
- `pageRef` struct (was in document.go)
- `patchKey` struct (was in document.go)
- `withCopiedPatches` method
- `copyPatches` function
- `patchedRotation` method
- `setPatch` method
- `pageRotation` function
- `metadataConfig` type and `readMetadata(doc *rawDocument)` function
- `buildMultiPagePDF`, `buildMultiPagePDFEx`, `buildPagePDF`, `writePage` (old writer entry points)
- `writePage` function (old writer)

- [ ] **Step 3: Verify build passes**

```bash
go build ./...
go test ./...
```

Expected: clean build, all tests pass.

- [ ] **Step 4: Commit**

```bash
git add .
git commit -m "Remove dead code: pageRef, patches, metadataConfig, old writer entry points"
```

---

## Task 8: Verify round-trip integrity

Run the full test suite including the splitter integration test which validates every output page.

- [ ] **Step 1: Run full test suite with verbose output**

```bash
go test -v ./... 2>&1 | tail -30
```

Expected: `ok github.com/aspose-pdf-foss/aspose-pdf-foss-for-go`.

- [ ] **Step 2: Run the splitter integration test specifically**

```bash
go test -v -run TestSplitFiles ./...
```

Expected: each split file passes `Validate`.

- [ ] **Step 3: Run the metadata round-trip tests**

```bash
go test -v -run TestMetadata ./...
```

Expected: `TestDocumentMetadataFields`, `TestMetadataCustomFieldsRoundTrip`, `TestSetMetadataRoundTrip`, `TestSetMetadataCustomFields`, `TestSetMetadataReplaces`, `TestClearMetadata` all pass.

Notably `TestSetMetadataRoundTrip` no longer needs a save/reload because `Metadata()` now reads from `doc.info` directly.

- [ ] **Step 4: Final commit**

```bash
git add .
git commit -m "Phase 1 complete: in-memory PDF object model"
```

---

## Quick Reference: Key API Changes

| Old (immutable) | New (mutable) |
|-----------------|---------------|
| `doc, err = doc.Rotate(90)` | `err = doc.Rotate(90)` |
| `doc, err = doc.SetRotation(0)` | `err = doc.SetRotation(0)` |
| `doc, err = doc.Reorder(order)` | `err = doc.Reorder(order)` |
| `doc = doc.Append(other)` | `doc.Append(other)` |
| `doc = doc.SetPassword(u, o)` | `doc.SetPassword(u, o)` |
| `doc = doc.SetMetadata(m)` | `doc.SetMetadata(m)` |
| `doc = doc.ClearMetadata()` | `doc.ClearMetadata()` |
| `meta, err = doc.Metadata()` | `meta, err = doc.Metadata()` ← reads live `doc.info` |
| `parts, err = doc.Split()` | `parts, err = doc.Split()` ← unchanged |
| `result, err = doc.Extract(r)` | `result, err = doc.Extract(r)` ← unchanged |
