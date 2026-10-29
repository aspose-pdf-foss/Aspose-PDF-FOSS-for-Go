// SPDX-License-Identifier: MIT

package asposepdf

import (
	"errors"
	"fmt"
	"math"
	"strings"
)

// HeadingRecognitionStrategy selects how AutoTag finds headings. Mirrors
// Aspose.PDF for .NET's HeadingRecognitionStrategy.
type HeadingRecognitionStrategy int

const (
	// HeadingRecognitionAuto uses the outline (bookmarks) when the document
	// has one, otherwise the heuristic. The default.
	HeadingRecognitionAuto HeadingRecognitionStrategy = iota
	// HeadingRecognitionOutlines makes a paragraph a heading when an
	// outline item pointing at its page carries the same text; the
	// heading level is the item's depth in the outline.
	HeadingRecognitionOutlines
	// HeadingRecognitionHeuristic decides by font size: HeadingLevels when
	// given, otherwise the size relative to the document's body text.
	HeadingRecognitionHeuristic
	// HeadingRecognitionNone tags no headings — useful for documents whose
	// formatting the heuristic misreads.
	HeadingRecognitionNone
)

// AutoTaggingSettings configures AutoTag. Mirrors Aspose.PDF for .NET's
// AutoTaggingSettings; its EnableAutoTagging switch has no counterpart
// because AutoTag is called directly rather than through conversion
// options.
type AutoTaggingSettings struct {
	HeadingRecognitionStrategy HeadingRecognitionStrategy
	// HeadingLevels are font sizes in points, largest first, for the
	// heuristic strategy: text at least HeadingLevels[i] points tall
	// becomes a level i+1 heading. Empty: headings are text at least 1.14×
	// the document's body size (1.7× → H1, 1.35× → H2, otherwise H3).
	HeadingLevels []float64
	// Language becomes /Catalog/Lang when the document declares none;
	// empty means "en".
	Language string
}

// DefaultAutoTaggingSettings returns the default settings: automatic
// heading recognition, English.
func DefaultAutoTaggingSettings() AutoTaggingSettings {
	return AutoTaggingSettings{HeadingRecognitionStrategy: HeadingRecognitionAuto, Language: "en"}
}

// AutoTagResult counts the structure elements AutoTag created.
type AutoTagResult struct {
	Pages      int // pages that received tags
	Headings   int
	Paragraphs int
	Lists      int
	Tables     int
	Figures    int
}

// ErrAlreadyTagged is returned by AutoTag for a document that already has
// a structure tree.
var ErrAlreadyTagged = errors.New("asposepdf: document is already tagged")

