package asposepdf_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	asposepdf "github.com/aspose/pdf-for-go"
)

const resultDir = "result_files"

const testPDF = "test_data/split/4pages.pdf"

func TestRotateAllPages(t *testing.T) {
	outputPath := filepath.Join(resultDir, "rotate_all_pages.pdf")
	if err := os.MkdirAll(resultDir, 0o755); err != nil {
		t.Fatalf("create result dir: %v", err)
	}

	if err := asposepdf.Rotate(testPDF, outputPath, asposepdf.Rotate90); err != nil {
		t.Fatalf("Rotate: %v", err)
	}

	n, err := asposepdf.PageCount(outputPath)
	if err != nil {
		t.Fatalf("PageCount: %v", err)
	}
	orig, err := asposepdf.PageCount(testPDF)
	if err != nil {
		t.Fatalf("PageCount original: %v", err)
	}
	if n != orig {
		t.Fatalf("page count changed: want %d, got %d", orig, n)
	}

	data, _ := os.ReadFile(outputPath)
	count := bytes.Count(data, []byte("/Rotate 90"))
	if count != orig {
		t.Errorf("expected /Rotate 90 on %d pages, found %d occurrence(s)", orig, count)
	}
}

func TestRotateSpecificPage(t *testing.T) {
	outputPath := filepath.Join(resultDir, "rotate_specific_page.pdf")
	if err := os.MkdirAll(resultDir, 0o755); err != nil {
		t.Fatalf("create result dir: %v", err)
	}

	// Rotate only page 1.
	if err := asposepdf.Rotate(testPDF, outputPath, asposepdf.Rotate180, 1); err != nil {
		t.Fatalf("Rotate: %v", err)
	}

	data, _ := os.ReadFile(outputPath)
	count := bytes.Count(data, []byte("/Rotate 180"))
	if count != 1 {
		t.Errorf("expected /Rotate 180 exactly once (page 1 only), got %d", count)
	}
}

func TestRotateAccumulates(t *testing.T) {
	if err := os.MkdirAll(resultDir, 0o755); err != nil {
		t.Fatalf("create result dir: %v", err)
	}

	// First rotation: 4pages.pdf → rotated 90°.
	step1 := filepath.Join(t.TempDir(), "step1.pdf")
	if err := asposepdf.Rotate(testPDF, step1, asposepdf.Rotate90); err != nil {
		t.Fatalf("Rotate step 1: %v", err)
	}

	// Second rotation: step1 (90°) → rotated another 90° = 180° total.
	outputPath := filepath.Join(resultDir, "rotate_accumulates.pdf")
	if err := asposepdf.Rotate(step1, outputPath, asposepdf.Rotate90); err != nil {
		t.Fatalf("Rotate step 2: %v", err)
	}

	data, _ := os.ReadFile(outputPath)
	if !bytes.Contains(data, []byte("/Rotate 180")) {
		t.Error("expected /Rotate 180 (accumulated 90+90) in output")
	}
}

func TestRotateInvalidAngle(t *testing.T) {
	if err := asposepdf.Rotate(testPDF, filepath.Join(t.TempDir(), "out.pdf"), 45); err == nil {
		t.Fatal("expected error for angle=45")
	}
}

func TestRotateInvalidPageNum(t *testing.T) {
	// 4pages.pdf has 4 pages; page 10 is out of range.
	if err := asposepdf.Rotate(testPDF, filepath.Join(t.TempDir(), "out.pdf"), 90, 10); err == nil {
		t.Fatal("expected error for page 10 in a 4-page PDF")
	}
}
