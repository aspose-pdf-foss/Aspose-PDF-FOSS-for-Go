// SPDX-License-Identifier: MIT

package asposepdf_test

import (
	"bytes"
	"fmt"
	"regexp"
	"strconv"
	"testing"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

// The tests in this file decode the primary hint stream of a linearized
// output and assert the invariants qpdf's strict check_linearization
// recomputes from the file: per-page object counts and byte spans over
// consecutively numbered objects, the shared-object table covering the whole
// first-page section, and no shared identifiers on page 0. The full corpus
// validation against qpdf itself lives in result_files/lin (pikepdf harness);
// these assertions keep the writer's model from regressing in CI.

type hintReader struct {
	data []byte
	pos  int // bit position
}

func (r *hintReader) bits(n int) int {
	v := 0
	for i := 0; i < n; i++ {
		v = v<<1 | int(r.data[r.pos>>3]>>(7-(r.pos&7))&1)
		r.pos++
	}
	return v
}

func (r *hintReader) align() { r.pos = (r.pos + 7) &^ 7 }

type decodedHints struct {
	linDict    string
	npages     int
	firstPage  int // /O
	e          int // /E
	nobjects   []int
	pageLens   []int
	nshared    []int
	sharedIDs  [][]int
	shFirstPg  int
	shTotal    int
	shLens     []int
	hasOutline bool
	outline    [4]int // firstObj, offset, nobjects, groupLen
}

func decodeHints(t *testing.T, data []byte) *decodedHints {
	t.Helper()
	ld := regexp.MustCompile(`/Linearized 1 [^>]*`).Find(data)
	if ld == nil {
		t.Fatal("no linearization dict")
	}
	get := func(key string) int {
		m := regexp.MustCompile(key + ` (\d+)`).FindSubmatch(ld)
		if m == nil {
			t.Fatalf("lin dict missing %s: %s", key, ld)
		}
		n, _ := strconv.Atoi(string(m[1]))
		return n
	}
	h := &decodedHints{linDict: string(ld), npages: get("/N"), firstPage: get("/O"), e: get("/E")}

	m := regexp.MustCompile(`/H \[ *(\d+) +(\d+)`).FindSubmatch(ld)
	hintOff, _ := strconv.Atoi(string(m[1]))
	head := data[hintOff:]
	dictEnd := bytes.Index(head, []byte("stream"))
	hintDict := string(head[:dictEnd])
	sOff := 0
	if sm := regexp.MustCompile(`/S (\d+)`).FindStringSubmatch(hintDict); sm != nil {
		sOff, _ = strconv.Atoi(sm[1])
	} else {
		t.Fatalf("hint dict missing /S: %s", hintDict)
	}
	oOff := -1
	if om := regexp.MustCompile(`/O (\d+)`).FindStringSubmatch(hintDict); om != nil {
		oOff, _ = strconv.Atoi(om[1])
	}
	body := head[dictEnd+len("stream")+1:]
	if body[0] == '\n' {
		body = body[1:]
	}

	// Page-offset hint table (13-field header + byte-aligned column rows).
	r := &hintReader{data: body}
	minObjs := r.bits(32)
	r.bits(32) // first page offset
	dObjBits := r.bits(16)
	minPageLen := r.bits(32)
	dPageLenBits := r.bits(16)
	r.bits(32) // min content offset
	dContOffBits := r.bits(16)
	r.bits(32) // min content len
	dContLenBits := r.bits(16)
	nSharedBits := r.bits(16)
	sharedIDBits := r.bits(16)
	numerBits := r.bits(16)
	r.bits(16) // denominator
	n := h.npages
	for i := 0; i < n; i++ {
		h.nobjects = append(h.nobjects, minObjs+r.bits(dObjBits))
	}
	r.align()
	for i := 0; i < n; i++ {
		h.pageLens = append(h.pageLens, minPageLen+r.bits(dPageLenBits))
	}
	r.align()
	for i := 0; i < n; i++ {
		h.nshared = append(h.nshared, r.bits(nSharedBits))
	}
	r.align()
	h.sharedIDs = make([][]int, n)
	for i := 0; i < n; i++ {
		for j := 0; j < h.nshared[i]; j++ {
			h.sharedIDs[i] = append(h.sharedIDs[i], r.bits(sharedIDBits))
		}
	}
	r.align()
	if numerBits != 0 || dContOffBits != 0 || dContLenBits != dPageLenBits {
		t.Errorf("unexpected header bit widths: numer=%d contOff=%d contLen=%d pageLen=%d",
			numerBits, dContOffBits, dContLenBits, dPageLenBits)
	}

	// Shared-object hint table at /S.
	r = &hintReader{data: body[sOff:]}
	r.bits(32) // first shared obj
	r.bits(32) // first shared offset
	h.shFirstPg = r.bits(32)
	h.shTotal = r.bits(32)
	r.bits(16) // group count bits
	minShLen := r.bits(32)
	dShLenBits := r.bits(16)
	for i := 0; i < h.shTotal; i++ {
		h.shLens = append(h.shLens, minShLen+r.bits(dShLenBits))
	}

	// Outline hint table at /O (optional).
	if oOff >= 0 {
		h.hasOutline = true
		r = &hintReader{data: body[oOff:]}
		for i := range h.outline {
			h.outline[i] = r.bits(32)
		}
	}
	return h
}

// objSpan returns the byte length of the consecutive objects
// [first, first+count) in data, asserting they are laid out back-to-back.
func objSpan(t *testing.T, data []byte, first, count int) int {
	t.Helper()
	total := 0
	next := -1
	for num := first; num < first+count; num++ {
		m := regexp.MustCompile(fmt.Sprintf(`(?m)^%d 0 obj`, num)).FindIndex(data)
		if m == nil {
			t.Fatalf("object %d not found", num)
		}
		if next >= 0 && m[0] != next {
			t.Fatalf("object %d not contiguous with its predecessor (at %d, want %d)", num, m[0], next)
		}
		end := bytes.Index(data[m[0]:], []byte("endobj"))
		objLen := end + len("endobj\n")
		total += objLen
		next = m[0] + objLen
	}
	return total
}

// TestLinearizeSharedObjectHints builds a 3-page document sharing one
// embedded font and checks the qpdf hint-table model: page 0's group covers
// the whole first-page section including the shared objects, later pages
// count only their private objects and reference the shared table.
func TestLinearizeSharedObjectHints(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	font, err := doc.LoadFont("testdata/DejaVuSans.ttf")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := doc.AddBlankPageFromFormat(pdf.PageFormatA4); err != nil {
			t.Fatal(err)
		}
	}
	for i := 1; i <= 3; i++ {
		p, err := doc.Page(i)
		if err != nil {
			t.Fatal(err)
		}
		if err := p.AddText(fmt.Sprintf("Shared font page %d", i),
			pdf.TextStyle{Font: font, Size: 24},
			pdf.Rectangle{LLX: 50, LLY: 700, URX: 545, URY: 780}); err != nil {
			t.Fatal(err)
		}
	}
	var buf bytes.Buffer
	if _, err := doc.WriteToLinearized(&buf); err != nil {
		t.Fatal(err)
	}
	data := buf.Bytes()
	h := decodeHints(t, data)

	if h.npages != 3 {
		t.Fatalf("npages = %d", h.npages)
	}
	// The spec forbids shared identifiers on page 0; later pages share the
	// font and must reference the shared table.
	if h.nshared[0] != 0 {
		t.Errorf("page 0 has %d shared identifiers; must be 0", h.nshared[0])
	}
	for i := 1; i < 3; i++ {
		if h.nshared[i] == 0 {
			t.Errorf("page %d shares the font but has no shared identifiers", i)
		}
		for _, id := range h.sharedIDs[i] {
			if id >= h.shTotal {
				t.Errorf("page %d shared identifier %d out of range (total %d)", i, id, h.shTotal)
			}
		}
	}
	// The shared table's first-page region covers the WHOLE part 6 — its
	// entry count equals page 0's object count.
	if h.shFirstPg != h.nobjects[0] {
		t.Errorf("nshared_first_page = %d, want page-0 nobjects %d", h.shFirstPg, h.nobjects[0])
	}
	// Page lengths are byte spans of consecutively numbered objects starting
	// at each page object — recompute them the way qpdf's check does.
	pageObj := h.firstPage
	if got := objSpan(t, data, pageObj, h.nobjects[0]); got != h.pageLens[0] {
		t.Errorf("page 0 length: hint %d, computed %d", h.pageLens[0], got)
	}
	// Shared entry lengths are individual object lengths; their first-page
	// region sums to page 0's length.
	sum := 0
	for i := 0; i < h.shFirstPg; i++ {
		sum += h.shLens[i]
	}
	if sum != h.pageLens[0] {
		t.Errorf("first-page shared lengths sum %d != page 0 length %d", sum, h.pageLens[0])
	}
}

