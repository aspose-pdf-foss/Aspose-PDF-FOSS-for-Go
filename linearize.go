// SPDX-License-Identifier: MIT

package asposepdf

import (
	"fmt"
	"io"
	"sort"
)

// SaveLinearized writes the document to outputPath as a linearized
// ("fast web view") PDF (ISO 32000-1 Annex F): the first page's objects and a
// hint table sit at the front so a viewer can render page 1 before the whole
// file has downloaded. The result is an ordinary PDF that any reader opens
// normally. Encryption and signing are not supported together with
// linearization. Mirrors the intent of Aspose.PDF for .NET's linearized save.
func (d *Document) SaveLinearized(outputPath string) error {
	data, err := buildLinearizedPDF(d)
	if err != nil {
		return err
	}
	return writeFile(outputPath, data)
}

// WriteToLinearized writes the document as a linearized PDF to w (implements an
// io.WriterTo-style contract). See SaveLinearized.
func (d *Document) WriteToLinearized(w io.Writer) (int64, error) {
	data, err := buildLinearizedPDF(d)
	if err != nil {
		return 0, err
	}
	n, err := w.Write(data)
	return int64(n), err
}

// buildLinearizedPDF serializes d as a linearized ("fast web view") PDF per
// ISO 32000-1 Annex F: a physical layout optimised for streaming, where the
// first page's objects and a hint table are reachable from the front of the
// file. The output is an ordinary, fully spec-compliant PDF that any reader
// opens normally (linearized files are also valid non-linearized files).
//
// Layout (classic cross-reference tables):
//
//	%PDF-…                          header
//	{linearization parameter dict}  first object, within first 1024 bytes
//	xref / trailer (/Prev → main)   first-page cross-reference, early
//	{catalog}
//	{primary hint stream}           page-offset + shared-object hint tables
//	{first page object + its private objects}
//	{shared objects}{pages 2..N}{page tree}{info}   the body
//	xref / trailer                  main cross-reference, at the end
//	startxref → first-page xref
//
// Encryption and signing are not supported in combination with linearization
// (rare for web-served PDFs); such documents return an error.
func buildLinearizedPDF(d *Document) ([]byte, error) {
	if d.sign != nil {
		return nil, fmt.Errorf("linearize: signing and linearization cannot be combined")
	}
	if d.encrypt != nil || d.preserved != nil {
		return nil, fmt.Errorf("linearize: encryption and linearization cannot be combined")
	}
	if len(d.pages) == 0 {
		return nil, fmt.Errorf("linearize: document has no pages")
	}

	asm, err := d.assemble()
	if err != nil {
		return nil, err
	}
	if asm.encState != nil {
		return nil, fmt.Errorf("linearize: encryption and linearization cannot be combined")
	}
	// --- 1. Build the full output object set in "new ID" (assemble) space. ---
	// values[newID] is the object value; refs inside are old pdfRefs (remapped
	// via remapFn) or pdfDirectRef in new space.
	values := make(map[int]pdfValue, asm.totalObjects)
	for _, oldID := range asm.contentIDs {
		values[asm.remap[oldID]] = d.objects[oldID].Value
	}
	// Page tree node.
	kids := make(pdfArray, len(d.pages))
	for i, page := range d.pages {
		kids[i] = pdfDirectRef{Num: asm.remap[page.Num]}
	}
	values[asm.pagesObjID] = pdfDict{
		"/Type":  pdfName("/Pages"),
		"/Count": len(d.pages),
		"/Kids":  kids,
	}
	values[asm.catalogObjID] = pdfValue(asm.catalog)
	if asm.infoObjID != 0 {
		values[asm.infoObjID] = pdfValue(d.info)
	}

	pageNewIDs := make([]int, len(d.pages))
	for i, page := range d.pages {
		pageNewIDs[i] = asm.remap[page.Num]
	}

	// --- 2. Partition objects following qpdf's categorization exactly, so that
	// qpdf's strict check_linearization (which recomputes every hint value from
	// the file and compares) agrees with the hint tables we write. Every content
	// object is classified by its set of users: the pages that reference it, the
	// catalog keys it is reachable from, and page /Thumb entries. ---
	structural := map[int]bool{asm.pagesObjID: true, asm.catalogObjID: true}
	if asm.infoObjID != 0 {
		structural[asm.infoObjID] = true
	}

	type objUsers struct {
		firstPage  bool
		otherPages int
		lastPage   int // page index of an other-page user (valid when otherPages == 1)
		thumbs     int
		others     int
		openDoc    bool
		outlines   bool
	}
	users := make(map[int]*objUsers)
	u := func(newID int) *objUsers {
		ou := users[newID]
		if ou == nil {
			ou = &objUsers{}
			users[newID] = ou
		}
		return ou
	}

	// Per-page dependency sets in new-ID space (page object + everything
	// reachable from its dict, except the /Thumb closure — qpdf attributes
	// thumbnails to a separate user class). qpdf's traversal is a LIFO stack
	// over sorted dict keys, so keys are effectively walked in REVERSE sorted
	// order with one shared visited set: /Thumb is processed before
	// /Resources, and an object reachable from both (a colour space shared by
	// the page image and its thumbnail) is attributed to the thumbnail only.
	pageDeps := make([]map[int]bool, len(d.pages))
	for i, page := range d.pages {
		deps := map[int]bool{page.Num: true}
		visited := map[int]bool{page.Num: true}
		var thumbDeps map[int]bool
		if dict, ok := page.Value.(pdfDict); ok {
			keys := make([]string, 0, len(dict))
			for k := range dict {
				keys = append(keys, k)
			}
			sort.Sort(sort.Reverse(sort.StringSlice(keys)))
			for _, k := range keys {
				v := dict[k]
				if linResolvesNull(d.objects, v) {
					continue
				}
				if k == "/Thumb" {
					thumbDeps = make(map[int]bool)
					linDepsValue(d.objects, v, thumbDeps, visited)
					continue
				}
				linDepsValue(d.objects, v, deps, visited)
			}
		}
		set := make(map[int]bool, len(deps))
		for oldID := range deps {
			newID := asm.remap[oldID]
			set[newID] = true
			ou := u(newID)
			if i == 0 {
				ou.firstPage = true
			} else {
				// deps is a set and remap is injective, so each page
				// contributes at most one user per object.
				ou.otherPages++
				ou.lastPage = i
			}
		}
		pageDeps[i] = set
		for oldID := range thumbDeps {
			u(asm.remap[oldID]).thumbs++
		}
	}

	// Catalog-key closures. The open-document keys go to part 4; /Outlines is
	// its own class; any other catalog key marks its closure as "other users".
	openDocKeys := map[string]bool{
		"/ViewerPreferences": true, "/PageMode": true, "/Threads": true,
		"/OpenAction": true, "/AcroForm": true,
	}
	walkKey := func(v pdfValue, mark func(*objUsers)) {
		if linResolvesNull(d.objects, v) {
			return // qpdf skips null-valued root/trailer keys
		}
		deps := make(map[int]bool)
		linDepsValue(d.objects, v, deps, map[int]bool{})
		for oldID := range deps {
			mark(u(asm.remap[oldID]))
		}
	}
	outlinesInFirstPage := false
	for k, v := range asm.catalog {
		switch {
		case k == "/Pages" || k == "/Type":
			// rebuilt page tree / plain name
		case openDocKeys[k]:
			walkKey(v, func(ou *objUsers) { ou.openDoc = true })
		case k == "/Outlines":
			if dict, isDirect := v.(pdfDict); isDirect {
				// The spec wants /Outlines indirect; qpdf's optimizer forces a
				// direct dict into its own object before categorizing, so its
				// recomputed outline hint table describes one object. Mirror
				// that: materialize the dict as a synthetic output object.
				maxID := 0
				for id := range values {
					if id > maxID {
						maxID = id
					}
				}
				synthID := maxID + 1
				values[synthID] = pdfValue(dict)
				asm.catalog[k] = pdfDirectRef{Num: synthID}
				u(synthID).outlines = true
			}
			walkKey(v, func(ou *objUsers) { ou.outlines = true })
			if dictGetName(asm.catalog, "/PageMode") == "/UseOutlines" {
				outlinesInFirstPage = true
			}
		default:
			walkKey(v, func(ou *objUsers) { ou.others++ })
		}
	}
	// Objects referenced from the /Info dictionary (a trailer key in qpdf's
	// model) are "other users" too; the Info object itself is structural.
	if asm.infoObjID != 0 {
		walkKey(pdfValue(d.info), func(ou *objUsers) { ou.others++ })
	}

	// qpdf's categorization chain, ported verbatim (QPDF_linearization.cc).
	var fpPrivate, fpShared, part4, part8 []int
	opPrivate := make([][]int, len(d.pages))
	var outlineObjs []int
	allIDs := make([]int, 0, len(users))
	for id := range users {
		allIDs = append(allIDs, id)
	}
	sort.Ints(allIDs)
	for _, id := range allIDs {
		if structural[id] {
			continue
		}
		if _, ok := values[id]; !ok {
			continue // unknown ref target — nothing to place
		}
		ou := users[id]
		switch {
		case ou.outlines:
			outlineObjs = append(outlineObjs, id)
		case ou.openDoc:
			part4 = append(part4, id)
		case ou.firstPage && ou.others == 0 && ou.otherPages == 0 && ou.thumbs == 0:
			fpPrivate = append(fpPrivate, id)
		case ou.firstPage:
			fpShared = append(fpShared, id)
		case ou.otherPages == 1 && ou.others == 0 && ou.thumbs == 0:
			opPrivate[ou.lastPage] = append(opPrivate[ou.lastPage], id)
		case ou.otherPages > 1:
			part8 = append(part8, id)
		default:
			// thumbnails (private or shared) and everything else land in
			// part 9 — they stay unplaced here and are swept into the body
			// tail below.
		}
	}

	// Page objects are only ever placed via their own groups. A malformed page
	// dict (missing /Type /Page) can slip through linDepsObj's page skip and
	// be classified into a bucket; placing it twice would corrupt the
	// numbering, so drop page IDs from every bucket.
	pageIDSet := make(map[int]bool, len(pageNewIDs))
	for _, id := range pageNewIDs {
		pageIDSet[id] = true
	}
	dropPages := func(xs []int) []int {
		out := xs[:0]
		for _, id := range xs {
			if !pageIDSet[id] {
				out = append(out, id)
			}
		}
		return out
	}
	fpPrivate = dropPages(fpPrivate)
	fpShared = dropPages(fpShared)
	part4 = dropPages(part4)
	part8 = dropPages(part8)
	outlineObjs = dropPages(outlineObjs)
	for i := range opPrivate {
		opPrivate[i] = dropPages(opPrivate[i])
	}

	// Order the outline objects with the /Outlines root dictionary first, as
	// qpdf's recomputation does — the outline hint table's first_object and
	// group length span consecutive numbers starting at the root.
	if len(outlineObjs) > 0 {
		rootID := 0
		switch ref := asm.catalog["/Outlines"].(type) {
		case pdfRef:
			rootID = asm.remap[ref.Num]
		case pdfDirectRef:
			rootID = ref.Num
		}
		for j, id := range outlineObjs {
			if id == rootID && j != 0 {
				copy(outlineObjs[1:j+1], outlineObjs[:j])
				outlineObjs[0] = rootID
				break
			}
		}
	}

	// Part 6 — the first-page section: the page object, its private objects,
	// the objects it shares with later pages / other users, and (when /PageMode
	// is /UseOutlines) the outline objects. nObjects for page 0 is len(part6),
	// and the shared-object hint table starts with ALL of part 6 in order.
	part6 := []int{pageNewIDs[0]}
	for _, id := range fpPrivate {
		if id != pageNewIDs[0] {
			part6 = append(part6, id)
		}
	}
	part6 = append(part6, fpShared...)
	if outlinesInFirstPage {
		part6 = append(part6, outlineObjs...)
	}

	// Part 7 — each later page's group: the page object then its privates.
	pageGroups := make([][]int, len(d.pages))
	pageGroups[0] = part6
	for i := 1; i < len(d.pages); i++ {
		group := []int{pageNewIDs[i]}
		for _, id := range opPrivate[i] {
			if id != pageNewIDs[i] {
				group = append(group, id)
			}
		}
		pageGroups[i] = group
	}

	// Shared-object hint table: all of part 6 in order, then all of part 8.
	sharedTable := append(append([]int(nil), part6...), part8...)
	tableIdx := make(map[int]int, len(sharedTable))
	for j, id := range sharedTable {
		tableIdx[id] = j
	}

	// Per-page shared identifiers (pages ≥ 1; the spec forbids them on page 0):
	// indices into the shared table of every table object the page references.
	pageSharedIdx := make([][]int, len(d.pages))
	for i := 1; i < len(d.pages); i++ {
		var refs []int
		for id := range pageDeps[i] {
			if j, ok := tableIdx[id]; ok {
				refs = append(refs, j)
			}
		}
		sort.Ints(refs)
		pageSharedIdx[i] = refs
	}

	// --- 3. Assemble part 2 (the body): later pages' groups (part 7), the
	// cross-page shared objects (part 8), then part 9 — the page-tree node,
	// outlines (unless placed in part 6), remaining objects, and /Info. ---
	placed := make(map[int]bool)
	for _, grp := range pageGroups {
		for _, id := range grp {
			placed[id] = true
		}
	}
	for _, id := range part8 {
		placed[id] = true
	}
	for _, id := range part4 {
		placed[id] = true
	}
	if outlinesInFirstPage {
		for _, id := range outlineObjs {
			placed[id] = true
		}
	}

	var part2 []int
	for i := 1; i < len(d.pages); i++ {
		part2 = append(part2, pageGroups[i]...)
	}
	part2 = append(part2, part8...)
	part2 = append(part2, asm.pagesObjID)
	if !outlinesInFirstPage {
		for _, id := range outlineObjs {
			placed[id] = true
			part2 = append(part2, id)
		}
	}
	var rest []int
	for newID := range values {
		if placed[newID] || structural[newID] {
			continue
		}
		rest = append(rest, newID)
	}
	sort.Ints(rest)
	part2 = append(part2, rest...)
	if asm.infoObjID != 0 {
		part2 = append(part2, asm.infoObjID)
	}

	lp := &linParams{
		d:             d,
		asm:           asm,
		values:        values,
		part2:         part2,
		part4:         part4,
		part6:         part6,
		part8:         part8,
		pageGroups:    pageGroups,
		sharedTable:   sharedTable,
		pageSharedIdx: pageSharedIdx,
		outlineObjs:   outlineObjs,
		pageNewIDs:    pageNewIDs,
	}
	return lp.emit()
}

