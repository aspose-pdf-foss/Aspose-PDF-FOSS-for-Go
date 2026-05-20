package asposepdf_test

import (
	"strings"
	"testing"

	pdf "github.com/aspose/pdf-for-go"
)

func TestVector_LineCapConstants(t *testing.T) {
	if pdf.LineCapButt != 0 {
		t.Errorf("LineCapButt = %d, want 0", pdf.LineCapButt)
	}
	if pdf.LineCapRound != 1 {
		t.Errorf("LineCapRound = %d, want 1", pdf.LineCapRound)
	}
	if pdf.LineCapSquare != 2 {
		t.Errorf("LineCapSquare = %d, want 2", pdf.LineCapSquare)
	}
}

func TestVector_LineJoinConstants(t *testing.T) {
	if pdf.LineJoinMiter != 0 || pdf.LineJoinRound != 1 || pdf.LineJoinBevel != 2 {
		t.Errorf("LineJoin enum mismatch: Miter=%d Round=%d Bevel=%d",
			pdf.LineJoinMiter, pdf.LineJoinRound, pdf.LineJoinBevel)
	}
}

func TestVector_LineStyleZeroValue(t *testing.T) {
	var s pdf.LineStyle
	if s.Color != nil || s.Width != 0 || s.DashPattern != nil ||
		s.Cap != pdf.LineCapButt || s.Join != pdf.LineJoinMiter {
		t.Errorf("LineStyle zero value mismatch: %+v", s)
	}
}

func TestVector_ShapeStyleEmbedsLineStyle(t *testing.T) {
	s := pdf.ShapeStyle{
		LineStyle: pdf.LineStyle{Width: 2},
		FillColor: &pdf.Color{R: 1, G: 0, B: 0, A: 1},
	}
	if s.Width != 2 {
		t.Errorf("embedded LineStyle.Width = %g, want 2", s.Width)
	}
	if s.FillColor == nil || s.FillColor.R != 1 {
		t.Error("FillColor not preserved")
	}
}

func TestDrawLine_BasicSolidStroke(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	err := page.DrawLine(
		pdf.Point{X: 100, Y: 100},
		pdf.Point{X: 200, Y: 150},
		pdf.LineStyle{Color: &pdf.Color{R: 1, G: 0, B: 0, A: 1}, Width: 2},
	)
	if err != nil {
		t.Fatal(err)
	}
	s := renderedContent(t, doc)
	for _, want := range []string{"100 100 m", "200 150 l", "S", "1 0 0 RG", "2 w"} {
		if !strings.Contains(s, want) {
			t.Errorf("output missing %q", want)
		}
	}
}

func TestDrawLine_DashPattern(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	err := page.DrawLine(
		pdf.Point{X: 0, Y: 0}, pdf.Point{X: 100, Y: 0},
		pdf.LineStyle{Width: 1, DashPattern: []float64{4, 2}},
	)
	if err != nil {
		t.Fatal(err)
	}
	s := renderedContent(t, doc)
	if !strings.Contains(s, "[4 2] 0 d") {
		t.Errorf("output missing dash pattern: %s", s)
	}
}

func TestDrawLine_WidthZero_NoStroke(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	err := page.DrawLine(pdf.Point{}, pdf.Point{X: 100}, pdf.LineStyle{Width: 0})
	if err != nil {
		t.Fatal(err)
	}
	s := renderedContent(t, doc)
	if strings.Contains(s, " S\n") {
		t.Error("width=0 should not emit stroke op")
	}
}

func TestDrawLine_LineCapRound(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	_ = page.DrawLine(pdf.Point{}, pdf.Point{X: 50}, pdf.LineStyle{
		Width: 4, Cap: pdf.LineCapRound,
	})
	s := renderedContent(t, doc)
	if !strings.Contains(s, "1 J") {
		t.Error("LineCapRound should emit `1 J`")
	}
}

