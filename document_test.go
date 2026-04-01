package asposepdf_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	asposepdf "github.com/aspose/pdf-for-go"
)

const marketingPDF = "testdata/split/marketing.pdf"
const marketingPages = 2

func TestDocumentOpen(t *testing.T) {
	doc, err := asposepdf.Open(marketingPDF)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if doc.PageCount() != marketingPages {
		t.Fatalf("expected %d pages, got %d", marketingPages, doc.PageCount())
	}
}

func TestDocumentSave(t *testing.T) {
	doc, err := asposepdf.Open(marketingPDF)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	outputPath := filepath.Join(resultDir, "document_save.pdf")
	if err := os.MkdirAll(resultDir, 0o755); err != nil {
		t.Fatalf("create result dir: %v", err)
	}
	if err := doc.Save(outputPath); err != nil {
		t.Fatalf("Save: %v", err)
	}

	if n := pageCountFromFile(t, outputPath); n != marketingPages {
		t.Fatalf("expected %d pages after save, got %d", marketingPages, n)
	}
}

func TestDocumentRotate(t *testing.T) {
	doc, err := asposepdf.Open(marketingPDF)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	doc, err = doc.Rotate(asposepdf.Rotate90)
	if err != nil {
		t.Fatalf("Rotate: %v", err)
	}

	outputPath := filepath.Join(resultDir, "document_rotate.pdf")
	if err := os.MkdirAll(resultDir, 0o755); err != nil {
		t.Fatalf("create result dir: %v", err)
	}
	if err := doc.Save(outputPath); err != nil {
		t.Fatalf("Save: %v", err)
	}

	data, _ := os.ReadFile(outputPath)
	count := bytes.Count(data, []byte("/Rotate 90"))
	if count != marketingPages {
		t.Errorf("expected /Rotate 90 on %d pages, found %d", marketingPages, count)
	}
}

func TestDocumentRotateSpecificPage(t *testing.T) {
	doc, err := asposepdf.Open(marketingPDF)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	doc, err = doc.Rotate(asposepdf.Rotate180, 1)
	if err != nil {
		t.Fatalf("Rotate: %v", err)
	}

	outputPath := filepath.Join(resultDir, "document_rotate_specific_page.pdf")
	if err := os.MkdirAll(resultDir, 0o755); err != nil {
		t.Fatalf("create result dir: %v", err)
	}
	if err := doc.Save(outputPath); err != nil {
		t.Fatalf("Save: %v", err)
	}

	data, _ := os.ReadFile(outputPath)
	count := bytes.Count(data, []byte("/Rotate 180"))
	if count != 1 {
		t.Errorf("expected /Rotate 180 exactly once (page 1 only), got %d", count)
	}
}

func TestDocumentRotateAccumulates(t *testing.T) {
	doc, err := asposepdf.Open(marketingPDF)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	doc, err = doc.Rotate(asposepdf.Rotate90)
	if err != nil {
		t.Fatalf("first Rotate: %v", err)
	}
	doc, err = doc.Rotate(asposepdf.Rotate90)
	if err != nil {
		t.Fatalf("second Rotate: %v", err)
	}

	outputPath := filepath.Join(resultDir, "document_rotate_accumulates.pdf")
	if err := os.MkdirAll(resultDir, 0o755); err != nil {
		t.Fatalf("create result dir: %v", err)
	}
	if err := doc.Save(outputPath); err != nil {
		t.Fatalf("Save: %v", err)
	}

	data, _ := os.ReadFile(outputPath)
	count := bytes.Count(data, []byte("/Rotate 180"))
	if count != marketingPages {
		t.Errorf("expected /Rotate 180 on %d pages (accumulated 90+90), found %d", marketingPages, count)
	}
}

