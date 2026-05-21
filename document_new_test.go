// SPDX-License-Identifier: MIT

package asposepdf

import "testing"

func TestPageFormatLandscapeSwaps(t *testing.T) {
	portrait := PageFormat{Width: 595, Height: 842}
	landscape := portrait.Landscape()
	if landscape.Width != 842 || landscape.Height != 595 {
		t.Errorf("Landscape() = {%.0f, %.0f}, want {842, 595}", landscape.Width, landscape.Height)
	}
}

func TestPageFormatConstants(t *testing.T) {
	cases := []struct {
		name   string
		format PageFormat
		width  float64
		height float64
	}{
		{"A3", PageFormatA3, 842, 1191},
		{"A4", PageFormatA4, 595, 842},
		{"Letter", PageFormatLetter, 612, 792},
		{"Legal", PageFormatLegal, 612, 1008},
	}
	for _, tc := range cases {
		if tc.format.Width != tc.width || tc.format.Height != tc.height {
			t.Errorf("%s = {%.0f, %.0f}, want {%.0f, %.0f}",
				tc.name, tc.format.Width, tc.format.Height, tc.width, tc.height)
		}
	}
}

func TestNewDocument(t *testing.T) {
	doc := NewDocument(595, 842)
	if doc.PageCount() != 1 {
		t.Fatalf("PageCount() = %d, want 1", doc.PageCount())
	}
	page, err := doc.Page(1)
	if err != nil {
		t.Fatalf("Page(1): %v", err)
	}
	size, err := page.Size()
	if err != nil {
		t.Fatalf("Size: %v", err)
	}
	if size.Width != 595 || size.Height != 842 {
		t.Errorf("size = {%.0f, %.0f}, want {595, 842}", size.Width, size.Height)
	}
}

func TestNewDocumentFromFormat(t *testing.T) {
	doc := NewDocumentFromFormat(PageFormatA4)
	if doc.PageCount() != 1 {
		t.Fatalf("PageCount() = %d, want 1", doc.PageCount())
	}
	page, _ := doc.Page(1)
	size, _ := page.Size()
	if size.Width != 595 || size.Height != 842 {
		t.Errorf("size = {%.0f, %.0f}, want {595, 842}", size.Width, size.Height)
	}
}

func TestNewDocumentLandscape(t *testing.T) {
	doc := NewDocumentFromFormat(PageFormatA4.Landscape())
	if doc.PageCount() != 1 {
		t.Fatalf("PageCount() = %d, want 1", doc.PageCount())
	}
	page, _ := doc.Page(1)
	size, _ := page.Size()
	if size.Width != 842 || size.Height != 595 {
		t.Errorf("size = {%.0f, %.0f}, want {842, 595}", size.Width, size.Height)
	}
}

func TestAddBlankPage(t *testing.T) {
	doc := NewDocument(612, 792) // Letter
	if err := doc.AddBlankPage(595, 842); err != nil {
		t.Fatalf("AddBlankPage: %v", err)
	}
	if doc.PageCount() != 2 {
		t.Fatalf("PageCount() = %d, want 2", doc.PageCount())
	}
	page, _ := doc.Page(2)
	size, _ := page.Size()
	if size.Width != 595 || size.Height != 842 {
		t.Errorf("page 2 size = {%.0f, %.0f}, want {595, 842}", size.Width, size.Height)
	}
}

func TestInsertBlankPage(t *testing.T) {
	doc := NewDocument(612, 792) // page 1: Letter
	doc.AddBlankPage(595, 842)   // page 2: A4

	// Insert at position 1 — becomes new page 1, others shift.
	if err := doc.InsertBlankPage(1, 842, 1191); err != nil {
		t.Fatalf("InsertBlankPage: %v", err)
	}
	if doc.PageCount() != 3 {
		t.Fatalf("PageCount() = %d, want 3", doc.PageCount())
	}

	// New page 1 should be A3.
	page1, _ := doc.Page(1)
	size1, _ := page1.Size()
	if size1.Width != 842 || size1.Height != 1191 {
		t.Errorf("page 1 size = {%.0f, %.0f}, want {842, 1191}", size1.Width, size1.Height)
	}

	// Original page 1 (Letter) is now page 2.
	page2, _ := doc.Page(2)
	size2, _ := page2.Size()
	if size2.Width != 612 || size2.Height != 792 {
		t.Errorf("page 2 size = {%.0f, %.0f}, want {612, 792}", size2.Width, size2.Height)
	}
}

func TestInsertBlankPageEnd(t *testing.T) {
	doc := NewDocument(595, 842)
	// Insert at PageCount()+1 = append.
	if err := doc.InsertBlankPage(2, 612, 792); err != nil {
		t.Fatalf("InsertBlankPage at end: %v", err)
	}
	if doc.PageCount() != 2 {
		t.Fatalf("PageCount() = %d, want 2", doc.PageCount())
	}
	page, _ := doc.Page(2)
	size, _ := page.Size()
	if size.Width != 612 || size.Height != 792 {
		t.Errorf("page 2 size = {%.0f, %.0f}, want {612, 792}", size.Width, size.Height)
	}
}

func TestInsertBlankPageInvalidPosition(t *testing.T) {
	doc := NewDocument(595, 842) // 1 page
	if err := doc.InsertBlankPage(0, 595, 842); err == nil {
		t.Fatal("expected error for position 0")
	}
	if err := doc.InsertBlankPage(3, 595, 842); err == nil {
		t.Fatal("expected error for position > PageCount()+1")
	}
}
