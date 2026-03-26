package asposepdf_test

import (
	"testing"

	asposepdf "github.com/aspose/pdf-for-go"
)

// pageCountFromFile opens a PDF file and returns its page count.
// Calls t.Fatal on any error.
func pageCountFromFile(t *testing.T, path string) int {
	t.Helper()
	doc, err := asposepdf.Open(path)
	if err != nil {
		t.Fatalf("Open %s: %v", path, err)
	}
	return doc.PageCount()
}