func TestDocumentRotateDuplicatePageNums(t *testing.T) {
	doc, err := asposepdf.Open(marketingPDF)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	doc, err = doc.Rotate(asposepdf.Rotate90, 1, 1)
	if err != nil {
		t.Fatalf("Rotate: %v", err)
	}

	outputPath := filepath.Join(resultDir, "document_rotate_duplicate_page_nums.pdf")
	if err := os.MkdirAll(resultDir, 0o755); err != nil {
		t.Fatalf("create result dir: %v", err)
	}
	if err := doc.Save(outputPath); err != nil {
		t.Fatalf("Save: %v", err)
	}

	data, _ := os.ReadFile(outputPath)
	// Duplicate page 1 must be deduplicated — rotation is 90°, not 180°.
	if count := bytes.Count(data, []byte("/Rotate 90")); count != 1 {
		t.Errorf("expected /Rotate 90 exactly once, got %d", count)
	}
}

func TestDocumentExtractPages(t *testing.T) {
	doc, err := asposepdf.Open(marketingPDF)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	extracted, err := doc.Extract(asposepdf.PageRange{From: 1, To: 1})
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if extracted.PageCount() != 1 {
		t.Fatalf("expected 1 page, got %d", extracted.PageCount())
	}

	outputPath := filepath.Join(resultDir, "document_extract_pages.pdf")
	if err := os.MkdirAll(resultDir, 0o755); err != nil {
		t.Fatalf("create result dir: %v", err)
	}
	if err := extracted.Save(outputPath); err != nil {
		t.Fatalf("Save: %v", err)
	}

	if n := pageCountFromFile(t, outputPath); n != 1 {
		t.Fatalf("expected 1 page in saved file, got %d", n)
	}
}

func TestDocumentReorder(t *testing.T) {
	doc, err := asposepdf.Open(marketingPDF)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	doc, err = doc.Reorder([]int{2, 1})
	if err != nil {
		t.Fatalf("Reorder: %v", err)
	}
	if doc.PageCount() != marketingPages {
		t.Fatalf("expected %d pages after Reorder, got %d", marketingPages, doc.PageCount())
	}

	outputPath := filepath.Join(resultDir, "document_reorder.pdf")
	if err := os.MkdirAll(resultDir, 0o755); err != nil {
		t.Fatalf("create result dir: %v", err)
	}
	if err := doc.Save(outputPath); err != nil {
		t.Fatalf("Save: %v", err)
	}

	if n := pageCountFromFile(t, outputPath); n != marketingPages {
		t.Fatalf("expected %d pages in saved file, got %d", marketingPages, n)
	}
}

func TestDocumentAppendFrom(t *testing.T) {
	doc1, err := asposepdf.Open(marketingPDF)
	if err != nil {
		t.Fatalf("Open doc1: %v", err)
	}
	doc2, err := asposepdf.Open(marketingPDF)
	if err != nil {
		t.Fatalf("Open doc2: %v", err)
	}
	doc3, err := asposepdf.Open(marketingPDF)
	if err != nil {
		t.Fatalf("Open doc3: %v", err)
	}

	// Variadic: append two documents at once.
	combined := doc1.AppendFrom(doc2, doc3)

	want := marketingPages * 3
	if combined.PageCount() != want {
		t.Fatalf("expected %d pages after AppendFrom, got %d", want, combined.PageCount())
	}

	outputPath := filepath.Join(resultDir, "document_append_from.pdf")
	if err := os.MkdirAll(resultDir, 0o755); err != nil {
		t.Fatalf("create result dir: %v", err)
	}
	if err := combined.Save(outputPath); err != nil {
		t.Fatalf("Save: %v", err)
	}

	if n := pageCountFromFile(t, outputPath); n != want {
		t.Fatalf("expected %d pages in saved file, got %d", want, n)
	}

	// nil arguments must be skipped.
	withNil := doc1.AppendFrom(nil, doc2, nil)
	if withNil.PageCount() != marketingPages*2 {
		t.Fatalf("nil args: expected %d pages, got %d", marketingPages*2, withNil.PageCount())
	}
}