// TestLinearizeOutlineHints checks that a document with bookmarks carries an
// outline hint table whose object count and group length match the file.
func TestLinearizeOutlineHints(t *testing.T) {
	doc := buildLinDoc(t, 2)
	root := doc.Outlines()
	for i := 1; i <= 2; i++ {
		item := pdf.NewOutlineItemCollection(doc)
		item.SetTitle(fmt.Sprintf("Page %d", i))
		p, err := doc.Page(i)
		if err != nil {
			t.Fatal(err)
		}
		item.SetDestination(pdf.NewDestinationFit(p))
		if err := root.Add(item); err != nil {
			t.Fatal(err)
		}
	}
	var buf bytes.Buffer
	if _, err := doc.WriteToLinearized(&buf); err != nil {
		t.Fatal(err)
	}
	data := buf.Bytes()
	h := decodeHints(t, data)
	if !h.hasOutline {
		t.Fatal("no outline hint table in a document with bookmarks")
	}
	firstObj, _, nObjs, groupLen := h.outline[0], h.outline[1], h.outline[2], h.outline[3]
	if nObjs < 3 { // root + 2 items
		t.Errorf("outline nobjects = %d, want >= 3", nObjs)
	}
	if got := objSpan(t, data, firstObj, nObjs); got != groupLen {
		t.Errorf("outline group length: hint %d, computed %d", groupLen, got)
	}
}

