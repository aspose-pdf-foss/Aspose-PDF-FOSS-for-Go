// SPDX-License-Identifier: MIT

package asposepdf_test

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
	"time"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

// renderPage1 renders page 1 to PNG bytes at a fixed DPI for comparison.
func renderPage1(t *testing.T, doc *pdf.Document) []byte {
	t.Helper()
	p, err := doc.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := p.RenderPNG(&buf, pdf.RenderOptions{DPI: 96}); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// buildDoc creates a small multi-page document with text, a table and a drawn
// shape — representative content for the optimizer to run over.
func buildDoc(t *testing.T) *pdf.Document {
	t.Helper()
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	for i := 0; i < 3; i++ {
		if i > 0 {
			if err := doc.AddBlankPageFromFormat(pdf.PageFormatA4); err != nil {
				t.Fatal(err)
			}
		}
		p, err := doc.Page(i + 1)
		if err != nil {
			t.Fatal(err)
		}
		if err := p.AddText("Optimization test page with some flowing text.", pdf.TextStyle{Size: 12}, pdf.Rectangle{LLX: 50, LLY: 700, URX: 545, URY: 780}); err != nil {
			t.Fatal(err)
		}
		if err := p.DrawRectangle(pdf.Rectangle{LLX: 50, LLY: 600, URX: 300, URY: 660}, pdf.ShapeStyle{FillColor: &pdf.Color{R: 0.8, G: 0.9, B: 1, A: 1}}); err != nil {
			t.Fatal(err)
		}
	}
	return doc
}

// TestOptimizeLossless: Optimize must not change the rendered output or the
// extractable text, and the result must still open and validate.
func TestOptimizeLossless(t *testing.T) {
	before := renderPage1(t, buildDoc(t))

	doc := buildDoc(t)
	res, err := doc.Optimize(pdf.DefaultOptimizationOptions())
	if err != nil {
		t.Fatal(err)
	}
	_ = res // counts vary with content; correctness is what matters here

	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	reopened, err := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if got := renderPage1(t, reopened); !bytes.Equal(got, before) {
		t.Error("Optimize changed the rendered output (not lossless)")
	}
	pages, err := reopened.ExtractText()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(pages, "\n"), "Optimization test page") {
		t.Error("text lost after Optimize")
	}
}

