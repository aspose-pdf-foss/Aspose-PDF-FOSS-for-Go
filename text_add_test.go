// SPDX-License-Identifier: MIT

package asposepdf

import (
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestStandardFontBaseFont(t *testing.T) {
	cases := []struct {
		font Font
		want string
	}{
		{FontHelvetica, "Helvetica"},
		{FontHelveticaBold, "Helvetica-Bold"},
		{FontHelveticaOblique, "Helvetica-Oblique"},
		{FontHelveticaBoldOblique, "Helvetica-BoldOblique"},
		{FontTimesRoman, "Times-Roman"},
		{FontTimesBold, "Times-Bold"},
		{FontTimesItalic, "Times-Italic"},
		{FontTimesBoldItalic, "Times-BoldItalic"},
		{FontCourier, "Courier"},
		{FontCourierBold, "Courier-Bold"},
		{FontCourierOblique, "Courier-Oblique"},
		{FontCourierBoldOblique, "Courier-BoldOblique"},
		{FontSymbol, "Symbol"},
		{FontZapfDingbats, "ZapfDingbats"},
	}
	for _, tc := range cases {
		if got := tc.font.BaseFont(); got != tc.want {
			t.Errorf("%T.BaseFont() = %q, want %q", tc.font, got, tc.want)
		}
		if tc.font.IsEmbedded() {
			t.Errorf("%v.IsEmbedded() = true, want false for standard 14", tc.font)
		}
	}
}

// helveticaWidthFn builds a widthFn for Helvetica at the given font size.
func helveticaWidthFn(t *testing.T, size float64) widthFn {
	t.Helper()
	w, ok := standard14Widths("/Helvetica")
	if !ok {
		t.Fatalf("standard14Widths Helvetica not found")
	}
	return func(r rune) float64 {
		code, ok := winAnsiEncodeRune(r)
		if !ok {
			code = byte('?')
		}
		return w[code] / 1000.0 * size
	}
}

func TestWrapTextSingleLine(t *testing.T) {
	lines := wrapText("Hello", helveticaWidthFn(t, 12), 500)
	if len(lines) != 1 || lines[0] != "Hello" {
		t.Errorf("wrapText single line = %v, want [Hello]", lines)
	}
}

func TestWrapTextMultipleLines(t *testing.T) {
	lines := wrapText("Hello World", helveticaWidthFn(t, 12), 40)
	if len(lines) != 2 {
		t.Fatalf("wrapText = %v, want 2 lines", lines)
	}
	if lines[0] != "Hello" || lines[1] != "World" {
		t.Errorf("wrapText = %v, want [Hello, World]", lines)
	}
}

func TestWrapTextLongWord(t *testing.T) {
	lines := wrapText("ABCDEFGHIJKLMNOP", helveticaWidthFn(t, 12), 50)
	if len(lines) < 2 {
		t.Fatalf("wrapText long word = %v, expected multiple lines", lines)
	}
}

func TestWrapTextNewlines(t *testing.T) {
	lines := wrapText("Line1\nLine2\nLine3", helveticaWidthFn(t, 12), 500)
	if len(lines) != 3 {
		t.Fatalf("wrapText newlines = %v, want 3 lines", lines)
	}
	if lines[0] != "Line1" || lines[1] != "Line2" || lines[2] != "Line3" {
		t.Errorf("wrapText newlines = %v", lines)
	}
}

func TestWrapTextEmpty(t *testing.T) {
	lines := wrapText("", helveticaWidthFn(t, 12), 500)
	if len(lines) != 0 {
		t.Errorf("wrapText empty = %v, want []", lines)
	}
}

func TestWrapTextRuneSafe(t *testing.T) {
	// Even with standard 14, long Cyrillic words should not be cut mid-rune.
	// Each ? (WinAnsi fallback) is 278 units wide at 12pt = ~3.3pt.
	// Force a break by using narrow rect relative to string length.
	lines := wrapText("АБВГДЕЖЗИЙ", helveticaWidthFn(t, 12), 10)
	for _, line := range lines {
		if !utf8.ValidString(line) {
			t.Errorf("wrapText produced invalid UTF-8 line: %q", line)
		}
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

func TestAddTextBehindAndRotation(t *testing.T) {
	doc := NewDocument(595, 842)
	page, _ := doc.Page(1)

	// Add foreground text first.
	err := page.AddText("Foreground", TextStyle{}, Rectangle{LLX: 50, LLY: 700, URX: 300, URY: 750})
	if err != nil {
		t.Fatalf("AddText foreground: %v", err)
	}

	// Add rotated text behind.
	err = page.AddText("Watermark", TextStyle{
		Rotation: 45,
		Behind:   true,
	}, Rectangle{LLX: 100, LLY: 300, URX: 500, URY: 700})
	if err != nil {
		t.Fatalf("AddText behind+rotation: %v", err)
	}

	data, _ := page.contentStreams()
	content := string(data)

	// Watermark should appear before foreground.
	wmIdx := strings.Index(content, "(Watermark) Tj")
	fgIdx := strings.Index(content, "(Foreground) Tj")
	if wmIdx < 0 || fgIdx < 0 {
		t.Fatalf("missing text operators; content:\n%s", content)
	}
	if wmIdx > fgIdx {
		t.Errorf("watermark should appear before foreground; wm at %d, fg at %d", wmIdx, fgIdx)
	}

	// Watermark block should contain cm operators.
	wmBlock := content[:fgIdx]
	if !strings.Contains(wmBlock, "cm") {
		t.Error("watermark block missing cm operator for rotation")
	}
}

func TestAddTextWatermarkAllPages(t *testing.T) {
	doc := NewDocument(595, 842)
	doc.AddBlankPage(595, 842)
	doc.AddBlankPage(595, 842)

	err := doc.AddTextWatermark("DRAFT", TextStyle{
		Font:     FontHelveticaBold,
		Size:     48,
		Rotation: 45,
		Behind:   true,
	})
	if err != nil {
		t.Fatalf("AddTextWatermark: %v", err)
	}

	for i := 0; i < doc.PageCount(); i++ {
		page, _ := doc.Page(i + 1)
		data, _ := page.contentStreams()
		content := string(data)
		if !strings.Contains(content, "(DRAFT) Tj") {
			t.Errorf("page %d missing watermark text", i+1)
		}
	}
}

func TestAddTextWatermarkSelectedPages(t *testing.T) {
	doc := NewDocument(595, 842)
	doc.AddBlankPage(595, 842)
	doc.AddBlankPage(595, 842)

	err := doc.AddTextWatermark("SECRET", TextStyle{
		Font: FontHelvetica,
		Size: 36,
	}, 1, 3)
	if err != nil {
		t.Fatalf("AddTextWatermark: %v", err)
	}

	for _, n := range []int{1, 3} {
		page, _ := doc.Page(n)
		data, _ := page.contentStreams()
		if !strings.Contains(string(data), "(SECRET) Tj") {
			t.Errorf("page %d should have watermark", n)
		}
	}

	page2, _ := doc.Page(2)
	data2, _ := page2.contentStreams()
	if strings.Contains(string(data2), "(SECRET) Tj") {
		t.Error("page 2 should not have watermark")
	}
}

func TestAddTextWatermarkInvalidPage(t *testing.T) {
	doc := NewDocument(595, 842)

	err := doc.AddTextWatermark("TEST", TextStyle{}, 0)
	if err == nil {
		t.Error("expected error for page 0")
	}

	err = doc.AddTextWatermark("TEST", TextStyle{}, 2)
	if err == nil {
		t.Error("expected error for page > PageCount")
	}
}

func TestAddTextWatermarkEmpty(t *testing.T) {
	doc := NewDocument(595, 842)
	err := doc.AddTextWatermark("", TextStyle{})
	if err != nil {
		t.Fatalf("expected nil for empty text, got: %v", err)
	}
}

func TestWinAnsiEncodeRune(t *testing.T) {
	cases := []struct {
		r    rune
		code byte
		ok   bool
	}{
		{'A', 'A', true},
		{' ', ' ', true},
		{'€', 0x80, true},    // euro at WinAnsi 0x80
		{'©', 0xA9, true},    // copyright at 0xA9
		{'ÿ', 0xFF, true},    // y-diaeresis at 0xFF
		{'日', 0, false},      // CJK — not in WinAnsi
		{'\uFFFD', 0, false}, // replacement — explicitly not mapped
	}
	for _, tc := range cases {
		code, ok := winAnsiEncodeRune(tc.r)
		if code != tc.code || ok != tc.ok {
			t.Errorf("winAnsiEncodeRune(%q) = (0x%02X, %v), want (0x%02X, %v)",
				tc.r, code, ok, tc.code, tc.ok)
		}
	}
}

func TestAddTextUnicode(t *testing.T) {
	doc := NewDocument(595, 842)
	font, err := doc.LoadFont("testdata/DejaVuSans.ttf")
	if err != nil {
		t.Fatal(err)
	}
	page, _ := doc.Page(1)
	err = page.AddText("Привет", TextStyle{Font: font, Size: 12},
		Rectangle{LLX: 50, LLY: 750, URX: 545, URY: 800})
	if err != nil {
		t.Fatalf("AddText: %v", err)
	}
	data, _ := page.contentStreams()
	content := string(data)
	// Embedded fonts emit hex strings with 2-byte glyphIDs.
	if !strings.Contains(content, "<") || !strings.Contains(content, "> Tj") {
		t.Errorf("content missing hex-string Tj: %q", content)
	}
	// Confirm at least one non-zero glyphID is written (Tj operand).
	ef := font.(*embeddedFont)
	gid := ef.ttf.glyphID('П')
	if gid == 0 {
		t.Fatal("glyphID('П') = 0 unexpectedly")
	}
	want := fmt.Sprintf("%04X", gid)
	if !strings.Contains(content, want) {
		t.Errorf("content missing expected glyphID %s (for 'П'):\n%s", want, content)
	}
}

func TestAddTextNotdef(t *testing.T) {
	doc := NewDocument(595, 842)
	font, err := doc.LoadFont("testdata/DejaVuSans.ttf")
	if err != nil {
		t.Fatal(err)
	}
	page, _ := doc.Page(1)
	// '日' is NOT in DejaVuSans; expect glyphID 0000.
	err = page.AddText("日", TextStyle{Font: font, Size: 12},
		Rectangle{LLX: 50, LLY: 750, URX: 545, URY: 800})
	if err != nil {
		t.Fatalf("AddText: %v", err)
	}
	data, _ := page.contentStreams()
	content := string(data)
	if !strings.Contains(content, "<0000>") {
		t.Errorf("expected <0000> (.notdef) in content, got: %q", content)
	}
}

func TestAddTextNilFontDefaultsToHelvetica(t *testing.T) {
	doc := NewDocument(595, 842)
	page, _ := doc.Page(1)
	err := page.AddText("Hello", TextStyle{Size: 12},
		Rectangle{LLX: 50, LLY: 750, URX: 545, URY: 800})
	if err != nil {
		t.Fatalf("AddText: %v", err)
	}
	data, _ := page.contentStreams()
	content := string(data)
	if !strings.Contains(content, "(Hello) Tj") {
		t.Errorf("nil Font should default to Helvetica with literal string: %q", content)
	}
}

func TestAddTextRejectsCrossDocumentFont(t *testing.T) {
	docA := NewDocument(595, 842)
	font, err := docA.LoadFont("testdata/DejaVuSans.ttf")
	if err != nil {
		t.Fatal(err)
	}
	docB := NewDocument(595, 842)
	page, _ := docB.Page(1)
	err = page.AddText("Привет", TextStyle{Font: font, Size: 12},
		Rectangle{LLX: 50, LLY: 750, URX: 545, URY: 800})
	if err == nil {
		t.Fatal("expected error when using font from another document")
	}
	if !strings.Contains(err.Error(), "different document") {
		t.Errorf("error = %q, want to mention 'different document'", err.Error())
	}
}

// TestAddTextRotationUsesPivotRelativeCoords verifies that when rotation is
// applied, the content operators inside the cm-wrapped block use coordinates
// relative to the pivot point (rect.LLX, rect.LLY), not absolute page coords.
//
// Before the fix (ae1c7f0 regression), renderTextInBuilder was called with
// the original rect, so the clipping path emitted "100 500 100 100 re W n"
// inside a cm block that already translated the origin to (100, 500),
// producing a double offset. After the fix the clipping rect must be "0 0".
func TestAddTextRotationUsesPivotRelativeCoords(t *testing.T) {
	doc := NewDocument(595, 842)
	page, _ := doc.Page(1)
	err := page.AddText("X", TextStyle{
		Font:     FontHelvetica,
		Size:     12,
		Rotation: 45,
	}, Rectangle{LLX: 100, LLY: 500, URX: 200, URY: 600})
	if err != nil {
		t.Fatalf("AddText: %v", err)
	}
	// Read the uncompressed in-memory content stream, which contains the raw
	// PDF operators we emitted (WriteTo compresses them, so we bypass that).
	data, err := page.contentStreams()
	if err != nil {
		t.Fatalf("contentStreams: %v", err)
	}
	content := string(data)
	// The rotated content must use pivot-relative coordinates (origin 0,0).
	// The clipping rectangle inside the rotation block must start at 0 0,
	// not at the absolute page coordinates 100 500.
	if strings.Contains(content, "100 500 100 100 re") {
		t.Errorf("rotated content uses absolute coords; expected pivot-relative "+
			"(found '100 500 100 100 re' inside cm-wrapped block)\ncontent:\n%s", content)
	}
	if !strings.Contains(content, "0 0 100 100 re") {
		t.Errorf("expected '0 0 100 100 re' (pivot-relative clip) inside rotation block;\ncontent:\n%s", content)
	}
}