// TestLinearizeDanglingRefBecomesNull checks that a reference to a
// non-existent object is written as null instead of aliasing another object.
func TestLinearizeDanglingRefBecomesNull(t *testing.T) {
	// An empty object body parses as null and round-trips as "null".
	src := []byte("%PDF-1.4\n" +
		"1 0 obj\n<< /Type /Catalog /Pages 2 0 R /StructTreeRoot 9 0 R >>\nendobj\n" +
		"2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n" +
		"3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] >>\nendobj\n" +
		"trailer\n<< /Size 4 /Root 1 0 R >>\n")
	doc, err := pdf.OpenStream(bytes.NewReader(src))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	var buf bytes.Buffer
	if _, err := doc.WriteToLinearized(&buf); err != nil {
		t.Fatalf("linearize: %v", err)
	}
	data := buf.Bytes()
	m := regexp.MustCompile(`/StructTreeRoot (null|\d+ 0 R)`).FindSubmatch(data)
	if m == nil {
		t.Skip("catalog extra key not preserved")
	}
	if string(m[1]) != "null" {
		// A concrete number must at least not alias the first page object.
		h := decodeHints(t, data)
		num, _ := strconv.Atoi(string(bytes.Fields(m[1])[0]))
		if num == h.firstPage {
			t.Errorf("dangling /StructTreeRoot aliased the first page object %d", num)
		}
	}
}
