// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"image"
	"strings"
	"testing"
)

func iconDoc(t *testing.T, subtype string) (*Document, *Page, pdfDict) {
	t.Helper()
	d := NewDocument(200, 200)
	p, _ := d.Page(1)
	ad := pdfDict{"/Type": pdfName("/Annot"), "/Subtype": pdfName(subtype), "/Rect": pdfArray{50, 50, 74, 74}, "/F": 28}
	p.pageDict()["/Annots"] = pdfArray{pdfRef{Num: d.addObject(ad)}}
	return d, p, ad
}

// PDF/A-2 wants an appearance of a sticky note, and the converter draws one; PDF/A-1
// does not, so there the viewer's own icon stays.
func TestPDFAIconAppearances(t *testing.T) {
	d, p, ad := iconDoc(t, "/Text")
	if !pdfaHasRule(d.ValidatePDFA(PDFA2B), "ANNOTATION_NO_AP") {
		t.Error("PDF/A-2 needs an appearance")
	}
	before, _ := p.RenderImage(RenderOptions{DPI: 72})
	if pdfaHasRule(d.ValidatePDFA(PDFA1B), "ANNOTATION_NO_AP") {
		t.Error("PDF/A-1 asks an appearance of a form field only")
	}
	rep, err := d.ConvertToPDFA(PDFA2B)
	if err != nil {
		t.Fatal(err)
	}
	if pdfaHasRule(rep, "ANNOTATION_NO_AP") {
		t.Error("still no appearance after converting")
	}
	if _, has := resolveRefToDict(d.objects, ad["/AP"]); !has {
		t.Fatal("no /AP")
	}
	after, _ := p.RenderImage(RenderOptions{DPI: 72})
	if bytes.Equal(before.(*image.RGBA).Pix, after.(*image.RGBA).Pix) {
		t.Error("the icon draws nothing")
	}

	d, _, ad = iconDoc(t, "/Text")
	if _, err := d.ConvertToPDFA(PDFA1B); err != nil {
		t.Fatal(err)
	}
	if _, has := ad["/AP"]; has {
		t.Error("PDF/A-1 does not need the appearance")
	}
}

func TestPDFAWidgetNeedsAppearanceInPart1(t *testing.T) {
	d, _, _ := iconDoc(t, "/Widget")
	if !pdfaHasRule(d.ValidatePDFA(PDFA1B), "ANNOTATION_NO_AP") {
		t.Error("a form field needs an appearance in PDF/A-1 as well")
	}
}

// An annotation with no area needs none, in any part; a link never does.
func TestPDFAAppearanceExemptions(t *testing.T) {
	d, _, ad := iconDoc(t, "/Text")
	ad["/Rect"] = pdfArray{50, 50, 50, 50}
	if pdfaHasRule(d.ValidatePDFA(PDFA2B), "ANNOTATION_NO_AP") {
		t.Error("no area, no appearance needed")
	}
	if _, err := d.ConvertToPDFA(PDFA2B); err != nil {
		t.Fatal(err)
	}
	if _, has := ad["/AP"]; has {
		t.Error("an appearance was drawn for no area")
	}
	d, _, _ = iconDoc(t, "/Link")
	if pdfaHasRule(d.ValidatePDFA(PDFA2B), "ANNOTATION_NO_AP") {
		t.Error("a link needs no appearance")
	}
	// One flat dimension is still an area to veraPDF, which exempts only both.
	d, _, ad = iconDoc(t, "/Stamp")
	ad["/Rect"] = pdfArray{50, 50, 80, 50}
	if !pdfaHasRule(d.ValidatePDFA(PDFA2B), "ANNOTATION_NO_AP") {
		t.Error("only a rectangle flat both ways is exempt")
	}
}

