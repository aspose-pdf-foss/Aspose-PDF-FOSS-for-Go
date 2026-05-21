package asposepdf_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	asposepdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

func writeTempPDF(t *testing.T, data []byte) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "*.pdf")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write(data); err != nil {
		t.Fatal(err)
	}
	f.Close()
	return f.Name()
}

func TestValidate_ValidPDF(t *testing.T) {
	path := writeTempPDF(t, buildMinimalPDF())

	report, err := asposepdf.Validate(path)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Valid {
		t.Errorf("expected valid PDF, got issues: %v", report.Issues)
	}
	if len(report.Issues) != 0 {
		t.Errorf("expected no issues, got %d: %v", len(report.Issues), report.Issues)
	}
}

func TestValidate_FileNotFound(t *testing.T) {
	_, err := asposepdf.Validate(filepath.Join(t.TempDir(), "nonexistent.pdf"))
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

func TestValidate_InvalidHeader(t *testing.T) {
	data := append([]byte("NOT_A_PDF_HEADER"), buildMinimalPDF()[8:]...)
	path := writeTempPDF(t, data)

	report, err := asposepdf.Validate(path)
	if err != nil {
		t.Fatal(err)
	}
	if report.Valid {
		t.Error("expected invalid report for bad header")
	}
	if !hasIssue(report, "INVALID_HEADER") {
		t.Errorf("expected INVALID_HEADER issue, got: %v", report.Issues)
	}
}

func TestValidate_TruncatedFile(t *testing.T) {
	pdf := buildMinimalPDF()
	// Keep only the first half — xref will be missing.
	truncated := pdf[:len(pdf)/2]
	path := writeTempPDF(t, truncated)

	report, err := asposepdf.Validate(path)
	if err != nil {
		t.Fatal(err)
	}
	if report.Valid {
		t.Error("expected invalid report for truncated file")
	}
	if !hasIssue(report, "XREF_ERROR") {
		t.Errorf("expected XREF_ERROR issue, got: %v", report.Issues)
	}
}

func TestValidate_EncryptedPDF(t *testing.T) {
	src := writeTempPDF(t, buildMinimalPDF())
	dst := filepath.Join(t.TempDir(), "encrypted.pdf")
	if err := asposepdf.Encrypt(src, dst, "user", "owner"); err != nil {
		t.Fatal(err)
	}

	report, err := asposepdf.Validate(dst)
	if err != nil {
		t.Fatal(err)
	}
	if !hasIssue(report, "ENCRYPTED") {
		t.Errorf("expected ENCRYPTED issue, got: %v", report.Issues)
	}
}

// splitAndSave splits doc into individual pages, saves each to outDir, and returns the paths.
func splitAndSave(t *testing.T, doc *asposepdf.Document, outDir string) []string {
	t.Helper()
	pages, err := doc.Split()
	if err != nil {
		t.Fatalf("Split: %v", err)
	}
	var paths []string
	for i, p := range pages {
		path := filepath.Join(outDir, fmt.Sprintf("page%03d.pdf", i+1))
		if err := p.Save(path); err != nil {
			t.Fatalf("Save page %d: %v", i+1, err)
		}
		paths = append(paths, path)
	}
	return paths
}

func TestValidate_OrphanedPagesNode(t *testing.T) {
	doc, err := asposepdf.Open(testFile(t))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	paths := splitAndSave(t, doc, t.TempDir())

	for _, p := range paths {
		report, err := asposepdf.Validate(p)
		if err != nil {
			t.Fatalf("Validate %s: %v", p, err)
		}
		for _, issue := range report.Issues {
			if issue.Code == "PAGE_TREE_ERROR" {
				t.Errorf("%s: unexpected PAGE_TREE_ERROR: %s", p, issue.Message)
			}
		}
	}
}

func TestValidate_PageParentRef(t *testing.T) {
	// Binder1.pdf: its object #2 is a content stream, not /Pages.
	// Before the pdfDirectRef fix, /Parent in split pages pointed to that stream.
	doc, err := asposepdf.Open(testFile(t))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	paths := splitAndSave(t, doc, t.TempDir())

	for _, p := range paths {
		report, err := asposepdf.Validate(p)
		if err != nil {
			t.Fatalf("Validate %s: %v", p, err)
		}
		for _, issue := range report.Issues {
			if issue.Code == "PAGE_TREE_ERROR" {
				t.Errorf("%s: %s", p, issue.Message)
			}
		}
	}
}

func TestValidate_StrippedStreamFilter(t *testing.T) {
	// Binder1.pdf: before the Decoded flag fix, JPEG image streams were written
	// without /Filter, triggering a STREAM_ERROR.
	doc, err := asposepdf.Open(testFile(t))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	paths := splitAndSave(t, doc, t.TempDir())

	for _, p := range paths {
		report, err := asposepdf.Validate(p)
		if err != nil {
			t.Fatalf("Validate %s: %v", p, err)
		}
		for _, issue := range report.Issues {
			if issue.Code == "STREAM_ERROR" {
				t.Errorf("%s: %s", p, issue.Message)
			}
		}
	}
}

// hasIssue reports whether the report contains an issue with the given code.
func hasIssue(r *asposepdf.ValidationReport, code string) bool {
	for _, issue := range r.Issues {
		if issue.Code == code {
			return true
		}
	}
	return false
}
