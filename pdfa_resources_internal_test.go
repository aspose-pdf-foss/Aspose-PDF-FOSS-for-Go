// SPDX-License-Identifier: MIT

package asposepdf

import (
	"strings"
	"testing"
)

func resForm(content string, res pdfDict) *pdfStream {
	dict := pdfDict{"/Type": pdfName("/XObject"), "/Subtype": pdfName("/Form"), "/BBox": pdfArray{0, 0, 200, 200}}
	if res != nil {
		dict["/Resources"] = res
	}
	return &pdfStream{Dict: dict, Data: []byte(content), Decoded: true}
}

func TestContentResourceUses(t *testing.T) {
	ops, err := parseContentStream([]byte(`/X0 Do /G0 gs /F0 12 Tf /S0 sh /C0 cs /C1 CS /DeviceRGB cs /Pattern cs /CMYK CS 0 0 0 /P0 scn /P1 SCN 1 0 0 scn /Span /M0 BDC /Span <</MCID 0>> BDC /Mark /M1 DP BI /CS /C2 /W 1 /H 1 /BPC 8 ID x EI`))
	if err != nil {
		t.Fatal(err)
	}
	want := map[resourceUse]bool{
		{"/XObject", "/X0"}: true, {"/ExtGState", "/G0"}: true, {"/Font", "/F0"}: true, {"/Shading", "/S0"}: true,
		{"/ColorSpace", "/C0"}: true, {"/ColorSpace", "/C1"}: true, {"/Pattern", "/P0"}: true, {"/Pattern", "/P1"}: true,
		{"/Properties", "/M0"}: true, {"/Properties", "/M1"}: true, {"/ColorSpace", "/C2"}: true,
	}
	got := map[resourceUse]bool{}
	for _, u := range contentResourceUses(ops) {
		got[u] = true
		if !want[u] {
			t.Errorf("unexpected %v", u)
		}
	}
	for u := range want {
		if !got[u] {
			t.Errorf("missing %v", u)
		}
	}
}

func TestInheritedResourcesRepaired(t *testing.T) {
	const rule = "INHERITED_RESOURCES"
	inner := resForm("0 g 0 0 10 10 re f /F0 12 Tf", nil)
	form := resForm("/Inner Do 1 0 0 rg 10 10 50 50 re f BT /F0 12 Tf 20 20 Td (hi) Tj ET", nil)
	d := NewDocument(200, 200)
	p, _ := d.Page(1)
	font := pdfDict{"/Type": pdfName("/Font"), "/Subtype": pdfName("/Type1"), "/BaseFont": pdfName("/Helvetica")}
	fontRef := pdfRef{Num: d.addObject(font)}
	formRef := pdfRef{Num: d.addObject(form)}
	innerRef := pdfRef{Num: d.addObject(inner)}
	p.pageDict()["/Resources"] = pdfDict{"/Font": pdfDict{"/F0": fontRef}, "/XObject": pdfDict{"/F": formRef, "/Inner": innerRef}, "/ExtGState": pdfDict{"/Unused": pdfDict{}}}
	p.pageDict()["/Contents"] = pdfRef{Num: d.addObject(&pdfStream{Dict: pdfDict{}, Data: []byte("/F Do\n"), Decoded: true})}

	for format, want := range map[PDFAFormat]bool{PDFA2B: true, PDFA3B: true, PDFA2U: true, PDFA1B: false} {
		if got := hasPDFARule(d.ValidatePDFA(format), rule); got != want {
			t.Errorf("%v: reported %v", format, got)
		}
	}
	if !hasPDFARule(d.ValidatePDFA(PDFA2B), rule) {
		t.Fatal("not reported")
	}
	if _, ok := d.formsInheritingResources()[inner]; !ok {
		t.Error("a nested form takes names through its caller as well")
	}
	before := cmykRender(t, p)
	if hasPDFARule(convertA(t, d, PDFA2B), rule) {
		t.Error("not repaired")
	}
	res, ok := form.Dict["/Resources"].(pdfDict)
	if !ok {
		t.Fatalf("no resources: %v", form.Dict)
	}
	if fonts, _ := res["/Font"].(pdfDict); fonts["/F0"] != fontRef {
		t.Errorf("fonts %v", res["/Font"])
	}
	if xo, _ := res["/XObject"].(pdfDict); xo["/Inner"] != innerRef {
		t.Errorf("xobjects %v", res["/XObject"])
	}
	if _, unused := res["/ExtGState"]; unused {
		t.Error("a name it does not use was copied")
	}
	if inner.Dict["/Resources"] == nil {
		t.Error("the nested form was not repaired")
	}
	if diff := cmykMaxDiff(before, cmykRender(t, p)); diff > 60 {
		t.Errorf("the page changed by %d", diff)
	}
	// The font reached through the form gets embedded with the rest.
	if hasPDFARule(d.ValidatePDFA(PDFA2B), "FONT_NOT_EMBEDDED") {
		t.Error("the form's font stayed unembedded")
	}
	if len(d.formsInheritingResources()) != 0 {
		t.Error("something still inherits")
	}
}