// AutoTag adds a logical structure tree to a document that has none: the
// text of each page is grouped into headings, paragraphs, lists and tables
// in reading order, images become figures, and everything else (page
// headers and footers, watermarks, drawn rules and backgrounds) is marked
// as an artifact. Page content is rewritten only to insert marked-content
// operators — the page renders and extracts exactly as before.
//
// The result is best-effort: it is built from the same layout analysis as
// Paragraphs and the table detector, so complex layouts may be grouped
// imperfectly. Figures get no alternate text (ValidatePDFUA reports them);
// ai.ImageDescriptionCopilot.FillAltTexts can write it. Annotations are not
// tagged, and a Form XObject is tagged as one unit.
//
// It refuses a document that already has a structure tree
// (ErrAlreadyTagged). Rewriting page content invalidates existing digital
// signatures. All pages are analysed and rewritten before anything is
// changed, so on error the document is left as it was.
//
// For a PDF/A "a" level, call AutoTag and then ConvertToPDFA. Mirrors the
// auto-tagging Aspose.PDF for .NET applies through
// PdfFormatConversionOptions.AutoTaggingSettings.
func (d *Document) AutoTag(s AutoTaggingSettings) (*AutoTagResult, error) {
	if d.tagged != nil {
		return nil, ErrAlreadyTagged
	}
	if d.catalog != nil {
		if _, ok := d.catalog["/StructTreeRoot"]; ok {
			return nil, ErrAlreadyTagged
		}
	}

	pages := d.Pages()
	sel := make([]int, len(pages))
	for i := range pages {
		sel[i] = i + 1
	}
	fd, err := buildFlowDoc(pages, sel, flowDocOptions{dropFurniture: true, dropRotated: true, detectTables: true})
	if err != nil {
		return nil, fmt.Errorf("AutoTag: %w", err)
	}
	headings := atHeadingPlan(d, s)

	// Phase 1: analyse and rewrite every page in memory.
	type pageWork struct {
		page *Page
		data []byte
		tops []*atElem
		mcid []*atElem // MCID → owning leaf
	}
	var work []pageWork
	for i, fp := range fd.pages {
		p := pages[fp.number-1]
		data, err := p.contentStreams()
		if err != nil {
			return nil, fmt.Errorf("AutoTag: page %d: %w", fp.number, err)
		}
		items, err := atWalk(data, d.objects, p.pageResources())
		if err != nil {
			return nil, fmt.Errorf("AutoTag: page %d: %w", fp.number, err)
		}
		pa := atPageAnalysis{bodySize: fd.bodySize, headings: headings, pageNum: fp.number, pageH: fp.pageH, furniture: fd.furniture, settings: s}
		tops := pa.elements(fd.pages[i])
		leaves := atAssign(items, &tops, fp.pageW*fp.pageH)

		var mcids []*atElem
		ops := atEmit(items, func(owner int) (int, StructType) {
			e := leaves[owner]
			e.mcids = append(e.mcids, len(mcids))
			mcids = append(mcids, e)
			return len(mcids) - 1, e.typ
		})
		work = append(work, pageWork{page: p, data: serializeContentOps(ops), tops: tops, mcid: mcids})
	}

	// Phase 2: commit.
	tc := d.TaggedContent()
	res := &AutoTagResult{}
	var oldContents []int
	for _, w := range work {
		oldContents = append(oldContents, atContentIDs(d.objects, w.page.pageDict()["/Contents"])...)
		// Pages with no tagged content are rewritten too, so their
		// artifacts are marked as such.
		if err := replacePageContents(w.page, w.data); err != nil {
			return nil, fmt.Errorf("AutoTag: page %d: %w", w.page.Number(), err)
		}
		if len(w.mcid) == 0 {
			continue
		}
		pageObj := w.page.pageObj()
		pageDict := pageObj.Value.(pdfDict)
		info := tc.pageInfo(w.page.index, pageDict)
		info.kids = make(pdfArray, len(w.mcid))
		for _, e := range w.tops {
			atWriteElem(tc, tc.root, e, pageObj.Num, res)
		}
		for i, e := range w.mcid {
			info.kids[i] = pdfRef{Num: e.objID}
		}
		tc.nextMCID[w.page.index] = len(w.mcid)
		res.Pages++
	}
	tc.rebuildParentTree()
	d.dropUnreachable(oldContents)

	if lang, _ := d.catalog["/Lang"].(string); lang == "" {
		l := s.Language
		if l == "" {
			l = "en"
		}
		tc.SetLanguage(l)
	}
	// PDF/UA requires a title. Without one, the first heading stands in,
	// else the first tagged text (shortened at a word boundary).
	if info, _ := d.Info(); info.Title == "" {
		title := ""
		for _, w := range work {
			if title = atFirstHeading(w.tops); title != "" {
				break
			}
		}
		for _, w := range work {
			if title != "" {
				break
			}
			title = atShortTitle(atFirstText(w.tops))
		}
		if title != "" {
			tc.SetTitle(title)
		}
	}
	return res, nil
}

// atElem is one structure element being built. Leaves own content (their
// MCIDs); groups (L, LI, Table, TR) only hold children.
type atElem struct {
	typ      StructType
	regions  []Rectangle
	attrs    pdfDict
	children []*atElem
	leaf     bool
	keep     bool // write even with no content (table cells keep the grid)
	text     string

	// figure placement, for reading order
	top float64

	mcids []int
	objID int
}

