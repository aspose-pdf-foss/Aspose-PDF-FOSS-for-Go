// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"fmt"
)

// linParams carries the partitioned object set into emit. Part numbering
// follows ISO 32000-1 Annex F / qpdf: part 4 = open-document objects, part 6 =
// the first-page section, part 7 = later pages' private groups, part 8 =
// cross-page shared objects, part 9 = everything else.
type linParams struct {
	d             *Document
	asm           *assembled
	values        map[int]pdfValue
	part2         []int   // new IDs, body file order (part 7 + part 8 + part 9)
	part4         []int   // open-document objects (placed after the catalog)
	part6         []int   // first-page section: page 0, privates, shared, outlines
	part8         []int   // shared objects referenced by >1 later page
	pageGroups    [][]int // per-page consecutive groups; [0] == part6
	sharedTable   []int   // shared-object hint table: part6 then part8, in order
	pageSharedIdx [][]int // per page >=1: indices into sharedTable it references
	outlineObjs   []int   // outline objects (root first), contiguous in the file
	pageNewIDs    []int
}

// emit lays out and serializes the linearized PDF.
func (lp *linParams) emit() ([]byte, error) {
	asm := lp.asm

	// --- Linearized object numbering: part 2 = 1..K2, part 1 = K2+1.. ---
	// part-1 file order: lin dict, catalog, open-document objects (part 4),
	// hint stream, then the first-page section (part 6). Each page's group is
	// numbered consecutively starting at its page object — qpdf's strict check
	// recomputes every page length as the byte span of nObjects consecutively
	// numbered objects from the page object.
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
	for _, id := range lp.part4 {
		newToLin[id] = lin
		lin++
	}
	linHint := lin
	lin++
	for _, id := range lp.part6 {
		newToLin[id] = lin
		lin++
	}
	size := lin // objects 0..size-1; obj 0 is free
	linFirstPart := k2 + 1
	part1Count := size - linFirstPart

	// A reference whose target has no assembled output object is dangling;
	// return 0 so rewriteToLin emits null instead of aliasing whatever object
	// owns that number in the linearized space.
	oldToLin := func(old int) int {
		newID, ok := lp.asm.remap[old]
		if !ok {
			return 0
		}
		return newToLin[newID]
	}

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
	for _, id := range lp.part4 {
		if err := serialize(newToLin[id], lp.values[id]); err != nil {
			return nil, err
		}
	}
	for _, id := range lp.part6 {
		if err := serialize(newToLin[id], lp.values[id]); err != nil {
			return nil, err
		}
	}
	for _, id := range lp.part2 {
		if err := serialize(newToLin[id], lp.values[id]); err != nil {
			return nil, err
		}
	}

	// --- Per-page and shared measurements (offset-independent). nObjects and
	// pageLen cover each page's consecutive group: for page 0 the whole part 6
	// (privates + shared + outlines), for later pages the page object plus its
	// private objects. Content length mirrors page length — the qpdf/Acrobat
	// convention (implementation note 127), since page objects are not
	// interleaved with their content streams. ---
	pageObjCount := make([]int, len(lp.pageGroups))
	pageLen := make([]int, len(lp.pageGroups))
	for i, grp := range lp.pageGroups {
		pageObjCount[i] = len(grp)
		for _, id := range grp {
			pageLen[i] += len(objBytes[newToLin[id]])
		}
	}
	// The shared-object hint table covers all of part 6 (shared or not), then
	// part 8; each entry is a single-object group with its own byte length.
	sharedLen := make([]int, len(lp.sharedTable))
	for j, id := range lp.sharedTable {
		sharedLen[j] = len(objBytes[newToLin[id]])
	}
	firstShObj := 0
	if len(lp.part8) > 0 {
		firstShObj = newToLin[lp.part8[0]]
	}

	// --- Build the hint stream once with placeholder offsets to get its
	// length (offset-independent), then again with real offsets later. ---
	var outl *hintOutline
	if len(lp.outlineObjs) > 0 {
		outl = &hintOutline{
			firstObj: newToLin[lp.outlineObjs[0]],
			nObjects: len(lp.outlineObjs),
		}
		for _, id := range lp.outlineObjs {
			outl.groupLen += len(objBytes[newToLin[id]])
		}
	}
	hb := &hintBuilder{
		pageObjCount: pageObjCount,
		pageLen:      pageLen,
		contentLen:   pageLen,
		sharedLen:    sharedLen,
		nShFirstPage: len(lp.part6),
		pageShared:   lp.pageSharedIdx,
		firstShObj:   firstShObj,
		outline:      outl,
	}
	hintContent, hintSOff, hintOOff := hb.build(0, 0) // placeholder offsets
	hintObj := makeHintObject(linHint, hintContent, hintSOff, hintOOff)
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
	offByLin := make(map[int]int)
	offByLin[linCatalog] = offCatalog
	for _, id := range lp.part4 {
		offByLin[newToLin[id]] = pos
		pos += len(objBytes[newToLin[id]])
	}
	offHint := pos
	pos += hintLen
	// the first-page section (part 6) immediately follows the hint stream
	for _, id := range lp.part6 {
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
	if len(lp.part8) > 0 {
		firstShOff = offByLin[newToLin[lp.part8[0]]] - hintLen
	}

	// --- Rebuild the hint stream with real offsets (same length). ---
	if outl != nil {
		outl.firstObjOff = offByLin[newToLin[lp.outlineObjs[0]]] - hintLen
	}
	hintContent, hintSOff, hintOOff = hb.build(offHint, firstShOff)
	hintObj = makeHintObject(linHint, hintContent, hintSOff, hintOOff)
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
	for _, id := range lp.part4 {
		out.Write(objBytes[newToLin[id]])
	}
	out.Write(hintObj)
	for _, id := range lp.part6 {
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
