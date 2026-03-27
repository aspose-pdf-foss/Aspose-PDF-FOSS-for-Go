package asposepdf_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	asposepdf "github.com/aspose/pdf-for-go"
)

func checkValidation(t *testing.T, label string, report *asposepdf.ValidationReport) {
	t.Helper()
	for _, issue := range report.Issues {
		t.Errorf("%s: %s: %s", label, issue.Code, issue.Message)
	}
}

func TestDocumentSplit(t *testing.T) {
	pdf := buildMinimalPDF()
	tmpDir := t.TempDir()

	inputPath := filepath.Join(tmpDir, "input.pdf")
	if err := os.WriteFile(inputPath, pdf, 0o644); err != nil {
		t.Fatalf("write test PDF: %v", err)
	}

	doc, err := asposepdf.Open(inputPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	pages, err := doc.Split()
	if err != nil {
		t.Fatalf("Split: %v", err)
	}
	if len(pages) != 2 {
		t.Fatalf("expected 2 pages, got %d", len(pages))
	}

	// Save each page and verify the file exists and is non-empty.
	for i, p := range pages {
		outPath := filepath.Join(tmpDir, fmt.Sprintf("page%03d.pdf", i+1))
		if err := p.Save(outPath); err != nil {
			t.Fatalf("Save page %d: %v", i+1, err)
		}
		info, err := os.Stat(outPath)
		if err != nil {
			t.Errorf("output file missing: %v", err)
			continue
		}
		if info.Size() == 0 {
			t.Errorf("output file is empty: %s", outPath)
		}
	}
}

func TestDocumentSplitRange(t *testing.T) {
	pdf := buildMinimalPDF()
	tmpDir := t.TempDir()
	inputPath := filepath.Join(tmpDir, "input.pdf")
	if err := os.WriteFile(inputPath, pdf, 0o644); err != nil {
		t.Fatalf("write test PDF: %v", err)
	}

	doc, err := asposepdf.Open(inputPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	// Split only page 2 of 2.
	page2doc, err := doc.Extract(asposepdf.PageRange{From: 2, To: 2})
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	pages, err := page2doc.Split()
	if err != nil {
		t.Fatalf("Split: %v", err)
	}
	if len(pages) != 1 {
		t.Fatalf("expected 1 page, got %d", len(pages))
	}

	// from > to must fail.
	if _, err := doc.Extract(asposepdf.PageRange{From: 3, To: 1}); err == nil {
		t.Fatal("expected error for invalid range")
	}
}

func TestDocumentExtract(t *testing.T) {
	pdf := buildMinimalPDF()
	tmpDir := t.TempDir()
	inputPath := filepath.Join(tmpDir, "input.pdf")
	if err := os.WriteFile(inputPath, pdf, 0o644); err != nil {
		t.Fatalf("write test PDF: %v", err)
	}

	doc, err := asposepdf.Open(inputPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	// Extract only page 2 (of 2) — original doc must stay unchanged.
	extracted, err := doc.Extract(asposepdf.PageRange{From: 2, To: 2})
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if extracted.PageCount() != 1 {
		t.Fatalf("expected 1 page in extracted doc, got %d", extracted.PageCount())
	}
	if doc.PageCount() != 2 {
		t.Fatalf("original doc must not be mutated, got %d pages", doc.PageCount())
	}

	// Save to file and verify.
	outputPath := filepath.Join(tmpDir, "extracted.pdf")
	if err := extracted.Save(outputPath); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if n := pageCountFromFile(t, outputPath); n != 1 {
		t.Fatalf("expected 1 page in saved file, got %d", n)
	}

	// Extract pages 1 and 2 (all).
	extracted2, err := doc.Extract(asposepdf.PageRange{From: 1, To: 2})
	if err != nil {
		t.Fatalf("Extract all: %v", err)
	}
	if extracted2.PageCount() != 2 {
		t.Fatalf("expected 2 pages, got %d", extracted2.PageCount())
	}

	// No ranges — must fail.
	if _, err := doc.Extract(); err == nil {
		t.Fatal("expected error for empty ranges")
	}
}

func TestExtractFiles(t *testing.T) {
	entries, err := os.ReadDir("testdata/split")
	if err != nil {
		t.Fatalf("read testdata/split: %v", err)
	}

	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		t.Run(name, func(t *testing.T) {
			inputPath := filepath.Join("testdata/split", name)
			stem := name[:len(name)-len(filepath.Ext(name))]
			outDir := filepath.Join("result_files", "TestExtractFiles", stem)

			total := pageCountFromFile(t, inputPath)
			if total < 2 {
				t.Skipf("need at least 2 pages, got %d", total)
			}

			mid := total / 2

			if err := os.MkdirAll(outDir, 0o755); err != nil {
				t.Fatalf("MkdirAll: %v", err)
			}

			doc, err := asposepdf.Open(inputPath)
			if err != nil {
				t.Fatalf("Open: %v", err)
			}

			cases := []struct {
				name      string
				from, to  int
				wantPages int
			}{
				{"first_half.pdf", 1, mid, mid},
				{"second_half.pdf", mid + 1, total, total - mid},
			}

			for _, c := range cases {
				extracted, err := doc.Extract(asposepdf.PageRange{From: c.from, To: c.to})
				if err != nil {
					t.Fatalf("Extract %s: %v", c.name, err)
				}

				outPath := filepath.Join(outDir, c.name)
				if err := extracted.Save(outPath); err != nil {
					t.Fatalf("Save %s: %v", c.name, err)
				}

				if got := pageCountFromFile(t, outPath); got != c.wantPages {
					t.Errorf("%s: expected %d pages, got %d", c.name, c.wantPages, got)
				}

				report, err := asposepdf.Validate(outPath)
				if err != nil {
					t.Fatalf("Validate %s: %v", c.name, err)
				}
				checkValidation(t, c.name, report)
			}
			t.Logf("%s (%d pages) → first_half=%d second_half=%d", stem, total, mid, total-mid)
		})
	}
}

func TestPageCount(t *testing.T) {
	pdf := buildMinimalPDF()
	tmpDir := t.TempDir()
	inputPath := filepath.Join(tmpDir, "input.pdf")
	if err := os.WriteFile(inputPath, pdf, 0o644); err != nil {
		t.Fatalf("write test PDF: %v", err)
	}

	if n := pageCountFromFile(t, inputPath); n != 2 {
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
	entries, err := os.ReadDir("testdata/split")
	if err != nil {
		t.Fatalf("read testdata/split: %v", err)
	}

	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		t.Run(name, func(t *testing.T) {
			inputPath := filepath.Join("testdata/split", name)
			stem := name[:len(name)-len(filepath.Ext(name))]
			outDir := filepath.Join("result_files", "TestSplitFiles", stem)

			doc, err := asposepdf.Open(inputPath)
			if err != nil {
				t.Fatalf("Open: %v", err)
			}

			pages, err := doc.Split()
			if err != nil {
				t.Fatalf("Split: %v", err)
			}
			if len(pages) == 0 {
				t.Fatal("no pages produced")
			}

			if err := os.MkdirAll(outDir, 0o755); err != nil {
				t.Fatalf("MkdirAll: %v", err)
			}

			for i, p := range pages {
				outPath := filepath.Join(outDir, fmt.Sprintf("%d.pdf", i+1))
				if err := p.Save(outPath); err != nil {
					t.Errorf("Save page %d: %v", i+1, err)
					continue
				}
				info, err := os.Stat(outPath)
				if err != nil {
					t.Errorf("output file missing: %v", err)
					continue
				}
				if info.Size() == 0 {
					t.Errorf("output file is empty: %s", outPath)
					continue
				}
				report, err := asposepdf.Validate(outPath)
				if err != nil {
					t.Errorf("Validate %s: %v", outPath, err)
					continue
				}
				checkValidation(t, fmt.Sprintf("%d.pdf", i+1), report)
			}
			t.Logf("split into %d pages → %s", len(pages), outDir)
		})
	}
}