func TestDrawRectangle_StrokeOnly(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	err := page.DrawRectangle(
		pdf.Rectangle{LLX: 50, LLY: 50, URX: 150, URY: 100},
		pdf.ShapeStyle{LineStyle: pdf.LineStyle{Width: 1, Color: &pdf.Color{R: 0, G: 0, B: 1, A: 1}}},
	)
	if err != nil {
		t.Fatal(err)
	}
	s := renderedContent(t, doc)
	if !strings.Contains(s, "50 50 100 50 re") {
		t.Errorf("missing rect op: %s", s)
	}
	if !strings.Contains(s, " S\n") {
		t.Error("stroke-only should emit S")
	}
	if strings.Contains(s, " f\n") || strings.Contains(s, " B\n") {
		t.Error("stroke-only should not emit f or B")
	}
}

func TestDrawRectangle_FillOnly(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	err := page.DrawRectangle(
		pdf.Rectangle{LLX: 0, LLY: 0, URX: 100, URY: 100},
		pdf.ShapeStyle{FillColor: &pdf.Color{R: 1, G: 1, B: 0, A: 1}},
	)
	if err != nil {
		t.Fatal(err)
	}
	s := renderedContent(t, doc)
	if !strings.Contains(s, "1 1 0 rg") {
		t.Errorf("missing fill color: %s", s)
	}
	if !strings.Contains(s, " f\n") {
		t.Error("fill-only should emit f")
	}
}

func TestDrawRectangle_StrokeAndFill(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	err := page.DrawRectangle(
		pdf.Rectangle{LLX: 0, LLY: 0, URX: 100, URY: 100},
		pdf.ShapeStyle{
			LineStyle: pdf.LineStyle{Width: 1, Color: &pdf.Color{R: 1, G: 0, B: 0, A: 1}},
			FillColor: &pdf.Color{R: 0, G: 1, B: 0, A: 1},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	s := renderedContent(t, doc)
	if !strings.Contains(s, " B\n") {
		t.Errorf("stroke+fill should emit B: %s", s)
	}
	if !strings.Contains(s, "1 0 0 RG") || !strings.Contains(s, "0 1 0 rg") {
		t.Error("both stroke and fill colors should be present")
	}
}

func TestDrawRectangle_NoStyleNoOp(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	err := page.DrawRectangle(pdf.Rectangle{LLX: 0, LLY: 0, URX: 100, URY: 100}, pdf.ShapeStyle{})
	if err != nil {
		t.Fatal(err)
	}
	s := renderedContent(t, doc)
	if strings.Contains(s, " re\n") {
		t.Error("empty ShapeStyle should produce no rectangle output")
	}
}

func TestDrawCircle_StrokeOnlyEmitsFourBeziers(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	err := page.DrawCircle(
		pdf.Point{X: 100, Y: 100}, 50,
		pdf.ShapeStyle{LineStyle: pdf.LineStyle{Width: 1}},
	)
	if err != nil {
		t.Fatal(err)
	}
	s := renderedContent(t, doc)
	curveCount := strings.Count(s, " c\n")
	if curveCount != 4 {
		t.Errorf("curve op count = %d, want 4", curveCount)
	}
	if !strings.Contains(s, " h\n") {
		t.Error("path should be closed (h)")
	}
}

func TestDrawCircle_NegativeRadiusErrors(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	err := page.DrawCircle(pdf.Point{}, -1, pdf.ShapeStyle{LineStyle: pdf.LineStyle{Width: 1}})
	if err == nil {
		t.Error("negative radius should error")
	}
}

func TestDrawEllipse_Basic(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	err := page.DrawEllipse(
		pdf.Point{X: 100, Y: 100}, 80, 40,
		pdf.ShapeStyle{LineStyle: pdf.LineStyle{Width: 1}},
	)
	if err != nil {
		t.Fatal(err)
	}
	s := renderedContent(t, doc)
	curveCount := strings.Count(s, " c\n")
	if curveCount != 4 {
		t.Errorf("curve op count = %d, want 4", curveCount)
	}
}

func TestDrawEllipse_NegativeAxisErrors(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	err := page.DrawEllipse(pdf.Point{}, -1, 1, pdf.ShapeStyle{LineStyle: pdf.LineStyle{Width: 1}})
	if err == nil {
		t.Error("negative rx should error")
	}
	err = page.DrawEllipse(pdf.Point{}, 1, -1, pdf.ShapeStyle{LineStyle: pdf.LineStyle{Width: 1}})
	if err == nil {
		t.Error("negative ry should error")
	}
}
