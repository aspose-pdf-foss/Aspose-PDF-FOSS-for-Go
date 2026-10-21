// SPDX-License-Identifier: MIT

package asposepdf_test

import (
	"bytes"
	"math"
	"strings"
	"testing"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

// GitHub issue #19 (shadowhunter1470-svg), point 1: AddText, AddTextAt,
// MeasureText and GetFontMetrics all accepted NaN/+Inf/-Inf for a
// coordinate, size, rotation or skew with no error, writing the literal
// text "NaN"/"+Inf"/"-Inf" into the content stream — not a PDF number
// ISO 32000-1 §7.3.3 defines, and not something this library's own
// parser can read back. Rectangle.validate's `URX <= LLX` comparison is
// false for NaN (any comparison with NaN is false), so a NaN rectangle
// side slipped through too.

func TestAddTextRejectsNonFiniteRectangle(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	p, err := doc.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	cases := []pdf.Rectangle{
		{LLX: math.NaN(), URX: 100, URY: 100},
		{LLX: 0, URX: math.Inf(1), URY: 100},
		{LLX: 0, URX: 100, URY: math.Inf(-1)},
	}
	for _, rect := range cases {
		if err := p.AddText("x", pdf.TextStyle{}, rect); err == nil {
			t.Errorf("AddText with rect %+v: want error, got nil", rect)
		}
	}
}

func TestAddTextRejectsNonFiniteSizeAndRotation(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	p, err := doc.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	rect := pdf.Rectangle{LLX: 0, LLY: 0, URX: 200, URY: 200}
	if err := p.AddText("x", pdf.TextStyle{Size: math.NaN()}, rect); err == nil {
		t.Error("AddText with Size=NaN: want error, got nil")
	}
	if err := p.AddText("x", pdf.TextStyle{Size: math.Inf(1)}, rect); err == nil {
		t.Error("AddText with Size=+Inf: want error, got nil")
	}
	if err := p.AddText("x", pdf.TextStyle{Rotation: math.NaN()}, rect); err == nil {
		t.Error("AddText with Rotation=NaN: want error, got nil")
	}
	if err := p.AddText("x", pdf.TextStyle{Skew: math.NaN()}, rect); err == nil {
		t.Error("AddText with Skew=NaN: want error, got nil")
	}
}

func TestAddTextAtRejectsNonFiniteInputs(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	p, err := doc.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.AddTextAt("x", pdf.TextStyle{}, math.NaN(), 500); err == nil {
		t.Error("AddTextAt with x=NaN: want error, got nil")
	}
	if err := p.AddTextAt("x", pdf.TextStyle{}, 50, math.Inf(1)); err == nil {
		t.Error("AddTextAt with y=+Inf: want error, got nil")
	}
	if err := p.AddTextAt("x", pdf.TextStyle{Size: math.NaN()}, 50, 500); err == nil {
		t.Error("AddTextAt with Size=NaN: want error, got nil")
	}
	if err := p.AddTextAt("x", pdf.TextStyle{Size: math.Inf(1)}, 50, 500); err == nil {
		t.Error("AddTextAt with Size=+Inf: want error, got nil")
	}
	if err := p.AddTextAt("x", pdf.TextStyle{Rotation: math.NaN()}, 50, 500); err == nil {
		t.Error("AddTextAt with Rotation=NaN: want error, got nil")
	}
	if err := p.AddTextAt("x", pdf.TextStyle{Skew: math.NaN()}, 50, 500); err == nil {
		t.Error("AddTextAt with Skew=NaN: want error, got nil")
	}
}

func TestMeasureTextRejectsNonFiniteSize(t *testing.T) {
	if _, err := pdf.MeasureText("x", pdf.TextStyle{Size: math.NaN()}); err == nil {
		t.Error("MeasureText with Size=NaN: want error, got nil")
	}
	if _, err := pdf.MeasureText("x", pdf.TextStyle{Size: math.Inf(1)}); err == nil {
		t.Error("MeasureText with Size=+Inf: want error, got nil")
	}
}

func TestGetFontMetricsRejectsNonFiniteSize(t *testing.T) {
	if _, err := pdf.GetFontMetrics(pdf.FontHelvetica, math.NaN()); err == nil {
		t.Error("GetFontMetrics with size=NaN: want error, got nil")
	}
	if _, err := pdf.GetFontMetrics(pdf.FontHelvetica, math.Inf(1)); err == nil {
		t.Error("GetFontMetrics with size=+Inf: want error, got nil")
	}
}

// TestAddTextAtRejectsSizeTooSmallToRepresent reproduces the reporter's
// Size=1e-9 case: formatFloat rounds to 6 decimal places, so a size that
// small writes the literal text "0" into the Tf operator — a real font
// size of zero, not the "unspecified, use 12" default the library
// applies for a literal 0 input — making the text invisible instead of
// merely small.
func TestAddTextAtRejectsSizeTooSmallToRepresent(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	p, err := doc.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.AddTextAt("x", pdf.TextStyle{Size: 1e-9}, 50, 500); err == nil {
		t.Error("AddTextAt with Size=1e-9: want error, got nil")
	}
	// A literal 0 is still the documented "use 12pt default" sentinel,
	// not an error -- the new check must not catch this case too.
	if err := p.AddTextAt("x", pdf.TextStyle{Size: 0}, 50, 500); err != nil {
		t.Errorf("AddTextAt with Size=0 (default sentinel): want nil, got %v", err)
	}
}

// TestGetFontMetricsNilFontDefaultsToHelvetica mirrors the default nil
// Font -> FontHelvetica behavior AddText/AddTextAt/MeasureText already
// document and apply, closing issue #19 point 3 (GetFontMetrics used to
// return "font: unsupported type <nil>" instead).
func TestGetFontMetricsNilFontDefaultsToHelvetica(t *testing.T) {
	got, err := pdf.GetFontMetrics(nil, 10)
	if err != nil {
		t.Fatalf("GetFontMetrics(nil, 10): %v", err)
	}
	want, err := pdf.GetFontMetrics(pdf.FontHelvetica, 10)
	if err != nil {
		t.Fatalf("GetFontMetrics(FontHelvetica, 10): %v", err)
	}
	if got != want {
		t.Errorf("GetFontMetrics(nil, 10) = %+v, want %+v (same as FontHelvetica)", got, want)
	}
}

// TestNonFiniteNeverReachesContentStream is the forensic counterpart:
// confirms that when a non-finite input IS (incorrectly) allowed through
// — simulated here by checking the saved file never contains "NaN" or
// "Inf" literally — nothing of the sort survives into the output for
// any of the accepted-and-should-error cases above, as an extra
// guardrail should one of the checks above regress to a no-op.
func TestNonFiniteNeverReachesContentStream(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	p, err := doc.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	_ = p.AddTextAt("x", pdf.TextStyle{}, math.NaN(), 500)
	_ = p.AddTextAt("x", pdf.TextStyle{Rotation: math.Inf(1)}, 50, 500)
	_ = p.AddText("x", pdf.TextStyle{Size: math.NaN()}, pdf.Rectangle{LLX: 0, LLY: 0, URX: 100, URY: 100})

	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if strings.Contains(out, "NaN") || strings.Contains(out, "Inf") {
		t.Error("saved document contains a non-finite literal (\"NaN\"/\"Inf\") in its bytes")
	}
}