// atPageAnalysis turns one page's flow blocks into structure elements.
type atPageAnalysis struct {
	bodySize float64
	headings map[int]map[string]int // page → normalised title → level
	pageNum  int
	pageH    float64
	// furniture drops repeating header/footer segments the extractor merged
	// into a content paragraph (whole furniture paragraphs never reach here).
	furniture *furnitureFilter
	settings  AutoTaggingSettings
}

func (pa atPageAnalysis) elements(fp flowDocPage) []*atElem {
	var tops []*atElem
	for _, blk := range fp.blocks {
		switch {
		case blk.table != nil:
			tops = append(tops, atTableElem(blk.table))
		case blk.para != nil:
			tops = append(tops, pa.paragraphElems(blk.para, blk.top)...)
		}
	}
	return tops
}

func (pa atPageAnalysis) paragraphElems(para *MarkupParagraph, top float64) []*atElem {
	var out []*atElem
	var list *atElem
	for _, seg := range segmentParagraph(para) {
		if pa.furniture.dropSegment(seg, pa.pageH) {
			continue
		}
		text := atSegText(seg)
		if seg.marker != "" {
			if list == nil {
				list = &atElem{typ: StructList, top: top}
				out = append(out, list)
			}
			list.children = append(list.children, atListItem(seg))
			continue
		}
		list = nil
		typ := StructP
		if lvl := pa.headingLevel(seg, text); lvl > 0 {
			typ = StructType(fmt.Sprintf("/H%d", lvl))
		}
		out = append(out, &atElem{typ: typ, leaf: true, regions: atLineRegions(seg.lines), text: text, top: top})
	}
	return out
}

func (pa atPageAnalysis) headingLevel(seg docSeg, text string) int {
	switch pa.settings.HeadingRecognitionStrategy {
	case HeadingRecognitionNone:
		return 0
	case HeadingRecognitionOutlines:
		return pa.headings[pa.pageNum][atNorm(text)]
	case HeadingRecognitionAuto:
		if pa.headings != nil {
			return pa.headings[pa.pageNum][atNorm(text)]
		}
	}
	if len([]rune(text)) >= 200 {
		return 0
	}
	if lv := pa.settings.HeadingLevels; len(lv) > 0 {
		for i, size := range lv {
			if seg.size >= size-0.01 {
				return min(i+1, 6)
			}
		}
		return 0
	}
	return headingLevel(seg.size, pa.bodySize, len(text))
}

// atHeadingPlan maps outline titles to heading levels per page, or returns
// nil when the strategy does not use the outline (or there is none).
func atHeadingPlan(d *Document, s AutoTaggingSettings) map[int]map[string]int {
	switch s.HeadingRecognitionStrategy {
	case HeadingRecognitionAuto, HeadingRecognitionOutlines:
	default:
		return nil
	}
	plan := map[int]map[string]int{}
	var walk func(items []*OutlineItemCollection, depth int)
	walk = func(items []*OutlineItemCollection, depth int) {
		for _, it := range items {
			if pg := outlineItemPage(it); pg > 0 {
				if plan[pg] == nil {
					plan[pg] = map[string]int{}
				}
				key := atNorm(it.Title())
				if _, seen := plan[pg][key]; !seen {
					plan[pg][key] = min(depth, 6)
				}
			}
			walk(it.All(), depth+1)
		}
	}
	walk(d.Outlines().All(), 1)
	if len(plan) == 0 {
		return nil
	}
	return plan
}

// atNorm normalises text for comparing a paragraph to an outline title.
func atNorm(s string) string {
	return strings.ToLower(strings.Join(strings.Fields(s), " "))
}

func atSegText(seg docSeg) string {
	parts := make([]string, 0, len(seg.lines))
	for _, l := range seg.lines {
		parts = append(parts, lineJoinedText(l))
	}
	return strings.Join(parts, " ")
}

