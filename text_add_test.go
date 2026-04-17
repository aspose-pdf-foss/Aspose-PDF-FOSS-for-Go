package asposepdf

import (
	"strings"
	"testing"
)

func TestFontPDFName(t *testing.T) {
	cases := []struct {
		font Font
		want string
	}{
		{FontHelvetica, "/Helvetica"},
		{FontHelveticaBold, "/Helvetica-Bold"},
		{FontHelveticaOblique, "/Helvetica-Oblique"},
		{FontHelveticaBoldOblique, "/Helvetica-BoldOblique"},
		{FontTimesRoman, "/Times-Roman"},
		{FontTimesBold, "/Times-Bold"},
		{FontTimesItalic, "/Times-Italic"},
		{FontTimesBoldItalic, "/Times-BoldItalic"},
		{FontCourier, "/Courier"},
		{FontCourierBold, "/Courier-Bold"},
		{FontCourierOblique, "/Courier-Oblique"},
		{FontCourierBoldOblique, "/Courier-BoldOblique"},
		{FontSymbol, "/Symbol"},
		{FontZapfDingbats, "/ZapfDingbats"},
	}
	for _, tc := range cases {
		got := fontPDFName(tc.font)
		if got != tc.want {
			t.Errorf("fontPDFName(%d) = %q, want %q", tc.font, got, tc.want)
		}
	}
}

func TestFontPDFNameInvalid(t *testing.T) {
	got := fontPDFName(Font(999))
	if got != "/Helvetica" {
		t.Errorf("fontPDFName(999) = %q, want /Helvetica (fallback)", got)
	}
}

func TestWrapTextSingleLine(t *testing.T) {
	widths, _ := standard14Widths("/Helvetica")
	lines := wrapText("Hello", widths, 12, 500)
	if len(lines) != 1 || lines[0] != "Hello" {
		t.Errorf("wrapText single line = %v, want [Hello]", lines)
	}
}

func TestWrapTextMultiLine(t *testing.T) {
	widths, _ := standard14Widths("/Helvetica")
	// At 12pt Helvetica, "Hello World" is about 60pt wide.
	// With maxWidth=40, "Hello" (~30pt) fits, "World" wraps.
	lines := wrapText("Hello World", widths, 12, 40)
	if len(lines) != 2 {
		t.Fatalf("wrapText = %v, want 2 lines", lines)
	}
	if lines[0] != "Hello" || lines[1] != "World" {
		t.Errorf("wrapText = %v, want [Hello, World]", lines)
	}
}

func TestWrapTextLongWord(t *testing.T) {
	widths, _ := standard14Widths("/Helvetica")
	// A single long word that exceeds maxWidth must be broken by character.
	lines := wrapText("ABCDEFGHIJKLMNOP", widths, 12, 50)
	if len(lines) < 2 {
		t.Fatalf("wrapText long word = %v, expected multiple lines", lines)
	}
	// Concatenation of all lines should equal the original.
	joined := ""
	for _, l := range lines {
		joined += l
	}
	if joined != "ABCDEFGHIJKLMNOP" {
		t.Errorf("joined = %q, want ABCDEFGHIJKLMNOP", joined)
	}
}

func TestWrapTextNewlines(t *testing.T) {
	widths, _ := standard14Widths("/Helvetica")
	lines := wrapText("Line1\nLine2\nLine3", widths, 12, 500)
	if len(lines) != 3 {
		t.Fatalf("wrapText newlines = %v, want 3 lines", lines)
	}
	if lines[0] != "Line1" || lines[1] != "Line2" || lines[2] != "Line3" {
		t.Errorf("wrapText newlines = %v", lines)
	}
}

func TestWrapTextEmpty(t *testing.T) {
	widths, _ := standard14Widths("/Helvetica")
	lines := wrapText("", widths, 12, 500)
	if len(lines) != 0 {
		t.Errorf("wrapText empty = %v, want []", lines)
	}
}

func TestAddTextEmptyString(t *testing.T) {
	doc := NewDocument(595, 842)
	page, _ := doc.Page(1)
	err := page.AddText("", TextStyle{}, Rectangle{LLX: 50, LLY: 700, URX: 300, URY: 750})
	if err != nil {
		t.Fatalf("AddText empty: %v", err)
	}
}