func TestInheritedResourcesLeftWhenUnsafe(t *testing.T) {
	const rule = "INHERITED_RESOURCES"
	// Callers giving one name different values: a copy per caller would be needed.
	form := resForm("/G0 gs", nil)
	d := NewDocument(200, 200)
	p, _ := d.Page(1)
	formRef := pdfRef{Num: d.addObject(form)}
	draw := func(p *Page, ca float64) {
		p.pageDict()["/Resources"] = pdfDict{"/XObject": pdfDict{"/F": formRef}, "/ExtGState": pdfDict{"/G0": pdfDict{"/LW": ca}}}
		p.pageDict()["/Contents"] = pdfRef{Num: d.addObject(&pdfStream{Dict: pdfDict{}, Data: []byte("/F Do\n"), Decoded: true})}
	}
	draw(p, 1)
	if err := d.AddBlankPage(200, 200); err != nil {
		t.Fatal(err)
	}
	q, _ := d.Page(2)
	draw(q, 2)
	if !hasPDFARule(convertA(t, d, PDFA2B), rule) {
		t.Error("conflicting callers not reported")
	}
	if form.Dict["/Resources"] != nil {
		t.Error("conflicting callers repaired")
	}

	// Equal values from two callers are no conflict.
	form = resForm("/G0 gs", nil)
	d = NewDocument(200, 200)
	p, _ = d.Page(1)
	formRef = pdfRef{Num: d.addObject(form)}
	draw(p, 1)
	if err := d.AddBlankPage(200, 200); err != nil {
		t.Fatal(err)
	}
	q, _ = d.Page(2)
	draw(q, 1)
	if hasPDFARule(convertA(t, d, PDFA2B), rule) || form.Dict["/Resources"] == nil {
		t.Errorf("equal callers not repaired: %v", form.Dict)
	}

	// Resources of its own that lack a name: adding it could draw more than it does.
	own := pdfDict{"/ExtGState": pdfDict{"/Other": pdfDict{}}}
	form = resForm("/G0 gs /Other gs", own)
	d = NewDocument(200, 200)
	p, _ = d.Page(1)
	formRef = pdfRef{Num: d.addObject(form)}
	draw(p, 1)
	if !hasPDFARule(convertA(t, d, PDFA2B), rule) {
		t.Error("partial resources not reported")
	}
	if e := own["/ExtGState"].(pdfDict); e["/G0"] != nil {
		t.Error("partial resources extended")
	}

	// A name nothing provides, or that its own resources hold, is not inherited.
	d = NewDocument(200, 200)
	p, _ = d.Page(1)
	formRef = pdfRef{Num: d.addObject(resForm("/Nowhere gs", nil))}
	p.pageDict()["/Resources"] = pdfDict{"/XObject": pdfDict{"/F": formRef}}
	p.pageDict()["/Contents"] = pdfRef{Num: d.addObject(&pdfStream{Dict: pdfDict{}, Data: []byte("/F Do\n"), Decoded: true})}
	if hasPDFARule(d.ValidatePDFA(PDFA2B), rule) {
		t.Error("a name nothing provides reported")
	}
	// A form reached through a soft mask, and one drawn by itself endlessly.
	d = NewDocument(200, 200)
	p, _ = d.Page(1)
	loop := resForm("/Self Do /G0 gs", nil)
	loopRef := pdfRef{Num: d.addObject(loop)}
	p.pageDict()["/Resources"] = pdfDict{
		"/XObject":   pdfDict{"/Self": loopRef},
		"/ExtGState": pdfDict{"/G0": pdfDict{"/SMask": pdfDict{"/S": pdfName("/Luminosity"), "/G": loopRef}}},
	}
	p.pageDict()["/Contents"] = pdfRef{Num: d.addObject(&pdfStream{Dict: pdfDict{}, Data: []byte("/G0 gs\n"), Decoded: true})}
	if !hasPDFARule(d.ValidatePDFA(PDFA2B), rule) {
		t.Error("a soft-mask form not followed")
	}
}

