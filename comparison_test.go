// SPDX-License-Identifier: MIT

package asposepdf_test

import (
	"bytes"
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

func TestCompareDocumentsNil(t *testing.T) {
	a := buildComparisonDoc(t, "alpha")
	if _, err := pdf.CompareDocumentsPageByPage(a, nil); err == nil {
		t.Fatal("a nil document must be rejected")
	}
	if _, err := pdf.CompareFlatDocuments(nil, a); err == nil {
		t.Fatal("a nil document must be rejected")
	}
}
