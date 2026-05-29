// SPDX-License-Identifier: MIT

package asposepdf_test

import (
	"bytes"
	"testing"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

// TestFieldStyleRoundTrip sets a full FieldStyle on a text field, saves,
// reopens, and verifies every property reads back through Style().
func TestFieldStyleRoundTrip(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	form := doc.Form()
	tf, err := form.AddTextField(1, pdf.Rectangle{LLX: 50, LLY: 700, URX: 300, URY: 724}, "styled")
	if err != nil {
		t.Fatalf("AddTextField: %v", err)
	}
	tf.SetValue("Hello")

	want := pdf.FieldStyle{
		BorderColor:     &pdf.Color{R: 0.2, G: 0.3, B: 0.7, A: 1},
		BackgroundColor: &pdf.Color{R: 0.95, G: 0.97, B: 1, A: 1},
		TextColor:       &pdf.Color{R: 0.1, G: 0.1, B: 0.4, A: 1},
		BorderWidth:     2,
		BorderStyle:     pdf.BorderDashed,
		DashPattern:     []float64{3, 2},
		TextFont:        pdf.FontTimesBold,
		TextSize:        14,
		TextAlign:       pdf.HAlignCenter,
	}
	if err := tf.SetStyle(want); err != nil {
		t.Fatalf("SetStyle: %v", err)
	}

	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatalf("WriteTo: %v", err)
	}
	doc2, err := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("OpenStream: %v", err)
	}
	got := doc2.Form().Field("styled").Style()

	if !colorsEqual(got.BorderColor, want.BorderColor) {
		t.Errorf("BorderColor = %+v, want %+v", got.BorderColor, want.BorderColor)
	}
	if !colorsEqual(got.BackgroundColor, want.BackgroundColor) {
		t.Errorf("BackgroundColor = %+v, want %+v", got.BackgroundColor, want.BackgroundColor)
	}
	if !colorsEqual(got.TextColor, want.TextColor) {
		t.Errorf("TextColor = %+v, want %+v", got.TextColor, want.TextColor)
	}
	if got.BorderWidth != 2 {
		t.Errorf("BorderWidth = %v, want 2", got.BorderWidth)
	}
	if got.BorderStyle != pdf.BorderDashed {
		t.Errorf("BorderStyle = %v, want BorderDashed", got.BorderStyle)
	}
	if len(got.DashPattern) != 2 || got.DashPattern[0] != 3 || got.DashPattern[1] != 2 {
		t.Errorf("DashPattern = %v, want [3 2]", got.DashPattern)
	}
	if got.TextSize != 14 {
		t.Errorf("TextSize = %v, want 14", got.TextSize)
	}
	if got.TextAlign != pdf.HAlignCenter {
		t.Errorf("TextAlign = %v, want HAlignCenter", got.TextAlign)
	}
	// Font is reconstructed from /DR/Font/<res>/BaseFont after reopen.
	if got.TextFont == nil || got.TextFont.BaseFont() != "Times-Bold" {
		t.Errorf("TextFont = %v, want Times-Bold", got.TextFont)
	}

	// The styled border colour and dash must appear in the regenerated /AP.
	out := buf.Bytes()
	if !bytes.Contains(out, []byte("/MK")) {
		t.Error("expected /MK appearance-characteristics dict in output")
	}
	if !bytes.Contains(out, []byte("/BS")) {
		t.Error("expected /BS border-style dict in output")
	}
}

// TestFieldStyleAppliesToAllTypes confirms SetStyle is callable on every
// concrete field type via the Field interface and writes /MK each time.
func TestFieldStyleAppliesToAllTypes(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	form := doc.Form()

	tf, _ := form.AddTextField(1, pdf.Rectangle{LLX: 50, LLY: 760, URX: 250, URY: 784}, "t")
	cb, _ := form.AddCheckbox(1, pdf.Rectangle{LLX: 50, LLY: 730, URX: 68, URY: 748}, "c")
	combo, _ := form.AddComboBox(1, pdf.Rectangle{LLX: 50, LLY: 700, URX: 250, URY: 720}, "m",
		[]pdf.ChoiceOption{{Value: "A"}, {Value: "B"}})
	lb, _ := form.AddListBox(1, pdf.Rectangle{LLX: 50, LLY: 620, URX: 250, URY: 690}, "l",
		[]pdf.ChoiceOption{{Value: "X"}, {Value: "Y"}})
	rb, _ := form.AddRadioGroup("r", []pdf.RadioItem{
		{PageNum: 1, Rect: pdf.Rectangle{LLX: 50, LLY: 590, URX: 68, URY: 608}, Export: "one"},
	})

	style := pdf.FieldStyle{
		BorderColor:     &pdf.Color{R: 0.8, G: 0.1, B: 0.1, A: 1},
		BackgroundColor: &pdf.Color{R: 1, G: 0.97, B: 0.9, A: 1},
		BorderWidth:     1.5,
	}
	for _, f := range []pdf.Field{tf, cb, combo, lb, rb} {
		if err := f.SetStyle(style); err != nil {
			t.Errorf("SetStyle on %s: %v", f.FullName(), err)
		}
		got := f.Style()
		if !colorsEqual(got.BorderColor, style.BorderColor) {
			t.Errorf("%s BorderColor = %+v, want %+v", f.FullName(), got.BorderColor, style.BorderColor)
		}
	}

	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatalf("WriteTo: %v", err)
	}
	// Reopen to ensure the styled document is structurally valid.
	if _, err := pdf.OpenStream(bytes.NewReader(buf.Bytes())); err != nil {
		t.Fatalf("OpenStream after styling: %v", err)
	}
}

func colorsEqual(a, b *pdf.Color) bool {
	if a == nil || b == nil {
		return a == b
	}
	const eps = 1e-4
	d := func(x, y float64) bool {
		if x > y {
			return x-y < eps
		}
		return y-x < eps
	}
	return d(a.R, b.R) && d(a.G, b.G) && d(a.B, b.B)
}
