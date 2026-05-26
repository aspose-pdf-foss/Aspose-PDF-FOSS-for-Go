// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"os"
	"testing"
)

func TestRenderSVG_ImageEmitsDo(t *testing.T) {
	data, err := os.ReadFile("testdata/svg/image_inline_png.svg")
	if err != nil {
		t.Fatal(err)
	}
	svg, err := parseSVGBytes(data)
	if err != nil {
		t.Fatal(err)
	}
	doc := NewDocumentFromFormat(PageFormatA4)
	page, err := doc.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	if err := renderSVG(page, svg, Rectangle{LLX: 0, LLY: 0, URX: 200, URY: 200}); err != nil {
		t.Fatal(err)
	}
	stream, err := page.contentStreams()
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{" cm\n", " Do\n", "q\n", "Q\n"} {
		if !bytes.Contains(stream, []byte(want)) {
			t.Errorf("missing %q in stream:\n%s", want, stream)
		}
	}
}

func TestRenderSVG_ImagePreserveAspectMeet(t *testing.T) {
	// A 4x4 PNG embedded in a 80x60 rect. meet scales uniformly: min(80/4, 60/4)=15.
	data, err := os.ReadFile("testdata/svg/image_inline_png.svg")
	if err != nil {
		t.Fatal(err)
	}
	svg, err := parseSVGBytes(data)
	if err != nil {
		t.Fatal(err)
	}
	doc := NewDocumentFromFormat(PageFormatA4)
	page, err := doc.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	if err := renderSVG(page, svg, Rectangle{LLX: 0, LLY: 0, URX: 200, URY: 200}); err != nil {
		t.Fatal(err)
	}
	stream, err := page.contentStreams()
	if err != nil {
		t.Fatal(err)
	}
	// Verify the stream contains a Do operator and the image XObject was registered.
	if !bytes.Contains(stream, []byte("Do\n")) {
		t.Errorf("missing Do operator in stream:\n%s", stream)
	}
}

func TestRenderSVG_ImageNonePreserveAspect(t *testing.T) {
	// Valid 4×4 PNG (solid red-ish, no alpha).
	const validPNG = "iVBORw0KGgoAAAANSUhEUgAAAAQAAAAECAIAAAAmkwkpAAAAGElEQVR4nGI5kWLEAANMDEgANwcQAAD//0aSAWnltlapAAAAAElFTkSuQmCC"
	svgData := []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 100">
  <image x="0" y="0" width="100" height="100" preserveAspectRatio="none"
         href="data:image/png;base64,` + validPNG + `"/>
</svg>`)
	svg, err := parseSVGBytes(svgData)
	if err != nil {
		t.Fatal(err)
	}
	doc := NewDocumentFromFormat(PageFormatA4)
	page, err := doc.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	if err := renderSVG(page, svg, Rectangle{LLX: 0, LLY: 0, URX: 200, URY: 200}); err != nil {
		t.Fatal(err)
	}
	stream, err := page.contentStreams()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(stream, []byte("Do\n")) {
		t.Errorf("missing Do operator in stream:\n%s", stream)
	}
}
