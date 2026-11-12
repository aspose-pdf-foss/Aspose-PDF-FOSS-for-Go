// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
)

// removeTextDoc builds two pages sharing a Form XObject that shows text.
// Page 1 also shows text of its own with Tj, TJ, ' and ", one span under
// /ActualText, a rectangle, and text in a font with no known widths.
func removeTextDoc(t *testing.T) *Document {
	t.Helper()
	d := NewDocument(400, 400)
	if err := d.AddBlankPage(400, 400); err != nil {
		t.Fatal(err)
	}
	font := d.addObject(pdfDict{"/Type": pdfName("/Font"), "/Subtype": pdfName("/Type1"), "/BaseFont": pdfName("/Helvetica")})
	odd := d.addObject(pdfDict{"/Type": pdfName("/Font"), "/Subtype": pdfName("/Type3")}) // nothing known about it
	formData := []byte("BT /F1 12 Tf 10 10 Td (FormSecret) Tj ET")
	form := d.addObject(&pdfStream{
		Dict: pdfDict{"/Type": pdfName("/XObject"), "/Subtype": pdfName("/Form"), "/BBox": pdfArray{0, 0, 200, 50},
			"/Resources": pdfDict{"/Font": pdfDict{"/F1": pdfRef{Num: font}}}, "/Length": len(formData)},
		Data: formData, Decoded: true,
	})
	pages := d.Pages()
	for i, p := range pages {
		p.pageDict()["/Resources"] = pdfDict{
			"/Font":    pdfDict{"/F1": pdfRef{Num: font}, "/F2": pdfRef{Num: odd}},
			"/XObject": pdfDict{"/Fm": pdfRef{Num: form}},
		}
		content := "q 1 0 0 1 50 300 cm /Fm Do Q "
		if i == 0 {
			content += "0 0 1 rg 20 20 100 50 re f " +
				"BT /F1 12 Tf 20 200 Td (PlainSecret) Tj [(Kern) -100 (Secret)] TJ 14 TL (QuoteSecret) ' 2 1 (DQuoteSecret) \" ET " +
				"/Span <</ActualText (ActualSecret)>> BDC BT /F1 12 Tf 20 150 Td (x) Tj ET EMC " +
				"BT /F2 12 Tf 20 100 Td (OddSecret) Tj ET"
		}
		if err := p.appendToContentStream([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	return d
}

func pageText(t *testing.T, p *Page) string {
	t.Helper()
	s, err := p.ExtractText()
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(s)
}

// TestRemoveAllTextDocument: every text operator goes from every page and
// from the form they draw, /ActualText too, while the rectangle stays; the
// removed text is gone from the saved file (pdf-go-7ukm).
func TestRemoveAllTextDocument(t *testing.T) {
	d := removeTextDoc(t)
	if err := d.RemoveAllText(); err != nil {
		t.Fatal(err)
	}
	// Both pages draw one clone; the original form, unused, is gone.
	if n := formCount(d); n != 1 {
		t.Errorf("%d forms, want 1", n)
	}
	for _, p := range d.Pages() {
		if s := pageText(t, p); s != "" {
			t.Errorf("page %d still has text %q", p.Number(), s)
		}
	}
	var buf bytes.Buffer
	if _, err := d.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	re, err := OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{"FormSecret", "PlainSecret", "Secret)", "QuoteSecret", "DQuoteSecret", "ActualSecret", "OddSecret"} {
		if found, _ := documentContainsBytes(re, s); found {
			t.Errorf("%q still in the saved file", s)
		}
	}
	p, _ := re.Page(1)
	data, _ := p.contentStreams()
	if !bytes.Contains(data, []byte("re")) || !bytes.Contains(data, []byte("Do")) {
		t.Errorf("graphics lost: %s", data)
	}
}

// TestRemoveAllTextPageKeepsSharedForm: removing the text of one page clones
// the form it shares with another page, which keeps its text; the form is
// cloned once however often the page draws it.
func TestRemoveAllTextPageKeepsSharedForm(t *testing.T) {
	d := removeTextDoc(t)
	p1, _ := d.Page(1)
	if err := p1.appendToContentStream([]byte("q 1 0 0 1 50 340 cm /Fm Do Q")); err != nil {
		t.Fatal(err)
	}
	before := formCount(d)
	if err := p1.RemoveAllText(); err != nil {
		t.Fatal(err)
	}
	if s := pageText(t, p1); s != "" {
		t.Errorf("page 1 still has text %q", s)
	}
	p2, _ := d.Page(2)
	if s := pageText(t, p2); s != "FormSecret" {
		t.Errorf("page 2 text = %q, want FormSecret", s)
	}
	// One clone for both placements; the original stays, page 2 draws it.
	if added := formCount(d) - before; added != 1 {
		t.Errorf("%d forms added, want 1", added)
	}
}

// TestRemoveAllTextNoText: a page with no text is left byte for byte.
func TestRemoveAllTextNoText(t *testing.T) {
	d := NewDocument(200, 200)
	p, _ := d.Page(1)
	if err := p.DrawRectangle(Rectangle{LLX: 10, LLY: 10, URX: 50, URY: 50}, ShapeStyle{FillColor: &Color{A: 1}}); err != nil {
		t.Fatal(err)
	}
	before, _ := p.contentStreams()
	contents := p.pageDict()["/Contents"]
	if err := d.RemoveAllText(); err != nil {
		t.Fatal(err)
	}
	after, _ := p.contentStreams()
	if !bytes.Equal(before, after) {
		t.Error("content of a page without text changed")
	}
	if got := p.pageDict()["/Contents"]; !equalValue(got, contents) {
		t.Error("/Contents replaced on a page without text")
	}
}

func equalValue(a, b pdfValue) bool { return fmt.Sprint(a) == fmt.Sprint(b) }

// formCount counts the Form XObjects in the document.
func formCount(d *Document) int {
	n := 0
	for _, o := range d.objects {
		if st, ok := o.Value.(*pdfStream); ok && st.Dict["/Subtype"] == pdfName("/Form") {
			n++
		}
	}
	return n
}

// TestRemoveAllTextInheritingForm: a form with no /Resources of its own,
// drawing through the page's resources a form that shows text, has the text
// removed too — the page's resources are redirected to both clones.
func TestRemoveAllTextInheritingForm(t *testing.T) {
	d := NewDocument(400, 400)
	font := d.addObject(pdfDict{"/Type": pdfName("/Font"), "/Subtype": pdfName("/Type1"), "/BaseFont": pdfName("/Helvetica")})
	innerData := []byte("BT /F1 12 Tf 10 10 Td (InnerSecret) Tj ET")
	inner := d.addObject(&pdfStream{
		Dict: pdfDict{"/Subtype": pdfName("/Form"), "/BBox": pdfArray{0, 0, 200, 50},
			"/Resources": pdfDict{"/Font": pdfDict{"/F1": pdfRef{Num: font}}}},
		Data: innerData, Decoded: true,
	})
	outerData := []byte("/In Do")
	outer := d.addObject(&pdfStream{Dict: pdfDict{"/Subtype": pdfName("/Form"), "/BBox": pdfArray{0, 0, 200, 50}}, Data: outerData, Decoded: true})
	p, _ := d.Page(1)
	p.pageDict()["/Resources"] = pdfDict{"/XObject": pdfDict{"/Out": pdfRef{Num: outer}, "/In": pdfRef{Num: inner}}}
	if err := p.appendToContentStream([]byte("q 1 0 0 1 50 300 cm /Out Do Q")); err != nil {
		t.Fatal(err)
	}
	if s := pageText(t, p); s != "InnerSecret" {
		t.Fatalf("before: %q", s)
	}
	if err := p.RemoveAllText(); err != nil {
		t.Fatal(err)
	}
	if s := pageText(t, p); s != "" {
		t.Errorf("text left: %q", s)
	}
	if found, _ := documentContainsBytes(d, "InnerSecret"); found {
		t.Error("InnerSecret still in the document")
	}
}

// TestRemoveAllTextSelfDrawingForm: a form with no /Resources of its own
// that draws itself through the page's resources — a cycle — is walked once.
func TestRemoveAllTextSelfDrawingForm(t *testing.T) {
	d := NewDocument(400, 400)
	font := d.addObject(pdfDict{"/Type": pdfName("/Font"), "/Subtype": pdfName("/Type1"), "/BaseFont": pdfName("/Helvetica")})
	selfData := []byte("BT /F1 12 Tf 10 10 Td (LoopSecret) Tj ET /Self Do")
	self := d.addObject(&pdfStream{Dict: pdfDict{"/Subtype": pdfName("/Form"), "/BBox": pdfArray{0, 0, 200, 50}}, Data: selfData, Decoded: true})
	p, _ := d.Page(1)
	p.pageDict()["/Resources"] = pdfDict{"/Font": pdfDict{"/F1": pdfRef{Num: font}}, "/XObject": pdfDict{"/Self": pdfRef{Num: self}}}
	if err := p.appendToContentStream([]byte("/Self Do")); err != nil {
		t.Fatal(err)
	}
	if err := p.RemoveAllText(); err != nil {
		t.Fatal(err)
	}
	if found, _ := documentContainsBytes(d, "LoopSecret"); found {
		t.Error("LoopSecret still in the document")
	}
}

// TestRemoveAllTextInheritingFormPerResources: one form with no /Resources of
// its own, drawn on two pages whose resources name different inner forms
// under the same name, is stripped per page — each page keeps its own inner
// form's graphics.
func TestRemoveAllTextInheritingFormPerResources(t *testing.T) {
	d := NewDocument(400, 400)
	if err := d.AddBlankPage(400, 400); err != nil {
		t.Fatal(err)
	}
	font := d.addObject(pdfDict{"/Type": pdfName("/Font"), "/Subtype": pdfName("/Type1"), "/BaseFont": pdfName("/Helvetica")})
	inner := func(rect string) int {
		data := []byte(rect + " re f BT /F1 12 Tf 10 10 Td (Inner) Tj ET")
		return d.addObject(&pdfStream{Dict: pdfDict{"/Subtype": pdfName("/Form"), "/BBox": pdfArray{0, 0, 200, 50},
			"/Resources": pdfDict{"/Font": pdfDict{"/F1": pdfRef{Num: font}}}}, Data: data, Decoded: true})
	}
	outer := d.addObject(&pdfStream{Dict: pdfDict{"/Subtype": pdfName("/Form"), "/BBox": pdfArray{0, 0, 200, 50}}, Data: []byte("/In Do"), Decoded: true})
	marks := []string{"1 1 11 11", "2 2 22 22"}
	for i, p := range d.Pages() {
		p.pageDict()["/Resources"] = pdfDict{"/XObject": pdfDict{"/Out": pdfRef{Num: outer}, "/In": pdfRef{Num: inner(marks[i])}}}
		if err := p.appendToContentStream([]byte("/Out Do")); err != nil {
			t.Fatal(err)
		}
	}
	if err := d.RemoveAllText(); err != nil {
		t.Fatal(err)
	}
	for i, p := range d.Pages() {
		xo, _ := resolveRefToDict(d.objects, p.pageResources()["/XObject"])
		st, _ := resolveRef(d.objects, xo["/In"]).(*pdfStream)
		if st == nil || !bytes.Contains(st.Data, []byte(marks[i])) || bytes.Contains(st.Data, []byte("Inner")) {
			t.Errorf("page %d /In = %v; want its own inner form, text-free", i+1, st)
		}
	}
}
