// SPDX-License-Identifier: MIT

package asposepdf_test

import (
	"path/filepath"
	"testing"

	asposepdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

// buildSizedDoc builds an in-memory document whose pages each have a distinct
// width (height fixed at 100), so a test can identify which pages survive a
// deletion by reading the remaining widths.
func buildSizedDoc(t *testing.T, widths ...float64) *asposepdf.Document {
	t.Helper()
	if len(widths) == 0 {
		t.Fatal("buildSizedDoc: need at least one width")
	}
	doc := asposepdf.NewDocument(widths[0], 100)
	for _, w := range widths[1:] {
		if err := doc.AddBlankPage(w, 100); err != nil {
			t.Fatalf("AddBlankPage(%v): %v", w, err)
		}
	}
	return doc
}

// pageWidths returns the width of every page in order.
func pageWidths(t *testing.T, doc *asposepdf.Document) []float64 {
	t.Helper()
	out := make([]float64, 0, doc.PageCount())
	for i := 1; i <= doc.PageCount(); i++ {
		p, err := doc.Page(i)
		if err != nil {
			t.Fatalf("Page(%d): %v", i, err)
		}
		sz, err := p.Size()
		if err != nil {
			t.Fatalf("Size(page %d): %v", i, err)
		}
		out = append(out, sz.Width)
	}
	return out
}

func equalWidths(a, b []float64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestDeletePagesRemovesSelected(t *testing.T) {
	doc := buildSizedDoc(t, 100, 200, 300, 400, 500)

	// Populate the page cache before deleting so the test also exercises
	// cache invalidation (positions shift after removal).
	_ = pageWidths(t, doc)

	if err := doc.DeletePages(2, 4); err != nil {
		t.Fatalf("DeletePages: %v", err)
	}
	if doc.PageCount() != 3 {
		t.Fatalf("PageCount: got %d, want 3", doc.PageCount())
	}
	got := pageWidths(t, doc)
	want := []float64{100, 300, 500}
	if !equalWidths(got, want) {
		t.Errorf("remaining widths: got %v, want %v", got, want)
	}
}

func TestDeletePageSingle(t *testing.T) {
	doc := buildSizedDoc(t, 100, 200, 300)
	if err := doc.DeletePage(2); err != nil {
		t.Fatalf("DeletePage: %v", err)
	}
	got := pageWidths(t, doc)
	want := []float64{100, 300}
	if !equalWidths(got, want) {
		t.Errorf("remaining widths: got %v, want %v", got, want)
	}
}

func TestDeletePagesDeduplicatesAndOrderIndependent(t *testing.T) {
	// Repeated and out-of-order numbers select the same set.
	doc := buildSizedDoc(t, 100, 200, 300, 400)
	if err := doc.DeletePages(4, 2, 2); err != nil {
		t.Fatalf("DeletePages: %v", err)
	}
	got := pageWidths(t, doc)
	want := []float64{100, 300}
	if !equalWidths(got, want) {
		t.Errorf("remaining widths: got %v, want %v", got, want)
	}
}

func TestDeletePagesValidation(t *testing.T) {
	t.Run("no numbers", func(t *testing.T) {
		doc := buildSizedDoc(t, 100, 200)
		if err := doc.DeletePages(); err == nil {
			t.Fatal("expected error for no page numbers")
		}
		if doc.PageCount() != 2 {
			t.Errorf("document changed on error: PageCount %d", doc.PageCount())
		}
	})

	t.Run("out of range", func(t *testing.T) {
		for _, n := range []int{0, 3, -1} {
			doc := buildSizedDoc(t, 100, 200)
			if err := doc.DeletePages(n); err == nil {
				t.Fatalf("expected error for page %d in a 2-page document", n)
			}
			if doc.PageCount() != 2 {
				t.Errorf("document changed on error (page %d): PageCount %d", n, doc.PageCount())
			}
		}
	})

	t.Run("atomic on partial error", func(t *testing.T) {
		// One valid, one invalid: nothing must be removed.
		doc := buildSizedDoc(t, 100, 200, 300)
		if err := doc.DeletePages(1, 99); err == nil {
			t.Fatal("expected error when any page number is out of range")
		}
		got := pageWidths(t, doc)
		want := []float64{100, 200, 300}
		if !equalWidths(got, want) {
			t.Errorf("document changed on error: got %v, want %v", got, want)
		}
	})

	t.Run("cannot delete all", func(t *testing.T) {
		doc := buildSizedDoc(t, 100, 200, 300)
		if err := doc.DeletePages(1, 2, 3); err == nil {
			t.Fatal("expected error when deleting every page")
		}
		if doc.PageCount() != 3 {
			t.Errorf("document changed on error: PageCount %d", doc.PageCount())
		}
		// Duplicates that still cover every page are rejected too.
		if err := doc.DeletePages(1, 2, 3, 3, 2, 1); err == nil {
			t.Fatal("expected error when duplicates still cover every page")
		}
	})
}

func TestDeletePagesSaveRoundTrip(t *testing.T) {
	doc := buildSizedDoc(t, 100, 200, 300, 400)
	if err := doc.DeletePages(1, 3); err != nil {
		t.Fatalf("DeletePages: %v", err)
	}

	out := filepath.Join(t.TempDir(), "deleted.pdf")
	if err := doc.Save(out); err != nil {
		t.Fatalf("Save: %v", err)
	}
	reopened, err := asposepdf.Open(out)
	if err != nil {
		t.Fatalf("Open saved: %v", err)
	}
	if reopened.PageCount() != 2 {
		t.Fatalf("reloaded PageCount: got %d, want 2", reopened.PageCount())
	}
	got := pageWidths(t, reopened)
	want := []float64{200, 400}
	if !equalWidths(got, want) {
		t.Errorf("reloaded widths: got %v, want %v", got, want)
	}
}
