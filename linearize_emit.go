// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"fmt"
	"sort"
)

// linParams carries the partitioned object set into emit.
type linParams struct {
	d                *Document
	asm              *assembled
	values           map[int]pdfValue
	remapFn          func(int) int
	part2            []int   // new IDs, part-2 file order
	firstPagePrivate []int   // new IDs: page-0 object then its private objects
	pageGroups       [][]int // physical private objects per page
	pageObjCount     []int   // attributed object count per page (incl. shared)
	sharedOrder      []int   // new IDs: shared objects (end of part 1)
	nSharedFirstPage int     // shared objects used by the first page
	pageNewIDs       []int
}

// emit lays out and serializes the linearized PDF.
func (lp *linParams) emit() ([]byte, error) {
	asm := lp.asm

	// --- Linearized object numbering: part 2 = 1..K2, part 1 = K2+1.. ---
	// part-1 file order: lin dict, catalog, hint stream, first-page private
	// objects, then the shared objects (which end the first-page section).
	newToLin := make(map[int]int)
	lin := 1
	for _, id := range lp.part2 {
		newToLin[id] = lin
		lin++
	}
	k2 := lin - 1
	linLinDict := lin
	lin++
	newToLin[asm.catalogObjID] = lin
	linCatalog := lin
	lin++
	linHint := lin
	lin++
	for _, id := range lp.firstPagePrivate {
		newToLin[id] = lin
		lin++
	}
	for _, id := range lp.sharedOrder {
		newToLin[id] = lin
		lin++
	}
	size := lin // objects 0..size-1; obj 0 is free
	linFirstPart := k2 + 1
	part1Count := size - linFirstPart

	oldToLin := func(old int) int { return newToLin[lp.remapFn(old)] }

	// --- Serialize each real object to lin-space bytes. ---
	objBytes := make(map[int][]byte) // lin number -> "N 0 obj…endobj\n"
	serialize := func(linNum int, v pdfValue) error {
		rewritten := rewriteToLin(v, oldToLin, newToLin)
		var b bytes.Buffer
		if err := writeObject(&b, linNum, rewritten, identityRemap, nil); err != nil {
			return err
		}
		objBytes[linNum] = b.Bytes()
		return nil
	}
	if err := serialize(linCatalog, lp.values[asm.catalogObjID]); err != nil {
		return nil, err
	}
	for _, id := range lp.firstPagePrivate {
		if err := serialize(newToLin[id], lp.values[id]); err != nil {
			return nil, err
		}
	}
	for _, id := range lp.sharedOrder {
		if err := serialize(newToLin[id], lp.values[id]); err != nil {
			return nil, err
		}
	}
	for _, id := range lp.part2 {
		if err := serialize(newToLin[id], lp.values[id]); err != nil {
			return nil, err
		}
	}

	// --- Per-page and shared measurements (offset-independent). pageObjCount is
	// the attributed count (includes shared objects); pageLen measures only the
	// page's physical private objects (shared objects live in part 2). ---
	pageLen := make([]int, len(lp.pageGroups))
	contentLen := make([]int, len(lp.pageGroups))
	for i, grp := range lp.pageGroups {
		for _, id := range grp {
			pageLen[i] += len(objBytes[newToLin[id]])
		}
		contentLen[i] = lp.pageContentLen(i, objBytes, newToLin)
	}
	// The shared-object hint table lists the shared objects (referenced by more
	// than one page); those used by the first page come first (nSharedFirstPage).
	sharedLen := make([]int, len(lp.sharedOrder))
	for j, id := range lp.sharedOrder {
		sharedLen[j] = len(objBytes[newToLin[id]])
	}
	firstShObj := 0
	if len(lp.sharedOrder) > 0 {
		firstShObj = newToLin[lp.sharedOrder[0]]
	}

	// --- Build the hint stream once with placeholder offsets to get its
	// length (offset-independent), then again with real offsets later. ---
	hb := &hintBuilder{
		pageObjCount: lp.pageObjCount,
		pageLen:      pageLen,
		contentLen:   contentLen,
		sharedLen:    sharedLen,
		nShFirstPage: lp.nSharedFirstPage,
		pageShared:   lp.pageSharedIndices(),
		firstShObj:   firstShObj,
	}
	hintContent, hintSOff := hb.build(0, 0) // placeholder offsets
	hintObj := makeHintObject(linHint, hintContent, hintSOff)
	hintLen := len(hintObj)

	// --- Fixed-size structural sections (widths padded so the layout is
	// stable regardless of the offset values they will hold). ---
	header := asm.header + "%\xe2\xe3\xcf\xd3\n"
	linDictTemplate := func(L, hOff, hLen, O, E, T int) string {
		return fmt.Sprintf("%d 0 obj\n<< /Linearized 1 /L %s /H [ %s %s ] /O %s /E %s /N %d /T %s >>\nendobj\n",
			linLinDict, pad(L, 11), pad(hOff, 10), pad(hLen, 7), pad(O, 8), pad(E, 10), len(lp.d.pages), pad(T, 10))
	}
	linDictLen := len(linDictTemplate(0, 0, 0, 0, 0, 0))

	// First-page xref: covers lin numbers [linFirstPart, size).
	infoLin := lp.infoLin(newToLin)
	npages := len(lp.d.pages)
	firstXrefLen := classicXrefLen(linFirstPart, part1Count) + len(buildFirstTrailer(size, linCatalog, infoLin, 0, npages))
	// Main xref: covers 0..k2.
	mainXrefLen := classicXrefLen(0, k2+1) + len(buildMainTrailer(size, linCatalog, infoLin, npages))

	// --- Compute absolute offsets in file order. ---
	pos := len(header)
	offLinDict := pos
	pos += linDictLen
	offFirstXref := pos
	pos += firstXrefLen
	offCatalog := pos
	pos += len(objBytes[linCatalog])
	offHint := pos
	pos += hintLen
	// first-page private objects, then the shared objects (end of part 1)
	offByLin := make(map[int]int)
	offByLin[linCatalog] = offCatalog
	for _, id := range lp.firstPagePrivate {
		offByLin[newToLin[id]] = pos
		pos += len(objBytes[newToLin[id]])
	}
	for _, id := range lp.sharedOrder {
		offByLin[newToLin[id]] = pos
		pos += len(objBytes[newToLin[id]])
	}
	offPart2Start := pos
	for _, id := range lp.part2 {
		offByLin[newToLin[id]] = pos
		pos += len(objBytes[newToLin[id]])
	}
	offMainXref := pos
	pos += mainXrefLen
	offStartxref := pos
	startxrefSection := fmt.Sprintf("startxref\n%d\n%%%%EOF\n", offFirstXref)
	totalLen := offStartxref + len(startxrefSection)

	// Hint-table offsets are expressed in "hint-excluded" coordinates: qpdf
	// computes them as if the hint stream had length 0 (the hint sits between
	// the catalog and the first page, so every object after it is shifted by
	// +hintLen in the real file). The first-page offset is the page object's
	// real offset minus hintLen, which equals offHint since the page object
	// immediately follows the hint stream. The shared section is likewise
	// shifted back by hintLen.
	firstShOff := 0
	if len(lp.sharedOrder) > 0 {
		firstShOff = offByLin[newToLin[lp.sharedOrder[0]]] - hintLen
	}

	// --- Rebuild the hint stream with real offsets (same length). ---
	hintContent, hintSOff = hb.build(offHint, firstShOff)
	hintObj = makeHintObject(linHint, hintContent, hintSOff)
	if len(hintObj) != hintLen {
		return nil, fmt.Errorf("linearize: hint length unstable (%d != %d)", len(hintObj), hintLen)
	}

	// /E = end of first page (start of part 2). /T is the offset of the first
	// entry (object 0) of the main cross-reference table, i.e. just past its
	// "xref\n0 N\n" subsection header.
	mainXrefT := offMainXref + len(fmt.Sprintf("xref\n%d %d\n", 0, k2+1))
	linDict := linDictTemplate(totalLen, offHint, hintLen, newToLin[lp.pageNewIDs[0]], offPart2Start, mainXrefT)

	// --- First-page xref (lin numbers [linFirstPart, size)). ---
	firstXrefOffsets := make([]int, 0, part1Count)
	for n := linFirstPart; n < size; n++ {
		switch n {
		case linLinDict:
			firstXrefOffsets = append(firstXrefOffsets, offLinDict)
		case linHint:
			firstXrefOffsets = append(firstXrefOffsets, offHint)
		default:
			firstXrefOffsets = append(firstXrefOffsets, offByLin[n])
		}
	}
	firstXref := classicXref(linFirstPart, firstXrefOffsets) +
		buildFirstTrailer(size, linCatalog, infoLin, offMainXref, npages)

	// --- Main xref (0..k2). ---
	mainOffsets := make([]int, k2+1)
	mainOffsets[0] = -1 // free
	for _, id := range lp.part2 {
		mainOffsets[newToLin[id]] = offByLin[newToLin[id]]
	}
	mainXref := classicXref(0, mainOffsets) +
		buildMainTrailer(size, linCatalog, infoLin, npages)

	// --- Assemble. ---
	var out bytes.Buffer
	out.Grow(totalLen)
	out.WriteString(header)
	out.WriteString(linDict)
	out.WriteString(firstXref)
	out.Write(objBytes[linCatalog])
	out.Write(hintObj)
	for _, id := range lp.firstPagePrivate {
		out.Write(objBytes[newToLin[id]])
	}
	for _, id := range lp.sharedOrder {
		out.Write(objBytes[newToLin[id]])
	}
	for _, id := range lp.part2 {
		out.Write(objBytes[newToLin[id]])
	}
	out.WriteString(mainXref)
	out.WriteString(startxrefSection)
	if out.Len() != totalLen {
		return nil, fmt.Errorf("linearize: length mismatch (%d != %d)", out.Len(), totalLen)
	}
	return out.Bytes(), nil
}

