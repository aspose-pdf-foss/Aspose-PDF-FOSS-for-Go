// SPDX-License-Identifier: MIT

package asposepdf_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

// buildComparisonDoc writes one text block per page and reopens the saved document.
func buildComparisonDoc(t *testing.T, pages ...string) *pdf.Document {
	t.Helper()
	doc := pdf.NewDocument(400, 200)
	for i, text := range pages {
		if i > 0 {
			if err := doc.AddBlankPage(400, 200); err != nil {
				t.Fatalf("add page: %v", err)
			}
		}
		page, err := doc.Page(i + 1)
		if err != nil {
			t.Fatalf("page: %v", err)
		}
		style := pdf.TextStyle{Font: pdf.FontHelvetica, Size: 14}
		if err := page.AddText(text, style, pdf.Rectangle{LLX: 20, LLY: 60, URX: 380, URY: 170}); err != nil {
			t.Fatalf("add text: %v", err)
		}
	}
	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatalf("write: %v", err)
	}
	re, err := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	return re
}

func firstPages(t *testing.T, a, b *pdf.Document) (*pdf.Page, *pdf.Page) {
	t.Helper()
	p1, err := a.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	p2, err := b.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	return p1, p2
}

func TestComparePagesFindsTheChangedWord(t *testing.T) {
	a := buildComparisonDoc(t, "total is 100 euro")
	b := buildComparisonDoc(t, "total is 200 euro")
	p1, p2 := firstPages(t, a, b)

	ops, err := pdf.ComparePages(p1, p2)
	if err != nil {
		t.Fatal(err)
	}
	var ins, del []pdf.DiffOperation
	for _, op := range ops {
		switch op.Operation {
		case pdf.OperationInsert:
			ins = append(ins, op)
		case pdf.OperationDelete:
			del = append(del, op)
		}
	}
	if len(ins) != 1 || ins[0].Text != "200" {
		t.Fatalf("insertions = %+v, want one carrying %q", ins, "200")
	}
	if len(del) != 1 || del[0].Text != "100" {
		t.Fatalf("deletions = %+v, want one carrying %q", del, "100")
	}
	if len(ins[0].DestRects) != 1 || ins[0].DestPage != 1 {
		t.Fatalf("insertion location = page %d rects %+v", ins[0].DestPage, ins[0].DestRects)
	}
	if len(del[0].SourceRects) != 1 || del[0].SourcePage != 1 {
		t.Fatalf("deletion location = page %d rects %+v", del[0].SourcePage, del[0].SourceRects)
	}
}

// The rectangle of a changed word must agree with what SearchText reports for
// the same word — two different paths to the same box.
func TestComparePagesRectMatchesSearch(t *testing.T) {
	a := buildComparisonDoc(t, "total is 100 euro")
	b := buildComparisonDoc(t, "total is 200 euro")
	p1, p2 := firstPages(t, a, b)

	ops, err := pdf.ComparePages(p1, p2)
	if err != nil {
		t.Fatal(err)
	}
	var got pdf.Rectangle
	for _, op := range ops {
		if op.Operation == pdf.OperationInsert {
			got = op.DestRects[0]
		}
	}
	matches, err := p2.SearchText("200")
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 {
		t.Fatalf("search found %d matches, want 1", len(matches))
	}
	want := matches[0].Rect
	const tol = 0.5
	if abs(got.LLX-want.LLX) > tol || abs(got.URX-want.URX) > tol ||
		abs(got.LLY-want.LLY) > tol || abs(got.URY-want.URY) > tol {
		t.Errorf("comparison rect %+v differs from search rect %+v", got, want)
	}
}

func abs(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
}

// The operations must account for every word of both documents.
func TestAssembleTextRoundTrip(t *testing.T) {
	a := buildComparisonDoc(t, "the quick brown fox jumps")
	b := buildComparisonDoc(t, "the slow brown cat jumps over")
	p1, p2 := firstPages(t, a, b)

	ops, err := pdf.ComparePages(p1, p2)
	if err != nil {
		t.Fatal(err)
	}
	srcText, err := p1.ExtractText()
	if err != nil {
		t.Fatal(err)
	}
	dstText, err := p2.ExtractText()
	if err != nil {
		t.Fatal(err)
	}
	if got, want := pdf.AssembleSourceText(ops), strings.Join(strings.Fields(srcText), " "); got != want {
		t.Errorf("AssembleSourceText = %q, want %q", got, want)
	}
	if got, want := pdf.AssembleDestinationText(ops), strings.Join(strings.Fields(dstText), " "); got != want {
		t.Errorf("AssembleDestinationText = %q, want %q", got, want)
	}
}

