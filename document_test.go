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

	n, err := asposepdf.PageCount(outputPath)
	if err != nil {
		t.Fatalf("PageCount: %v", err)
	}
	if n != marketingPages {
		t.Fatalf("expected %d pages after save, got %d", marketingPages, n)
	}
}

func TestDocumentRotate(t *testing.T) {
	doc, err := asposepdf.Open(marketingPDF)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := doc.Rotate(asposepdf.Rotate90); err != nil {
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
	// Rotate only page 1.
	if err := doc.Rotate(asposepdf.Rotate180, 1); err != nil {
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
	if err := doc.Rotate(asposepdf.Rotate90); err != nil {
		t.Fatalf("first Rotate: %v", err)
	}
	if err := doc.Rotate(asposepdf.Rotate90); err != nil {
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

func TestDocumentExtractPages(t *testing.T) {
	doc, err := asposepdf.Open(marketingPDF)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	// Keep only page 1.
	if err := doc.ExtractPages(asposepdf.PageRange{From: 1, To: 1}); err != nil {
		t.Fatalf("ExtractPages: %v", err)
	}
	if doc.PageCount() != 1 {
		t.Fatalf("expected 1 page after ExtractPages, got %d", doc.PageCount())
	}

	outputPath := filepath.Join(resultDir, "document_extract_pages.pdf")
	if err := os.MkdirAll(resultDir, 0o755); err != nil {
		t.Fatalf("create result dir: %v", err)
	}
	if err := doc.Save(outputPath); err != nil {
		t.Fatalf("Save: %v", err)
	}

	n, err := asposepdf.PageCount(outputPath)
	if err != nil {
		t.Fatalf("PageCount: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 page in saved file, got %d", n)
	}
}

func TestDocumentReorder(t *testing.T) {
	doc, err := asposepdf.Open(marketingPDF)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	// Reverse page order: [2, 1].
	if err := doc.Reorder([]int{2, 1}); err != nil {
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

	n, err := asposepdf.PageCount(outputPath)
	if err != nil {
		t.Fatalf("PageCount: %v", err)
	}
	if n != marketingPages {
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

	doc1.AppendFrom(doc2)

	want := marketingPages * 2
	if doc1.PageCount() != want {
		t.Fatalf("expected %d pages after AppendFrom, got %d", want, doc1.PageCount())
	}

	outputPath := filepath.Join(resultDir, "document_append_from.pdf")
	if err := os.MkdirAll(resultDir, 0o755); err != nil {
		t.Fatalf("create result dir: %v", err)
	}
	if err := doc1.Save(outputPath); err != nil {
		t.Fatalf("Save: %v", err)
	}

	n, err := asposepdf.PageCount(outputPath)
	if err != nil {
		t.Fatalf("PageCount: %v", err)
	}
	if n != want {
		t.Fatalf("expected %d pages in saved file, got %d", want, n)
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

	// Save buffer to result_files for visual inspection.
	if err := os.MkdirAll(resultDir, 0o755); err != nil {
		t.Fatalf("create result dir: %v", err)
	}
	outputPath := filepath.Join(resultDir, "document_write_to.pdf")
	if err := os.WriteFile(outputPath, buf.Bytes(), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	got, err := asposepdf.PageCount(outputPath)
	if err != nil {
		t.Fatalf("PageCount: %v", err)
	}
	if got != 4 {
		t.Fatalf("expected 4 pages, got %d", got)
	}
}

func TestDocumentInvalidRotateAngle(t *testing.T) {
	doc, err := asposepdf.Open(marketingPDF)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := doc.Rotate(asposepdf.RotationAngle(45)); err == nil {
		t.Fatal("expected error for angle=45")
	}
}

func TestDocumentInvalidReorderPageNum(t *testing.T) {
	doc, err := asposepdf.Open(marketingPDF)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := doc.Reorder([]int{1, 5}); err == nil {
		t.Fatal("expected error for page 5 in a 2-page document")
	}
}

func TestDocumentInvalidExtractPages(t *testing.T) {
	doc, err := asposepdf.Open(marketingPDF)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := doc.ExtractPages(); err == nil {
		t.Fatal("expected error for empty ranges")
	}
}
