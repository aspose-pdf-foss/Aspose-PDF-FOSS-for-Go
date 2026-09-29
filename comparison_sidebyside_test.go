// SPDX-License-Identifier: MIT

package asposepdf_test

import (
	"bytes"
	"strings"
	"testing"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

func sbsTextPage(t *testing.T, w, h float64, text string) *pdf.Page {
	t.Helper()
	doc := pdf.NewDocument(w, h)
	p, err := doc.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.AddText(text, pdf.TextStyle{Font: pdf.FontHelvetica, Size: 12},
		pdf.Rectangle{LLX: 20, LLY: h - 40, URX: w - 20, URY: h - 10}); err != nil {
		t.Fatal(err)
	}
	return p
}

// TestSideBySideComparePagesNoChanges: identical pages report no changes and
// still produce a valid, openable one-page spread.
func TestSideBySideComparePagesNoChanges(t *testing.T) {
	p1 := sbsTextPage(t, 300, 150, "The quick brown fox jumps over the lazy dog.")
	p2 := sbsTextPage(t, 300, 150, "The quick brown fox jumps over the lazy dog.")

	var buf bytes.Buffer
	res, err := pdf.WriteSideBySidePages(p1, p2, &buf)
	if err != nil {
		t.Fatal(err)
	}
	if res.HasChanges() {
		t.Error("HasChanges() = true for identical pages")
	}
	out, err := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("reopen spread: %v", err)
	}
	if out.PageCount() != 1 {
		t.Errorf("spread has %d pages, want 1", out.PageCount())
	}
}

// TestSideBySideComparePagesWithChanges is the decisive test: the changed
// word from BOTH sides must be present and readable on the SAME output page
// (proving both panes were actually stamped, not just one, and that the
// Flatten fix keeps the text itself — not only the markup annotations —
// intact through the Form-XObject stamping path), and the markup-critical
// fix (Flatten: true before PdfPageStamp) must have actually taken visible
// effect: reopening and rendering must show highlight/strikeout, which this
// test checks indirectly by confirming the page has annotations of neither
// kind (they were flattened away) while the text survives.
func TestSideBySideComparePagesWithChanges(t *testing.T) {
	p1 := sbsTextPage(t, 300, 150, "The quick brown fox jumps over the lazy dog.")
	p2 := sbsTextPage(t, 300, 150, "The quick brown fox leaps over the lazy dog.")

	var buf bytes.Buffer
	res, err := pdf.WriteSideBySidePages(p1, p2, &buf)
	if err != nil {
		t.Fatal(err)
	}
	if !res.HasChanges() {
		t.Fatal("HasChanges() = false for a changed word")
	}

	out, err := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("reopen spread: %v", err)
	}
	if out.PageCount() != 1 {
		t.Fatalf("spread has %d pages, want 1", out.PageCount())
	}
	page, err := out.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	text, err := page.ExtractText()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "jumps") {
		t.Errorf("spread page text = %q, missing the source word %q", text, "jumps")
	}
	if !strings.Contains(text, "leaps") {
		t.Errorf("spread page text = %q, missing the destination word %q", text, "leaps")
	}
	if !strings.Contains(text, "Before") || !strings.Contains(text, "After") {
		t.Errorf("spread page text = %q, missing the Before/After pane labels", text)
	}

	// Flatten: true (comparison_sidebyside.go's markupOptions) bakes the
	// highlight/strikeout/caret annotations into the STAMPED source pages'
	// content before they're imported as Form XObjects, per
	// (*AnnotationCollection).Flatten's contract. The spread's OWN page (the
	// one PdfPageStamp drew onto) never had annotations added to it directly
	// in the first place, so checking it has none here is a weak signal by
	// itself -- the real proof that flattening worked is that the diff is
	// visually present despite no live annotation carrying it; a regression
	// where Flatten were dropped would still pass this specific assertion
	// (annotation-free) while silently losing the markup, which is exactly
	// why TestSideBySideComparePagesWithChanges above checks for the words
	// AND the DiffMarkupOptionsFlattenRequired test below checks the
	// mechanism directly.
	if n := page.Annotations().Count(); n != 0 {
		t.Errorf("spread page has %d annotations, want 0 (PdfPageStamp does not import /Annots — see markupOptions' Flatten comment)", n)
	}
}

