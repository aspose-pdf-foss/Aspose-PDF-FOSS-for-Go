// SPDX-License-Identifier: MIT

package asposepdf_test

import (
	"encoding/xml"
	"os"
	"path/filepath"
	"strings"
	"testing"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

// svgTestDoc builds a page with text, vector shapes and an image.
func svgTestDoc(t *testing.T) *pdf.Document {
	t.Helper()
	dir := t.TempDir()
	imgPath := filepath.Join(dir, "pic.png")
	writeTestPNG(t, imgPath)

	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	p, err := doc.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.AddText("Vector SVG export sample", pdf.TextStyle{Size: 18}, pdf.Rectangle{LLX: 50, LLY: 720, URX: 545, URY: 780}); err != nil {
		t.Fatal(err)
	}
	if err := p.DrawCircle(pdf.Point{X: 150, Y: 600}, 40, pdf.ShapeStyle{
		LineStyle: pdf.LineStyle{Color: &pdf.Color{R: 0.8, A: 1}, Width: 2},
		FillColor: &pdf.Color{R: 1, G: 0.9, B: 0.2, A: 1},
	}); err != nil {
		t.Fatal(err)
	}
	if err := p.AddImage(imgPath, pdf.Rectangle{LLX: 300, LLY: 550, URX: 400, URY: 650}); err != nil {
		t.Fatal(err)
	}
	return doc
}

func TestPageWriteSVG(t *testing.T) {
	doc := svgTestDoc(t)
	p, _ := doc.Page(1)

	var out strings.Builder
	if err := p.WriteSVG(&out); err != nil {
		t.Fatal(err)
	}
	svg := out.String()

	for _, want := range []string{
		`<?xml version="1.0"`,
		`xmlns="http://www.w3.org/2000/svg"`,
		"viewBox=",
		"<path",          // glyph outlines + circle
		"data:image/png", // embedded image bytes
	} {
		if !strings.Contains(svg, want) {
			t.Errorf("SVG missing %q", want)
		}
	}
	// Text arrives as outlines, not <text> elements.
	if strings.Contains(svg, "<text") {
		t.Error("unexpected <text> element (v1 renders text as outline paths)")
	}
	// Glyphs mean many paths — well more than the one circle.
	if n := strings.Count(svg, "<path"); n < 10 {
		t.Errorf("only %d <path> elements; glyph outlines missing?", n)
	}

	// Well-formed XML.
	dec := xml.NewDecoder(strings.NewReader(svg))
	for {
		_, err := dec.Token()
		if err != nil {
			if err.Error() == "EOF" {
				break
			}
			t.Fatalf("SVG is not well-formed XML: %v", err)
		}
	}

	// Round-trip: our own SVG parser accepts the export.
	loaded, err := doc.LoadSVGFromStream(strings.NewReader(svg))
	if err != nil {
		t.Fatalf("exported SVG rejected by our SVG parser: %v", err)
	}
	if _, _, w, h := loaded.ViewBox(); w <= 0 || h <= 0 {
		t.Errorf("bad viewBox after round-trip: %g x %g", w, h)
	}
}

func TestDocumentSaveSVGMultiPage(t *testing.T) {
	doc := svgTestDoc(t)
	if err := doc.AddBlankPageFromFormat(pdf.PageFormatA4); err != nil {
		t.Fatal(err)
	}
	p2, _ := doc.Page(2)
	if err := p2.AddText("Second page", pdf.TextStyle{Size: 12}, pdf.Rectangle{LLX: 50, LLY: 700, URX: 545, URY: 780}); err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	out := filepath.Join(dir, "doc.svg")
	if err := doc.SaveSVG(out); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"doc_p1.svg", "doc_p2.svg"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("missing %s: %v", name, err)
		}
	}

	// Single-page selection writes to the exact path.
	single := filepath.Join(dir, "one.svg")
	if err := doc.SaveSVG(single, pdf.SVGSaveOptions{Pages: []int{2}}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(single)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "<svg") {
		t.Error("single-page output is not SVG")
	}
}

func TestPageSVGResourceWriter(t *testing.T) {
	doc := svgTestDoc(t)
	p, _ := doc.Page(1)

	written := map[string][]byte{}
	var out strings.Builder
	err := p.WriteSVG(&out, pdf.SVGSaveOptions{
		ResourceWriter: func(name string, data []byte) (string, error) {
			written[name] = data
			return "assets/" + name, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(written) == 0 {
		t.Fatal("ResourceWriter was never called")
	}
	if strings.Contains(out.String(), "data:image") {
		t.Error("data: URL present despite ResourceWriter")
	}
	if !strings.Contains(out.String(), "assets/") {
		t.Error("externalized URL missing from SVG")
	}
}
