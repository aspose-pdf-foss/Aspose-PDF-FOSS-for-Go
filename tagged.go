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
	StructTFoot    StructType = "/TFoot"
	StructLink     StructType = "/Link"
	// StructAnnot wraps content associated with an annotation other than a
	// link (e.g. a comment's visible marker).
	StructAnnot StructType = "/Annot"
	// StructForm marks a form field's visible representation as part of the
	// reading order — distinct from the AcroForm field itself.
	StructForm StructType = "/Form"
	// StructBibEntry is one entry in a bibliography.
	StructBibEntry StructType = "/BibEntry"
	// StructBlockQuote is a block-level (multi-paragraph) quotation, as
	// opposed to StructQuote's inline one.
	StructBlockQuote StructType = "/BlockQuote"
	// StructIndex is a sequence of entries in a back-of-book index.
	StructIndex StructType = "/Index"
	// StructTOC is a table of contents; its entries are StructTOCI.
	StructTOC StructType = "/TOC"
	// StructTOCI is one table-of-contents entry, typically containing a
	// StructLink (AddObjectReference) to its target.
	StructTOCI StructType = "/TOCI"
	// StructNonStruct groups content that carries no useful structural
	// meaning of its own without excluding it from the tree entirely (unlike
	// TagArtifact, which removes content from the tree).
	StructNonStruct StructType = "/NonStruct"
	// StructPrivate is application-specific content a generic reader can
	// ignore the internal structure of, but should not discard.
	StructPrivate StructType = "/Private"
	// StructReference is a citation to content elsewhere in the document.
	StructReference StructType = "/Reference"
	// StructRuby and StructWarichu are East Asian typography annotations
	// (ISO 32000-2): a ruby gloss above/beside base text, and interlinear
	// (two-line) annotation text respectively.
	StructRuby    StructType = "/Ruby"
	StructWarichu StructType = "/Warichu"
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
	nextSP   int                     // next /StructParents / /StructParent index (shared key space)
	nextMCID map[int]int             // page index → next MCID
	// objRefs holds one entry per annotation associated with the structure
	// tree via AddObjectReference: the /StructParent index assigned to that
	// annotation maps directly to a reference to the owning structure
	// element (not an array, unlike a page's /StructParents entry) — the two
	// kinds of entry share one /ParentTree number tree, distinguished only by
	// the shape of their value.
	objRefs map[int]pdfValue
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
		objRefs:  map[int]pdfValue{},
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
	child := e.newChild(t)
	child.dict["/K"] = pdfArray{}
	return child
}

// newChild creates and registers a structure element of type t as a child of
// e, without initializing /K — AddChild is its only caller, and sets /K to
// an empty array right after. Factored out so a future caller needing a
// bare, unregistered-/K element (TagContent and AddObjectReference don't:
// TagContent builds its own leaf dict directly since it also needs /Pg, and
// AddObjectReference operates on an already-existing element) doesn't have
// to duplicate the object-registration boilerplate.
func (e *StructElement) newChild(t StructType) *StructElement {
	dict := pdfDict{
		"/Type": pdfName("/StructElem"),
		"/S":    pdfName(string(t)),
		"/P":    pdfRef{Num: e.objID},
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

// AddObjectReference ties e directly to annot via an Object Reference
// (/OBJR, ISO 32000-1 §14.7.4.4) rather than marked content — the mechanism
// a /Link structure element uses to point at its LinkAnnotation, so
// assistive technology can associate the two (also valid for any other
// annotation type, e.g. bringing a Widget into reading order). annot must
// already be attached to a page (via (*AnnotationCollection).Add) before
// calling this — /Pg is read from that attachment, not asked of the caller,
// so it can never disagree with where the annotation actually lives. annot
// gains a /StructParent entry pointing back at e; an annotation already
// referenced by another structure element is rejected; each annotation is
// meant to appear in the reading order exactly once.
//
// Composes with the existing primitives rather than replacing them: a link
// whose visible text is itself tagged calls this on the element TagContent
// returned; a link with no separately-tagged visible text (e.g. an image
// link tagged as /Figure elsewhere) calls it on a bare AddChild(StructLink).
func (e *StructElement) AddObjectReference(annot Annotation) error {
	base := annot.annotationBaseRef()
	if base.objID == 0 || base.attachedPage == nil {
		return fmt.Errorf("AddObjectReference: annotation must be added to a page (Annotations().Add) first")
	}
	if _, tagged := base.dict["/StructParent"]; tagged {
		return fmt.Errorf("AddObjectReference: annotation is already referenced by a structure element")
	}
	objr := pdfDict{
		"/Type": pdfName("/OBJR"),
		"/Pg":   pdfRef{Num: base.attachedPage.Num},
		"/Obj":  pdfRef{Num: base.objID},
	}
	e.addKidRef(objr)

	tc := e.tc
	idx := tc.nextSP
	tc.nextSP++
	base.dict["/StructParent"] = idx
	tc.objRefs[idx] = pdfRef{Num: e.objID}
	tc.rebuildParentTree()
	return nil
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

// rebuildParentTree writes the /ParentTree /Nums, keyed by /StructParents (a
// page's MCID array) or /StructParent (a single annotation's direct owning
// element, from AddObjectReference) index, in ascending order — the two
// kinds of entry share one number tree and one index space, distinguished
// only by the shape of their value.
func (tc *TaggedContent) rebuildParentTree() {
	type entry struct {
		idx int
		val pdfValue
	}
	entries := make([]entry, 0, len(tc.pages)+len(tc.objRefs))
	for _, info := range tc.pages {
		entries = append(entries, entry{info.structParent, info.kids})
	}
	for idx, ref := range tc.objRefs {
		entries = append(entries, entry{idx, ref})
	}
	sort.Slice(entries, func(a, b int) bool { return entries[a].idx < entries[b].idx })
	nums := pdfArray{}
	for _, e := range entries {
		nums = append(nums, e.idx, e.val)
	}
	if obj, ok := tc.doc.objects[tc.parentNum]; ok {
		if d, ok := obj.Value.(pdfDict); ok {
			d["/Nums"] = nums
		}
	}
}