// atFragmentRect is a fragment's text box (the box SearchText reports),
// padded so a glyph's test point on the baseline falls safely inside.
func atFragmentRect(fr TextFragment) Rectangle {
	const pad = 0.5
	if fr.Rotation != 0 {
		m := atRotateAbout(fr.Rotation, fr.X, fr.Y)
		x0, y0, x1, y1 := transformedBBox(m, fr.X, fr.Y+fr.descent, fr.X+fr.Width, fr.Y+fr.descent+fr.Height)
		return Rectangle{LLX: x0 - pad, LLY: y0 - pad, URX: x1 + pad, URY: y1 + pad}
	}
	return Rectangle{LLX: fr.X - pad, LLY: fr.Y + fr.descent - pad, URX: fr.X + fr.Width + pad, URY: fr.Y + fr.descent + fr.Height + pad}
}

func atLineRegions(lines []TextLine) []Rectangle {
	var rs []Rectangle
	for _, l := range lines {
		for _, fr := range l.Fragments {
			rs = append(rs, atFragmentRect(fr))
		}
	}
	return rs
}

// atListItem builds L > LI > (Lbl, LBody) for one list segment. The label
// region covers the marker's glyphs only; it comes first in reading order,
// so the body region (the whole segment) gets everything else.
func atListItem(seg docSeg) *atElem {
	li := &atElem{typ: StructListItem}
	lbl := &atElem{typ: StructLabel, leaf: true}
	if len(seg.lines) > 0 && len(seg.lines[0].Fragments) > 0 {
		fr := seg.lines[0].Fragments[0]
		runes := []rune(fr.Text)
		n := 0
		for n < len(runes) && (runes[n] == ' ' || runes[n] == '\t') {
			n++
		}
		for n < len(runes) && runes[n] != ' ' && runes[n] != '\t' {
			n++
		}
		r := atFragmentRect(fr)
		if n < len(runes) && n < len(fr.runeX) {
			r.URX = fr.runeX[n]
		}
		lbl.regions = []Rectangle{r}
	}
	body := &atElem{typ: StructListBody, leaf: true, regions: atLineRegions(seg.lines)}
	li.children = []*atElem{lbl, body}
	return li
}

// atTableElem builds Table > TR > TH/TD from a detected table; the first
// row is the header row when there is more than one.
func atTableElem(t *AbsorbedTable) *atElem {
	tbl := &atElem{typ: StructTable, top: t.Rect.URY}
	rows := t.RowList()
	for ri, row := range rows {
		tr := &atElem{typ: StructTR}
		for _, cell := range row.CellList() {
			if cell.Covered {
				continue
			}
			typ := StructTD
			if ri == 0 && len(rows) > 1 {
				typ = StructTH
			}
			c := &atElem{typ: typ, leaf: true, keep: true, regions: []Rectangle{cell.Rect}}
			if cell.RowSpan > 1 || cell.ColSpan > 1 {
				a := pdfDict{"/O": pdfName("/Table")}
				if cell.RowSpan > 1 {
					a["/RowSpan"] = cell.RowSpan
				}
				if cell.ColSpan > 1 {
					a["/ColSpan"] = cell.ColSpan
				}
				c.attrs = a
			}
			tr.children = append(tr.children, c)
		}
		tbl.children = append(tbl.children, tr)
	}
	return tbl
}

// atLeaves flattens the element tree's leaves in reading order.
func atLeaves(es []*atElem, out []*atElem) []*atElem {
	for _, e := range es {
		if e.leaf {
			out = append(out, e)
		}
		out = atLeaves(e.children, out)
	}
	return out
}

func atContains(r Rectangle, x, y float64) bool {
	return x >= r.LLX && x <= r.URX && y >= r.LLY && y <= r.URY
}