func TestDocumentWriteTo(t *testing.T) {
	doc, err := asposepdf.Open("testdata/split/4pages.pdf")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	var buf bytes.Buffer
	n, err := doc.WriteTo(&buf)
	if err != nil {
		t.Fatalf("WriteTo: %v", err)
	}
	if n == 0 {
		t.Fatal("WriteTo wrote 0 bytes")
	}
	if !bytes.HasPrefix(buf.Bytes(), []byte("%PDF-")) {
		t.Fatal("output does not start with PDF header")
	}

	if err := os.MkdirAll(resultDir, 0o755); err != nil {
		t.Fatalf("create result dir: %v", err)
	}
	outputPath := filepath.Join(resultDir, "document_write_to.pdf")
	if err := os.WriteFile(outputPath, buf.Bytes(), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if got := pageCountFromFile(t, outputPath); got != 4 {
		t.Fatalf("expected 4 pages, got %d", got)
	}
}

func TestDocumentSetRotation(t *testing.T) {
	doc, err := asposepdf.Open(marketingPDF)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	// First rotate page 1 to 180°, then set it absolutely to 90°.
	doc, err = doc.Rotate(asposepdf.Rotate180, 1)
	if err != nil {
		t.Fatalf("Rotate: %v", err)
	}
	doc, err = doc.SetRotation(asposepdf.Rotate90, 1)
	if err != nil {
		t.Fatalf("SetRotation: %v", err)
	}

	outputPath := filepath.Join(resultDir, "document_set_rotation.pdf")
	if err := os.MkdirAll(resultDir, 0o755); err != nil {
		t.Fatalf("create result dir: %v", err)
	}
	if err := doc.Save(outputPath); err != nil {
		t.Fatalf("Save: %v", err)
	}

	data, _ := os.ReadFile(outputPath)
	// Must be 90°, not 270° (180+90).
	if count := bytes.Count(data, []byte("/Rotate 90")); count != 1 {
		t.Errorf("expected /Rotate 90 exactly once, got %d", count)
	}
	if bytes.Contains(data, []byte("/Rotate 270")) {
		t.Error("found /Rotate 270 — SetRotation must not accumulate")
	}
}

func TestDocumentInvalidRotateAngle(t *testing.T) {
	doc, err := asposepdf.Open(marketingPDF)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if _, err := doc.Rotate(asposepdf.RotationAngle(45)); err == nil {
		t.Fatal("expected error for angle=45")
	}
	if _, err := doc.SetRotation(asposepdf.RotationAngle(45)); err == nil {
		t.Fatal("SetRotation: expected error for angle=45")
	}
}

func TestDocumentRotateZeroIsNoop(t *testing.T) {
	doc, err := asposepdf.Open(marketingPDF)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	result, err := doc.Rotate(asposepdf.Rotate0)
	if err != nil {
		t.Fatalf("Rotate(Rotate0): %v", err)
	}
	if result.PageCount() != doc.PageCount() {
		t.Errorf("expected %d pages, got %d", doc.PageCount(), result.PageCount())
	}
}

func TestDocumentInvalidReorderPageNum(t *testing.T) {
	doc, err := asposepdf.Open(marketingPDF)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if _, err := doc.Reorder([]int{1, 5}); err == nil {
		t.Fatal("expected error for page 5 in a 2-page document")
	}
}

func TestDocumentInvalidExtract(t *testing.T) {
	doc, err := asposepdf.Open(marketingPDF)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if _, err := doc.Extract(); err == nil {
		t.Fatal("expected error for empty ranges")
	}
	if _, err := doc.Extract(asposepdf.PageRange{From: 0, To: 1}); err == nil {
		t.Fatal("expected error for from=0")
	}
	if _, err := doc.Extract(asposepdf.PageRange{From: 1, To: 999}); err == nil {
		t.Fatal("expected error for to out of bounds")
	}
	if _, err := doc.Extract(asposepdf.PageRange{From: 2, To: 1}); err == nil {
		t.Fatal("expected error for from > to")
	}
}
