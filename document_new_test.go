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