func atOverlap(r Rectangle, minX, minY, maxX, maxY float64) float64 {
	w := min(r.URX, maxX) - max(r.LLX, minX)
	h := min(r.URY, maxY) - max(r.LLY, minY)
	if w <= 0 || h <= 0 {
		return 0
	}
	return w * h
}

// atAssign decides every painting item's owner and returns the leaf list
// the owners index into. Figures for images (and image-bearing forms
// outside any text) are created here and inserted into tops by position.
func atAssign(items []atItem, tops *[]*atElem, pageArea float64) []*atElem {
	textLeaves := atLeaves(*tops, nil)
	lookup := func(x, y float64) *atElem {
		for _, e := range textLeaves {
			for _, r := range e.regions {
				if atContains(r, x, y) {
					return e
				}
			}
		}
		return nil
	}
	pageHasText := len(textLeaves) > 0

	owner := map[*atItem]*atElem{}
	ownersText := map[*atItem][]*atElem{}
	var figures []*atElem
	for i := range items {
		it := &items[i]
		if it.inArtifact {
			continue
		}
		switch it.kind {
		case atText:
			var os []*atElem
			for _, t := range it.toks {
				if t.glyph {
					os = append(os, lookup(t.x, t.y))
				}
			}
			ownersText[it] = os
		case atImage:
			w, h := it.maxX-it.minX, it.maxY-it.minY
			if w < 4 || h < 4 || (pageHasText && pageArea > 0 && w*h >= 0.7*pageArea) {
				continue // decoration, or a scan under its OCR text
			}
			f := &atElem{typ: StructFigure, leaf: true, top: it.maxY}
			figures = append(figures, f)
			owner[it] = f
		case atForm:
			var best *atElem
			bestA := 0.0
			for _, e := range textLeaves {
				a := 0.0
				for _, r := range e.regions {
					a += atOverlap(r, it.minX, it.minY, it.maxX, it.maxY)
				}
				if a > bestA {
					best, bestA = e, a
				}
			}
			if best == nil && it.formHasImage {
				best = &atElem{typ: StructFigure, leaf: true, top: it.maxY}
				figures = append(figures, best)
			}
			owner[it] = best
		}
	}

	// Figures join the reading order before the first text block that
	// starts below them.
	for _, f := range figures {
		at := len(*tops)
		for i, e := range *tops {
			if e.typ != StructFigure && e.top < f.top {
				at = i
				break
			}
		}
		*tops = append(*tops, nil)
		copy((*tops)[at+1:], (*tops)[at:])
		(*tops)[at] = f
	}

	leaves := atLeaves(*tops, nil)
	index := make(map[*atElem]int, len(leaves))
	for i, e := range leaves {
		index[e] = i
	}
	idx := func(e *atElem) int {
		if e == nil {
			return atOwnerArtifact
		}
		return index[e]
	}
	for i := range items {
		it := &items[i]
		switch it.kind {
		case atText:
			it.owners = make([]int, 0, len(ownersText[it]))
			for _, e := range ownersText[it] {
				it.owners = append(it.owners, idx(e))
			}
		case atImage, atForm:
			it.owner = idx(owner[it])
		case atVector:
			it.owner = atOwnerArtifact
		}
	}
	return leaves
}

// atHasContent reports whether e or any descendant owns content.
func atHasContent(e *atElem) bool {
	if len(e.mcids) > 0 {
		return true
	}
	for _, c := range e.children {
		if atHasContent(c) {
			return true
		}
	}
	return false
}

