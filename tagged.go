// SPDX-License-Identifier: MIT

package asposepdf

import (
	"fmt"
	"sort"
)

// StructType is a PDF standard structure type (ISO 32000-1 §14.8.4), used as the
// /S value of a structure element. The value includes the leading slash.
type StructType string

// Standard structure types. Grouping types (Document, Part, Sect, Div, Table,
// TR, L, …) hold child elements; the rest typically wrap marked content.
const (
	StructDocument StructType = "/Document"
	StructPart     StructType = "/Part"
	StructArt      StructType = "/Art"
	StructSect     StructType = "/Sect"
	StructDiv      StructType = "/Div"
	StructP        StructType = "/P"
	StructH        StructType = "/H"
	StructH1       StructType = "/H1"
	StructH2       StructType = "/H2"
	StructH3       StructType = "/H3"
	StructH4       StructType = "/H4"
	StructH5       StructType = "/H5"
	StructH6       StructType = "/H6"
	StructSpan     StructType = "/Span"
	StructQuote    StructType = "/Quote"
	StructNote     StructType = "/Note"
	StructCode     StructType = "/Code"
	StructFigure   StructType = "/Figure"
	StructFormula  StructType = "/Formula"
	StructCaption  StructType = "/Caption"
	StructList     StructType = "/L"
	StructListItem StructType = "/LI"
	StructLabel    StructType = "/Lbl"
	StructListBody StructType = "/LBody"
	StructTable    StructType = "/Table"
	StructTR       StructType = "/TR"
	StructTH       StructType = "/TH"
	StructTD       StructType = "/TD"
	StructTHead    StructType = "/THead"
	StructTBody    StructType = "/TBody"
	StructLink     StructType = "/Link"
)

// TaggedContent is the facade for authoring a Tagged PDF (ISO 32000-1 §14.8): it
// owns the document's logical structure tree and sets the catalog marks PDF/UA
// requires. Obtain it with (*Document).TaggedContent. Mirrors the intent of
// Aspose.PDF for .NET's Document.TaggedContent / ITaggedContent.
type TaggedContent struct {
	doc       *Document
	root      *StructElement
	treeRoot  int // /StructTreeRoot object number
	parentNum int // /ParentTree object number

	// Per-page bookkeeping for the /ParentTree number tree.
	pages    map[int]*pageStructInfo // page index → info
	nextSP   int                     // next /StructParents index
	nextMCID map[int]int             // page index → next MCID
}

type pageStructInfo struct {
	structParent int
	kids         pdfArray // MCID → owning struct-element reference
}

// StructElement is a node in the logical structure tree.
type StructElement struct {
	tc    *TaggedContent
	objID int
	dict  pdfDict
}

// TaggedContent returns the document's tagged-content facade, creating the
// structure tree and the /MarkInfo, /ViewerPreferences and /StructTreeRoot
// catalog entries on first call. Idempotent.
func (d *Document) TaggedContent() *TaggedContent {
	if d.tagged != nil {
		return d.tagged
	}
	if d.catalog == nil {
		d.catalog = pdfDict{}
	}
	tc := &TaggedContent{
		doc:      d,
		pages:    map[int]*pageStructInfo{},
		nextMCID: map[int]int{},
	}

	// Document root structure element.
	rootDict := pdfDict{"/Type": pdfName("/StructElem"), "/S": pdfName(string(StructDocument)), "/K": pdfArray{}}
	rootID := d.addObject(rootDict)
	tc.root = &StructElement{tc: tc, objID: rootID, dict: rootDict}

	// ParentTree (number tree) and StructTreeRoot.
	parentDict := pdfDict{"/Nums": pdfArray{}}
	tc.parentNum = d.addObject(parentDict)
	treeDict := pdfDict{
		"/Type":       pdfName("/StructTreeRoot"),
		"/K":          pdfRef{Num: rootID},
		"/ParentTree": pdfRef{Num: tc.parentNum},
	}
	tc.treeRoot = d.addObject(treeDict)
	rootDict["/P"] = pdfRef{Num: tc.treeRoot}

	d.catalog["/StructTreeRoot"] = pdfRef{Num: tc.treeRoot}
	d.catalog["/MarkInfo"] = pdfDict{"/Marked": true}
	vp, ok := resolveRefToDict(d.objects, d.catalog["/ViewerPreferences"])
	if !ok {
		vp = pdfDict{}
		d.catalog["/ViewerPreferences"] = vp
	}
	vp["/DisplayDocTitle"] = true

	d.tagged = tc
	return tc
}

// Root returns the document-level (/Document) structure element — the default
// parent for top-level content.
func (tc *TaggedContent) Root() *StructElement { return tc.root }

// SetTitle sets the document title (so PDF/UA's title requirement is met).
func (tc *TaggedContent) SetTitle(title string) {
	info, _ := tc.doc.Info()
	info.Title = title
	tc.doc.SetInfo(info)
}

