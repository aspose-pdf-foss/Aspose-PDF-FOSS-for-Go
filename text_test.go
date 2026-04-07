package asposepdf_test

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	asposepdf "github.com/aspose/pdf-for-go"
)

func TestExtractTextMinimal(t *testing.T) {
	pdf := buildMinimalPDF()
	doc, err := asposepdf.OpenStream(bytes.NewReader(pdf))
	if err != nil {
		t.Fatalf("OpenStream: %v", err)
	}
	page1, err := doc.Page(1)
	if err != nil {
		t.Fatalf("Page(1): %v", err)
	}
	text, err := page1.ExtractText()
	if err != nil {
		t.Fatalf("ExtractText: %v", err)
	}
	if !strings.Contains(text, "Page 1") {
		t.Errorf("page 1 text=%q, want it to contain 'Page 1'", text)
	}

	page2, err := doc.Page(2)
	if err != nil {
		t.Fatalf("Page(2): %v", err)
	}
	text2, err := page2.ExtractText()
	if err != nil {
		t.Fatalf("ExtractText: %v", err)
	}
	if !strings.Contains(text2, "Page 2") {
		t.Errorf("page 2 text=%q, want it to contain 'Page 2'", text2)
	}
}

func TestDocumentExtractText(t *testing.T) {
	pdf := buildMinimalPDF()
	doc, err := asposepdf.OpenStream(bytes.NewReader(pdf))
	if err != nil {
		t.Fatalf("OpenStream: %v", err)
	}
	texts, err := doc.ExtractText()
	if err != nil {
		t.Fatalf("ExtractText: %v", err)
	}
	if len(texts) != 2 {
		t.Fatalf("expected 2 pages, got %d", len(texts))
	}
	if !strings.Contains(texts[0], "Page 1") {
		t.Errorf("page 1: %q", texts[0])
	}
	if !strings.Contains(texts[1], "Page 2") {
		t.Errorf("page 2: %q", texts[1])
	}
}

func TestExtractTextTJ(t *testing.T) {
	content := []byte("BT /F1 12 Tf 100 700 Td [(H) -10 (ello)] TJ ET")
	pdf := buildPDFWithContent(content)
	doc, err := asposepdf.OpenStream(bytes.NewReader(pdf))
	if err != nil {
		t.Fatalf("OpenStream: %v", err)
	}
	page, _ := doc.Page(1)
	text, err := page.ExtractText()
	if err != nil {
		t.Fatalf("ExtractText: %v", err)
	}
	if !strings.Contains(text, "Hello") {
		t.Errorf("text=%q, want it to contain 'Hello'", text)
	}
}

func TestExtractTextMultiline(t *testing.T) {
	content := []byte("BT /F1 12 Tf 100 700 Td (Line One) Tj 0 -14 Td (Line Two) Tj ET")
	pdf := buildPDFWithContent(content)
	doc, err := asposepdf.OpenStream(bytes.NewReader(pdf))
	if err != nil {
		t.Fatalf("OpenStream: %v", err)
	}
	page, _ := doc.Page(1)
	text, err := page.ExtractText()
	if err != nil {
		t.Fatalf("ExtractText: %v", err)
	}
	if !strings.Contains(text, "Line One") {
		t.Errorf("text=%q, missing 'Line One'", text)
	}
	if !strings.Contains(text, "Line Two") {
		t.Errorf("text=%q, missing 'Line Two'", text)
	}
	if !strings.Contains(text, "\n") {
		t.Errorf("text=%q, expected newline between lines", text)
	}
}

