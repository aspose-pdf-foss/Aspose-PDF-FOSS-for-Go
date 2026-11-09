// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"testing"
)

func TestUnknownPrinterMarkStaysGeneric(t *testing.T) {
	d := NewDocument(400, 400)
	p := d.Pages()[0]
	base := annotationBase{dict: pdfDict{"/Subtype": pdfName("/PrinterMark"), "/MN": pdfName("/SomeoneElses")}, doc: d, page: p}
	a := parseAnnotation(base)
	if _, ok := a.(*GenericAnnotation); !ok {
		t.Errorf("parsed as %T", a)
	}
}

func TestColorBarInkDefaultsWithoutAppearance(t *testing.T) {
	d := NewDocument(400, 400)
	base := annotationBase{dict: pdfDict{}, doc: d}
	if got := base.colorBarInk(); got != ColorsOfCMYKCyan {
		t.Errorf("ink %v", got)
	}
}

func TestPrinterMarkIsPrinted(t *testing.T) {
	d := NewDocument(400, 400)
	a := NewTrimMarkAnnotation(d.Pages()[0], PrinterMarkCornerTopLeft)
	if f, _ := a.dict["/F"].(int); f&4 == 0 {
		t.Errorf("/F = %v, want the Print flag", a.dict["/F"])
	}
}

func TestPageInformationRedrawsItsText(t *testing.T) {
	d := NewDocument(400, 400)
	a := NewPageInformationAnnotation(d.Pages()[0], Rectangle{LLX: 10, LLY: 380, URX: 200, URY: 390})
	stream := func() []byte {
		ap, _ := a.dict["/AP"].(pdfDict)
		st, _ := resolveRef(d.objects, ap["/N"]).(*pdfStream)
		return bytes.Clone(st.Data)
	}
	before := stream()
	a.SetContents("Something else")
	if bytes.Equal(before, stream()) {
		t.Error("the appearance still shows the old text")
	}
}
