package asposepdf

import (
	"fmt"
	"regexp"
)

// rawDocument is a parsed PDF file. It is immutable — it only reads data.
// The public Document type holds references to one or more rawDocuments as page sources.
type rawDocument struct {
	data    []byte
	xref    *xrefTable
	trailer pdfDict

	// Cache of parsed objects.
	cache map[int]*pdfObject

	// Object streams cache: streamObjNum -> parsed objects inside that stream.
	objStreams map[int][]*pdfObject
}

// openDocument reads and parses a PDF file.
func openDocument(path string) (*rawDocument, error) {
	data, err := readFile(path)
	if err != nil {
		return nil, err
	}

	startOff, err := findStartXRef(data)
	if err != nil {
		return nil, err
	}

	xref, trailer, err := parseXRef(data, startOff)
	if err != nil {
		return nil, err
	}

	return &rawDocument{
		data:      data,
		xref:      xref,
		trailer:   trailer,
		cache:     make(map[int]*pdfObject),
		objStreams: make(map[int][]*pdfObject),
	}, nil
}

// getObject returns the parsed pdfObject for the given object number.
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

// getFromObjStream retrieves an object stored inside an object stream.
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

	n := dictGetInt(s.Dict, "/N")    // number of objects in the stream
	first := dictGetInt(s.Dict, "/First") // byte offset of first object

	// Parse the header: pairs of (objNum, offset) for each object.
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

	// Parse each object from the stream body.
	var parsed []*pdfObject
	for i, oo := range offsets {
		end := len(s.Data)
		if i+1 < len(offsets) {
			end = offsets[i+1].offset
		}
		objData := s.Data[oo.offset:end]
		l := newLexer(objData)
		val, err := parseValue(l)
		if err != nil {
			continue
		}
		parsed = append(parsed, &pdfObject{Num: oo.num, Gen: 0, Value: val})
	}

	d.objStreams[streamObjNum] = parsed

	for _, o := range parsed {
		if o.Num == targetNum {
			return o, nil
		}
	}
	return nil, fmt.Errorf("object %d not found in stream %d", targetNum, streamObjNum)
}

// toInt from raw token bytes
func toIntBytes(raw []byte) int {
	n := 0
	for _, b := range raw {
		if b >= '0' && b <= '9' {
			n = n*10 + int(b-'0')
		}
	}
	return n
}

// resolve follows an indirect reference.
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

// resolveDict resolves a value to a pdfDict.
func (d *rawDocument) resolveDict(v pdfValue) (pdfDict, error) {
	rv, err := d.resolve(v)
	if err != nil {
		return nil, err
	}
	switch rd := rv.(type) {
	case pdfDict:
		return rd, nil
	case *pdfStream:
		return rd.Dict, nil
	}
	return nil, fmt.Errorf("expected dict, got %T", rv)
}

// pages walks the page tree and returns a list of pageInfo for each page.
func (d *rawDocument) pages() ([]*pageInfo, error) {
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

	var result []*pageInfo
	if err := d.walkPageTree(pagesRef, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// pageInfo describes a single PDF page and all objects it needs.
type pageInfo struct {
	objNum int
	deps   map[int]bool // all object numbers needed (including the page itself)
}

func (d *rawDocument) walkPageTree(nodeRef pdfValue, result *[]*pageInfo) error {
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
	case "/Page", "": // empty /Type is tolerated for compatibility with some malformed PDFs
		deps := make(map[int]bool)
		if err := d.collectDeps(ref.Num, deps); err != nil {
			return err
		}
		// Also collect inherited resources from ancestor Pages nodes.
		if err := d.collectInheritedDeps(nodeRef, deps); err != nil {
			return err
		}
		*result = append(*result, &pageInfo{objNum: ref.Num, deps: deps})
	default:
		return fmt.Errorf("unknown page tree node type: %s", nodeType)
	}
	return nil
}

// collectInheritedDeps collects resource dependencies from parent Pages nodes.
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

// reRef matches PDF indirect references like "5 0 R".
var reRef = regexp.MustCompile(`\b(\d+)\s+\d+\s+R\b`)

// collectDeps recursively collects all object numbers referenced by objNum.
func (d *rawDocument) collectDeps(objNum int, deps map[int]bool) error {
	if deps[objNum] {
		return nil // already visited
	}

	obj, err := d.getObject(objNum)
	if err != nil {
		return nil // best-effort
	}

	// Skip /Pages and /Catalog nodes — they belong to the page tree and are
	// rebuilt by the writer. Including them would produce duplicate /Pages objects.
	if dict, ok := obj.Value.(pdfDict); ok {
		switch dictGetName(dict, "/Type") {
		case "/Pages", "/Catalog":
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
		// Also scan raw stream data for references (e.g. content streams).
		refs := reRef.FindAllSubmatch(val.Data, -1)
		for _, m := range refs {
			n := toIntBytes(m[1])
			if n > 0 {
				d.collectDeps(n, deps)
			}
		}
	}
}