// atWriteElem writes e under parent: leaves with their MCIDs on page
// pageNum, groups as plain grouping elements. Elements that ended up with
// no content are skipped, except table cells inside a table that has some.
func atWriteElem(tc *TaggedContent, parent *StructElement, e *atElem, pageNum int, res *AutoTagResult) {
	if !e.leaf {
		if !atHasContent(e) {
			return
		}
		g := parent.AddChild(e.typ)
		switch e.typ {
		case StructList:
			res.Lists++
		case StructTable:
			res.Tables++
		}
		for _, c := range e.children {
			atWriteElem(tc, g, c, pageNum, res)
		}
		return
	}
	if len(e.mcids) == 0 && !e.keep {
		return
	}
	dict := pdfDict{
		"/Type": pdfName("/StructElem"),
		"/S":    pdfName(string(e.typ)),
		"/P":    pdfRef{Num: parent.objID},
		"/Pg":   pdfRef{Num: pageNum},
	}
	switch len(e.mcids) {
	case 0:
	case 1:
		dict["/K"] = e.mcids[0]
	default:
		k := make(pdfArray, len(e.mcids))
		for i, m := range e.mcids {
			k[i] = m
		}
		dict["/K"] = k
	}
	if e.attrs != nil {
		dict["/A"] = e.attrs
	}
	e.objID = tc.doc.addObject(dict)
	parent.addKidRef(pdfRef{Num: e.objID})
	switch {
	case e.typ == StructP:
		res.Paragraphs++
	case e.typ == StructFigure:
		res.Figures++
	case strings.HasPrefix(string(e.typ), "/H") && len(e.typ) == 3:
		res.Headings++
	}
}

// atFirstHeading returns the text of the first heading that received
// content, searching the pages' elements in reading order.
func atFirstHeading(es []*atElem) string {
	for _, e := range es {
		if e.leaf && len(e.mcids) > 0 && strings.HasPrefix(string(e.typ), "/H") && len(e.typ) == 3 {
			return e.text
		}
		if t := atFirstHeading(e.children); t != "" {
			return t
		}
	}
	return ""
}

// atFirstText returns the text of the first leaf that received content.
func atFirstText(es []*atElem) string {
	for _, e := range es {
		if e.leaf && len(e.mcids) > 0 && strings.TrimSpace(e.text) != "" {
			return e.text
		}
		if t := atFirstText(e.children); t != "" {
			return t
		}
	}
	return ""
}

// atShortTitle trims text to at most 100 characters, at a word boundary.
func atShortTitle(text string) string {
	text = strings.Join(strings.Fields(text), " ")
	r := []rune(text)
	if len(r) <= 100 {
		return text
	}
	cut := string(r[:100])
	if i := strings.LastIndex(cut, " "); i > 40 {
		cut = cut[:i]
	}
	return cut + "…"
}

// atRotateAbout is a rotation by deg degrees counter-clockwise about (x, y).
func atRotateAbout(deg, x, y float64) [6]float64 {
	r := deg * math.Pi / 180
	c, s := math.Cos(r), math.Sin(r)
	return matMul(matMul(translateMatrix(-x, -y), [6]float64{c, s, -s, c, 0, 0}), translateMatrix(x, y))
}

// atContentIDs lists the object numbers a page's /Contents entry uses: the
// stream(s) and an indirect array holding them.
func atContentIDs(objects map[int]*pdfObject, v pdfValue) []int {
	var ids []int
	if ref, ok := v.(pdfRef); ok {
		ids = append(ids, ref.Num)
		v = resolveRef(objects, v)
	}
	if arr, ok := v.(pdfArray); ok {
		for _, el := range arr {
			if ref, ok := el.(pdfRef); ok {
				ids = append(ids, ref.Num)
			}
		}
	}
	return ids
}

// dropUnreachable deletes the given objects when nothing reachable from a
// page or the catalog still uses them — the targeted counterpart of
// RemoveUnusedObjects, which would also reclaim unrelated objects the
// caller has not placed yet (an XForm still being drawn, say).
func (d *Document) dropUnreachable(ids []int) {
	if len(ids) == 0 {
		return
	}
	reachable := collectReachableIDs(d.objects, d.pages)
	for key, v := range d.catalog {
		if !catalogKeyRebuiltByWriter(d, key) {
			markReachable(d.objects, v, reachable)
		}
	}
	for _, id := range ids {
		if !reachable[id] {
			delete(d.objects, id)
		}
	}
}