// TestSideBySideMarkupIsVisible renders the finished spread and checks for
// the actual highlight/strikeout colours in the pixels — not just that the
// underlying words survive (they would either way, since AddText's glyphs
// are page content, independent of any annotation) but that the VISUAL
// diff markup specifically made it through PdfPageStamp's Form-XObject
// placement. This is the test that actually catches a dropped
// DiffMarkupOptions.Flatten: true (TestSideBySideComparePagesWithChanges's
// annotation-count check does not, since PdfPageStamp never imports /Annots
// either way, flattened or not — confirmed by temporarily reverting the
// Flatten:true fix and rerunning the suite during development).
func TestSideBySideMarkupIsVisible(t *testing.T) {
	doc1 := pdf.NewDocument(300, 150)
	p1, err := doc1.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	if err := p1.AddText("alpha bravo charlie", pdf.TextStyle{Font: pdf.FontHelvetica, Size: 14},
		pdf.Rectangle{LLX: 20, LLY: 100, URX: 280, URY: 130}); err != nil {
		t.Fatal(err)
	}
	doc2 := pdf.NewDocument(300, 150)
	p2, err := doc2.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	if err := p2.AddText("alpha delta charlie", pdf.TextStyle{Font: pdf.FontHelvetica, Size: 14},
		pdf.Rectangle{LLX: 20, LLY: 100, URX: 280, URY: 130}); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	if _, err := pdf.WriteSideBySidePages(p1, p2, &buf); err != nil {
		t.Fatal(err)
	}
	out, err := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	page, err := out.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	size, err := page.Size()
	if err != nil {
		t.Fatal(err)
	}
	img, err := page.RenderImage(pdf.RenderOptions{DPI: 150})
	if err != nil {
		t.Fatal(err)
	}
	b := img.Bounds()
	scaleX := float64(b.Dx()) / size.Width

	// The gutter sits at the page midpoint's neighbourhood regardless of the
	// exact margin/gutter constants (both panes are the same 300pt width),
	// so splitting the render at the horizontal midpoint reliably separates
	// the "Before" (source, left) pane from the "After" (destination, right)
	// one without depending on comparison_sidebyside.go's layout constants.
	midX := int(size.Width / 2 * scaleX)

	green := [3]int{51, 184, 89} // DiffMarkupOptions' default InsertColor (0.20,0.72,0.35) * 255
	red := [3]int{224, 56, 56}   // default DeleteColor (0.88,0.22,0.22) * 255

	hasColor := func(x0, x1 int, want [3]int) bool {
		for y := b.Min.Y; y < b.Max.Y; y++ {
			for x := x0; x < x1; x++ {
				r, g, bl, _ := img.At(x, y).RGBA()
				rr, gg, bb := int(r>>8), int(g>>8), int(bl>>8)
				if sbsAbsDiff(rr, want[0]) <= 20 && sbsAbsDiff(gg, want[1]) <= 20 && sbsAbsDiff(bb, want[2]) <= 20 {
					return true
				}
			}
		}
		return false
	}

	if !hasColor(midX, b.Max.X, green) {
		t.Error("no highlight-green pixel found in the 'After' (destination) pane — the insertion markup was lost")
	}
	if !hasColor(b.Min.X, midX, red) {
		t.Error("no strikeout-red pixel found in the 'Before' (source) pane — the deletion markup was lost")
	}
}

func sbsAbsDiff(a, b int) int {
	if a < b {
		return b - a
	}
	return a - b
}

