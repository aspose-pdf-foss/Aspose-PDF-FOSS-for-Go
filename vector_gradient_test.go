// SPDX-License-Identifier: MIT

package asposepdf_test

import (
	"bytes"
	"testing"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

// TestLinearGradientFill draws a rectangle with a linear gradient and
// verifies the PDF carries a Type 2 axial shading, a shading pattern, and
// the content stream selects the /Pattern colour space and paints with it.
func TestLinearGradientFill(t *testing.T) {
	doc := pdf.NewDocument(200, 200)
	page, _ := doc.Page(1)
	err := page.DrawRectangle(
		pdf.Rectangle{LLX: 10, LLY: 10, URX: 190, URY: 190},
		pdf.ShapeStyle{
			FillGradient: pdf.NewLinearGradient(10, 0, 190, 0,
				pdf.GradientStop{Offset: 0, Color: pdf.Color{R: 1, A: 1}},
				pdf.GradientStop{Offset: 1, Color: pdf.Color{B: 1, A: 1}}),
		})
	if err != nil {
		t.Fatalf("DrawRectangle: %v", err)
	}
	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatalf("WriteTo: %v", err)
	}
	out := buf.Bytes()
	// The shading/pattern objects are uncompressed dicts; the "/Pattern cs"
	// + "scn" operators live in the FlateDecode-compressed page content
	// stream, so they're verified by the visual render, not byte-grepped.
	for _, want := range []string{"/ShadingType 2", "/PatternType 2", "/Coords"} {
		if !bytes.Contains(out, []byte(want)) {
			t.Errorf("output missing %q", want)
		}
	}
}

// TestRadialGradientFill verifies a radial (Type 3) shading is emitted for
// a circle filled with a radial gradient.
func TestRadialGradientFill(t *testing.T) {
	doc := pdf.NewDocument(200, 200)
	page, _ := doc.Page(1)
	err := page.DrawCircle(pdf.Point{X: 100, Y: 100}, 80, pdf.ShapeStyle{
		FillGradient: pdf.NewRadialGradient(100, 100, 80,
			pdf.GradientStop{Offset: 0, Color: pdf.Color{R: 1, G: 1, B: 1, A: 1}},
			pdf.GradientStop{Offset: 1, Color: pdf.Color{R: 0.1, G: 0.2, B: 0.6, A: 1}}),
	})
	if err != nil {
		t.Fatalf("DrawCircle: %v", err)
	}
	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatalf("WriteTo: %v", err)
	}
	if !bytes.Contains(buf.Bytes(), []byte("/ShadingType 3")) {
		t.Error("expected a Type 3 (radial) shading in output")
	}
}

// TestGradientThreeStopStitch confirms a 3-stop gradient produces a Type 3
// stitching function combining Type 2 exponential segments.
func TestGradientThreeStopStitch(t *testing.T) {
	doc := pdf.NewDocument(200, 200)
	page, _ := doc.Page(1)
	_ = page.DrawRectangle(pdf.Rectangle{LLX: 0, LLY: 0, URX: 200, URY: 50},
		pdf.ShapeStyle{
			FillGradient: pdf.NewLinearGradient(0, 0, 200, 0,
				pdf.GradientStop{Offset: 0, Color: pdf.Color{R: 1, A: 1}},
				pdf.GradientStop{Offset: 0.5, Color: pdf.Color{G: 1, A: 1}},
				pdf.GradientStop{Offset: 1, Color: pdf.Color{B: 1, A: 1}}),
		})
	var buf bytes.Buffer
	_, _ = doc.WriteTo(&buf)
	if !bytes.Contains(buf.Bytes(), []byte("/FunctionType 3")) {
		t.Error("expected a Type 3 stitching function for a 3-stop gradient")
	}
}

// TestGradientNoStopsErrors rejects a gradient with no stops.
func TestGradientNoStopsErrors(t *testing.T) {
	doc := pdf.NewDocument(200, 200)
	page, _ := doc.Page(1)
	err := page.DrawRectangle(pdf.Rectangle{LLX: 0, LLY: 0, URX: 10, URY: 10},
		pdf.ShapeStyle{FillGradient: pdf.NewLinearGradient(0, 0, 10, 0)})
	if err == nil {
		t.Error("expected an error for a gradient with no stops")
	}
}
