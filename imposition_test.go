// SPDX-License-Identifier: MIT

package asposepdf_test

import (
	"fmt"
	"testing"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

// buildContentPages makes an n-page document where each page carries a line of
// text, so imposed sheets have real content to carry through the Form XObject.
func buildContentPages(t *testing.T, n int) *pdf.Document {
	t.Helper()
	doc := pdf.NewDocument(200, 300)
	for i := 0; i < n; i++ {
		if i > 0 {
			mustNoErr(t, doc.AddBlankPage(200, 300))
		}
		p, _ := doc.Page(i + 1)
		mustNoErr(t, p.AddText(fmt.Sprintf("Page %d", i+1),
			pdf.TextStyle{Font: pdf.FontHelveticaBold, Size: 24, Color: &pdf.Color{A: 1}},
			pdf.Rectangle{LLX: 20, LLY: 140, URX: 180, URY: 180}))
	}
	return doc
}

func TestNUpPageCount(t *testing.T) {
	src := buildContentPages(t, 5)
	out, err := src.NUp(pdf.NUpOptions{Rows: 2, Cols: 2, Margin: 10, Gutter: 8})
	if err != nil {
		t.Fatalf("NUp: %v", err)
	}
	// 5 pages, 4 per sheet → ceil(5/4) = 2 sheets.
	if out.PageCount() != 2 {
		t.Errorf("NUp pages = %d, want 2", out.PageCount())
	}
	// Receiver must be untouched.
	if src.PageCount() != 5 {
		t.Errorf("source PageCount = %d, want 5 (NUp must not mutate receiver)", src.PageCount())
	}
	// Round-trips to a valid PDF that still has 2 pages.
	out2 := reopen(t, out)
	if out2.PageCount() != 2 {
		t.Errorf("after reopen pages = %d, want 2", out2.PageCount())
	}
}

func TestNUpCarriesContent(t *testing.T) {
	src := buildContentPages(t, 4)
	out, err := src.NUp(pdf.NUpOptions{Rows: 2, Cols: 2, Margin: 10, DrawBorder: true})
	if err != nil {
		t.Fatalf("NUp: %v", err)
	}
	out2 := reopen(t, out)
	p1, _ := out2.Page(1)
	if !hasNonWhitePixel(t, p1) {
		t.Error("NUp sheet rendered blank — page content not carried through the Form XObject")
	}
}

func TestNUpSingleColumn(t *testing.T) {
	src := buildContentPages(t, 3)
	// 1×2 grid (2 per sheet) → ceil(3/2) = 2 sheets.
	out, err := src.NUp(pdf.NUpOptions{Rows: 1, Cols: 2})
	if err != nil {
		t.Fatalf("NUp: %v", err)
	}
	if out.PageCount() != 2 {
		t.Errorf("NUp 1x2 pages = %d, want 2", out.PageCount())
	}
}

func TestNUpErrors(t *testing.T) {
	src := buildContentPages(t, 2)
	if _, err := src.NUp(pdf.NUpOptions{Rows: 0, Cols: 2}); err == nil {
		t.Error("NUp with Rows=0 = nil error, want error")
	}
	if _, err := src.NUp(pdf.NUpOptions{Rows: 2, Cols: 0}); err == nil {
		t.Error("NUp with Cols=0 = nil error, want error")
	}
	// Margin larger than the sheet leaves no room for cells.
	if _, err := src.NUp(pdf.NUpOptions{Rows: 1, Cols: 1, Margin: 1000}); err == nil {
		t.Error("NUp with oversized margin = nil error, want error")
	}
}

func TestBookletPadsToMultipleOfFour(t *testing.T) {
	src := buildContentPages(t, 6)
	out, err := src.Booklet(pdf.BookletOptions{})
	if err != nil {
		t.Fatalf("Booklet: %v", err)
	}
	// 6 pages padded to 8 → 8/2 = 4 spread pages.
	if out.PageCount() != 4 {
		t.Errorf("Booklet pages = %d, want 4", out.PageCount())
	}
	if src.PageCount() != 6 {
		t.Errorf("source PageCount = %d, want 6 (Booklet must not mutate receiver)", src.PageCount())
	}
	out2 := reopen(t, out)
	if out2.PageCount() != 4 {
		t.Errorf("after reopen pages = %d, want 4", out2.PageCount())
	}
	// First spread carries page 1 (right half) → not blank.
	p1, _ := out2.Page(1)
	if !hasNonWhitePixel(t, p1) {
		t.Error("first booklet spread rendered blank")
	}
}

func TestBookletExactMultiple(t *testing.T) {
	src := buildContentPages(t, 4)
	out, err := src.Booklet(pdf.BookletOptions{Binding: pdf.BindingRight})
	if err != nil {
		t.Fatalf("Booklet: %v", err)
	}
	// 4 pages → 2 spread pages, no padding.
	if out.PageCount() != 2 {
		t.Errorf("Booklet pages = %d, want 2", out.PageCount())
	}
}

func TestBookletSheetSizeDefault(t *testing.T) {
	src := buildContentPages(t, 4) // pages are 200×300
	out, err := src.Booklet(pdf.BookletOptions{})
	if err != nil {
		t.Fatalf("Booklet: %v", err)
	}
	p1, _ := out.Page(1)
	sz, _ := p1.Size()
	// Default sheet = 2×width × height of the first page.
	if sz.Width != 400 || sz.Height != 300 {
		t.Errorf("default booklet sheet = %.0f×%.0f, want 400×300", sz.Width, sz.Height)
	}
}
