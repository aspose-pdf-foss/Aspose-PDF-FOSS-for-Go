package asposepdf_test

import (
	"os"
	"path/filepath"
	"testing"

	asposepdf "github.com/aspose/pdf-for-go"
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
