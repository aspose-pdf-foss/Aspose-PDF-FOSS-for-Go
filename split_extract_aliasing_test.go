// SPDX-License-Identifier: MIT

package asposepdf_test

import (
	"testing"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

// TestSplitDoesNotAliasOriginal verifies that mutating a document returned
// by Split does not leak into the original. Regression: Split reused the
// same *pdfObject pointers (and the same underlying pdfDict maps) from the
// parent's d.objects and d.pages, so any mutation on a split result was
// visible on the original and vice versa.
func TestSplitDoesNotAliasOriginal(t *testing.T) {
	doc, err := pdf.Open("testdata/4pages.pdf")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	parts, err := doc.Split()
	if err != nil {
		t.Fatalf("Split: %v", err)
	}

	if err := parts[0].SetRotation(pdf.Rotate90); err != nil {
		t.Fatalf("SetRotation on split[0]: %v", err)
	}

	origPage1, err := doc.Page(1)
	if err != nil {
		t.Fatalf("original Page(1): %v", err)
	}
	if got := origPage1.Rotation(); got != pdf.Rotate0 {
		t.Errorf("original page 1 rotation = %d after mutating split[0]; want unchanged 0 (Rotate0)", got)
	}
}

// TestSplitResultsDoNotAliasEachOther verifies splits are independent of
// each other, not just of the original. Each split owns a distinct page
// dict; mutating one must not touch another.
func TestSplitResultsDoNotAliasEachOther(t *testing.T) {
	doc, err := pdf.Open("testdata/4pages.pdf")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	parts, err := doc.Split()
	if err != nil {
		t.Fatalf("Split: %v", err)
	}

	if err := parts[0].SetRotation(pdf.Rotate180); err != nil {
		t.Fatalf("SetRotation on split[0]: %v", err)
	}

	p1, err := parts[1].Page(1)
	if err != nil {
		t.Fatalf("parts[1].Page(1): %v", err)
	}
	if got := p1.Rotation(); got != pdf.Rotate0 {
		t.Errorf("parts[1] rotation = %d after mutating parts[0]; want unchanged 0", got)
	}
}

// TestExtractDoesNotAliasOriginal is the Extract counterpart of
// TestSplitDoesNotAliasOriginal.
func TestExtractDoesNotAliasOriginal(t *testing.T) {
	doc, err := pdf.Open("testdata/4pages.pdf")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	ext, err := doc.Extract(pdf.PageRange{From: 1, To: 2})
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}

	if err := ext.SetRotation(pdf.Rotate90, 1); err != nil {
		t.Fatalf("SetRotation on extracted page 1: %v", err)
	}

	origPage1, err := doc.Page(1)
	if err != nil {
		t.Fatalf("original Page(1): %v", err)
	}
	if got := origPage1.Rotation(); got != pdf.Rotate0 {
		t.Errorf("original page 1 rotation = %d after mutating extracted; want unchanged 0", got)
	}
}

// TestOriginalMutationDoesNotAliasSplit covers the reverse leak direction:
// mutating the parent after Split should not change the splits.
func TestOriginalMutationDoesNotAliasSplit(t *testing.T) {
	doc, err := pdf.Open("testdata/4pages.pdf")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	parts, err := doc.Split()
	if err != nil {
		t.Fatalf("Split: %v", err)
	}

	if err := doc.SetRotation(pdf.Rotate270, 1); err != nil {
		t.Fatalf("SetRotation on original page 1: %v", err)
	}

	p1, err := parts[0].Page(1)
	if err != nil {
		t.Fatalf("parts[0].Page(1): %v", err)
	}
	if got := p1.Rotation(); got != pdf.Rotate0 {
		t.Errorf("parts[0] rotation = %d after mutating original; want unchanged 0", got)
	}
}
