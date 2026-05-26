// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"os"
	"testing"
)

func TestBuildMaskFormXObject_Smoke(t *testing.T) {
	doc := NewDocumentFromFormat(PageFormatA4)
	page, err := doc.Page(1)
	if err != nil {
		t.Fatal(err)
	}

	svg := &SVG{
		defs:      map[string]svgNode{},
		gradients: map[string]svgGradient{},
		root:      &svgGroup{style: defaultSVGStyle()},
	}

	whiteStyle := defaultSVGStyle()
	whiteStyle.fill = &svgPaint{color: &Color{R: 1, G: 1, B: 1, A: 1}}

	mask := &svgMask{
		units:        svgGradientObjectBBox,
		contentUnits: svgGradientUserSpace,
		children: []svgNode{
			&svgRect{
				x: 0, y: 0, w: 100, h: 100,
				style: whiteStyle,
			},
		},
	}

	ref, err := buildMaskFormXObject(page, svg, mask, Rectangle{LLX: 0, LLY: 0, URX: 100, URY: 100})
	if err != nil {
		t.Fatal(err)
	}
	if ref.Num == 0 {
		t.Error("expected a non-zero pdfRef.Num")
	}

	obj, ok := doc.objects[ref.Num]
	if !ok {
		t.Fatalf("indirect object %d not registered in doc.objects", ref.Num)
	}

	// Verify the object is a Form XObject stream with the expected dict entries.
	stream, ok := obj.Value.(*pdfStream)
	if !ok {
		t.Fatalf("expected *pdfStream, got %T", obj.Value)
	}
	if stream.Dict["/Subtype"] != pdfName("/Form") {
		t.Errorf("/Subtype = %v, want /Form", stream.Dict["/Subtype"])
	}
	grp, ok := stream.Dict["/Group"].(pdfDict)
	if !ok {
		t.Fatalf("/Group is %T, want pdfDict", stream.Dict["/Group"])
	}
	if grp["/S"] != pdfName("/Transparency") {
		t.Errorf("/Group/S = %v, want /Transparency", grp["/S"])
	}
	if grp["/CS"] != pdfName("/DeviceGray") {
		t.Errorf("/Group/CS = %v, want /DeviceGray", grp["/CS"])
	}
}

func TestBuildMaskFormXObject_NilMask(t *testing.T) {
	doc := NewDocumentFromFormat(PageFormatA4)
	page, _ := doc.Page(1)
	svg := &SVG{
		defs:      map[string]svgNode{},
		gradients: map[string]svgGradient{},
		root:      &svgGroup{style: defaultSVGStyle()},
	}
	ref, err := buildMaskFormXObject(page, svg, nil, Rectangle{LLX: 0, LLY: 0, URX: 100, URY: 100})
	if err != nil {
		t.Fatal(err)
	}
	if ref.Num != 0 {
		t.Errorf("nil mask should return Num==0, got %d", ref.Num)
	}
}

func TestBuildMaskFormXObject_EmptyChildren(t *testing.T) {
	doc := NewDocumentFromFormat(PageFormatA4)
	page, _ := doc.Page(1)
	svg := &SVG{
		defs:      map[string]svgNode{},
		gradients: map[string]svgGradient{},
		root:      &svgGroup{style: defaultSVGStyle()},
	}
	mask := &svgMask{
		units:    svgGradientObjectBBox,
		children: nil,
	}
	ref, err := buildMaskFormXObject(page, svg, mask, Rectangle{LLX: 0, LLY: 0, URX: 100, URY: 100})
	if err != nil {
		t.Fatal(err)
	}
	if ref.Num != 0 {
		t.Errorf("empty mask should return Num==0, got %d", ref.Num)
	}
}

func TestRenderSVG_MaskEmitsGS(t *testing.T) {
	data, err := os.ReadFile("testdata/svg/mask_circle.svg")
	if err != nil {
		t.Fatal(err)
	}
	svg, err := parseSVGBytes(data)
	if err != nil {
		t.Fatal(err)
	}
	doc := NewDocumentFromFormat(PageFormatA4)
	page, _ := doc.Page(1)
	if err := renderSVG(page, svg, Rectangle{LLX: 0, LLY: 0, URX: 200, URY: 200}); err != nil {
		t.Fatal(err)
	}
	stream, _ := page.contentStreams()
	if !bytes.Contains(stream, []byte(" gs\n")) {
		t.Errorf("expected /GS<n> gs operator for soft mask in stream:\n%s", stream)
	}
}
