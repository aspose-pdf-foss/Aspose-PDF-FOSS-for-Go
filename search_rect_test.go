// SPDX-License-Identifier: MIT

package asposepdf_test

import (
	"testing"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

func TestSearchTextRectangle(t *testing.T) {
	doc := pdf.NewDocument(400, 400)
	p, _ := doc.Page(1)
	style := pdf.TextStyle{Font: pdf.FontHelvetica, Size: 18, Color: &pdf.Color{A: 1}}
	if err := p.AddText("alpha", style, pdf.Rectangle{LLX: 20, LLY: 350, URX: 200, URY: 385}); err != nil {
		t.Fatal(err)
	}
	if err := p.AddText("alpha", style, pdf.Rectangle{LLX: 20, LLY: 20, URX: 200, URY: 55}); err != nil {
		t.Fatal(err)
	}
	all, err := p.SearchText("alpha")
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Fatalf("unfiltered search found %d, want 2", len(all))
	}
	top := pdf.Rectangle{LLX: 0, LLY: 200, URX: 400, URY: 400}
	got, err := p.SearchText("alpha", pdf.SearchOptions{Rectangle: &top})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("rectangle search found %d, want 1", len(got))
	}
	if got[0].Rect.LLY < 200 {
		t.Errorf("matched the bottom occurrence (LLY=%.0f), want the top one", got[0].Rect.LLY)
	}
	none, err := p.SearchText("alpha", pdf.SearchOptions{Rectangle: &pdf.Rectangle{LLX: 250, LLY: 150, URX: 390, URY: 250}})
	if err != nil {
		t.Fatal(err)
	}
	if len(none) != 0 {
		t.Errorf("empty-region search found %d, want 0", len(none))
	}
}

func TestSearchTextRectangleDocument(t *testing.T) {
	doc := pdf.NewDocument(400, 400)
	doc.AddBlankPage(400, 400)
	style := pdf.TextStyle{Font: pdf.FontHelvetica, Size: 18, Color: &pdf.Color{A: 1}}
	for i := 1; i <= 2; i++ {
		p, _ := doc.Page(i)
		p.AddText("beta", style, pdf.Rectangle{LLX: 20, LLY: 350, URX: 200, URY: 385})
		p.AddText("beta", style, pdf.Rectangle{LLX: 20, LLY: 20, URX: 200, URY: 55})
	}
	top := pdf.Rectangle{LLX: 0, LLY: 200, URX: 400, URY: 400}
	got, err := doc.SearchText("beta", pdf.SearchOptions{Rectangle: &top})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Errorf("document rectangle search found %d, want 2 (top of each page)", len(got))
	}
}
