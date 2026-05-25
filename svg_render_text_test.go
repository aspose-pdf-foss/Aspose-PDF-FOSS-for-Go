// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"os"
	"testing"
)

func TestRenderSVG_TextEmitsBTET(t *testing.T) {
	data, err := os.ReadFile("testdata/svg/text_basic.svg")
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
	if err := renderSVG(page, svg, Rectangle{LLX: 0, LLY: 0, URX: 400, URY: 200}); err != nil {
		t.Fatal(err)
	}
	stream, err := page.contentStreams()
	if err != nil {
		t.Fatal(err)
	}
	// PDF text block landmarks:
	for _, want := range []string{"BT", "Tf", "Tm", "Tj", "ET"} {
		if !bytes.Contains(stream, []byte(want)) {
			t.Errorf("missing %q in stream", want)
		}
	}
}

func TestRenderSVG_TextAnchorMiddle(t *testing.T) {
	svgData := []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 200 100">
		<text x="100" y="50" font-size="16" text-anchor="middle">Centered</text>
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
	if err := renderSVG(page, svg, Rectangle{LLX: 0, LLY: 0, URX: 400, URY: 200}); err != nil {
		t.Fatal(err)
	}
	stream, err := page.contentStreams()
	if err != nil {
		t.Fatal(err)
	}
	// Stream should contain text operators.
	if !bytes.Contains(stream, []byte("BT")) {
		t.Errorf("expected BT in stream for text-anchor=middle")
	}
}

func TestRenderSVG_TextFillColor(t *testing.T) {
	svgData := []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 200 100">
		<text x="10" y="50" font-size="14" fill="red">Red text</text>
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
	if err := renderSVG(page, svg, Rectangle{LLX: 0, LLY: 0, URX: 400, URY: 200}); err != nil {
		t.Fatal(err)
	}
	stream, err := page.contentStreams()
	if err != nil {
		t.Fatal(err)
	}
	// Should contain rg (fill color) operator and BT/ET.
	if !bytes.Contains(stream, []byte("rg")) {
		t.Errorf("expected rg color operator for fill=red")
	}
	if !bytes.Contains(stream, []byte("BT")) {
		t.Errorf("expected BT in stream")
	}
}

func TestRenderSVG_TextDisplayNone(t *testing.T) {
	svgData := []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 200 100">
		<text x="10" y="50" font-size="14" display="none">Hidden</text>
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
	if err := renderSVG(page, svg, Rectangle{LLX: 0, LLY: 0, URX: 400, URY: 200}); err != nil {
		t.Fatal(err)
	}
	stream, err := page.contentStreams()
	if err != nil {
		t.Fatal(err)
	}
	// display=none: no BT should appear.
	if bytes.Contains(stream, []byte("BT")) {
		t.Errorf("display=none text should not emit BT, got:\n%s", stream)
	}
}

func TestRenderSVG_TextWithGradientFill(t *testing.T) {
	svg, err := parseSVGBytes([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 200 100">
		<defs>
			<linearGradient id="g1" x1="0" y1="0" x2="100" y2="0" gradientUnits="userSpaceOnUse">
				<stop offset="0" stop-color="red"/>
				<stop offset="1" stop-color="blue"/>
			</linearGradient>
		</defs>
		<text x="10" y="50" font-family="Arial" font-size="24" fill="url(#g1)">Gradient Text</text>
	</svg>`))
	if err != nil {
		t.Fatal(err)
	}
	doc := NewDocumentFromFormat(PageFormatA4)
	page, _ := doc.Page(1)
	if err := renderSVG(page, svg, Rectangle{LLX: 0, LLY: 0, URX: 400, URY: 200}); err != nil {
		t.Fatal(err)
	}
	stream, _ := page.contentStreams()
	if !bytes.Contains(stream, []byte("/Pattern cs")) {
		t.Errorf("expected /Pattern cs for gradient fill on text:\n%s", stream)
	}
	if !bytes.Contains(stream, []byte(" scn")) {
		t.Error("expected pattern setter (scn op)")
	}
	if !bytes.Contains(stream, []byte("BT")) {
		t.Error("expected BT/ET text block")
	}
}

func TestRenderSVG_TextAnchorsProduceDifferentXOffsets(t *testing.T) {
	data, err := os.ReadFile("testdata/svg/text_anchors.svg")
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
	if err := renderSVG(page, svg, Rectangle{LLX: 0, LLY: 0, URX: 400, URY: 200}); err != nil {
		t.Fatal(err)
	}
	stream, err := page.contentStreams()
	if err != nil {
		t.Fatal(err)
	}
	// All three texts present (as PDF string literals in Tj operators)
	for _, want := range []string{"(START)", "(MIDDLE)", "(END)"} {
		if !bytes.Contains(stream, []byte(want)) {
			t.Errorf("missing %q in stream:\n%s", want, stream)
		}
	}
	// Save a PDF for visual verification
	if err := os.MkdirAll("result_files", 0755); err != nil {
		t.Fatal(err)
	}
	out := "result_files/TestRenderSVG_TextAnchorsProduceDifferentXOffsets.pdf"
	if err := doc.Save(out); err != nil {
		t.Fatal(err)
	}
}
