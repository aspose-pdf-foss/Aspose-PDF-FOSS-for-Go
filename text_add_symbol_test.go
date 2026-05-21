package asposepdf_test

import (
	"bytes"
	"strings"
	"testing"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

// TestAddTextSymbolUsesSymbolEncoding verifies that AddText with FontSymbol
// encodes glyphs via the Symbol font's built-in encoding table, not WinAnsi.
//
// Regression: AddText took the same WinAnsi path for every standardFont.
// Characters outside WinAnsi (Greek letters, math operators) fell back to
// '?' on the write side, while the reader side in font.go already handles
// Symbol's encoding correctly. Result: AddText("α", FontSymbol, ...) round-
// tripped through Save+Open+ExtractText as "?" instead of "α".
func TestAddTextSymbolUsesSymbolEncoding(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, err := doc.Page(1)
	if err != nil {
		t.Fatalf("Page(1): %v", err)
	}

	const input = "αβ≤"
	err = page.AddText(input, pdf.TextStyle{
		Font: pdf.FontSymbol,
		Size: 20,
	}, pdf.Rectangle{LLX: 50, LLY: 700, URX: 545, URY: 780})
	if err != nil {
		t.Fatalf("AddText: %v", err)
	}

	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatalf("WriteTo: %v", err)
	}

	reopened, err := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	rePage, err := reopened.Page(1)
	if err != nil {
		t.Fatalf("reopen Page(1): %v", err)
	}
	text, err := rePage.ExtractText()
	if err != nil {
		t.Fatalf("ExtractText: %v", err)
	}
	for _, r := range input {
		if !strings.ContainsRune(text, r) {
			t.Errorf("extracted text %q does not contain %q — Symbol encoding not applied in AddText", text, r)
		}
	}
	if strings.Contains(text, "?") {
		t.Errorf("extracted text %q contains '?' — WinAnsi fallback emitted instead of Symbol encoding", text)
	}
}

// TestAddTextZapfDingbatsUsesZapfEncoding is the ZapfDingbats counterpart.
// Uses a few common ZapfDingbats glyphs to verify encoding selection.
func TestAddTextZapfDingbatsUsesZapfEncoding(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, err := doc.Page(1)
	if err != nil {
		t.Fatalf("Page(1): %v", err)
	}

	// ✈ (U+2708 airplane), ✔ (U+2714 heavy checkmark), ❤ (U+2764 heart)
	const input = "✈✔❤"
	err = page.AddText(input, pdf.TextStyle{
		Font: pdf.FontZapfDingbats,
		Size: 20,
	}, pdf.Rectangle{LLX: 50, LLY: 700, URX: 545, URY: 780})
	if err != nil {
		t.Fatalf("AddText: %v", err)
	}

	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatalf("WriteTo: %v", err)
	}

	reopened, err := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	rePage, err := reopened.Page(1)
	if err != nil {
		t.Fatalf("reopen Page(1): %v", err)
	}
	text, err := rePage.ExtractText()
	if err != nil {
		t.Fatalf("ExtractText: %v", err)
	}
	for _, r := range input {
		if !strings.ContainsRune(text, r) {
			t.Errorf("extracted text %q does not contain %q — ZapfDingbats encoding not applied", text, r)
		}
	}
	if strings.Contains(text, "?") {
		t.Errorf("extracted text %q contains '?' — WinAnsi fallback emitted for ZapfDingbats", text)
	}
}
