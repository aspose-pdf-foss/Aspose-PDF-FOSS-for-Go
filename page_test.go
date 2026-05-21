// SPDX-License-Identifier: MIT

package asposepdf_test

import (
	"testing"

	asposepdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

func TestPageSizes(t *testing.T) {
	sizes, err := asposepdf.PageSizes(testFile(t))
	if err != nil {
		t.Fatalf("PageSizes: %v", err)
	}
	if len(sizes) != fourPagesCount {
		t.Fatalf("expected %d sizes, got %d", fourPagesCount, len(sizes))
	}
	for i, s := range sizes {
		if s.Width != letterWidth || s.Height != letterHeight {
			t.Errorf("page %d: expected %.0fx%.0f, got %.2fx%.2f", i+1, letterWidth, letterHeight, s.Width, s.Height)
		}
	}
}

func TestDocumentPages(t *testing.T) {
	doc, err := asposepdf.Open(testFile(t))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	pages := doc.Pages()
	if len(pages) != fourPagesCount {
		t.Fatalf("expected %d pages, got %d", fourPagesCount, len(pages))
	}
	for _, p := range pages {
		if p.Number() < 1 || p.Number() > fourPagesCount {
			t.Errorf("unexpected page number %d", p.Number())
		}
		sz, err := p.Size()
		if err != nil {
			t.Fatalf("page %d Size: %v", p.Number(), err)
		}
		if sz.Width != letterWidth || sz.Height != letterHeight {
			t.Errorf("page %d: expected %.0fx%.0f, got %.2fx%.2f", p.Number(), letterWidth, letterHeight, sz.Width, sz.Height)
		}
	}
}

func TestDocumentPage(t *testing.T) {
	doc, err := asposepdf.Open(testFile(t))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	p, err := doc.Page(2)
	if err != nil {
		t.Fatalf("Page(2): %v", err)
	}
	if p.Number() != 2 {
		t.Fatalf("expected Number()=2, got %d", p.Number())
	}
	sz, err := p.Size()
	if err != nil {
		t.Fatalf("Size: %v", err)
	}
	if sz.Width != letterWidth || sz.Height != letterHeight {
		t.Errorf("expected %.0fx%.0f, got %.2fx%.2f", letterWidth, letterHeight, sz.Width, sz.Height)
	}
}

func TestPageRotation(t *testing.T) {
	doc, err := asposepdf.Open(testFile(t))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	// All pages should have 0 rotation initially.
	for _, p := range doc.Pages() {
		if r := p.Rotation(); r != 0 {
			t.Errorf("page %d: expected initial rotation 0, got %d", p.Number(), r)
		}
	}

	// Rotate page 1 by 90° and verify it is reflected via Page.Rotation().
	if err := doc.Rotate(asposepdf.Rotate90, 1); err != nil {
		t.Fatalf("Rotate: %v", err)
	}
	p1, _ := doc.Page(1)
	if r := p1.Rotation(); r != 90 {
		t.Errorf("page 1: expected rotation 90 after Rotate(90), got %d", r)
	}
	// Page 2 should be unaffected.
	p2, _ := doc.Page(2)
	if r := p2.Rotation(); r != 0 {
		t.Errorf("page 2: expected rotation 0 (unaffected), got %d", r)
	}

	// Rotate page 1 again by 90° — should accumulate to 180°.
	if err := doc.Rotate(asposepdf.Rotate90, 1); err != nil {
		t.Fatalf("second Rotate: %v", err)
	}
	p1, _ = doc.Page(1)
	if r := p1.Rotation(); r != 180 {
		t.Errorf("page 1: expected rotation 180 after two Rotate(90), got %d", r)
	}
}

func TestPageBoxesFallbackToMediaBox(t *testing.T) {
	// 4pages.pdf has no explicit CropBox/TrimBox/BleedBox/ArtBox,
	// so all boxes must fall back to the MediaBox (612x792 pt).
	doc, err := asposepdf.Open(testFile(t))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	p, err := doc.Page(1)
	if err != nil {
		t.Fatalf("Page(1): %v", err)
	}

	checks := []struct {
		name string
		fn   func() (asposepdf.PageSize, error)
	}{
		{"CropBox", p.CropBox},
		{"TrimBox", p.TrimBox},
		{"BleedBox", p.BleedBox},
		{"ArtBox", p.ArtBox},
	}
	for _, c := range checks {
		sz, err := c.fn()
		if err != nil {
			t.Errorf("%s: unexpected error: %v", c.name, err)
			continue
		}
		if sz.Width != letterWidth || sz.Height != letterHeight {
			t.Errorf("%s: expected %.0fx%.0f, got %.2fx%.2f",
				c.name, letterWidth, letterHeight, sz.Width, sz.Height)
		}
	}
}

func TestDocumentPageInvalidNumber(t *testing.T) {
	doc, err := asposepdf.Open(testFile(t))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if _, err := doc.Page(0); err == nil {
		t.Fatal("expected error for page 0")
	}
	if _, err := doc.Page(fourPagesCount + 1); err == nil {
		t.Fatalf("expected error for page %d", fourPagesCount+1)
	}
}
