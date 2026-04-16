package asposepdf

import "testing"

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
