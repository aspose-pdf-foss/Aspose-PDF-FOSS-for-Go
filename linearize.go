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
	remapFn := asm.remapFn()

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

	// --- 2. Partition objects into part 1 (first page section) and part 2. ---
	// Per-page dependency sets, in new-ID space.
	pageDeps := make([]map[int]bool, len(d.pages))
	refCount := make(map[int]int)
	for i, page := range d.pages {
		deps := collectPageDeps(d.objects, page)
		set := make(map[int]bool, len(deps))
		for oldID := range deps {
			set[asm.remap[oldID]] = true
		}
		pageDeps[i] = set
		for id := range set {
			refCount[id]++
		}
	}

	structural := map[int]bool{asm.pagesObjID: true, asm.catalogObjID: true}
	if asm.infoObjID != 0 {
		structural[asm.infoObjID] = true
	}

	// Attribute every object to the first (lowest-index) page that references
	// it — the qpdf convention. The first page owns all the objects it uses
	// (including ones a later page also references); each later page owns only
	// the objects no earlier page used.
	firstUse := make(map[int]int)
	for i := range d.pages {
		for id := range pageDeps[i] {
			if u, ok := firstUse[id]; !ok || i < u {
				firstUse[id] = i
			}
		}
	}

	// Object count per page (qpdf's nObjects): every object attributed to the
	// page (first-used by it), including ones it shares with later pages.
	pageObjCount := make([]int, len(d.pages))
	for _, id := range sortedFirstUse(firstUse) {
		if structural[id] {
			continue
		}
		pageObjCount[firstUse[id]]++
	}

	// Shared objects (referenced by >1 page) are placed physically in part 2 and
	// listed in the shared-object hint table — those used by the first page come
	// first (nSharedFirstPage). A page's own length counts only its *private*
	// objects, so shared objects must not sit in any page's physical group.
	var sharedOrder []int
	for _, id := range sortedFirstUse(firstUse) {
		if structural[id] || refCount[id] < 2 {
			continue
		}
		if firstUse[id] == 0 {
			sharedOrder = append(sharedOrder, id)
		}
	}
	nSharedFirstPage := len(sharedOrder)
	for _, id := range sortedFirstUse(firstUse) {
		if structural[id] || refCount[id] < 2 {
			continue
		}
		if firstUse[id] >= 1 {
			sharedOrder = append(sharedOrder, id)
		}
	}

	// Physical object groups: page i's private objects (first-used by it and
	// referenced by no other page), page object first. Shared objects excluded.
	pageGroups := make([][]int, len(d.pages))
	for i := range d.pages {
		group := []int{pageNewIDs[i]}
		for _, id := range sortedFirstUse(firstUse) {
			if id == pageNewIDs[i] || structural[id] {
				continue
			}
			if firstUse[id] == i && refCount[id] == 1 {
				group = append(group, id)
			}
		}
		pageGroups[i] = group
	}
	firstPagePrivate := pageGroups[0]

	// Orphans: content objects in no page's deps and not structural/shared/part1.
	placed := make(map[int]bool)
	for _, id := range firstPagePrivate {
		placed[id] = true
	}
	for _, id := range sharedOrder {
		placed[id] = true
	}
	for i := 1; i < len(d.pages); i++ {
		for _, id := range pageGroups[i] {
			placed[id] = true
		}
	}
	var orphans []int
	for newID := range values {
		if placed[newID] || structural[newID] {
			continue
		}
		orphans = append(orphans, newID)
	}
	sort.Ints(orphans)

	// --- 3. Lay out and emit. The shared objects sit at the end of part 1 (the
	// first-page section, after page 0's private objects and before /E). part 2
	// holds pages 2..N, the page-tree node, orphans and info. ---
	var part2 []int
	for i := 1; i < len(d.pages); i++ {
		part2 = append(part2, pageGroups[i]...)
	}
	part2 = append(part2, asm.pagesObjID)
	part2 = append(part2, orphans...)
	if asm.infoObjID != 0 {
		part2 = append(part2, asm.infoObjID)
	}

	lp := &linParams{
		d:                d,
		asm:              asm,
		values:           values,
		remapFn:          remapFn,
		part2:            part2,
		firstPagePrivate: firstPagePrivate,
		pageGroups:       pageGroups,
		pageObjCount:     pageObjCount,
		sharedOrder:      sharedOrder,
		nSharedFirstPage: nSharedFirstPage,
		pageNewIDs:       pageNewIDs,
	}
	return lp.emit()
}

// sortedFirstUse returns the object IDs (map keys) in ascending order, for
// deterministic partition ordering.
func sortedFirstUse(m map[int]int) []int {
	out := make([]int, 0, len(m))
	for id := range m {
		out = append(out, id)
	}
	sort.Ints(out)
	return out
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
		return pdfDirectRef{Num: oldToLin(val.Num)}
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
