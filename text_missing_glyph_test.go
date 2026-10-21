// SPDX-License-Identifier: MIT

package asposepdf_test

import (
	"bytes"
	"os"
	"strings"
	"testing"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

// GitHub issue #19 (shadowhunter1470-svg), point 2: a character the
// font has no glyph for used to draw .notdef silently, with no way for
// a caller to find out. TextStyle.MissingGlyph is opt-in (zero value =
// MissingGlyphAllow, today's existing behavior) so no existing caller
// is affected; MissingGlyphError/MissingGlyphReplace let a caller who
// wants to know opt in.

func loadDejaVu(t *testing.T, doc *pdf.Document) pdf.Font {
	t.Helper()
	data, err := os.ReadFile("testdata/DejaVuSans.ttf")
	if err != nil {
		t.Skipf("font not available: %v", err)
	}
	f, err := doc.LoadFontFromStream(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func TestAddTextMissingGlyphAllowIsDefaultAndUnchanged(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	f := loadDejaVu(t, doc)
	p, err := doc.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	// '中' is not in DejaVu Sans; the zero-value policy must still draw
	// .notdef (not error, not replace) exactly as before this option
	// existed.
	rect := pdf.Rectangle{LLX: 50, LLY: 700, URX: 500, URY: 800}
	if err := p.AddText("中", pdf.TextStyle{Font: f, Size: 12}, rect); err != nil {
		t.Errorf("AddText with default MissingGlyph policy: want nil, got %v", err)
	}
}

func TestAddTextMissingGlyphErrorNamesTheRune(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	f := loadDejaVu(t, doc)
	p, err := doc.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	rect := pdf.Rectangle{LLX: 50, LLY: 700, URX: 500, URY: 800}
	err = p.AddText("a中b", pdf.TextStyle{Font: f, Size: 12, MissingGlyph: pdf.MissingGlyphError}, rect)
	if err == nil {
		t.Fatal("AddText with MissingGlyphError over an unsupported CJK character: want error, got nil")
	}
	if !strings.Contains(err.Error(), "中") {
		t.Errorf("error %q does not name the offending rune", err.Error())
	}
}

func TestAddTextAtMissingGlyphErrorOnControlCharacter(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	f := loadDejaVu(t, doc)
	p, err := doc.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	// AddTextAt keeps whitespace/control characters verbatim (unlike
	// AddText, which normalizes them away) -- a NUL byte has no glyph
	// in any normal font and must be flagged like any other character.
	if err := p.AddTextAt("a\x00b", pdf.TextStyle{Font: f, Size: 12, MissingGlyph: pdf.MissingGlyphError}, 50, 700); err == nil {
		t.Error("AddTextAt with MissingGlyphError over a NUL byte: want error, got nil")
	}
}

func TestMeasureTextMissingGlyphErrorMatchesAddTextAt(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	f := loadDejaVu(t, doc)
	p, err := doc.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	style := pdf.TextStyle{Font: f, Size: 12, MissingGlyph: pdf.MissingGlyphError}
	_, measureErr := pdf.MeasureText("a中b", style)
	drawErr := p.AddTextAt("a中b", style, 50, 700)
	if measureErr == nil || drawErr == nil {
		t.Errorf("MeasureText/AddTextAt with MissingGlyphError over an unsupported character: want both to error, got measure=%v draw=%v", measureErr, drawErr)
	}
}

func TestAddTextMissingGlyphReplaceSubstitutesConfiguredRune(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	f := loadDejaVu(t, doc)
	p, err := doc.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	rect := pdf.Rectangle{LLX: 50, LLY: 700, URX: 500, URY: 800}
	// '?' (U+003F) is covered by any Latin font, including DejaVu Sans.
	style := pdf.TextStyle{Font: f, Size: 12, MissingGlyph: pdf.MissingGlyphReplace, ReplacementRune: '?'}
	if err := p.AddText("a中b", style, rect); err != nil {
		t.Fatalf("AddText with MissingGlyphReplace: %v", err)
	}
	texts, err := doc.ExtractText()
	if err != nil {
		t.Fatal(err)
	}
	if len(texts) == 0 || !strings.Contains(texts[0], "a?b") {
		t.Errorf("extracted text = %q, want it to contain \"a?b\" (the replacement)", texts)
	}
}

func TestAddTextMissingGlyphReplaceDefaultsToReplacementCharacter(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	f := loadDejaVu(t, doc)
	p, err := doc.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	rect := pdf.Rectangle{LLX: 50, LLY: 700, URX: 500, URY: 800}
	// ReplacementRune left at its zero value -> U+FFFD, the same
	// fallback this library's own text extraction already uses.
	style := pdf.TextStyle{Font: f, Size: 12, MissingGlyph: pdf.MissingGlyphReplace}
	if err := p.AddText("a中b", style, rect); err != nil {
		t.Fatalf("AddText with MissingGlyphReplace (default rune): %v", err)
	}
	texts, err := doc.ExtractText()
	if err != nil {
		t.Fatal(err)
	}
	if len(texts) == 0 || !strings.Contains(texts[0], "a�b") {
		t.Errorf("extracted text = %q, want it to contain the U+FFFD replacement", texts)
	}
}

// TestAddTextMissingGlyphErrorIgnoresDefaultIgnorables confirms a
// default-ignorable code point is never flagged as "missing" even
// under MissingGlyphError -- drawing nothing for one of those is
// correct, not a gap, matching the existing extraction-side
// hideDefaultIgnorables convention. Deliberately uses U+061C (the
// Arabic Letter Mark, a Default_Ignorable_Code_Point) rather than a
// zero-width space: DejaVu Sans actually HAS a glyph for U+200B, which
// would let this test pass even with the ignorable-skip logic
// completely disabled -- confirmed directly by checking fontHasGlyph
// for several candidates during development, and by temporarily
// disabling the skip and seeing a zero-width-space version of this test
// still pass (it only fails on a character the font genuinely lacks).
func TestAddTextMissingGlyphErrorIgnoresDefaultIgnorables(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	f := loadDejaVu(t, doc)
	p, err := doc.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	rect := pdf.Rectangle{LLX: 50, LLY: 700, URX: 500, URY: 800}
	style := pdf.TextStyle{Font: f, Size: 12, MissingGlyph: pdf.MissingGlyphError}
	if err := p.AddText("a\u061cb", style, rect); err != nil {
		t.Errorf("AddText with MissingGlyphError over a default-ignorable character: want nil, got %v", err)
	}
}
