package asposepdf_test

import (
	"bytes"
	"strings"
	"testing"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
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

func TestDrawPolyline_TwoPoints(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	err := page.DrawPolyline(
		[]pdf.Point{{X: 0, Y: 0}, {X: 100, Y: 100}},
		pdf.LineStyle{Width: 1},
	)
	if err != nil {
		t.Fatal(err)
	}
	s := renderedContent(t, doc)
	if !strings.Contains(s, "0 0 m") || !strings.Contains(s, "100 100 l") {
		t.Errorf("missing polyline path ops: %s", s)
	}
	if !strings.Contains(s, "S\n") {
		t.Error("polyline should stroke")
	}
	if strings.Contains(s, " h\n") || strings.Contains(s, "B\n") || strings.Contains(s, "f\n") {
		t.Error("polyline should not close or fill")
	}
}

func TestDrawPolyline_OnePointErrors(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	err := page.DrawPolyline([]pdf.Point{{X: 0, Y: 0}}, pdf.LineStyle{Width: 1})
	if err == nil {
		t.Error("polyline with one point should error")
	}
}

func TestDrawPolygon_Triangle(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	err := page.DrawPolygon(
		[]pdf.Point{{X: 0, Y: 0}, {X: 100, Y: 0}, {X: 50, Y: 87}},
		pdf.ShapeStyle{
			LineStyle: pdf.LineStyle{Width: 1},
			FillColor: &pdf.Color{R: 0, G: 1, B: 0, A: 1},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	s := renderedContent(t, doc)
	lineCount := strings.Count(s, " l\n")
	if lineCount < 2 {
		t.Errorf("triangle should have >= 2 line ops, got %d", lineCount)
	}
	if !strings.Contains(s, " h\n") {
		t.Error("polygon should close (h)")
	}
	if !strings.Contains(s, "B\n") {
		t.Error("polygon with stroke+fill should emit B")
	}
}

func TestDrawPolygon_TwoPointsErrors(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	err := page.DrawPolygon(
		[]pdf.Point{{X: 0, Y: 0}, {X: 100, Y: 100}},
		pdf.ShapeStyle{LineStyle: pdf.LineStyle{Width: 1}},
	)
	if err == nil {
		t.Error("polygon with two points should error")
	}
}

func TestDrawPath_NilErrors(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	err := page.DrawPath(nil, pdf.ShapeStyle{LineStyle: pdf.LineStyle{Width: 1}})
	if err == nil {
		t.Error("nil path should error")
	}
}

func TestDrawPath_EmptyPathNoOp(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	err := page.DrawPath(pdf.NewPath(), pdf.ShapeStyle{LineStyle: pdf.LineStyle{Width: 1}})
	if err != nil {
		t.Fatal(err)
	}
	s := renderedContent(t, doc)
	if strings.Contains(s, " m\n") {
		t.Error("empty path should emit nothing")
	}
}

func TestDrawPath_BuilderChain(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	path := pdf.NewPath().MoveTo(50, 50).LineTo(150, 50).CurveTo(180, 80, 180, 120, 150, 150).Close()
	err := page.DrawPath(path, pdf.ShapeStyle{LineStyle: pdf.LineStyle{Width: 2}})
	if err != nil {
		t.Fatal(err)
	}
	s := renderedContent(t, doc)
	for _, want := range []string{"50 50 m", " l\n", " c\n", " h\n", "S\n"} {
		if !strings.Contains(s, want) {
			t.Errorf("output missing %q", want)
		}
	}
}

func TestDrawRoundedRectangle_Basic(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	err := page.DrawRoundedRectangle(
		pdf.Rectangle{LLX: 50, LLY: 50, URX: 200, URY: 150}, 10,
		pdf.ShapeStyle{
			LineStyle: pdf.LineStyle{Width: 1},
			FillColor: &pdf.Color{R: 0.9, G: 0.9, B: 0.9, A: 1},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	s := renderedContent(t, doc)
	// Expect at least 3 line ops (4 straight edges; some may be omitted if
	// the radius == half-side reduces a side to zero length, but with
	// radius 10 on 100×150 rect that won't happen).
	if strings.Count(s, " l\n") < 3 {
		t.Errorf("expected >=3 line ops for edges, got %d", strings.Count(s, " l\n"))
	}
	// 4 corner arcs = at least 4 curve ops.
	if strings.Count(s, " c\n") < 4 {
		t.Errorf("expected >=4 curve ops for corners, got %d", strings.Count(s, " c\n"))
	}
}

func TestDrawRoundedRectangle_NegativeRadiusErrors(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	err := page.DrawRoundedRectangle(
		pdf.Rectangle{LLX: 0, LLY: 0, URX: 100, URY: 100}, -5,
		pdf.ShapeStyle{LineStyle: pdf.LineStyle{Width: 1}},
	)
	if err == nil {
		t.Error("negative radius should error")
	}
}

func TestDrawRoundedRectangle_LargeRadiusClampedToHalfShorterSide(t *testing.T) {
	// Rect 100×40, radius 50 → clamped to 20 (half of shorter side).
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	err := page.DrawRoundedRectangle(
		pdf.Rectangle{LLX: 0, LLY: 0, URX: 100, URY: 40}, 50,
		pdf.ShapeStyle{LineStyle: pdf.LineStyle{Width: 1}},
	)
	if err != nil {
		t.Fatal(err)
	}
	// No assertion on exact output — just verify it doesn't error and produces some output.
	s := renderedContent(t, doc)
	if strings.Count(s, " c\n") < 4 {
		t.Errorf("clamped radius should still emit 4 corner arcs, got %d", strings.Count(s, " c\n"))
	}
}

func TestDrawLine_AlphaUsesExtGState(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	err := page.DrawLine(
		pdf.Point{}, pdf.Point{X: 100},
		pdf.LineStyle{
			Width: 2,
			Color: &pdf.Color{R: 1, G: 0, B: 0, A: 0.5},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	s := renderedContent(t, doc)
	if !strings.Contains(s, " gs\n") {
		t.Errorf("alpha < 1 should emit gs op: %s", s)
	}
}

func TestDrawRectangle_FillAlphaUsesExtGState(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	err := page.DrawRectangle(
		pdf.Rectangle{LLX: 0, LLY: 0, URX: 100, URY: 100},
		pdf.ShapeStyle{FillColor: &pdf.Color{R: 0, G: 0, B: 1, A: 0.3}},
	)
	if err != nil {
		t.Fatal(err)
	}
	s := renderedContent(t, doc)
	if !strings.Contains(s, " gs\n") {
		t.Errorf("fill alpha < 1 should emit gs op: %s", s)
	}
}

func TestDrawLine_FullOpacityNoExtGState(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	_ = page.DrawLine(
		pdf.Point{}, pdf.Point{X: 100},
		pdf.LineStyle{Width: 1, Color: &pdf.Color{R: 0, G: 0, B: 0, A: 1}},
	)
	s := renderedContent(t, doc)
	if strings.Contains(s, " gs\n") {
		t.Error("alpha = 1 should not emit gs (no transparency needed)")
	}
}

// Aspose .NET sample:
//   Graph graph = new Graph(width, height);
//   Line line = new Line(new float[] {x1, y1, x2, y2});
//   line.GraphInfo.Color = Color.Red;
//   line.GraphInfo.LineWidth = 2;
//   line.GraphInfo.DashArray = new int[] {4, 2};
//   graph.Shapes.Add(line);
//   page.Paragraphs.Add(graph);
//
// In this library — methods directly on Page (no Graph container needed):
func TestAsposeParity_DrawLineWithDash(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	err := page.DrawLine(
		pdf.Point{X: 50, Y: 50}, pdf.Point{X: 200, Y: 200},
		pdf.LineStyle{
			Color:       &pdf.Color{R: 1, G: 0, B: 0, A: 1},
			Width:       2,
			DashPattern: []float64{4, 2},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
}

// Aspose .NET sample:
//   Circle circle = new Circle(cx, cy, radius);
//   circle.GraphInfo.Color = Color.Blue;
//   circle.GraphInfo.FillColor = Color.LightBlue;
//   circle.GraphInfo.LineWidth = 1;
//   graph.Shapes.Add(circle);
func TestAsposeParity_DrawCircleWithFill(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	err := page.DrawCircle(
		pdf.Point{X: 200, Y: 200}, 50,
		pdf.ShapeStyle{
			LineStyle: pdf.LineStyle{
				Color: &pdf.Color{R: 0, G: 0, B: 1, A: 1},
				Width: 1,
			},
			FillColor: &pdf.Color{R: 0.7, G: 0.9, B: 1, A: 1},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
}

// Aspose .NET sample (arbitrary path via line segments + bezier):
//   Curve curve = new Curve(new float[] {p0x, p0y, c1x, c1y, c2x, c2y, p3x, p3y});
//   graph.Shapes.Add(curve);
func TestAsposeParity_DrawPathArbitrary(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	path := pdf.NewPath().
		MoveTo(50, 50).
		CurveTo(100, 0, 200, 100, 250, 50).
		LineTo(300, 100).
		Close()
	err := page.DrawPath(path, pdf.ShapeStyle{LineStyle: pdf.LineStyle{Width: 1}})
	if err != nil {
		t.Fatal(err)
	}
}

func TestVector_AES128Roundtrip(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	_ = page.DrawCircle(pdf.Point{X: 100, Y: 100}, 50, pdf.ShapeStyle{
		LineStyle: pdf.LineStyle{Width: 2, Color: &pdf.Color{R: 1, G: 0, B: 0, A: 1}},
	})
	doc.SetEncryption(pdf.EncryptionOptions{
		UserPassword: "x",
		Algorithm:    pdf.EncryptionAlgAES128,
	})

	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	doc2, err := pdf.OpenStreamWithPassword(bytes.NewReader(buf.Bytes()), "x")
	if err != nil {
		t.Fatal(err)
	}
	page2, _ := doc2.Page(1)
	if _, err := page2.ExtractText(); err != nil {
		t.Fatal(err) // Page must at least be parseable.
	}
}

func TestVector_MultiplePages(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	if err := doc.AddBlankPage(595, 842); err != nil {
		t.Fatal(err)
	}
	if err := doc.AddBlankPage(595, 842); err != nil {
		t.Fatal(err)
	}

	for i := 1; i <= doc.PageCount(); i++ {
		page, _ := doc.Page(i)
		_ = page.DrawCircle(
			pdf.Point{X: 100, Y: float64(100 + i*50)}, 30,
			pdf.ShapeStyle{LineStyle: pdf.LineStyle{Width: 1}},
		)
	}

	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	doc2, err := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if doc2.PageCount() != 3 {
		t.Errorf("PageCount after roundtrip = %d, want 3", doc2.PageCount())
	}
}
