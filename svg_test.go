// SPDX-License-Identifier: MIT

package asposepdf_test

import (
	"bytes"
	"os"
	"testing"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

func TestPage_AddSVG_FromPath(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	page, _ := doc.Page(1)
	if err := page.AddSVG("testdata/svg/all_shapes.svg", pdf.Rectangle{LLX: 50, LLY: 600, URX: 250, URY: 800}); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll("result_files", 0755); err != nil {
		t.Fatal(err)
	}
	if err := doc.Save("result_files/TestPage_AddSVG_FromPath.pdf"); err != nil {
		t.Fatal(err)
	}
}

func TestPage_AddSVGFromStream(t *testing.T) {
	data, _ := os.ReadFile("testdata/svg/all_shapes.svg")
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	page, _ := doc.Page(1)
	if err := page.AddSVGFromStream(bytes.NewReader(data), pdf.Rectangle{LLX: 50, LLY: 600, URX: 250, URY: 800}); err != nil {
		t.Fatal(err)
	}
}

func TestPage_AddSVG_InvalidXMLReturnsError(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	page, _ := doc.Page(1)
	if err := os.MkdirAll("result_files", 0755); err != nil {
		t.Fatal(err)
	}
	tmp := "result_files/_invalid.svg"
	os.WriteFile(tmp, []byte("<svg><not-closed"), 0644)
	defer os.Remove(tmp)
	if err := page.AddSVG(tmp, pdf.Rectangle{LLX: 0, LLY: 0, URX: 100, URY: 100}); err == nil {
		t.Error("expected error for malformed XML")
	}
}

func TestDocument_LoadSVG(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	svg, err := doc.LoadSVG("testdata/svg/all_shapes.svg")
	if err != nil {
		t.Fatal(err)
	}
	if svg == nil {
		t.Fatal("nil SVG")
	}
}

func TestDocument_LoadSVGFromStream(t *testing.T) {
	data, _ := os.ReadFile("testdata/svg/all_shapes.svg")
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	svg, err := doc.LoadSVGFromStream(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if svg == nil {
		t.Fatal("nil")
	}
}

func TestDocument_AddSVGWatermark_AllPages(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	doc.AddBlankPageFromFormat(pdf.PageFormatA4)
	doc.AddBlankPageFromFormat(pdf.PageFormatA4)
	if err := doc.AddSVGWatermark("testdata/aspose-logo.svg"); err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 3; i++ {
		p, err := doc.Page(i)
		if err != nil {
			t.Errorf("page %d error: %v", i, err)
		} else if p == nil {
			t.Errorf("page %d nil", i)
		}
	}
	if err := os.MkdirAll("result_files", 0755); err != nil {
		t.Fatal(err)
	}
	doc.Save("result_files/TestDocument_AddSVGWatermark_AllPages.pdf")
}

func TestDocument_AddSVGWatermark_SelectPages(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	doc.AddBlankPageFromFormat(pdf.PageFormatA4)
	doc.AddBlankPageFromFormat(pdf.PageFormatA4)
	if err := doc.AddSVGWatermark("testdata/aspose-logo.svg", 1, 3); err != nil {
		t.Fatal(err)
	}
}

func TestDocument_AddSVGObjectWatermark(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	svg, _ := doc.LoadSVG("testdata/aspose-logo.svg")
	doc.AddBlankPageFromFormat(pdf.PageFormatA4)
	if err := doc.AddSVGObjectWatermark(svg); err != nil {
		t.Fatal(err)
	}
}

func TestSVG_Inspectors(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	svg, _ := doc.LoadSVG("testdata/svg/rect.svg")
	w, h := svg.Size()
	if w != 100 || h != 50 {
		t.Errorf("Size() = (%g, %g), want (100, 50)", w, h)
	}
	vx, vy, vw, vh := svg.ViewBox()
	if vx != 0 || vy != 0 || vw != 100 || vh != 50 {
		t.Errorf("ViewBox() = (%g, %g, %g, %g)", vx, vy, vw, vh)
	}
}
