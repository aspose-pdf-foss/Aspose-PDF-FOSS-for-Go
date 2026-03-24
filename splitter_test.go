package asposepdf_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	asposepdf "github.com/aspose/pdf-for-go"
)

func TestSplitSmallPDF(t *testing.T) {
	pdf := buildMinimalPDF()
	tmpDir := t.TempDir()

	inputPath := filepath.Join(tmpDir, "input.pdf")
	if err := os.WriteFile(inputPath, pdf, 0o644); err != nil {
		t.Fatalf("write test PDF: %v", err)
	}

	outDir := filepath.Join(tmpDir, "out")
	paths, err := asposepdf.Split(inputPath, outDir)
	if err != nil {
		t.Fatalf("Split: %v", err)
	}

	if len(paths) != 2 {
		t.Fatalf("expected 2 pages, got %d", len(paths))
	}

	for _, p := range paths {
		info, err := os.Stat(p)
		if err != nil {
			t.Errorf("output file missing: %v", err)
			continue
		}
		if info.Size() == 0 {
			t.Errorf("output file is empty: %s", p)
		}
	}
}

func TestSplitRange(t *testing.T) {
	pdf := buildMinimalPDF()
	tmpDir := t.TempDir()
	inputPath := filepath.Join(tmpDir, "input.pdf")
	if err := os.WriteFile(inputPath, pdf, 0o644); err != nil {
		t.Fatalf("write test PDF: %v", err)
	}

	outDir := filepath.Join(tmpDir, "out")

	// Split only page 2 of 2.
	paths, err := asposepdf.SplitRange(inputPath, outDir, 2, 2)
	if err != nil {
		t.Fatalf("SplitRange: %v", err)
	}
	if len(paths) != 1 {
		t.Fatalf("expected 1 file, got %d", len(paths))
	}

	// from > to must fail
	_, err = asposepdf.SplitRange(inputPath, outDir, 3, 1)
	if err == nil {
		t.Fatal("expected error for invalid range")
	}
}

func TestExtract(t *testing.T) {
	pdf := buildMinimalPDF()
	tmpDir := t.TempDir()
	inputPath := filepath.Join(tmpDir, "input.pdf")
	if err := os.WriteFile(inputPath, pdf, 0o644); err != nil {
		t.Fatalf("write test PDF: %v", err)
	}

	outputPath := filepath.Join(tmpDir, "extracted.pdf")

	// Extract only page 2 (of 2).
	err := asposepdf.Extract(inputPath, outputPath,
		asposepdf.PageRange{From: 2, To: 2},
	)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}

	n, err := asposepdf.PageCount(outputPath)
	if err != nil {
		t.Fatalf("PageCount on extracted PDF: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 page in extracted PDF, got %d", n)
	}

	// Extract pages 1 and 2 (all).
	outputPath2 := filepath.Join(tmpDir, "extracted2.pdf")
	err = asposepdf.Extract(inputPath, outputPath2,
		asposepdf.PageRange{From: 1, To: 2},
	)
	if err != nil {
		t.Fatalf("Extract all: %v", err)
	}
	n, err = asposepdf.PageCount(outputPath2)
	if err != nil {
		t.Fatalf("PageCount: %v", err)
	}
	if n != 2 {
		t.Fatalf("expected 2 pages, got %d", n)
	}

	// No ranges — must fail.
	if err := asposepdf.Extract(inputPath, outputPath); err == nil {
		t.Fatal("expected error for empty ranges")
	}
}

func TestSplitFunc(t *testing.T) {
	pdf := buildMinimalPDF()
	tmpDir := t.TempDir()
	inputPath := filepath.Join(tmpDir, "input.pdf")
	if err := os.WriteFile(inputPath, pdf, 0o644); err != nil {
		t.Fatalf("write test PDF: %v", err)
	}

	outDir := filepath.Join(tmpDir, "out")
	paths, err := asposepdf.SplitFunc(inputPath, outDir, 1, 0,
		func(page, total int) string {
			return fmt.Sprintf("p%d_of_%d.pdf", page, total)
		},
	)
	if err != nil {
		t.Fatalf("SplitFunc: %v", err)
	}
	if len(paths) != 2 {
		t.Fatalf("expected 2 pages, got %d", len(paths))
	}
	wantNames := []string{"p1_of_2.pdf", "p2_of_2.pdf"}
	for i, p := range paths {
		if filepath.Base(p) != wantNames[i] {
			t.Errorf("page %d: got filename %q, want %q", i+1, filepath.Base(p), wantNames[i])
		}
	}
}