func TestPDFAEmptyAppearance(t *testing.T) {
	build := func(ap pdfDict, subtype string) (*Document, pdfDict) {
		d, p := conformDoc(t)
		ad := pdfDict{"/Type": pdfName("/Annot"), "/Subtype": pdfName(subtype), "/Rect": pdfArray{10, 10, 60, 40}, "/F": 4, "/AP": ap}
		p.pageDict()["/Annots"] = pdfArray{pdfRef{Num: d.addObject(ad)}}
		return d, ad
	}
	for _, ap := range []pdfDict{{}, {"/D": pdfRef{Num: 1}}, {"/N": pdfRef{Num: 99999}}} {
		d, _ := build(ap, "/Link")
		if !hasPDFARule(d.ValidatePDFA(PDFA2B), "ANNOTATION_AP_ENTRIES") {
			t.Errorf("%v not reported", ap)
		}
		d, ad := build(ap, "/Link")
		if hasPDFARule(convertA(t, d, PDFA2B), "ANNOTATION_AP_ENTRIES") {
			t.Errorf("%v not repaired", ap)
		}
		if ad["/AP"] != nil {
			t.Errorf("%v stayed", ad["/AP"])
		}
	}
	// A form field's empty appearance is no appearance.
	d, _ := build(pdfDict{}, "/Widget")
	if !hasPDFARule(d.ValidatePDFA(PDFA2B), "ANNOTATION_NO_AP") {
		t.Error("a widget with an empty appearance has none")
	}
	// A real one stays.
	d, ad := build(pdfDict{"/N": pdfRef{Num: 1}}, "/Square")
	d.dropAppearancesWithoutN()
	if ad["/AP"] == nil {
		t.Error("a real appearance was dropped")
	}
}

func TestPDFAAnnotationTypeList(t *testing.T) {
	build := func(subtype string) *Document {
		d, p := conformDoc(t)
		p.pageDict()["/Annots"] = pdfArray{pdfRef{Num: d.addObject(pdfDict{"/Subtype": pdfName(subtype), "/Rect": pdfArray{0, 0, 0, 0}, "/F": 4})}}
		return d
	}
	for sub, want := range map[string]bool{
		"/WJAnnot": true, "/3D": true, "/Sound": true, "/Screen": true, "/Movie": true,
		"/Caret": false, "/Polygon": false, "/PolyLine": false, "/FileAttachment": false, "/Watermark": false, "/Redact": false, "/Link": false,
	} {
		if got := hasPDFARule(build(sub).ValidatePDFA(PDFA2B), "ANNOTATION_TYPE_FORBIDDEN"); got != want {
			t.Errorf("%s under PDF/A-2: reported %v", sub, got)
		}
	}
	if !hasPDFARule(build("/Caret").ValidatePDFA(PDFA1B), "ANNOTATION_TYPE_FORBIDDEN") {
		t.Error("PDF/A-1 has the shorter list")
	}
}

func TestPDFAInvalidDates(t *testing.T) {
	for date, want := range map[string]bool{
		"": true, "2013": true, "2013-05": true, "2013-05-10": true, "2013-05-10T13:31": true,
		"2013-05-10T13:31:00": true, "2013-05-10T13:31:00.5Z": true, "2013-05-10T13:31:00+02:00": true,
		"5/10/2013 13:31:00": false, "2013-5-10": false, "D:20130510": false, "2013-05-10 13:31": false,
	} {
		if got := reXMPDate.MatchString(date); got != want {
			t.Errorf("%q: %v", date, got)
		}
	}
	// In /Info it goes from both stores.
	d, _ := conformDoc(t)
	d.SetInfo(DocumentInfo{Title: "t", CreationDate: "5/10/2013 13:31:00", ModDate: "D:20130510133100Z"})
	rep := convertA(t, d, PDFA2B)
	if hasPDFARule(rep, "XMP_DATE_INVALID") || hasPDFARule(rep, "INFO_XMP_MISMATCH") {
		t.Errorf("%+v", rep.Issues)
	}
	info, err := d.Info()
	if err != nil {
		t.Fatal(err)
	}
	x, err := d.XMP()
	if err != nil {
		t.Fatal(err)
	}
	if info.CreationDate != "" || x.CreateDate != "" {
		t.Errorf("kept: %q %q", info.CreationDate, x.CreateDate)
	}
	if info.ModDate == "" || x.ModifyDate != "2013-05-10T13:31:00Z" {
		t.Errorf("a good date lost: %q %q", info.ModDate, x.ModifyDate)
	}

	// A good XMP date survives a bad /Info one; a bad XMP date is reported and goes.
	d, _ = conformDoc(t)
	if err := d.SetXMP(XMPMetadata{CreateDate: "2020-01-02", ModifyDate: "yesterday", MetadataDate: "soon"}); err != nil {
		t.Fatal(err)
	}
	d.SetInfo(DocumentInfo{CreationDate: "5/10/2013"})
	if !hasPDFARule(d.ValidatePDFA(PDFA2B), "XMP_DATE_INVALID") {
		t.Error("not reported")
	}
	if rep := convertA(t, d, PDFA2B); hasPDFARule(rep, "XMP_DATE_INVALID") {
		t.Errorf("%+v", rep.Issues)
	}
	if x, _ := d.XMP(); x.CreateDate != "2020-01-02" || x.ModifyDate != "" || x.MetadataDate == "soon" {
		t.Errorf("%+v", x)
	}
}

