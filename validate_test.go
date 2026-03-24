package asposepdf_test

import (
	"os"
	"path/filepath"
	"testing"

	asposepdf "github.com/aspose/pdf-for-go"
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
	// Build an encrypted PDF by running Encrypt on our minimal PDF.
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

func TestValidate_OrphanedPagesNode(t *testing.T) {
	// Split a real PDF and validate each page — the fix in collectDeps must ensure
	// no orphaned /Pages objects appear in the output.
	inputPath := "test_data/split/4pages.pdf"
	outDir := t.TempDir()

	paths, err := asposepdf.Split(inputPath, outDir)
	if err != nil {
		t.Fatalf("Split: %v", err)
	}

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
	// Split Binder1.pdf — its object #2 is a content stream, not /Pages.
	// Before the pdfDirectRef fix, /Parent in split pages pointed to that stream.
	inputPath := "test_data/split/Binder1.pdf"
	outDir := t.TempDir()

	paths, err := asposepdf.Split(inputPath, outDir)
	if err != nil {
		t.Fatalf("Split: %v", err)
	}

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
	// Split Binder1.pdf and validate — before the Decoded flag fix, JPEG image
	// streams were written without /Filter, triggering a STREAM_ERROR.
	inputPath := "test_data/split/Binder1.pdf"
	outDir := t.TempDir()

	paths, err := asposepdf.Split(inputPath, outDir)
	if err != nil {
		t.Fatalf("Split: %v", err)
	}

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
