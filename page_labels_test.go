package asposepdf_test

import (
	"fmt"
	"testing"

	asposepdf "github.com/aspose/pdf-for-go"
)

// TestPageLabelDefaultDecimal verifies that pages without /PageLabels return
// their decimal page numbers ("1", "2", …).
func TestPageLabelDefaultDecimal(t *testing.T) {
	doc, err := asposepdf.Open(fourPagesPDF)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	for _, p := range doc.Pages() {
		want := fmt.Sprintf("%d", p.Number())
		if got := p.Label(); got != want {
			t.Errorf("page %d: Label()=%q, want %q", p.Number(), got, want)
		}
	}
}