func TestExtractTextUnknownFont(t *testing.T) {
	content := []byte("BT /F1 12 Tf 100 700 Td (ABC) Tj ET")
	unknownFont := []byte("<< /Type /Font /Subtype /Type1 /BaseFont /UnknownCustomFont+XYZ >>")
	pdf := buildPDFWithContentAndFont(content, unknownFont)
	doc, err := asposepdf.OpenStream(bytes.NewReader(pdf))
	if err != nil {
		t.Fatalf("OpenStream: %v", err)
	}
	page, _ := doc.Page(1)
	text, err := page.ExtractText()
	if err != nil {
		t.Fatalf("ExtractText should not error: %v", err)
	}
	if !strings.ContainsRune(text, '\uFFFD') {
		t.Errorf("expected U+FFFD for unknown font, got %q", text)
	}
}

func TestExtractTextFormXObject(t *testing.T) {
	formContent := []byte("BT /F1 12 Tf 100 700 Td (Form Text) Tj ET")
	formStream := fmt.Sprintf("<< /Type /XObject /Subtype /Form /BBox [0 0 612 792] /Resources << /Font << /F1 6 0 R >> >> /Length %d >>\nstream\n%s\nendstream", len(formContent), formContent)
	pageContent := []byte("/Fm1 Do BT /F1 12 Tf 100 500 Td (Page Text) Tj ET")
	pageStream := fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(pageContent), pageContent)

	pdf := extAssemblePDF([]extTestObj{
		{1, []byte("<< /Type /Catalog /Pages 2 0 R >>")},
		{2, []byte("<< /Type /Pages /Kids [3 0 R] /Count 1 >>")},
		{3, []byte("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents 4 0 R /Resources << /Font << /F1 6 0 R >> /XObject << /Fm1 5 0 R >> >> >>")},
		{4, []byte(pageStream)},
		{5, []byte(formStream)},
		{6, []byte("<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>")},
	})
	doc, err := asposepdf.OpenStream(bytes.NewReader(pdf))
	if err != nil {
		t.Fatalf("OpenStream: %v", err)
	}
	page, _ := doc.Page(1)
	text, err := page.ExtractText()
	if err != nil {
		t.Fatalf("ExtractText: %v", err)
	}
	if !strings.Contains(text, "Page Text") {
		t.Errorf("text=%q, missing 'Page Text'", text)
	}
	if !strings.Contains(text, "Form Text") {
		t.Errorf("text=%q, missing 'Form Text'", text)
	}
}

func TestExtractTextFiles(t *testing.T) {
	for _, inputPath := range testFiles(t) {
		t.Run(stem(inputPath), func(t *testing.T) {
			doc, err := asposepdf.Open(inputPath)
			if err != nil {
				t.Fatalf("Open: %v", err)
			}
			texts, err := doc.ExtractText()
			if err != nil {
				t.Fatalf("ExtractText: %v", err)
			}
			if len(texts) != doc.PageCount() {
				t.Fatalf("expected %d pages, got %d", doc.PageCount(), len(texts))
			}

			outDir := filepath.Join(resultDir, "TestExtractTextFiles", stem(inputPath))
			if err := os.MkdirAll(outDir, 0o755); err != nil {
				t.Fatalf("mkdir: %v", err)
			}

			var allText strings.Builder
			for i, text := range texts {
				trimmed := strings.TrimSpace(text)
				if len(trimmed) == 0 {
					t.Logf("page %d: empty text (may be image-only)", i+1)
				} else {
					cleaned := strings.ReplaceAll(trimmed, "\uFFFD", "")
					cleaned = strings.TrimSpace(cleaned)
					if len(cleaned) == 0 {
						t.Logf("page %d: all characters are U+FFFD (unknown encoding)", i+1)
					}
				}

				// Save per-page text.
				pagePath := filepath.Join(outDir, fmt.Sprintf("page_%d.txt", i+1))
				if err := os.WriteFile(pagePath, []byte(text), 0o644); err != nil {
					t.Fatalf("write page %d: %v", i+1, err)
				}

				if i > 0 {
					allText.WriteString("\n\n")
				}
				allText.WriteString(fmt.Sprintf("--- Page %d ---\n", i+1))
				allText.WriteString(text)
			}

			// Save full document text.
			fullPath := filepath.Join(outDir, "full_text.txt")
			if err := os.WriteFile(fullPath, []byte(allText.String()), 0o644); err != nil {
				t.Fatalf("write full text: %v", err)
			}

			t.Logf("%s: extracted text from %d pages, saved to %s", stem(inputPath), len(texts), outDir)
		})
	}
}

