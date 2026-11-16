// SPDX-License-Identifier: MIT

package asposepdf_test

import (
	"testing"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

// TestSearchTextBackgroundColor: with SearchForTextRelatedGraphics each match
// reports the colour of the last filled shape under it — a rectangle, a
// highlight drawn over it, a filled path that is not a rectangle — and nil
// where there is none (a stroked frame is not one) or the option is off (pdf-go-qjao).
func TestSearchTextBackgroundColor(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	p, _ := doc.Page(1)
	style := pdf.TextStyle{Font: pdf.FontHelvetica, Size: 12}
	yellow := pdf.Color{R: 1, G: 1, B: 0, A: 1}
	navy := pdf.Color{R: 0, G: 0, B: 0.5, A: 1}
	green := pdf.Color{R: 0.8, G: 1, B: 0.8, A: 1}
	// A cell shading with a yellow highlight drawn over its right half.
	if err := p.DrawRectangle(pdf.Rectangle{LLX: 60, LLY: 690, URX: 300, URY: 715}, pdf.ShapeStyle{FillColor: &navy}); err != nil {
		t.Fatal(err)
	}
	if err := p.DrawRectangle(pdf.Rectangle{LLX: 180, LLY: 692, URX: 300, URY: 713}, pdf.ShapeStyle{FillColor: &yellow}); err != nil {
		t.Fatal(err)
	}
	// A filled triangle behind the third word.
	tri := []pdf.Point{{X: 60, Y: 640}, {X: 160, Y: 640}, {X: 110, Y: 680}}
	if err := p.DrawPolygon(tri, pdf.ShapeStyle{FillColor: &green}); err != nil {
		t.Fatal(err)
	}
	// A frame drawn round the fourth word, stroked only: not a background.
	if err := p.DrawRectangle(pdf.Rectangle{LLX: 390, LLY: 640, URX: 450, URY: 665}, pdf.ShapeStyle{LineStyle: pdf.LineStyle{Color: &navy, Width: 1}}); err != nil {
		t.Fatal(err)
	}
	for _, w := range []struct {
		s    string
		x, y float64
	}{{"alpha", 70, 698}, {"beta", 200, 698}, {"gamma", 95, 650}, {"delta", 400, 650}} {
		if err := p.AddTextAt(w.s, style, w.x, w.y); err != nil {
			t.Fatal(err)
		}
	}
	want := map[string]*pdf.Color{"alpha": &navy, "beta": &yellow, "gamma": &green, "delta": nil}
	ms, err := doc.SearchText(`alpha|beta|gamma|delta`, pdf.SearchOptions{Regex: true, SearchForTextRelatedGraphics: true})
	if err != nil || len(ms) != 4 {
		t.Fatalf("%d matches, %v", len(ms), err)
	}
	for _, m := range ms {
		got, exp := m.BackgroundColor, want[m.Text]
		if (got == nil) != (exp == nil) || (got != nil && *got != *exp) {
			t.Errorf("%s: background %v, want %v", m.Text, got, exp)
		}
	}
	pms, _ := p.SearchText("beta", pdf.SearchOptions{SearchForTextRelatedGraphics: true})
	if len(pms) != 1 || pms[0].BackgroundColor == nil || *pms[0].BackgroundColor != yellow {
		t.Errorf("Page.SearchText: %+v", pms)
	}
	plain, _ := doc.SearchText("beta")
	if len(plain) != 1 || plain[0].BackgroundColor != nil {
		t.Errorf("background reported without the option: %+v", plain)
	}
}