// The two kinds differ, an existing appearance stays, and /C decides the colour.
func TestIconAppearanceDrawing(t *testing.T) {
	note := string(generateIconAppearance("/Text", Color{R: 1, A: 1}, 20, 20).Data)
	file := string(generateIconAppearance("/FileAttachment", Color{R: 1, A: 1}, 20, 20).Data)
	if note == file {
		t.Error("the two icons are the same")
	}
	if len(generateIconAppearance("/Text", Color{}, 0, 20).Data) != 0 {
		t.Error("nothing to draw on a flat rectangle")
	}
	d, _, ad := iconDoc(t, "/FileAttachment")
	ad["/C"] = pdfArray{0, 0.5, 1}
	d.generateIconAppearances()
	ap, _ := resolveRefToDict(d.objects, ad["/AP"])
	form, _ := resolveRef(d.objects, ap["/N"]).(*pdfStream)
	if form == nil || !strings.Contains(string(decodedStreamData(form)), "0 0.5 1 rg") {
		t.Errorf("appearance %v", form)
	}
	ad["/AP"] = pdfDict{"/N": pdfRef{Num: 1}}
	d.generateIconAppearances()
	if ap, _ := ad["/AP"].(pdfDict); ap["/N"] != (pdfRef{Num: 1}) {
		t.Error("an existing appearance was replaced")
	}
}

func TestPDFAReversedRect(t *testing.T) {
	r, ok := pdfaAnnotRect(map[int]*pdfObject{}, pdfDict{"/Rect": pdfArray{74, 74, 50, 50}})
	if !ok || r != (Rectangle{LLX: 50, LLY: 50, URX: 74, URY: 74}) {
		t.Errorf("%v %v", r, ok)
	}
	if _, ok := pdfaAnnotRect(map[int]*pdfObject{}, pdfDict{}); ok {
		t.Error("no /Rect")
	}
}

// A decoded stream goes out with Flate, so LZW is only a problem while it stays raw.
func TestPDFALZWOnlyWhenWritten(t *testing.T) {
	d := NewDocument(100, 100)
	st := &pdfStream{Dict: pdfDict{"/Filter": pdfName("/LZWDecode")}, Data: []byte("x"), Decoded: true}
	d.addObject(st)
	if pdfaHasRule(d.ValidatePDFA(PDFA1B), "LZW_FILTER") {
		t.Error("a decoded stream is not written with LZW")
	}
	st.Decoded = false
	if !pdfaHasRule(d.ValidatePDFA(PDFA1B), "LZW_FILTER") {
		t.Error("a raw LZW stream is")
	}
}

func TestPDFDocToString(t *testing.T) {
	for in, want := range map[string]string{
		"plain":                "plain",
		"caf\xc3\xa9":          "café", // valid UTF-8 stays
		"iText\x92 \xa92018":   "iText™ ©2018",
		"\xff\xff":             "ÿÿ",
		"\xa0 \x93x \x18y\xad": "€ ﬁx ˘y�",
		"\x9f\xe9":             "�é",
	} {
		if got := pdfDocToString(in); got != want {
			t.Errorf("%q: %q, want %q", in, got, want)
		}
		if got := decodeFormString(in); got != want {
			t.Errorf("form %q: %q, want %q", in, got, want)
		}
		if got := decodeTextString(in); got != want {
			t.Errorf("text %q: %q, want %q", in, got, want)
		}
	}
	// What veraPDF compares: /Info as PDFDocEncoding against the XMP written from it.
	d := NewDocument(100, 100)
	d.info = pdfDict{"/Producer": "iText\x92 \xa92018", "/Title": "\xff\xff"}
	info, err := d.Info()
	if err != nil {
		t.Fatal(err)
	}
	if info.Producer != "iText™ ©2018" || info.Title != "ÿÿ" {
		t.Errorf("info %q %q", info.Producer, info.Title)
	}
	if _, err := d.ConvertToPDFA(PDFA1B); err != nil {
		t.Fatal(err)
	}
	if rep := d.ValidatePDFA(PDFA1B); pdfaHasRule(rep, "INFO_XMP_MISMATCH") {
		t.Errorf("%+v", rep.Issues)
	}
	x, err := d.XMP()
	if err != nil {
		t.Fatal(err)
	}
	if x.Producer != "iText™ ©2018" || x.Title != "ÿÿ" {
		t.Errorf("xmp %q %q", x.Producer, x.Title)
	}
}
