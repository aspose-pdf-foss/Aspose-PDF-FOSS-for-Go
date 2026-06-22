// SPDX-License-Identifier: MIT

package asposepdf_test

import (
	"bytes"
	"testing"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

// TestTilingPatternFill fills shapes with a user-drawn tiling pattern cell and
// checks one shared /Pattern object renders the repeated motif and round-trips.
func TestTilingPatternFill(t *testing.T) {
	doc := pdf.NewDocument(360, 260)

	tp := doc.CreateTilingPattern(16, 16)
	c := tp.Canvas()
	if err := c.DrawLine(pdf.Point{X: 0, Y: 0}, pdf.Point{X: 16, Y: 16},
		pdf.LineStyle{Color: &pdf.Color{R: 0.2, G: 0.4, B: 0.8, A: 1}, Width: 1.5}); err != nil {
		t.Fatalf("cell DrawLine: %v", err)
	}
	if err := c.DrawCircle(pdf.Point{X: 8, Y: 8}, 2,
		pdf.ShapeStyle{FillColor: &pdf.Color{R: 0.9, G: 0.3, B: 0.3, A: 1}}); err != nil {
		t.Fatalf("cell DrawCircle: %v", err)
	}

	p, _ := doc.Page(1)
	// Fill a rectangle and a circle with the same pattern.
	if err := p.DrawRectangle(pdf.Rectangle{LLX: 30, LLY: 140, URX: 180, URY: 230},
		pdf.ShapeStyle{LineStyle: pdf.LineStyle{Color: &pdf.Color{A: 1}, Width: 2}, FillTiling: tp}); err != nil {
		t.Fatalf("DrawRectangle: %v", err)
	}
	if err := p.DrawCircle(pdf.Point{X: 270, Y: 130}, 60,
		pdf.ShapeStyle{FillTiling: tp}); err != nil {
		t.Fatalf("DrawCircle: %v", err)
	}

	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatalf("WriteTo: %v", err)
	}
	out, err := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	img, err := out.RenderImage(1, pdf.RenderOptions{DPI: 120})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if px := nonWhitePixels(img); px < 2000 {
		t.Errorf("tiling fill rendered too little (%d px)", px)
	}
}

// TestTilingPatternStepAndErrors covers SetStep and the cross-document guard.
func TestTilingPatternStepAndErrors(t *testing.T) {
	doc := pdf.NewDocument(200, 200)
	tp := doc.CreateTilingPattern(10, 10)
	tp.SetStep(20, 20) // leave gaps between tiles
	tp.Canvas().DrawRectangle(pdf.Rectangle{LLX: 0, LLY: 0, URX: 6, URY: 6},
		pdf.ShapeStyle{FillColor: &pdf.Color{R: 0.3, G: 0.6, B: 0.3, A: 1}})

	p, _ := doc.Page(1)
	if err := p.DrawRectangle(pdf.Rectangle{LLX: 20, LLY: 20, URX: 180, URY: 180},
		pdf.ShapeStyle{FillTiling: tp}); err != nil {
		t.Fatalf("DrawRectangle with spaced tiling: %v", err)
	}

	// A tiling pattern from another document is rejected.
	other := pdf.NewDocument(100, 100)
	foreign := other.CreateTilingPattern(10, 10)
	foreign.Canvas().DrawRectangle(pdf.Rectangle{URX: 10, URY: 10}, pdf.ShapeStyle{FillColor: &pdf.Color{A: 1}})
	if err := p.DrawRectangle(pdf.Rectangle{LLX: 0, LLY: 0, URX: 10, URY: 10},
		pdf.ShapeStyle{FillTiling: foreign}); err == nil {
		t.Error("filling with a tiling pattern from another document = nil error, want an error")
	}
}
