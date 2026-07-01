// SPDX-License-Identifier: MIT

package asposepdf_test

import (
	"testing"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

// Hebrew "שלום עולם" (shalom olam) — needs only reordering, no shaping.
const hebrewHello = "שלום עולם"

// TestRTLHebrewRendersRightAligned: Hebrew drawn with the default (left) HAlign
// renders non-blank and is right-aligned (its glyphs sit in the right half of
// the page), because the auto-detected RTL base flips the default alignment.
func TestRTLHebrewRendersRightAligned(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	deja, err := doc.LoadFont("testdata/DejaVuSans.ttf")
	if err != nil {
		t.Fatal(err)
	}
	p, _ := doc.Page(1)
	rect := pdf.Rectangle{LLX: 50, LLY: 700, URX: 545, URY: 760}
	if err := p.AddText(hebrewHello, pdf.TextStyle{Font: deja, Size: 28}, rect); err != nil {
		t.Fatal(err)
	}
	img, err := doc.RenderImage(1, pdf.RenderOptions{DPI: 96})
	if err != nil {
		t.Fatal(err)
	}
	if nonWhitePixels(img) == 0 {
		t.Fatal("Hebrew text rendered blank")
	}
	left, right := nonWhiteHalves(img)
	if right == 0 {
		t.Fatal("no text in the right half — expected right-aligned RTL")
	}
	if left > right {
		t.Errorf("expected right-aligned RTL text: left=%d right=%d", left, right)
	}
}

// TestRTLExplicitFlagRightAlignsLatin: TextStyle.RTL makes even Latin text use a
// right-to-left base, so it right-aligns by default.
func TestRTLExplicitFlagRightAlignsLatin(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	p, _ := doc.Page(1)
	rect := pdf.Rectangle{LLX: 50, LLY: 700, URX: 545, URY: 740}
	if err := p.AddText("Word", pdf.TextStyle{Font: pdf.FontHelvetica, Size: 24, RTL: true}, rect); err != nil {
		t.Fatal(err)
	}
	img, err := doc.RenderImage(1, pdf.RenderOptions{DPI: 96})
	if err != nil {
		t.Fatal(err)
	}
	left, right := nonWhiteHalves(img)
	if right == 0 || left > right {
		t.Errorf("RTL base should right-align: left=%d right=%d", left, right)
	}
}

// TestRTLArabicShapedRightAligned: Arabic renders (shaped to connected
// Presentation Forms-B) and right-aligns; the shaped string differs from the
// input (contextual forms were substituted).
func TestRTLArabicShapedRightAligned(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	deja, err := doc.LoadFont("testdata/DejaVuSans.ttf")
	if err != nil {
		t.Fatal(err)
	}
	p, _ := doc.Page(1)
	arabic := "مرحبا بالعالم" // marhaban bil-'alam
	rect := pdf.Rectangle{LLX: 50, LLY: 700, URX: 545, URY: 750}
	if err := p.AddText(arabic, pdf.TextStyle{Font: deja, Size: 30}, rect); err != nil {
		t.Fatal(err)
	}
	img, err := doc.RenderImage(1, pdf.RenderOptions{DPI: 96})
	if err != nil {
		t.Fatal(err)
	}
	if nonWhitePixels(img) == 0 {
		t.Fatal("Arabic rendered blank — is the font missing Presentation Forms-B?")
	}
	left, right := nonWhiteHalves(img)
	if right == 0 || left > right {
		t.Errorf("Arabic should be right-aligned: left=%d right=%d", left, right)
	}
}

// TestRTLDoesNotDisturbLTR: plain LTR text is untouched (left-aligned, renders).
func TestRTLDoesNotDisturbLTR(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	p, _ := doc.Page(1)
	rect := pdf.Rectangle{LLX: 50, LLY: 700, URX: 545, URY: 740}
	if err := p.AddText("Hello world", pdf.TextStyle{Font: pdf.FontHelvetica, Size: 24}, rect); err != nil {
		t.Fatal(err)
	}
	img, err := doc.RenderImage(1, pdf.RenderOptions{DPI: 96})
	if err != nil {
		t.Fatal(err)
	}
	left, right := nonWhiteHalves(img)
	if left == 0 {
		t.Fatal("LTR text rendered blank or not left-aligned")
	}
	if right > left {
		t.Errorf("LTR text should be left-aligned: left=%d right=%d", left, right)
	}
}
