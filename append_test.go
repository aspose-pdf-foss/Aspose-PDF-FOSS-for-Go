package asposepdf_test

import (
	"os"
	"path/filepath"
	"testing"

	asposepdf "github.com/aspose/pdf-for-go"
)

func TestDocumentAppend(t *testing.T) {
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
	combined := doc1.Append(doc2, doc3)

	want := marketingPages * 3
	if combined.PageCount() != want {
		t.Fatalf("expected %d pages after Append, got %d", want, combined.PageCount())
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
	withNil := doc1.Append(nil, doc2, nil)
	if withNil.PageCount() != marketingPages*2 {
		t.Fatalf("nil args: expected %d pages, got %d", marketingPages*2, withNil.PageCount())
	}
}