func identityRemap(n int) int { return n }

// infoLin returns the lin number of the /Info object, or 0.
func (lp *linParams) infoLin(newToLin map[int]int) int {
	if lp.asm.infoObjID == 0 {
		return 0
	}
	return newToLin[lp.asm.infoObjID]
}

// pageSharedIndices returns, per page (>=1), the indices into the shared-object
// hint table (= sharedOrder) of the shared objects that page references. Page 0
// returns nil — its shared objects are implicit (the first nSharedFirstPage).
func (lp *linParams) pageSharedIndices() [][]int {
	idx := make(map[int]int, len(lp.sharedOrder))
	for j, id := range lp.sharedOrder {
		idx[id] = j
	}
	out := make([][]int, len(lp.pageGroups))
	for i := 1; i < len(lp.pageGroups); i++ {
		deps := collectPageDeps(lp.d.objects, lp.d.pages[i])
		var refs []int
		for oldID := range deps {
			if j, ok := idx[lp.remapFn(oldID)]; ok {
				refs = append(refs, j)
			}
		}
		sort.Ints(refs)
		out[i] = refs
	}
	return out
}

// pageContentLen returns the byte length of page i's content stream object(s).
func (lp *linParams) pageContentLen(i int, objBytes map[int][]byte, newToLin map[int]int) int {
	page := lp.d.pages[i]
	dict, ok := page.Value.(pdfDict)
	if !ok {
		return 0
	}
	total := 0
	switch c := dict["/Contents"].(type) {
	case pdfRef:
		total += len(objBytes[newToLin[lp.remapFn(c.Num)]])
	case pdfArray:
		for _, e := range c {
			if r, ok := e.(pdfRef); ok {
				total += len(objBytes[newToLin[lp.remapFn(r.Num)]])
			}
		}
	}
	return total
}
