package asposepdf_test

import (
	"testing"

	pdf "github.com/aspose/pdf-for-go"
)

func TestPageAnnotationsNonNilOnPlainDoc(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, err := doc.Page(1)
	if err != nil {
		t.Fatalf("Page(1): %v", err)
	}
	ac := page.Annotations()
	if ac == nil {
		t.Fatal("Annotations() returned nil; want non-nil empty collection")
	}
	if got := ac.Count(); got != 0 {
		t.Errorf("Count() = %d on plain doc, want 0", got)
	}
	if got := ac.All(); len(got) != 0 {
		t.Errorf("All() len = %d, want 0", len(got))
	}
}