// SetLanguage sets the document's default natural language (/Catalog/Lang),
// e.g. "en-US".
func (tc *TaggedContent) SetLanguage(lang string) {
	tc.doc.catalog["/Lang"] = lang
}

// AddChild creates a grouping structure element of type t as a child of e and
// returns it. Use it for containers (Sect, Table, TR, TD, L, LI, …) that hold
// other elements rather than wrapping content directly.
func (e *StructElement) AddChild(t StructType) *StructElement {
	dict := pdfDict{
		"/Type": pdfName("/StructElem"),
		"/S":    pdfName(string(t)),
		"/P":    pdfRef{Num: e.objID},
		"/K":    pdfArray{},
	}
	id := e.tc.doc.addObject(dict)
	e.addKidRef(pdfRef{Num: id})
	return &StructElement{tc: e.tc, objID: id, dict: dict}
}

// SetAlt sets alternate text (/Alt) — required on Figure/Formula for PDF/UA.
func (e *StructElement) SetAlt(text string) { e.dict["/Alt"] = text }

// SetActualText sets the exact text (/ActualText) the element represents.
func (e *StructElement) SetActualText(text string) { e.dict["/ActualText"] = text }

// SetLanguage overrides the natural language for this element's content.
func (e *StructElement) SetLanguage(lang string) { e.dict["/Lang"] = lang }

// addKidRef appends a child reference to e's /K, promoting it to an array.
func (e *StructElement) addKidRef(ref pdfValue) {
	switch k := e.dict["/K"].(type) {
	case nil:
		e.dict["/K"] = ref
	case pdfArray:
		e.dict["/K"] = append(k, ref)
	default:
		e.dict["/K"] = pdfArray{k, ref}
	}
}

// TagContent draws a block of page content (everything the draw callback emits)
// inside a marked-content sequence and adds a corresponding leaf structure
// element of type t as a child of parent (nil = the document root). The returned
// element can carry alternate text (e.g. for a Figure). Returns an error if the
// document is not set up for tagging or the draw callback fails.
func (p *Page) TagContent(parent *StructElement, t StructType, draw func() error) (*StructElement, error) {
	doc := p.doc
	if doc == nil || doc.tagged == nil {
		return nil, fmt.Errorf("TagContent: call Document.TaggedContent() first")
	}
	tc := doc.tagged
	if parent == nil {
		parent = tc.root
	}
	pageDict, ok := p.pageObj().Value.(pdfDict)
	if !ok {
		return nil, fmt.Errorf("TagContent: page has no dictionary")
	}

	info := tc.pageInfo(p.index, pageDict)
	mcid := tc.nextMCID[p.index]
	tc.nextMCID[p.index] = mcid + 1

	if err := p.appendToContentStream([]byte(fmt.Sprintf("%s <</MCID %d>> BDC\n", t, mcid))); err != nil {
		return nil, err
	}
	if err := draw(); err != nil {
		return nil, err
	}
	if err := p.appendToContentStream([]byte("EMC\n")); err != nil {
		return nil, err
	}

	dict := pdfDict{
		"/Type": pdfName("/StructElem"),
		"/S":    pdfName(string(t)),
		"/P":    pdfRef{Num: parent.objID},
		"/Pg":   pdfRef{Num: p.pageObj().Num},
		"/K":    mcid,
	}
	id := doc.addObject(dict)
	parent.addKidRef(pdfRef{Num: id})

	// Record the MCID → element mapping for the page's /ParentTree array.
	for len(info.kids) <= mcid {
		info.kids = append(info.kids, pdfNull{})
	}
	info.kids[mcid] = pdfRef{Num: id}
	tc.pages[p.index] = info
	tc.rebuildParentTree()

	return &StructElement{tc: tc, objID: id, dict: dict}, nil
}

// pageInfo returns the per-page structure bookkeeping, assigning a
// /StructParents index on first use.
func (tc *TaggedContent) pageInfo(pageIndex int, pageDict pdfDict) *pageStructInfo {
	if info, ok := tc.pages[pageIndex]; ok {
		return info
	}
	info := &pageStructInfo{structParent: tc.nextSP}
	tc.nextSP++
	pageDict["/StructParents"] = info.structParent
	tc.pages[pageIndex] = info
	return info
}

// rebuildParentTree writes the /ParentTree /Nums from the per-page arrays, keyed
// by /StructParents index in ascending order.
func (tc *TaggedContent) rebuildParentTree() {
	idx := make([]int, 0, len(tc.pages))
	for i := range tc.pages {
		idx = append(idx, i)
	}
	sort.Slice(idx, func(a, b int) bool {
		return tc.pages[idx[a]].structParent < tc.pages[idx[b]].structParent
	})
	nums := pdfArray{}
	for _, i := range idx {
		info := tc.pages[i]
		nums = append(nums, info.structParent, info.kids)
	}
	if obj, ok := tc.doc.objects[tc.parentNum]; ok {
		if d, ok := obj.Value.(pdfDict); ok {
			d["/Nums"] = nums
		}
	}
}
