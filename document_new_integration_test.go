// SPDX-License-Identifier: MIT

package asposepdf_test

import (
	"os"
	"path/filepath"
	"testing"

	asposepdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

func TestNewDocumentRoundTrip(t *testing.T) {
	doc := asposepdf.NewDocumentFromFormat(asposepdf.PageFormatA4)
	if doc.PageCount() != 1 {
		t.Fatalf("PageCount() = %d, want 1", doc.PageCount())
	}

	outDir := filepath.Join("result_files", "TestNewDocumentRoundTrip")
	os.MkdirAll(outDir, 0o755)
	outPath := filepath.Join(outDir, "blank_a4.pdf")
	if err := doc.Save(outPath); err != nil {
		t.Fatalf("save: %v", err)
	}

	// Reopen and validate.
	report, err := asposepdf.Validate(outPath)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if !report.Valid {
		for _, issue := range report.Issues {
			t.Errorf("validation issue: [%s] %s", issue.Code, issue.Message)
		}
	}

	// Verify dimensions survived round-trip.
	reopened, err := asposepdf.Open(outPath)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if reopened.PageCount() != 1 {
		t.Fatalf("reopened PageCount() = %d, want 1", reopened.PageCount())
	}
	page, _ := reopened.Page(1)
	size, _ := page.Size()
	if size.Width != 595 || size.Height != 842 {
		t.Errorf("reopened size = {%.0f, %.0f}, want {595, 842}", size.Width, size.Height)
	}
}

func TestAddBlankPageRoundTrip(t *testing.T) {
	doc, err := asposepdf.Open("testdata/PdfWithImages.pdf")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	origCount := doc.PageCount()

	err = doc.AddBlankPageFromFormat(asposepdf.PageFormatA4)
	if err != nil {
		t.Fatalf("AddBlankPageFromFormat: %v", err)
	}
	if doc.PageCount() != origCount+1 {
		t.Fatalf("PageCount() = %d, want %d", doc.PageCount(), origCount+1)
	}

	outDir := filepath.Join("result_files", "TestAddBlankPageRoundTrip")
	os.MkdirAll(outDir, 0o755)
	outPath := filepath.Join(outDir, "output.pdf")
	if err := doc.Save(outPath); err != nil {
		t.Fatalf("save: %v", err)
	}

	// Validate.
	report, err := asposepdf.Validate(outPath)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if !report.Valid {
		for _, issue := range report.Issues {
			t.Errorf("validation issue: [%s] %s", issue.Code, issue.Message)
		}
	}

	// Reopen and verify.
	reopened, err := asposepdf.Open(outPath)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if reopened.PageCount() != origCount+1 {
		t.Fatalf("reopened PageCount() = %d, want %d", reopened.PageCount(), origCount+1)
	}
	lastPage, _ := reopened.Page(reopened.PageCount())
	size, _ := lastPage.Size()
	if size.Width != 595 || size.Height != 842 {
		t.Errorf("last page size = {%.0f, %.0f}, want {595, 842}", size.Width, size.Height)
	}
}