// linDepsValue walks the object graph the way qpdf's optimizer does when it
// classifies objects for linearization: parsed values only. Unlike
// collectValueDepsDoc it does not scan raw stream bytes for "N G R" patterns
// (qpdf attributes users from parsed structures alone), and it ignores a
// stream dict's /Length reference — the writer always emits /Length as a
// direct integer, so an indirect length object never survives into the output
// and must not count as a page/user reference. Page-tree and catalog nodes
// are skipped like collectObjDeps (they are rebuilt by the writer).
func linDepsValue(objects map[int]*pdfObject, v pdfValue, deps map[int]bool, visited map[int]bool) {
	switch val := v.(type) {
	case pdfRef:
		linDepsObj(objects, val.Num, deps, visited)
	case pdfDict:
		for _, dv := range val {
			if linResolvesNull(objects, dv) {
				continue
			}
			linDepsValue(objects, dv, deps, visited)
		}
	case pdfArray:
		for _, av := range val {
			linDepsValue(objects, av, deps, visited)
		}
	case *pdfStream:
		for k, dv := range val.Dict {
			if k == "/Length" || linResolvesNull(objects, dv) {
				continue
			}
			linDepsValue(objects, dv, deps, visited)
		}
	}
}

// linResolvesNull reports whether a dict value is a reference to an object
// stored as null (or an empty/unparseable object, which readers treat as
// null). qpdf skips null-valued dict entries when attributing users, so a
// reference to such an object must not create a page/user relationship.
func linResolvesNull(objects map[int]*pdfObject, v pdfValue) bool {
	r, ok := v.(pdfRef)
	if !ok {
		return false
	}
	obj, ok := objects[r.Num]
	if !ok {
		return false // missing target: linDepsObj already adds no user
	}
	if obj.Value == nil {
		return true
	}
	_, isNull := obj.Value.(pdfNull)
	return isNull
}

