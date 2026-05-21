// SPDX-License-Identifier: MIT

package asposepdf_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	asposepdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

func TestAddTextRoundTrip(t *testing.T) {
	// Create a blank A4 document.
	doc := asposepdf.NewDocumentFromFormat(asposepdf.PageFormatA4)
	page, err := doc.Page(1)
	if err != nil {
		t.Fatalf("page: %v", err)
	}

	// Add text with various styles.
	title := asposepdf.TextStyle{
		Font:   asposepdf.FontHelveticaBold,
		Size:   24,
		Color:  &asposepdf.Color{R: 0, G: 0, B: 0.8, A: 1},
		HAlign: asposepdf.HAlignCenter,
	}
	err = page.AddText("Hello, PDF!", title, asposepdf.Rectangle{
		LLX: 50, LLY: 750, URX: 545, URY: 800,
	})
	if err != nil {
		t.Fatalf("AddText title: %v", err)
	}

	body := asposepdf.TextStyle{
		Font:        asposepdf.FontTimesRoman,
		Size:        12,
		LineSpacing: 1.5,
		Underline:   true,
	}
	err = page.AddText("This is a longer paragraph that should wrap across multiple lines when the text exceeds the width of the rectangle.", body, asposepdf.Rectangle{
		LLX: 50, LLY: 600, URX: 300, URY: 740,
	})
	if err != nil {
		t.Fatalf("AddText body: %v", err)
	}

	bg := asposepdf.Color{R: 1, G: 1, B: 0, A: 0.3}
	highlight := asposepdf.TextStyle{
		Font:       asposepdf.FontCourier,
		Size:       10,
		Background: &bg,
		HAlign:     asposepdf.HAlignRight,
		VAlign:     asposepdf.VAlignBottom,
	}
	err = page.AddText("Right-bottom aligned", highlight, asposepdf.Rectangle{
		LLX: 300, LLY: 600, URX: 545, URY: 740,
	})
	if err != nil {
		t.Fatalf("AddText highlight: %v", err)
	}

	// Save.
	outDir := filepath.Join("result_files", "TestAddTextRoundTrip")
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

	// Reopen and extract text.
	reopened, err := asposepdf.Open(outPath)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	texts, err := reopened.ExtractText()
	if err != nil {
		t.Fatalf("extract text: %v", err)
	}
	if len(texts) == 0 {
		t.Fatal("no text extracted")
	}
	if !strings.Contains(texts[0], "Hello") {
		t.Errorf("extracted text does not contain 'Hello': %q", texts[0])
	}
	if !strings.Contains(texts[0], "paragraph") {
		t.Errorf("extracted text does not contain 'paragraph': %q", texts[0])
	}
}

func TestAddTextRotationRoundTrip(t *testing.T) {
	// Create a blank A4 document.
	doc := asposepdf.NewDocumentFromFormat(asposepdf.PageFormatA4)
	page, err := doc.Page(1)
	if err != nil {
		t.Fatalf("page: %v", err)
	}

	// Add normal foreground text.
	err = page.AddText("Normal text", asposepdf.TextStyle{
		Font: asposepdf.FontHelvetica,
		Size: 14,
	}, asposepdf.Rectangle{LLX: 50, LLY: 750, URX: 545, URY: 800})
	if err != nil {
		t.Fatalf("AddText normal: %v", err)
	}

	// Add rotated text behind content (watermark-style).
	gray := asposepdf.Color{R: 0.8, G: 0.8, B: 0.8, A: 0.3}
	err = page.AddText("CONFIDENTIAL", asposepdf.TextStyle{
		Font:     asposepdf.FontHelveticaBold,
		Size:     60,
		Color:    &gray,
		Rotation: 45,
		HAlign:   asposepdf.HAlignCenter,
		VAlign:   asposepdf.VAlignMiddle,
		Behind:   true,
	}, asposepdf.Rectangle{LLX: 50, LLY: 200, URX: 545, URY: 650})
	if err != nil {
		t.Fatalf("AddText rotated behind: %v", err)
	}

	// Save.
	outDir := filepath.Join("result_files", "TestAddTextRotationRoundTrip")
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

	// Reopen and extract text — both texts should be present.
	reopened, err := asposepdf.Open(outPath)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	texts, err := reopened.ExtractText()
	if err != nil {
		t.Fatalf("extract text: %v", err)
	}
	if len(texts) == 0 {
		t.Fatal("no text extracted")
	}
	if !strings.Contains(texts[0], "Normal") {
		t.Errorf("extracted text missing 'Normal': %q", texts[0])
	}
	if !strings.Contains(texts[0], "CONFIDENTIAL") {
		t.Errorf("extracted text missing 'CONFIDENTIAL': %q", texts[0])
	}
}

