package asposepdf_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	asposepdf "github.com/aspose/pdf-for-go"
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