// TestSideBySideFlattenIsRequired is the mutation-style regression test for
// the bug found while designing this feature: PdfPageStamp places a page via
// importPageAsXObject, which captures content + /Resources only, never
// /Annots (imposition.go). Without DiffMarkupOptions.Flatten: true, the
// highlight/strikeout/caret markup WriteMarkup draws as real annotations
// would be silently dropped the moment the marked-up page is stamped. This
// test proves the drop actually happens on the un-flattened path, so the
// fix (markupOptions always setting Flatten: true) is not accidentally
// correct.
func TestSideBySideFlattenIsRequired(t *testing.T) {
	doc1 := pdf.NewDocument(300, 150)
	p1, err := doc1.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	if err := p1.AddText("alpha bravo charlie", pdf.TextStyle{Font: pdf.FontHelvetica, Size: 12},
		pdf.Rectangle{LLX: 20, LLY: 100, URX: 280, URY: 130}); err != nil {
		t.Fatal(err)
	}
	doc2 := pdf.NewDocument(300, 150)
	p2, err := doc2.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	if err := p2.AddText("alpha delta charlie", pdf.TextStyle{Font: pdf.FontHelvetica, Size: 12},
		pdf.Rectangle{LLX: 20, LLY: 100, URX: 280, URY: 130}); err != nil {
		t.Fatal(err)
	}

	result, err := pdf.CompareDocumentsPageByPage(doc1, doc2)
	if err != nil {
		t.Fatal(err)
	}

	// Without Flatten: mark up, stamp as a Form XObject, and confirm the
	// destination-side highlight is gone (the bug this test pins).
	var unflattened bytes.Buffer
	if err := result.WriteMarkup(&unflattened, pdf.DiffMarkupOptions{Side: pdf.DiffMarkupDestination}); err != nil {
		t.Fatal(err)
	}
	markedDoc, err := pdf.OpenStream(bytes.NewReader(unflattened.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	markedPage, err := markedDoc.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	if markedPage.Annotations().Count() == 0 {
		t.Fatal("setup: expected the un-flattened marked-up copy to carry the highlight annotation")
	}

	stampTarget := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	stamp, err := pdf.NewPdfPageStamp(markedDoc, 1)
	if err != nil {
		t.Fatal(err)
	}
	stamp.Rect = pdf.Rectangle{LLX: 0, LLY: 0, URX: 300, URY: 150}
	tp, err := stampTarget.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	if err := tp.AddStamp(stamp); err != nil {
		t.Fatal(err)
	}
	if n := tp.Annotations().Count(); n != 0 {
		t.Fatalf("stamped page has %d annotations, want 0 -- PdfPageStamp is not supposed to import /Annots; if this now fails, importPageAsXObject's behaviour changed and markupOptions' Flatten:true workaround may be obsolete", n)
	}
}

// TestSideBySideCompareDocumentsPageCount: mismatched document lengths still
// produce max(len1, len2) spread pages, and each pane shows the right side's
// content only where it exists.
func TestSideBySideCompareDocumentsPageCount(t *testing.T) {
	doc1 := pdf.NewDocument(300, 150)
	p1a, _ := doc1.Page(1)
	if err := p1a.AddText("first page one", pdf.TextStyle{Font: pdf.FontHelvetica, Size: 12},
		pdf.Rectangle{LLX: 20, LLY: 100, URX: 280, URY: 130}); err != nil {
		t.Fatal(err)
	}
	if err := doc1.AddBlankPage(300, 150); err != nil {
		t.Fatal(err)
	}
	p1b, _ := doc1.Page(2)
	if err := p1b.AddText("first page two", pdf.TextStyle{Font: pdf.FontHelvetica, Size: 12},
		pdf.Rectangle{LLX: 20, LLY: 100, URX: 280, URY: 130}); err != nil {
		t.Fatal(err)
	}

	doc2 := pdf.NewDocument(300, 150) // only one page
	p2a, _ := doc2.Page(1)
	if err := p2a.AddText("second page one", pdf.TextStyle{Font: pdf.FontHelvetica, Size: 12},
		pdf.Rectangle{LLX: 20, LLY: 100, URX: 280, URY: 130}); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	res, err := pdf.WriteSideBySideDocuments(doc1, doc2, &buf)
	if err != nil {
		t.Fatal(err)
	}
	if !res.HasChanges() {
		t.Error("HasChanges() = false, want true (doc1 has an extra page)")
	}
	out, err := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if out.PageCount() != 2 {
		t.Fatalf("spread has %d pages, want 2 (max(2,1))", out.PageCount())
	}
	p2, err := out.Page(2)
	if err != nil {
		t.Fatal(err)
	}
	text2, err := p2.ExtractText()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text2, "first page two") {
		t.Errorf("spread page 2 = %q, missing doc1's page 2 text (the source-only pane)", text2)
	}
	if strings.Contains(text2, "second page") {
		t.Errorf("spread page 2 = %q, should not carry doc2 content (doc2 has no page 2)", text2)
	}
}

// TestSideBySideComparePagesNilPage covers the nil-page guard.
func TestSideBySideComparePagesNilPage(t *testing.T) {
	if _, err := pdf.WriteSideBySidePages(nil, nil, &bytes.Buffer{}); err == nil {
		t.Error("WriteSideBySidePages(nil, nil, ...) = nil error, want an error")
	}
}

// TestSideBySideDocsResultEmbedsComparisonResult: SideBySideDocsComparisonResult
// must expose the full phase-1 ComparisonResult surface (Statistics,
// Operations, PageOperations), not just HasChanges.
func TestSideBySideDocsResultEmbedsComparisonResult(t *testing.T) {
	doc1 := pdf.NewDocument(300, 150)
	p1, _ := doc1.Page(1)
	if err := p1.AddText("one two three", pdf.TextStyle{Font: pdf.FontHelvetica, Size: 12},
		pdf.Rectangle{LLX: 20, LLY: 100, URX: 280, URY: 130}); err != nil {
		t.Fatal(err)
	}
	doc2 := pdf.NewDocument(300, 150)
	p2, _ := doc2.Page(1)
	if err := p2.AddText("one two four", pdf.TextStyle{Font: pdf.FontHelvetica, Size: 12},
		pdf.Rectangle{LLX: 20, LLY: 100, URX: 280, URY: 130}); err != nil {
		t.Fatal(err)
	}

	res, err := pdf.SideBySideCompareDocuments(doc1, doc2, tmpOut(t, "sbs_stats.pdf"))
	if err != nil {
		t.Fatal(err)
	}
	stats := res.Statistics()
	if stats.InsertedWords != 1 || stats.DeletedWords != 1 || stats.EqualWords != 2 {
		t.Errorf("Statistics() = %+v, want {EqualWords:2 InsertedWords:1 DeletedWords:1 ...}", stats)
	}
}

func tmpOut(t *testing.T, name string) string {
	t.Helper()
	return t.TempDir() + "/" + name
}