func TestPageCount(t *testing.T) {
	pdf := buildMinimalPDF()
	tmpDir := t.TempDir()
	inputPath := filepath.Join(tmpDir, "input.pdf")
	if err := os.WriteFile(inputPath, pdf, 0o644); err != nil {
		t.Fatalf("write test PDF: %v", err)
	}

	n, err := asposepdf.PageCount(inputPath)
	if err != nil {
		t.Fatalf("PageCount: %v", err)
	}
	if n != 2 {
		t.Fatalf("expected 2, got %d", n)
	}
}

// buildMinimalPDF creates a hand-crafted 2-page PDF for testing.
func buildMinimalPDF() []byte {
	content1 := []byte("BT /F1 12 Tf 100 700 Td (Page 1) Tj ET")
	content2 := []byte("BT /F1 12 Tf 100 700 Td (Page 2) Tj ET")

	type pdfObj struct {
		num  int
		body []byte
	}

	objs := []pdfObj{
		{1, []byte("<< /Type /Catalog /Pages 2 0 R >>")},
		{2, []byte("<< /Type /Pages /Kids [3 0 R 5 0 R] /Count 2 >>")},
		{3, []byte("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents 4 0 R /Resources << /Font << /F1 7 0 R >> >> >>")},
		{4, makeStream(content1)},
		{5, []byte("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents 6 0 R /Resources << /Font << /F1 7 0 R >> >> >>")},
		{6, makeStream(content2)},
		{7, []byte("<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>")},
	}

	var buf []byte
	buf = append(buf, "%PDF-1.4\n"...)
	offsets := make([]int, len(objs))

	for i, o := range objs {
		offsets[i] = len(buf)
		buf = append(buf, fmt.Sprintf("%d 0 obj\n", o.num)...)
		buf = append(buf, o.body...)
		buf = append(buf, "\nendobj\n"...)
	}

	xrefOffset := len(buf)
	buf = append(buf, "xref\n"...)
	buf = append(buf, fmt.Sprintf("0 %d\n", len(objs)+1)...)
	buf = append(buf, "0000000000 65535 f \r\n"...)
	for _, off := range offsets {
		buf = append(buf, fmt.Sprintf("%010d 00000 n \r\n", off)...)
	}
	buf = append(buf, "trailer\n"...)
	buf = append(buf, fmt.Sprintf("<< /Size %d /Root 1 0 R >>\n", len(objs)+1)...)
	buf = append(buf, "startxref\n"...)
	buf = append(buf, fmt.Sprintf("%d\n", xrefOffset)...)
	buf = append(buf, "%%EOF\n"...)

	return buf
}

func makeStream(data []byte) []byte {
	return []byte(fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(data), data))
}

func TestSplitFiles(t *testing.T) {
	entries, err := os.ReadDir("test_data/split")
	if err != nil {
		t.Fatalf("read test_data/split: %v", err)
	}

	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		t.Run(name, func(t *testing.T) {
			inputPath := filepath.Join("test_data/split", name)
			stem := name[:len(name)-len(filepath.Ext(name))]
			outDir := filepath.Join("result_files", stem)

			paths, err := asposepdf.SplitFunc(inputPath, outDir, 1, 0,
				func(page, _ int) string {
					return fmt.Sprintf("%d.pdf", page)
				},
			)
			if err != nil {
				t.Fatalf("Split: %v", err)
			}
			if len(paths) == 0 {
				t.Fatal("no output files produced")
			}
			for _, p := range paths {
				info, err := os.Stat(p)
				if err != nil {
					t.Errorf("output file missing: %v", err)
					continue
				}
				if info.Size() == 0 {
					t.Errorf("output file is empty: %s", p)
					continue
				}
				report, err := asposepdf.Validate(p)
				if err != nil {
					t.Errorf("Validate %s: %v", p, err)
					continue
				}
				if !report.Valid {
					for _, issue := range report.Issues {
						t.Errorf("%s: %s: %s", filepath.Base(p), issue.Code, issue.Message)
					}
				}
			}
			t.Logf("split into %d pages → %s", len(paths), outDir)
		})
	}
}