// TestOptimizeCompressesUncompressedContent: a document whose content stream is
// stored uncompressed shrinks after Optimize, and stays lossless.
func TestOptimizeCompressesUncompressedContent(t *testing.T) {
	// Build, save, and reopen so the content stream comes back as a parsed
	// (uncompressed, since our writer wrote it Flate — reopen keeps it raw
	// only if uncompressed). To guarantee an uncompressed input, hand-craft a
	// minimal PDF with a plain content stream.
	raw := "%PDF-1.4\n" +
		"1 0 obj<</Type/Catalog/Pages 2 0 R>>endobj\n" +
		"2 0 obj<</Type/Pages/Kids[3 0 R]/Count 1>>endobj\n" +
		"3 0 obj<</Type/Page/Parent 2 0 R/MediaBox[0 0 200 200]/Contents 4 0 R/Resources<</Font<</F1 5 0 R>>>>>>endobj\n"
	content := "BT /F1 12 Tf 20 100 Td (" + strings.Repeat("Hello world. ", 30) + ") Tj ET"
	raw += "4 0 obj<</Length " + itoaTest(len(content)) + ">>stream\n" + content + "\nendstream endobj\n"
	raw += "5 0 obj<</Type/Font/Subtype/Type1/BaseFont/Helvetica>>endobj\n"
	// A minimal (invalid-offset) xref; the parser reconstructs from object scan.
	raw += "trailer<</Root 1 0 R/Size 6>>\n%%EOF"

	doc, err := pdf.OpenStream(strings.NewReader(raw))
	if err != nil {
		t.Fatalf("open crafted pdf: %v", err)
	}
	var plain bytes.Buffer
	if _, err := doc.WriteTo(&plain); err != nil {
		t.Fatal(err)
	}

	doc2, _ := pdf.OpenStream(strings.NewReader(raw))
	res, err := doc2.Optimize(pdf.OptimizationOptions{CompressStreams: true})
	if err != nil {
		t.Fatal(err)
	}
	if res.CompressedStreams < 1 {
		t.Fatalf("expected at least one stream compressed, got %d", res.CompressedStreams)
	}
	var packed bytes.Buffer
	if _, err := doc2.WriteTo(&packed); err != nil {
		t.Fatal(err)
	}
	if packed.Len() >= plain.Len() {
		t.Errorf("compressed output %d not smaller than plain %d", packed.Len(), plain.Len())
	}
	// Lossless: text survives.
	reopened, err := pdf.OpenStream(bytes.NewReader(packed.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	pages, _ := reopened.ExtractText()
	if !strings.Contains(strings.Join(pages, ""), "Hello world.") {
		t.Error("text lost after compression")
	}
}

// buildManyAddTextDoc reproduces GitHub issue #9's repro shape: 20 pages,
// 200 AddText calls each. Every call replaces a page's /Contents with a
// fresh object holding the whole concatenated content
// ((*Page).appendToContentStream), leaving every earlier copy dead in the
// object table — the root cause the pre-pass in Optimize's doc comment
// addresses.
func buildManyAddTextDoc(t testing.TB) *pdf.Document {
	t.Helper()
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	font, err := doc.LoadFont("testdata/DejaVuSans.ttf")
	if err != nil {
		t.Fatal(err)
	}
	for p := 1; p <= 20; p++ {
		if p > 1 {
			if err := doc.AddBlankPageFromFormat(pdf.PageFormatA4); err != nil {
				t.Fatal(err)
			}
		}
		page, err := doc.Page(p)
		if err != nil {
			t.Fatal(err)
		}
		for i := 0; i < 200; i++ {
			y, x := 800-float64(i%50)*15, 50+float64(i/50)*120
			if err := page.AddText(fmt.Sprintf("cell %d/%d", p, i), pdf.TextStyle{Font: font, Size: 8},
				pdf.Rectangle{LLX: x, LLY: y - 12, URX: x + 110, URY: y}); err != nil {
				t.Fatal(err)
			}
		}
	}
	return doc
}

// TestOptimizeManyAddTextCallsStaysFast is a loose-timing regression guard
// for the performance side of GitHub issue #9: TestOptimizeRemovedObjects
// CountsBothPasses (optimize_internal_test.go) only pins the RemovedObjects
// accumulation arithmetic, not that the pre-pass actually runs before
// RemoveDuplicateStreams/CompressStreams get to it — a regression that
// deleted the pre-pass block entirely while leaving the final pass's +=
// untouched would keep that test green (confirmed by mutation-testing during
// review). This asserts Optimize finishes within a generous bound on the
// issue's own reproduction shape: on the machine this was written on, the
// fix measures ~70ms and the regressed code ~2.5s: a 2s budget leaves wide
// margin on both sides (room for a much slower CI machine on the fast path,
// while still falling well short of the slow path) rather than trying to
// assert a precise number.
func TestOptimizeManyAddTextCallsStaysFast(t *testing.T) {
	doc := buildManyAddTextDoc(t)
	start := time.Now()
	if _, err := doc.Optimize(pdf.OptimizationOptions{RemoveUnusedObjects: true, SubsetFonts: true, RemoveDuplicateStreams: true}); err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("Optimize took %v on a 20-page/200-AddText-calls-per-page document, want well under 2s -- "+
			"the RemoveUnusedObjects pre-pass may have regressed (GitHub issue #9)", elapsed)
	}
}

// BenchmarkOptimizeManyAddTextCalls is TestOptimizeManyAddTextCallsStaysFast's
// precise-timing counterpart: run with
// `go test -bench BenchmarkOptimizeManyAddTextCalls -benchtime=5x -run NONE .`
// to see the actual speedup (the reporter measured roughly 25x on this exact
// shape before the fix).
func BenchmarkOptimizeManyAddTextCalls(b *testing.B) {
	// A benchmark function's timer is already running on entry, so without
	// this the first iteration's buildManyAddTextDoc (untimed on every later
	// iteration, since each one ends with b.StopTimer()) would leak into the
	// very first measurement.
	b.StopTimer()
	for i := 0; i < b.N; i++ {
		doc := buildManyAddTextDoc(b)
		b.StartTimer()
		if _, err := doc.Optimize(pdf.OptimizationOptions{RemoveUnusedObjects: true, SubsetFonts: true, RemoveDuplicateStreams: true}); err != nil {
			b.Fatal(err)
		}
		b.StopTimer()
	}
}

func itoaTest(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