func TestAddTextWatermarkRoundTrip(t *testing.T) {
	// Create a 2-page document with some content.
	doc := asposepdf.NewDocumentFromFormat(asposepdf.PageFormatA4)
	doc.AddBlankPageFromFormat(asposepdf.PageFormatA4)

	page1, _ := doc.Page(1)
	page1.AddText("Page one content", asposepdf.TextStyle{
		Font: asposepdf.FontHelvetica,
		Size: 14,
	}, asposepdf.Rectangle{LLX: 50, LLY: 750, URX: 545, URY: 800})

	page2, _ := doc.Page(2)
	page2.AddText("Page two content", asposepdf.TextStyle{
		Font: asposepdf.FontHelvetica,
		Size: 14,
	}, asposepdf.Rectangle{LLX: 50, LLY: 750, URX: 545, URY: 800})

	// Add watermark to all pages.
	gray := asposepdf.Color{R: 0.8, G: 0.8, B: 0.8, A: 0.3}
	err := doc.AddTextWatermark("CONFIDENTIAL", asposepdf.TextStyle{
		Font:     asposepdf.FontHelveticaBold,
		Size:     60,
		Color:    &gray,
		Rotation: 45,
		HAlign:   asposepdf.HAlignCenter,
		VAlign:   asposepdf.VAlignMiddle,
		Behind:   true,
	})
	if err != nil {
		t.Fatalf("AddTextWatermark: %v", err)
	}

	// Save.
	outDir := filepath.Join("result_files", "TestAddTextWatermarkRoundTrip")
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

	// Reopen and verify text on both pages.
	reopened, err := asposepdf.Open(outPath)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	texts, err := reopened.ExtractText()
	if err != nil {
		t.Fatalf("extract text: %v", err)
	}
	if len(texts) < 2 {
		t.Fatalf("expected 2 pages, got %d", len(texts))
	}
	for i, text := range texts {
		if !strings.Contains(text, "CONFIDENTIAL") {
			t.Errorf("page %d missing watermark text: %q", i+1, text)
		}
	}
	if !strings.Contains(texts[0], "Page one") {
		t.Errorf("page 1 missing original content: %q", texts[0])
	}
	if !strings.Contains(texts[1], "Page two") {
		t.Errorf("page 2 missing original content: %q", texts[1])
	}
}

func TestAddTextEmbeddedFontRoundTrip(t *testing.T) {
	doc := asposepdf.NewDocumentFromFormat(asposepdf.PageFormatA4)

	font, err := doc.LoadFont("testdata/DejaVuSans.ttf")
	if err != nil {
		t.Fatalf("LoadFont: %v", err)
	}

	page, _ := doc.Page(1)
	err = page.AddText("Привет, мир!", asposepdf.TextStyle{
		Font: font,
		Size: 18,
	}, asposepdf.Rectangle{LLX: 50, LLY: 750, URX: 545, URY: 800})
	if err != nil {
		t.Fatalf("AddText cyrillic: %v", err)
	}
	err = page.AddText("Γειά σου κόσμε!", asposepdf.TextStyle{
		Font: font,
		Size: 18,
	}, asposepdf.Rectangle{LLX: 50, LLY: 700, URX: 545, URY: 750})
	if err != nil {
		t.Fatalf("AddText greek: %v", err)
	}

	outDir := filepath.Join("result_files", "TestAddTextEmbeddedFontRoundTrip")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatal(err)
	}
	outPath := filepath.Join(outDir, "output.pdf")
	if err := doc.Save(outPath); err != nil {
		t.Fatalf("save: %v", err)
	}

	report, err := asposepdf.Validate(outPath)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if !report.Valid {
		for _, issue := range report.Issues {
			t.Errorf("validation issue: [%s] %s", issue.Code, issue.Message)
		}
	}

	reopened, err := asposepdf.Open(outPath)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	texts, err := reopened.ExtractText()
	if err != nil {
		t.Fatalf("extract text: %v", err)
	}
	if len(texts) < 1 {
		t.Fatalf("no pages extracted")
	}
	if !strings.Contains(texts[0], "Привет") {
		t.Errorf("extracted text missing Cyrillic: %q", texts[0])
	}
	if !strings.Contains(texts[0], "κόσμε") {
		t.Errorf("extracted text missing Greek: %q", texts[0])
	}
}