func linDepsObj(objects map[int]*pdfObject, num int, deps map[int]bool, visited map[int]bool) {
	if visited[num] {
		return
	}
	obj, ok := objects[num]
	if !ok {
		return
	}
	if d, ok := obj.Value.(pdfDict); ok {
		switch dictGetName(d, "/Type") {
		case "/Pages", "/Catalog", "/Page":
			return
		}
	}
	visited[num] = true
	deps[num] = true
	linDepsValue(objects, obj.Value, deps, visited)
}

// rewriteToLin deep-copies v, translating every reference into the linearized
// object-number space: an original pdfRef via oldToLin, a writer pdfDirectRef
// (new-ID space) via newToLin. All become pdfDirectRef so they serialize as
// "N 0 R" without further remapping.
func rewriteToLin(v pdfValue, oldToLin func(int) int, newToLin map[int]int) pdfValue {
	switch val := v.(type) {
	case pdfDict:
		out := make(pdfDict, len(val))
		for k, vv := range val {
			out[k] = rewriteToLin(vv, oldToLin, newToLin)
		}
		return out
	case pdfArray:
		out := make(pdfArray, len(val))
		for i, vv := range val {
			out[i] = rewriteToLin(vv, oldToLin, newToLin)
		}
		return out
	case *pdfStream:
		nd := make(pdfDict, len(val.Dict))
		for k, dv := range val.Dict {
			nd[k] = rewriteToLin(dv, oldToLin, newToLin)
		}
		return &pdfStream{Dict: nd, Data: append([]byte(nil), val.Data...), Decoded: val.Decoded}
	case pdfRef:
		linNum := oldToLin(val.Num)
		if linNum == 0 {
			// Dangling reference — the target object does not exist. Emitting
			// it through the identity fallback would alias whatever object
			// happens to own that number in the linearized space (seen in the
			// wild: a missing /StructTreeRoot aliased the first page, which
			// broke qpdf's categorization). PDF defines a reference to a
			// non-existent object as null (ISO 32000-1 §7.3.10).
			return pdfNull{}
		}
		return pdfDirectRef{Num: linNum}
	case pdfDirectRef:
		if linNum, ok := newToLin[val.Num]; ok {
			return pdfDirectRef{Num: linNum}
		}
		return val
	case pdfHexString:
		return append(pdfHexString(nil), val...)
	default:
		return v
	}
}
