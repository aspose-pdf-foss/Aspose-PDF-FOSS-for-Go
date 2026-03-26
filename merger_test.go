package asposepdf_test

import (
	"os"
	"path/filepath"
	"testing"

	asposepdf "github.com/aspose/pdf-for-go"
)

func TestMerge(t *testing.T) {
	pdf := buildMinimalPDF()
	tmpDir := t.TempDir()

	// Write two identical 2-page PDFs.
	input1 := filepath.Join(tmpDir, "a.pdf")
	input2 := filepath.Join(tmpDir, "b.pdf")
	for _, p := range []string{input1, input2} {
		if err := os.WriteFile(p, pdf, 0o644); err != nil {
			t.Fatalf("write test PDF: %v", err)
		}
	}

	outputPath := filepath.Join(tmpDir, "merged.pdf")
	if err := asposepdf.Merge(outputPath, input1, input2); err != nil {
		t.Fatalf("Merge: %v", err)
	}

	if n := pageCountFromFile(t, outputPath); n != 4 {
		t.Fatalf("expected 4 pages, got %d", n)
	}

	// No inputs — must fail.
	if err := asposepdf.Merge(outputPath); err == nil {
		t.Fatal("expected error for empty input list")
	}
}