func TestExtractTextNoSpuriousSpaces(t *testing.T) {
	// Simulate the pattern that causes "shap e":
	// (shap) Tj <advance-by-width-of-shap> Td (e the) Tj
	// With Helvetica at 12pt, "shap" widths: s=556, h=556, a=556, p=556 = 2224
	// In text space: 2224/1000 * 12 = 26.688 points
	content := []byte("BT /F1 12 Tf 100 700 Td (shap) Tj 26.688 0 Td (e the) Tj ET")
	pdf := buildPDFWithContent(content)
	doc, err := asposepdf.OpenStream(bytes.NewReader(pdf))
	if err != nil {
		t.Fatalf("OpenStream: %v", err)
	}
	page, _ := doc.Page(1)
	text, err := page.ExtractText()
	if err != nil {
		t.Fatalf("ExtractText: %v", err)
	}
	// With correct glyph advance, "shap" advances tm by ~26.688,
	// then Td moves by 26.688, so dx ≈ 0 — no space inserted.
	if strings.Contains(text, "shap e") {
		t.Errorf("spurious space detected: %q", text)
	}
	if !strings.Contains(text, "shape the") {
		t.Errorf("text=%q, want it to contain 'shape the'", text)
	}
}

func TestExtractTextHorizScaling(t *testing.T) {
	// Tz 200 doubles horizontal scaling — glyph advance doubles.
	// Helvetica 'A' width = 667, at 12pt normal advance = 667/1000*12 = 8.004
	// With Tz 200: advance = 8.004 * 2 = 16.008
	content := []byte("BT /F1 12 Tf 200 Tz 100 700 Td (A) Tj 16.008 0 Td (B) Tj ET")
	pdf := buildPDFWithContent(content)
	doc, err := asposepdf.OpenStream(bytes.NewReader(pdf))
	if err != nil {
		t.Fatalf("OpenStream: %v", err)
	}
	page, _ := doc.Page(1)
	text, err := page.ExtractText()
	if err != nil {
		t.Fatalf("ExtractText: %v", err)
	}
	if strings.Contains(text, "A B") {
		t.Errorf("spurious space with Tz: %q", text)
	}
	if !strings.Contains(text, "AB") {
		t.Errorf("text=%q, want 'AB'", text)
	}
}

// --- Test helpers ---

type extTestObj struct {
	num  int
	body []byte
}

func extAssemblePDF(objs []extTestObj) []byte {
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

func extMakeStream(data []byte) []byte {
	return []byte(fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(data), data))
}

func buildPDFWithContent(content []byte) []byte {
	return extAssemblePDF([]extTestObj{
		{1, []byte("<< /Type /Catalog /Pages 2 0 R >>")},
		{2, []byte("<< /Type /Pages /Kids [3 0 R] /Count 1 >>")},
		{3, []byte("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents 4 0 R /Resources << /Font << /F1 5 0 R >> >> >>")},
		{4, extMakeStream(content)},
		{5, []byte("<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>")},
	})
}

func buildPDFWithContentAndFont(content []byte, fontObj []byte) []byte {
	return extAssemblePDF([]extTestObj{
		{1, []byte("<< /Type /Catalog /Pages 2 0 R >>")},
		{2, []byte("<< /Type /Pages /Kids [3 0 R] /Count 1 >>")},
		{3, []byte("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents 4 0 R /Resources << /Font << /F1 5 0 R >> >> >>")},
		{4, extMakeStream(content)},
		{5, fontObj},
	})
}
