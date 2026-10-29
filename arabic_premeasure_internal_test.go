// SPDX-License-Identifier: MIT

package asposepdf

import "testing"

// pdf-go-qxsz: AddText shapes Arabic into Presentation Forms-B (shapeArabic)
// before wrapping when the font's own GSUB does not shape Arabic, but the
// callers that measure text in advance — table row heights, Flow
// paragraphs, text beside floats, styled runs (lineMeasurerForFont), TOC
// entries and Markdown labels (shapedOrPlainWidth) — measured the unshaped
// letters, so their line breaks could disagree with what AddText draws.
// The width function below gives presentation forms their own width, as a
// real font does, so the two measurements differ visibly.
func arabicTestWidth(r rune) float64 {
	if r >= 0xFE70 && r <= 0xFEFF { // Arabic Presentation Forms-B
		return 7
	}
	return 5
}

func TestLineMeasurerAppliesArabicShaping(t *testing.T) {
	const text = "السلام عليكم ورحمة الله"
	want := measureString(shapeArabic(text), arabicTestWidth)
	m := lineMeasurerForFont(FontHelvetica, 10, text, false, arabicTestWidth)
	if got := m.exact(text); got != want {
		t.Errorf("exact = %v, want %v (the width AddText draws, after shapeArabic)", got, want)
	}
	for _, w := range []string{"السلام", "عليكم", "الله"} {
		if got, want := m.word(w), measureString(shapeArabic(w), arabicTestWidth); got != want {
			t.Errorf("word(%q) = %v, want %v", w, got, want)
		}
	}
	if got := shapedOrPlainWidth(FontHelvetica, 10, text, arabicTestWidth); got != want {
		t.Errorf("shapedOrPlainWidth = %v, want %v", got, want)
	}
	// Latin text is untouched.
	if got := lineMeasurerForFont(FontHelvetica, 10, "plain words", false, arabicTestWidth).exact("plain words"); got != 55 {
		t.Errorf("Latin exact = %v, want 55", got)
	}
}

// A font that shapes Arabic itself (DejaVu Sans' GSUB) must NOT get the
// Presentation-Forms pass — AddText skips it there, so the measurer must too.
func TestLineMeasurerSkipsArabicPassForShapingFont(t *testing.T) {
	doc := NewDocument(100, 100)
	fnt, err := doc.LoadFont("testdata/DejaVuSans.ttf")
	if err != nil {
		t.Skipf("font not available: %v", err)
	}
	ef := fnt.(*embeddedFont)
	if !fontShapesArabic(ef.ttf) {
		t.Skip("test font does not shape Arabic")
	}
	const text = "السلام عليكم"
	m := lineMeasurerForFont(fnt, 12, text, false, nil)
	needBidi := bidiHasStrongRTL(text)
	want := shapeLine(ef.ttf, text, bidiBaseLevel(text, false), needBidi).width(12)
	if got := m.exact(text); got != want {
		t.Errorf("exact = %v, want %v (OpenType-shaped original text)", got, want)
	}
}
