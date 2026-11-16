// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
)

// Incremental save (pdf-go-d10c), mirroring Aspose.PDF for .NET's
// Document.Save() to the stream a document was opened from: the file's bytes
// stay as they are and the changes are appended as a new revision — the
// objects that differ from the file, the new ones, a cross-reference section
// and a trailer pointing at the previous one (ISO 32000-1 §7.5.6). A
// signature already in the file therefore stays valid.

// SaveIncremental writes the document to path as the file it was opened from
// followed by one revision holding what changed since. See WriteIncremental.
func (d *Document) SaveIncremental(path string) error {
	data, err := buildIncrementalPDF(d)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// WriteIncremental writes the bytes of the file the document was opened from
// unchanged, then one revision with the changes: the objects whose value
// differs from the file's, the new ones, the catalog and page tree when they
// changed, and the document information. Each call appends to the opened
// file, not to an earlier incremental save; with nothing changed the file is
// written as it was. Mirrors Aspose.PDF for .NET's Document.Save() to the
// stream the document was opened from.
//
// It needs a document opened from a file whose cross-reference table did not
// have to be rebuilt (NeedsRepair), with the encryption it was opened with:
// a document whose encryption was removed or changed, one to be signed (Sign
// already saves incrementally on Save/WriteTo), or a built one is refused —
// use WriteTo. The PDF version in the header cannot change in a revision.
func (d *Document) WriteIncremental(w io.Writer) (int64, error) {
	data, err := buildIncrementalPDF(d)
	if err != nil {
		return 0, err
	}
	n, err := w.Write(data)
	return int64(n), err
}

// originalFile is the opened file parsed again: its objects as they are in
// the file, which the document's current ones are compared with.
type originalFile struct {
	objects  map[int]*pdfObject
	catalog  pdfDict
	pages    []*pdfObject
	rootNum  int          // the page tree's root object
	kept     map[int]bool // objects the document holds (all but the page tree, catalog, /Encrypt)
	maxNum   int
	infoDict pdfDict
}

func (d *Document) reparseSource() (*originalFile, error) {
	start, err := findStartXRef(d.source)
	if err != nil {
		return nil, err
	}
	xref, trailer, err := parseXRef(d.source, start)
	if err != nil {
		return nil, err
	}
	raw := newRawDocument(d.source, xref, trailer)
	if d.encryptObjNum > 0 {
		raw.encState = d.preserved
		raw.encryptObjNum = d.encryptObjNum
	}
	objects, err := parseAllObjectsFrom(raw)
	if err != nil {
		return nil, err
	}
	resolveIndirectStreamFilters(objects)
	if d.encryptObjNum > 0 {
		delete(objects, d.encryptObjNum)
	}
	catalog, err := extractCatalog(objects, trailer)
	if err != nil {
		return nil, err
	}
	pages, err := resolvePageTree(objects, catalog)
	if err != nil {
		return nil, err
	}
	o := &originalFile{objects: objects, catalog: catalog, pages: pages, kept: map[int]bool{}, infoDict: extractInfo(objects, trailer)}
	if r, ok := catalog["/Pages"].(pdfRef); ok {
		o.rootNum = r.Num
	}
	for n, obj := range objects {
		o.maxNum = max(o.maxNum, n)
		if dict, ok := obj.Value.(pdfDict); ok {
			if t := dictGetName(dict, "/Type"); t == "/Pages" || t == "/Catalog" {
				continue
			}
		}
		o.kept[n] = true
	}
	return o, nil
}

// canonicalObject identifies an object's value for comparison: a stream by its
// dictionary (without /Length) and decoded data, anything else by its
// canonical serialization.
func canonicalObject(v pdfValue) string {
	if st, ok := v.(*pdfStream); ok {
		sum := sha256.Sum256(decodedStreamData(st))
		return canonicalDict(st.Dict) + string(sum[:])
	}
	return canonicalValue(v)
}

// withoutParent returns a page dictionary's canonical form without /Parent,
// which the writer sets.
func withoutParent(v pdfValue) string {
	dict, ok := v.(pdfDict)
	if !ok {
		return canonicalValue(v)
	}
	c := make(pdfDict, len(dict))
	for k, val := range dict {
		if k != "/Parent" {
			c[k] = val
		}
	}
	return canonicalDict(c)
}

func buildIncrementalPDF(d *Document) ([]byte, error) {
	switch {
	case len(d.source) == 0:
		return nil, fmt.Errorf("incremental save: the document was not opened from a file")
	case d.repair.XRefReconstructed:
		return nil, fmt.Errorf("incremental save: the file's cross-reference table had to be rebuilt; save it whole with WriteTo")
	case d.sign != nil:
		return nil, fmt.Errorf("incremental save: a signature is configured; Save and WriteTo already sign incrementally")
	case d.encryptObjNum > 0 && d.preserved == nil:
		return nil, fmt.Errorf("incremental save: the encryption was removed or changed; save the document whole with WriteTo")
	case d.encryptObjNum == 0 && d.encrypt != nil:
		return nil, fmt.Errorf("incremental save: encryption was added; save the document whole with WriteTo")
	case d.catalogNum == 0:
		return nil, fmt.Errorf("incremental save: the file's catalog number is unknown")
	}
	if err := d.ApplyTextChanges(); err != nil {
		return nil, err
	}
	orig, err := d.reparseSource()
	if err != nil {
		return nil, fmt.Errorf("incremental save: read the file: %w", err)
	}

	d.refreshShapedText()
	refs := d.injectWriterObjects()

	// Objects the file does not hold as the same object (new ones, and new
	// ones that took the number of a page-tree node, the catalog or a free
	// entry) get numbers above everything in the file.
	ids := sortedObjectIDs(d.objects)
	next := max(d.origSize, orig.maxNum+1)
	idMap := map[int]int{}
	for _, n := range ids {
		if !orig.kept[n] {
			idMap[n] = next
			next++
		}
	}
	num := func(n int) int {
		if m, ok := idMap[n]; ok {
			return m
		}
		return n
	}

	// The page tree is kept when the pages are the file's, in its order;
	// otherwise its root is rewritten flat, under its old number.
	samePages := len(orig.pages) == len(d.pages)
	for i := 0; samePages && i < len(d.pages); i++ {
		samePages = num(d.pages[i].Num) == orig.pages[i].Num
	}
	rootNum := orig.rootNum
	if rootNum == 0 {
		rootNum = next
		next++
	}
	isPage := map[int]bool{}
	for _, p := range d.pages {
		isPage[p.Num] = true
	}
	origParent := map[int]pdfValue{}
	for _, p := range orig.pages {
		if dict, ok := p.Value.(pdfDict); ok {
			origParent[p.Num] = dict["/Parent"]
		}
	}

	modified := map[int]pdfValue{}
	for _, n := range ids {
		if n == d.infoNum && n != 0 {
			continue // written from d.info below
		}
		cur := d.objects[n].Value
		out := num(n)
		if isPage[n] {
			dict, ok := cur.(pdfDict)
			if !ok {
				continue
			}
			parent := pdfValue(pdfRef{Num: rootNum})
			if samePages {
				parent = origParent[n]
			}
			if samePages {
				if o, ok := orig.objects[n]; ok && withoutParent(o.Value) == withoutParent(cur) {
					continue
				}
			}
			// /Parent is set after renumbering: in memory a new object may
			// hold the number the file gives the page-tree node.
			c := rewriteRefs(dict, idMap).(pdfDict)
			c["/Parent"] = parent
			modified[out] = c
			continue
		}
		if o, ok := orig.objects[n]; ok && orig.kept[n] && canonicalObject(o.Value) == canonicalObject(cur) {
			continue
		}
		modified[out] = rewriteRefs(cur, idMap)
	}
	if !samePages {
		kids := make(pdfArray, len(d.pages))
		for i, p := range d.pages {
			kids[i] = pdfRef{Num: num(p.Num)}
		}
		modified[rootNum] = pdfDict{"/Type": pdfName("/Pages"), "/Kids": kids, "/Count": len(d.pages)}
	}
	cat := rewriteRefs(d.outputCatalog(nil, refs), idMap).(pdfDict)
	cat["/Pages"] = pdfRef{Num: rootNum}
	if canonicalValue(cat) != canonicalValue(orig.catalog) {
		modified[d.catalogNum] = cat
	}

	// The document information: written when it differs from the file's,
	// under its number (a new one when the file had none); without any, the
	// newest trailer names none.
	infoNum := d.infoNum
	switch {
	case d.info == nil && orig.infoDict == nil:
		// The file names none, or one that is not a dictionary: as it was.
	case d.info == nil:
		infoNum = 0
	case orig.infoDict == nil || canonicalValue(d.info) != canonicalValue(orig.infoDict):
		if infoNum == 0 {
			infoNum = next
		}
		modified[infoNum] = rewriteRefs(d.info, idMap)
	}
	if len(modified) == 0 && infoNum == d.infoNum {
		return append([]byte(nil), d.source...), nil
	}

	saved := d.infoNum
	d.infoNum = infoNum
	defer func() { d.infoNum = saved }()
	var encState *encryptState
	if d.encryptObjNum > 0 {
		encState = d.preserved
	}
	// Every object is passed in modified, already numbered for the file, so
	// nothing is taken from d.objects by number.
	return d.appendRevisionObjects(modified, encState)
}

// appendRevisionObjects is appendRevision for objects already numbered for
// the file: none is taken from d.objects, and each is written with
// generation 0 unless the file holds that number with another generation.
func (d *Document) appendRevisionObjects(objs map[int]pdfValue, encState *encryptState) ([]byte, error) {
	saveNext := d.nextID
	defer func() { d.nextID = saveNext }()
	d.nextID = 0 // appendRevision takes objects numbered from baseNextID to d.nextID: none
	out, err := d.appendRevision(0, objs, encState)
	if err != nil {
		return nil, err
	}
	if !bytes.HasPrefix(out, d.source) {
		return nil, fmt.Errorf("incremental save: the file's bytes changed")
	}
	return out, nil
}