func TestComparePagesIgnoreCase(t *testing.T) {
	a := buildComparisonDoc(t, "Alpha Beta")
	b := buildComparisonDoc(t, "alpha beta")
	p1, p2 := firstPages(t, a, b)

	ops, err := pdf.ComparePages(p1, p2, pdf.ComparisonOptions{IgnoreCase: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, op := range ops {
		if op.Operation != pdf.OperationEqual {
			t.Fatalf("case-insensitive comparison reported %v %q", op.Operation, op.Text)
		}
	}
}

func TestComparePagesExtractionArea(t *testing.T) {
	a := buildComparisonDoc(t, "keep this line\nand change this one")
	b := buildComparisonDoc(t, "keep this line\nand CHANGED this one")
	p1, p2 := firstPages(t, a, b)

	// An area covering only the first line's box ([158.8, 172.8] at these
	// coordinates): the edit sits on the second line ([142, 156]), below it.
	area := pdf.Rectangle{LLX: 0, LLY: 157, URX: 400, URY: 200}
	ops, err := pdf.ComparePages(p1, p2, pdf.ComparisonOptions{ExtractionArea: &area})
	if err != nil {
		t.Fatal(err)
	}
	for _, op := range ops {
		if op.Operation != pdf.OperationEqual {
			t.Fatalf("edit outside the extraction area was reported: %v %q", op.Operation, op.Text)
		}
	}
}

func TestComparisonOptionsIncompatible(t *testing.T) {
	a := buildComparisonDoc(t, "alpha")
	b := buildComparisonDoc(t, "alpha")
	p1, p2 := firstPages(t, a, b)

	area := pdf.Rectangle{LLX: 0, LLY: 0, URX: 400, URY: 200}
	if _, err := pdf.ComparePages(p1, p2, pdf.ComparisonOptions{
		ExtractionArea: &area,
		ExcludeTables:  true,
	}); err == nil {
		t.Fatal("ExtractionArea together with ExcludeTables must be rejected")
	}
}

// ExcludeAreas1/ExcludeAreas2 drop the words inside the given regions before
// diffing. Excluding the changed word on both sides must silence the
// change; without the exclusion the same comparison must report it, so the
// test cannot pass vacuously (e.g. if the option were silently ignored).
func TestComparePagesExcludeAreas(t *testing.T) {
	a := buildComparisonDoc(t, "total is 100 euro")
	b := buildComparisonDoc(t, "total is 200 euro")
	p1, p2 := firstPages(t, a, b)

	baseline, err := pdf.ComparePages(p1, p2)
	if err != nil {
		t.Fatal(err)
	}
	var baselineChanged bool
	for _, op := range baseline {
		if op.Operation != pdf.OperationEqual {
			baselineChanged = true
		}
	}
	if !baselineChanged {
		t.Fatal("baseline comparison (no exclusion) reported no change; the fixture is broken")
	}

	m1, err := p1.SearchText("100")
	if err != nil {
		t.Fatal(err)
	}
	m2, err := p2.SearchText("200")
	if err != nil {
		t.Fatal(err)
	}
	if len(m1) != 1 || len(m2) != 1 {
		t.Fatalf("got %d/%d matches for the changed word, want 1/1", len(m1), len(m2))
	}

	ops, err := pdf.ComparePages(p1, p2, pdf.ComparisonOptions{
		ExcludeAreas1: []pdf.Rectangle{m1[0].Rect},
		ExcludeAreas2: []pdf.Rectangle{m2[0].Rect},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, op := range ops {
		if op.Operation != pdf.OperationEqual {
			t.Fatalf("edit inside an excluded area was reported: %v %q", op.Operation, op.Text)
		}
	}
}

// buildComparisonTableDoc writes a small ruled table (SetBorder on both the
// table and its default cell border, so the lattice detector sees a grid)
// with one cell carrying cellText, and reopens the saved document.
func buildComparisonTableDoc(t *testing.T, cellText string) *pdf.Document {
	t.Helper()
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	page, err := doc.Page(1)
	if err != nil {
		t.Fatalf("page: %v", err)
	}
	tbl := pdf.NewTable().
		SetColumnWidths([]float64{120, 120, 120}).
		SetBorder(pdf.BorderInfo{Sides: pdf.BorderSideAll, Width: 1}).
		SetDefaultCellBorder(pdf.BorderInfo{Sides: pdf.BorderSideAll, Width: 1})
	tbl.AddRow().AddCells("Item", "Qty", "Price")
	tbl.AddRow().AddCells(cellText, "7", "9.10")
	if _, err := page.AddTable(tbl, pdf.Rectangle{LLX: 60, LLY: 500, URX: 420, URY: 700}); err != nil {
		t.Fatalf("add table: %v", err)
	}
	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatalf("write: %v", err)
	}
	re, err := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	return re
}

// ExcludeTables drops the words inside tables the TableAbsorber detects. Two
// documents differing only inside a ruled table cell must report no changes
// with ExcludeTables set, and must report the change without it — otherwise
// a failure to detect the table at all would make this pass vacuously.
func TestComparePagesExcludeTables(t *testing.T) {
	a := buildComparisonTableDoc(t, "Pears")
	b := buildComparisonTableDoc(t, "Plums")
	p1, p2 := firstPages(t, a, b)

	tables := detectOn(t, a)
	if len(tables) != 1 {
		t.Fatalf("table detection found %d tables in the comparison fixture, want 1 (the fixture must produce a detectable table or this test passes vacuously)", len(tables))
	}

	baseline, err := pdf.ComparePages(p1, p2)
	if err != nil {
		t.Fatal(err)
	}
	var baselineChanged bool
	for _, op := range baseline {
		if op.Operation != pdf.OperationEqual {
			baselineChanged = true
		}
	}
	if !baselineChanged {
		t.Fatal("baseline comparison (no exclusion) reported no change; the fixture is broken")
	}

	ops, err := pdf.ComparePages(p1, p2, pdf.ComparisonOptions{ExcludeTables: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, op := range ops {
		if op.Operation != pdf.OperationEqual {
			t.Fatalf("ExcludeTables left a change reported: %v %q", op.Operation, op.Text)
		}
	}
}

func TestComparePagesNilPage(t *testing.T) {
	a := buildComparisonDoc(t, "alpha")
	p1, err := a.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pdf.ComparePages(p1, nil); err == nil {
		t.Fatal("a nil page must be rejected")
	}
}

func TestCompareDocumentsPageByPage(t *testing.T) {
	a := buildComparisonDoc(t, "page one alpha", "page two beta")
	b := buildComparisonDoc(t, "page one alpha", "page two gamma")

	res, err := pdf.CompareDocumentsPageByPage(a, b)
	if err != nil {
		t.Fatal(err)
	}
	if !res.HasChanges() {
		t.Fatal("HasChanges = false, want true")
	}
	if got := res.PageOperations(1); len(got) == 0 {
		t.Fatal("page 1 reported no operations at all")
	} else {
		for _, op := range got {
			if op.Operation != pdf.OperationEqual {
				t.Errorf("page 1 is unchanged but reported %v %q", op.Operation, op.Text)
			}
		}
	}
	var changed bool
	for _, op := range res.PageOperations(2) {
		if op.Operation == pdf.OperationInsert && op.Text == "gamma" {
			changed = true
		}
	}
	if !changed {
		t.Errorf("page 2 operations = %+v, want an insertion of %q", res.PageOperations(2), "gamma")
	}
	st := res.Statistics()
	if st.InsertedWords != 1 || st.DeletedWords != 1 {
		t.Errorf("statistics = %+v, want one word inserted and one deleted", st)
	}
	if len(st.ChangedPages) != 1 || st.ChangedPages[0] != 2 {
		t.Errorf("ChangedPages = %v, want [2]", st.ChangedPages)
	}
}

func TestCompareDocumentsIdenticalHasNoChanges(t *testing.T) {
	a := buildComparisonDoc(t, "alpha beta", "gamma delta")
	b := buildComparisonDoc(t, "alpha beta", "gamma delta")

	res, err := pdf.CompareDocumentsPageByPage(a, b)
	if err != nil {
		t.Fatal(err)
	}
	if res.HasChanges() {
		t.Fatalf("identical documents reported changes: %+v", res.Operations())
	}
	flat, err := pdf.CompareFlatDocuments(a, b)
	if err != nil {
		t.Fatal(err)
	}
	if flat.HasChanges() {
		t.Fatalf("identical documents reported changes in flat mode: %+v", flat.Operations())
	}
}

func TestCompareDocumentsPageByPageExtraPage(t *testing.T) {
	a := buildComparisonDoc(t, "alpha")
	b := buildComparisonDoc(t, "alpha", "beta gamma")

	res, err := pdf.CompareDocumentsPageByPage(a, b)
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, op := range res.PageOperations(2) {
		if op.Operation == pdf.OperationInsert && op.Text == "beta gamma" {
			found = true
		}
	}
	if !found {
		t.Errorf("the added page's text = %+v, want one insertion of %q", res.PageOperations(2), "beta gamma")
	}
}

// Text that moved to another page reads as a move in flat mode: the words are
// equal, only their page changed.
func TestCompareFlatDocumentsAcrossPages(t *testing.T) {
	a := buildComparisonDoc(t, "alpha beta gamma delta", "")
	b := buildComparisonDoc(t, "alpha beta", "gamma delta")

	res, err := pdf.CompareFlatDocuments(a, b)
	if err != nil {
		t.Fatal(err)
	}
	for _, op := range res.Operations() {
		if op.Operation != pdf.OperationEqual {
			t.Fatalf("moving text across a page break reported %v %q", op.Operation, op.Text)
		}
	}
}

// CompareFlatDocuments leaves the per-page index nil, so PageOperations must
// fall back to scanning Operations() by page instead of indexing into it.
func TestPageOperationsOnFlatResult(t *testing.T) {
	a := buildComparisonDoc(t, "alpha beta", "gamma delta")
	b := buildComparisonDoc(t, "alpha beta", "gamma epsilon")

	res, err := pdf.CompareFlatDocuments(a, b)
	if err != nil {
		t.Fatal(err)
	}
	got := res.PageOperations(2)
	if len(got) == 0 {
		t.Fatal("PageOperations(2) on a flat-mode result returned nothing")
	}
	var found bool
	for _, op := range got {
		if op.Operation == pdf.OperationInsert && op.Text == "epsilon" {
			found = true
		}
	}
	if !found {
		t.Errorf("page 2 operations = %+v, want an insertion of %q", got, "epsilon")
	}
	// Page 1 is unchanged, so it must not be pulled in by the scan.
	for _, op := range res.PageOperations(1) {
		if op.Operation != pdf.OperationEqual {
			t.Errorf("page 1 is unchanged but PageOperations(1) reported %v %q", op.Operation, op.Text)
		}
	}
}

func TestCompareDocumentsNil(t *testing.T) {
	a := buildComparisonDoc(t, "alpha")
	if _, err := pdf.CompareDocumentsPageByPage(a, nil); err == nil {
		t.Fatal("a nil document must be rejected")
	}
	if _, err := pdf.CompareFlatDocuments(nil, a); err == nil {
		t.Fatal("a nil document must be rejected")
	}
}

func TestSaveMarkupAnnotatesTheDestination(t *testing.T) {
	a := buildComparisonDoc(t, "total is 100 euro")
	b := buildComparisonDoc(t, "total is 200 euro")

	res, err := pdf.CompareDocumentsPageByPage(a, b)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join("result_files", "TestSaveMarkupAnnotatesTheDestination")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "markup.pdf")
	if err := res.SaveMarkup(out); err != nil {
		t.Fatal(err)
	}

	doc, err := pdf.Open(out)
	if err != nil {
		t.Fatal(err)
	}
	page, err := doc.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	matches, err := page.SearchText("200")
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 {
		t.Fatalf("search found %d matches for %q, want 1", len(matches), "200")
	}
	wantRect := matches[0].Rect
	wantColor := pdf.Color{R: 0.20, G: 0.72, B: 0.35, A: 1} // the default insert colour

	var highlights, carets int
	for _, ann := range page.Annotations().All() {
		switch ann.AnnotationType() {
		case pdf.AnnotationTypeHighlight:
			highlights++
			if ann.Contents() != "200" {
				t.Errorf("highlight contents = %q, want %q", ann.Contents(), "200")
			}
			if ann.Title() != "Comparison" {
				t.Errorf("highlight title = %q, want %q", ann.Title(), "Comparison")
			}
			h, ok := ann.(*pdf.HighlightAnnotation)
			if !ok {
				t.Fatalf("highlight annotation has concrete type %T, want *pdf.HighlightAnnotation", ann)
			}
			if c := h.Color(); c == nil || !closeColor(*c, wantColor) {
				t.Errorf("highlight colour = %+v, want %+v", c, wantColor)
			}
			quads := h.QuadPoints()
			if len(quads) != 1 {
				t.Fatalf("highlight quad points = %d, want 1 (one rectangle in the run)", len(quads))
			}
			// The quad must sit over the changed word: compare its corners
			// against an independent oracle, SearchText's rectangle for "200".
			q := quads[0]
			const tol = 0.5
			if abs(q.X1-wantRect.LLX) > tol || abs(q.Y1-wantRect.URY) > tol ||
				abs(q.X2-wantRect.URX) > tol || abs(q.Y2-wantRect.URY) > tol ||
				abs(q.X3-wantRect.LLX) > tol || abs(q.Y3-wantRect.LLY) > tol ||
				abs(q.X4-wantRect.URX) > tol || abs(q.Y4-wantRect.LLY) > tol {
				t.Errorf("highlight quad = %+v, want corners of %+v", q, wantRect)
			}
		case pdf.AnnotationTypeCaret:
			carets++
			if ann.Contents() != "100" {
				t.Errorf("caret contents = %q, want the deleted text %q", ann.Contents(), "100")
			}
		}
	}
	if highlights != 1 || carets != 1 {
		t.Fatalf("got %d highlights and %d carets, want 1 and 1", highlights, carets)
	}
}

// closeColor compares two colours within a small float tolerance.
func closeColor(a, b pdf.Color) bool {
	const tol = 0.01
	return abs(a.R-b.R) <= tol && abs(a.G-b.G) <= tol && abs(a.B-b.B) <= tol && abs(a.A-b.A) <= tol
}

func TestSaveMarkupSourceSideStrikesOut(t *testing.T) {
	a := buildComparisonDoc(t, "total is 100 euro")
	b := buildComparisonDoc(t, "total is 200 euro")

	res, err := pdf.CompareDocumentsPageByPage(a, b)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := res.WriteMarkup(&buf, pdf.DiffMarkupOptions{Side: pdf.DiffMarkupSource}); err != nil {
		t.Fatal(err)
	}
	doc, err := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	page, err := doc.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	var strikes, carets int
	for _, ann := range page.Annotations().All() {
		switch ann.AnnotationType() {
		case pdf.AnnotationTypeStrikeOut:
			strikes++
			if ann.Contents() != "100" {
				t.Errorf("strike-out contents = %q, want %q", ann.Contents(), "100")
			}
		case pdf.AnnotationTypeCaret:
			// The insertion has no text on the source side, so it is marked
			// with a caret carrying the inserted text — the paired half of
			// the replacement, mirroring the strike-out asserted above.
			carets++
			if ann.Contents() != "200" {
				t.Errorf("caret contents = %q, want the inserted text %q", ann.Contents(), "200")
			}
		}
	}
	if strikes != 1 {
		t.Fatalf("got %d strike-outs, want 1", strikes)
	}
	if carets != 1 {
		t.Fatalf("got %d carets, want 1 (the paired insertion on the source side)", carets)
	}
}

// Marking up must not touch the documents the caller handed in.
func TestSaveMarkupLeavesInputsAlone(t *testing.T) {
	a := buildComparisonDoc(t, "alpha beta")
	b := buildComparisonDoc(t, "alpha gamma")

	res, err := pdf.CompareDocumentsPageByPage(a, b)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := res.WriteMarkup(&buf); err != nil {
		t.Fatal(err)
	}
	for _, doc := range []*pdf.Document{a, b} {
		page, err := doc.Page(1)
		if err != nil {
			t.Fatal(err)
		}
		if n := page.Annotations().Count(); n != 0 {
			t.Errorf("input document gained %d annotations", n)
		}
	}
}

func TestSaveMarkupFlatten(t *testing.T) {
	a := buildComparisonDoc(t, "alpha beta")
	b := buildComparisonDoc(t, "alpha gamma")

	res, err := pdf.CompareDocumentsPageByPage(a, b)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := res.WriteMarkup(&buf, pdf.DiffMarkupOptions{Flatten: true}); err != nil {
		t.Fatal(err)
	}
	doc, err := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	page, err := doc.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	if n := page.Annotations().Count(); n != 0 {
		t.Fatalf("flattened output still carries %d annotations", n)
	}
}

// The highlight must actually paint: its appearance stream is generated, so
// our own renderer shows it too.
func TestSaveMarkupRendersDifferently(t *testing.T) {
	a := buildComparisonDoc(t, "alpha beta")
	b := buildComparisonDoc(t, "alpha gamma")

	res, err := pdf.CompareDocumentsPageByPage(a, b)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := res.WriteMarkup(&buf, pdf.DiffMarkupOptions{Flatten: true}); err != nil {
		t.Fatal(err)
	}
	marked, err := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}

	before, err := renderPNG(t, b)
	if err != nil {
		t.Fatal(err)
	}
	after, err := renderPNG(t, marked)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(before, after) {
		t.Fatal("the marked-up page renders identically to the unmarked one")
	}
}

func renderPNG(t *testing.T, doc *pdf.Document) ([]byte, error) {
	t.Helper()
	page, err := doc.Page(1)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := page.RenderPNG(&buf, pdf.RenderOptions{DPI: 72}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Arabic is drawn in visual order; comparison must work on the logical order
// extraction restores, so the changed word is the one that is reported.
func TestComparePagesArabic(t *testing.T) {
	build := func(text string) *pdf.Document {
		doc := pdf.NewDocument(400, 200)
		font, err := doc.LoadFont("testdata/DejaVuSans.ttf")
		if err != nil {
			t.Fatalf("load font: %v", err)
		}
		page, err := doc.Page(1)
		if err != nil {
			t.Fatal(err)
		}
		if err := page.AddText(text, pdf.TextStyle{Font: font, Size: 18},
			pdf.Rectangle{LLX: 20, LLY: 80, URX: 380, URY: 140}); err != nil {
			t.Fatalf("add text: %v", err)
		}
		var buf bytes.Buffer
		if _, err := doc.WriteTo(&buf); err != nil {
			t.Fatal(err)
		}
		re, err := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
		if err != nil {
			t.Fatal(err)
		}
		return re
	}

	a := build("\u0627\u0644\u0633\u0644\u0627\u0645 \u0639\u0644\u064a\u0643\u0645")       // as-salamu alaykum
	b := build("\u0627\u0644\u0633\u0644\u0627\u0645 \u0644\u0644\u0639\u0627\u0644\u0645") // as-salamu lil-alam
	p1, p2 := firstPages(t, a, b)

	ops, err := pdf.ComparePages(p1, p2)
	if err != nil {
		t.Fatal(err)
	}
	var equal, changed int
	for _, op := range ops {
		switch op.Operation {
		case pdf.OperationEqual:
			equal += len(strings.Fields(op.Text))
		default:
			changed += len(strings.Fields(op.Text))
		}
	}
	if equal != 1 {
		t.Errorf("got %d unchanged words, want the shared first word", equal)
	}
	if changed != 2 {
		t.Errorf("got %d changed words, want one deleted and one inserted", changed)
	}

	// The word-count assertions above would also pass if the tokenizer read
	// glyphs in visual (rendering) order instead of logical order — the
	// shared word is a common suffix either way. Pin logical order directly:
	// under logical order the shared word is the *first* thing typed, so it
	// must be the first operation reported.
	if len(ops) == 0 {
		t.Fatal("comparison produced no operations")
	}
	wantFirst := "السلام" // as-salamu
	if ops[0].Operation != pdf.OperationEqual || ops[0].Text != wantFirst {
		t.Fatalf("first operation = %+v, want an equal run carrying %q (logical order)", ops[0], wantFirst)
	}
}