// A structure has no form as one text value, whichever way it is written.
func TestXMPStructuredPropertiesLeftOut(t *testing.T) {
	const head = `<x:xmpmeta xmlns:x="adobe:ns:meta/"><rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#"><rdf:Description rdf:about="" xmlns:xmpMM="http://ns.adobe.com/xap/1.0/mm/" xmlns:stRef="http://ns.adobe.com/xap/1.0/sType/ResourceRef#" xmlns:dc="http://purl.org/dc/elements/1.1/">`
	const tail = `</rdf:Description></rdf:RDF></x:xmpmeta>`
	for name, body := range map[string]string{
		"parseType":   `<xmpMM:DerivedFrom rdf:parseType="Resource"><stRef:instanceID>uuid:a</stRef:instanceID><stRef:documentID>uuid:b</stRef:documentID></xmpMM:DerivedFrom>`,
		"description": `<xmpMM:DerivedFrom><rdf:Description><stRef:instanceID>uuid:a</stRef:instanceID></rdf:Description></xmpMM:DerivedFrom>`,
		"in a list":   `<xmpMM:History><rdf:Seq><rdf:li rdf:parseType="Resource"><stRef:instanceID>uuid:a</stRef:instanceID></rdf:li></rdf:Seq></xmpMM:History>`,
	} {
		d, _ := conformDoc(t)
		if err := d.SetXMPRaw([]byte(head + body + `<xmpMM:DocumentID>uuid:keep</xmpMM:DocumentID><dc:title><rdf:Alt><rdf:li xml:lang="x-default">T</rdf:li></rdf:Alt></dc:title>` + tail)); err != nil {
			t.Fatal(err)
		}
		m, err := d.XMP()
		if err != nil {
			t.Fatal(err)
		}
		if m.Title != "T" {
			t.Errorf("%s: title %q", name, m.Title)
		}
		found := false
		for _, p := range m.Custom {
			if strings.Contains(p.Value, "uuid:a") {
				t.Errorf("%s: flattened into %+v", name, p)
			}
			found = found || p.Name == "DocumentID" && p.Value == "uuid:keep"
		}
		if !found {
			t.Errorf("%s: a plain property after it was lost: %+v", name, m.Custom)
		}
	}
}

// A form's own resources are what the forms it draws take names from.
func TestInheritedResourcesThroughOwn(t *testing.T) {
	d := NewDocument(200, 200)
	p, _ := d.Page(1)
	inner := resForm("/G0 gs", nil)
	innerRef := pdfRef{Num: d.addObject(inner)}
	outerRes := pdfDict{"/XObject": pdfDict{"/I": innerRef}, "/ExtGState": pdfDict{"/G0": pdfDict{"/LW": 1}}}
	outerRef := pdfRef{Num: d.addObject(resForm("/I Do", outerRes))}
	p.pageDict()["/Resources"] = pdfDict{"/XObject": pdfDict{"/O": outerRef}, "/ExtGState": pdfDict{"/G0": pdfDict{"/LW": 2}}}
	p.pageDict()["/Contents"] = pdfRef{Num: d.addObject(&pdfStream{Dict: pdfDict{}, Data: []byte("/O Do\n"), Decoded: true})}
	d.fixInheritedResources()
	res, ok := inner.Dict["/Resources"].(pdfDict)
	if !ok {
		t.Fatalf("not repaired: %v", inner.Dict)
	}
	g := res["/ExtGState"].(pdfDict)["/G0"].(pdfDict)
	if g["/LW"] != 1 {
		t.Errorf("took the page's value, not its caller's: %v", g)
	}
}
