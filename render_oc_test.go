// SPDX-License-Identifier: MIT

package asposepdf

import "testing"

// TestRenderOptionalContentHidden checks that content inside a /OC marked-content
// section whose OCG is OFF in the default config is not drawn, while content
// outside it renders normally.
func TestRenderOptionalContentHidden(t *testing.T) {
	doc := NewDocument(100, 100)
	p, _ := doc.Page(1)

	const ocgID = 9999
	doc.objects[ocgID] = &pdfObject{Num: ocgID, Value: pdfDict{
		"/Type": pdfName("/OCG"), "/Name": "Layer1",
	}}
	ocgRef := pdfRef{Num: ocgID}
	if doc.catalog == nil {
		doc.catalog = pdfDict{}
	}
	doc.catalog["/OCProperties"] = pdfDict{"/D": pdfDict{"/OFF": pdfArray{ocgRef}}}
	p.pageResources()["/Properties"] = pdfDict{"/MC0": ocgRef}

	content := "/OC /MC0 BDC\n1 0 0 rg 0 0 100 100 re f\nEMC\n" + // hidden red (OCG OFF)
		"0 1 0 rg 0 0 50 50 re f\n" // visible green
	if err := p.appendToContentStream([]byte(content)); err != nil {
		t.Fatal(err)
	}
	img, err := p.RenderImage(RenderOptions{DPI: 72})
	if err != nil {
		t.Fatal(err)
	}

	col := func(x, y int) (int, int, int) {
		r, g, b, _ := img.At(x, y).RGBA()
		return int(r >> 8), int(g >> 8), int(b >> 8)
	}
	// Green rect: user (0,0)-(50,50) → device x[0,50] y[50,100].
	if r, g, b := col(10, 90); g < 200 || r > 60 || b > 60 {
		t.Errorf("visible green = (%d,%d,%d), want green", r, g, b)
	}
	// The OFF layer's red fill must not appear; this spot stays white.
	if r, g, b := col(75, 25); r < 240 || g < 240 || b < 240 {
		t.Errorf("hidden layer painted at (75,25) = (%d,%d,%d), want white", r, g, b)
	}
}

// TestRenderOptionalContentVisible checks the same content renders the red layer
// when the OCG is not in the OFF set.
func TestRenderOptionalContentVisible(t *testing.T) {
	doc := NewDocument(100, 100)
	p, _ := doc.Page(1)

	const ocgID = 9998
	doc.objects[ocgID] = &pdfObject{Num: ocgID, Value: pdfDict{"/Type": pdfName("/OCG")}}
	ocgRef := pdfRef{Num: ocgID}
	if doc.catalog == nil {
		doc.catalog = pdfDict{}
	}
	doc.catalog["/OCProperties"] = pdfDict{"/D": pdfDict{"/ON": pdfArray{ocgRef}}}
	p.pageResources()["/Properties"] = pdfDict{"/MC0": ocgRef}

	if err := p.appendToContentStream([]byte("/OC /MC0 BDC\n1 0 0 rg 0 0 100 100 re f\nEMC\n")); err != nil {
		t.Fatal(err)
	}
	img, err := p.RenderImage(RenderOptions{DPI: 72})
	if err != nil {
		t.Fatal(err)
	}
	if r, g, b, _ := img.At(50, 50).RGBA(); !(r>>8 > 200 && g>>8 < 60 && b>>8 < 60) {
		t.Errorf("ON layer not painted: (%d,%d,%d), want red", r>>8, g>>8, b>>8)
	}
}