func TestAddTextInvalidRect(t *testing.T) {
	doc := NewDocument(595, 842)
	page, _ := doc.Page(1)
	err := page.AddText("Hello", TextStyle{}, Rectangle{LLX: 300, LLY: 700, URX: 50, URY: 750})
	if err == nil {
		t.Fatal("expected error for invalid rect")
	}
}

func TestAddTextInvalidSize(t *testing.T) {
	doc := NewDocument(595, 842)
	page, _ := doc.Page(1)
	err := page.AddText("Hello", TextStyle{Size: -5}, Rectangle{LLX: 50, LLY: 700, URX: 300, URY: 750})
	if err == nil {
		t.Fatal("expected error for negative size")
	}
}

func TestAddTextInvalidFont(t *testing.T) {
	doc := NewDocument(595, 842)
	page, _ := doc.Page(1)
	err := page.AddText("Hello", TextStyle{Font: Font(999)}, Rectangle{LLX: 50, LLY: 700, URX: 300, URY: 750})
	if err == nil {
		t.Fatal("expected error for invalid font")
	}
}

func TestAddTextDefaultStyle(t *testing.T) {
	doc := NewDocument(595, 842)
	page, _ := doc.Page(1)
	err := page.AddText("Hello", TextStyle{}, Rectangle{LLX: 50, LLY: 700, URX: 300, URY: 750})
	if err != nil {
		t.Fatalf("AddText default style: %v", err)
	}
	data, err := page.contentStreams()
	if err != nil {
		t.Fatalf("contentStreams: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "BT") {
		t.Error("content stream missing BT")
	}
	if !strings.Contains(content, "(Hello) Tj") {
		t.Error("content stream missing (Hello) Tj")
	}
	if !strings.Contains(content, "/F") {
		t.Error("content stream missing font reference")
	}
}

func TestAddTextAlignment(t *testing.T) {
	doc := NewDocument(595, 842)
	page, _ := doc.Page(1)
	style := TextStyle{
		Size:   12,
		HAlign: HAlignCenter,
		VAlign: VAlignMiddle,
	}
	err := page.AddText("Test", style, Rectangle{LLX: 0, LLY: 0, URX: 200, URY: 100})
	if err != nil {
		t.Fatalf("AddText center/middle: %v", err)
	}
	data, _ := page.contentStreams()
	content := string(data)
	if !strings.Contains(content, "BT") || !strings.Contains(content, "(Test) Tj") {
		t.Error("missing text operators")
	}
}

func TestAddTextBackground(t *testing.T) {
	doc := NewDocument(595, 842)
	page, _ := doc.Page(1)
	bg := Color{R: 1, G: 1, B: 0, A: 1}
	style := TextStyle{Background: &bg}
	err := page.AddText("Hello", style, Rectangle{LLX: 50, LLY: 700, URX: 300, URY: 750})
	if err != nil {
		t.Fatalf("AddText background: %v", err)
	}
	data, _ := page.contentStreams()
	content := string(data)
	if !strings.Contains(content, "re f") {
		t.Error("content stream missing background rectangle fill")
	}
}

func TestAddTextTransparency(t *testing.T) {
	doc := NewDocument(595, 842)
	page, _ := doc.Page(1)
	c := Color{R: 0, G: 0, B: 0, A: 0.5}
	style := TextStyle{Color: &c}
	err := page.AddText("Hello", style, Rectangle{LLX: 50, LLY: 700, URX: 300, URY: 750})
	if err != nil {
		t.Fatalf("AddText transparency: %v", err)
	}
	resources := page.pageResources()
	gsVal := resolveRef(page.doc.objects, resources["/ExtGState"])
	gsDict, ok := gsVal.(pdfDict)
	if !ok || len(gsDict) == 0 {
		t.Error("expected ExtGState resource for alpha < 1")
	}
}

func TestAddTextUnderline(t *testing.T) {
	doc := NewDocument(595, 842)
	page, _ := doc.Page(1)
	style := TextStyle{Underline: true}
	err := page.AddText("Hello", style, Rectangle{LLX: 50, LLY: 700, URX: 300, URY: 750})
	if err != nil {
		t.Fatalf("AddText underline: %v", err)
	}
	data, _ := page.contentStreams()
	content := string(data)
	etIdx := strings.Index(content, "ET")
	reIdx := strings.LastIndex(content, "re f")
	if etIdx < 0 || reIdx < 0 || reIdx < etIdx {
		t.Error("expected underline rectangle after ET")
	}
}

func TestAddTextStrikethrough(t *testing.T) {
	doc := NewDocument(595, 842)
	page, _ := doc.Page(1)
	style := TextStyle{Strikethrough: true}
	err := page.AddText("Hello", style, Rectangle{LLX: 50, LLY: 700, URX: 300, URY: 750})
	if err != nil {
		t.Fatalf("AddText strikethrough: %v", err)
	}
	data, _ := page.contentStreams()
	content := string(data)
	etIdx := strings.Index(content, "ET")
	reIdx := strings.LastIndex(content, "re f")
	if etIdx < 0 || reIdx < 0 || reIdx < etIdx {
		t.Error("expected strikethrough rectangle after ET")
	}
}

func TestAddTextBehind(t *testing.T) {
	doc := NewDocument(595, 842)
	page, _ := doc.Page(1)

	// Add initial text (appears first in content stream).
	err := page.AddText("Foreground", TextStyle{}, Rectangle{LLX: 50, LLY: 700, URX: 300, URY: 750})
	if err != nil {
		t.Fatalf("AddText foreground: %v", err)
	}

	// Add behind text (should appear before the foreground text).
	err = page.AddText("Background", TextStyle{Behind: true}, Rectangle{LLX: 50, LLY: 700, URX: 300, URY: 750})
	if err != nil {
		t.Fatalf("AddText behind: %v", err)
	}

	data, _ := page.contentStreams()
	content := string(data)
	bgIdx := strings.Index(content, "(Background) Tj")
	fgIdx := strings.Index(content, "(Foreground) Tj")
	if bgIdx < 0 || fgIdx < 0 {
		t.Fatalf("missing text operators; content:\n%s", content)
	}
	if bgIdx > fgIdx {
		t.Errorf("Behind text should appear before foreground text in content stream; bg at %d, fg at %d", bgIdx, fgIdx)
	}
}

func TestAddTextClipping(t *testing.T) {
	doc := NewDocument(595, 842)
	page, _ := doc.Page(1)
	err := page.AddText("Hello", TextStyle{}, Rectangle{LLX: 50, LLY: 700, URX: 300, URY: 750})
	if err != nil {
		t.Fatalf("AddText: %v", err)
	}
	data, _ := page.contentStreams()
	content := string(data)
	if !strings.Contains(content, "re W n") {
		t.Error("content stream missing clipping path")
	}
	if !strings.Contains(content, "\nq\n") {
		t.Error("content stream missing q (save state)")
	}
	if !strings.Contains(content, "\nQ\n") {
		t.Error("content stream missing Q (restore state)")
	}
}

func TestAddTextRotation(t *testing.T) {
	doc := NewDocument(595, 842)
	page, _ := doc.Page(1)
	style := TextStyle{Rotation: 45}
	err := page.AddText("Rotated", style, Rectangle{LLX: 100, LLY: 500, URX: 200, URY: 600})
	if err != nil {
		t.Fatalf("AddText rotation: %v", err)
	}
	data, _ := page.contentStreams()
	content := string(data)
	if !strings.Contains(content, "cm") {
		t.Error("content stream missing cm operator for rotation")
	}
	if !strings.Contains(content, "(Rotated) Tj") {
		t.Error("content stream missing text")
	}
}

func TestAddTextRotationZero(t *testing.T) {
	doc := NewDocument(595, 842)
	page, _ := doc.Page(1)
	err := page.AddText("NoRotation", TextStyle{}, Rectangle{LLX: 50, LLY: 700, URX: 300, URY: 750})
	if err != nil {
		t.Fatalf("AddText: %v", err)
	}
	data, _ := page.contentStreams()
	content := string(data)
	if strings.Contains(content, "cm") {
		t.Error("content stream should not contain cm operator when Rotation is 0")
	}
}
