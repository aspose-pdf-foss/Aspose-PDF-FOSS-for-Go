// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"strings"
	"testing"
)

func pdfa4Doc(t *testing.T) *Document {
	t.Helper()
	d := NewDocument(300, 200)
	d.catalog = pdfDict{}
	if err := d.Pages()[0].AddText("Hello", TextStyle{Size: 12}, Rectangle{LLX: 20, LLY: 100, URX: 280, URY: 180}); err != nil {
		t.Fatal(err)
	}
	return d
}

func pdfa4Attach(t *testing.T, d *Document) {
	t.Helper()
	if _, err := d.EmbeddedFiles().AddFromStream("note.txt", strings.NewReader("attached")); err != nil {
		t.Fatal(err)
	}
}

func saved(t *testing.T, d *Document) []byte {
	t.Helper()
	var buf bytes.Buffer
	if _, err := d.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestPDFA4FormatNames(t *testing.T) {
	for f, want := range map[PDFAFormat]string{PDFA4: "PDF/A-4", PDFA4E: "PDF/A-4E", PDFA4F: "PDF/A-4F"} {
		if f.String() != want || f.part() != 4 {
			t.Errorf("%v: part %d, name %q", want, f.part(), f.String())
		}
	}
}

func TestConvertToPDFA4WritesPDF20AndNoInfo(t *testing.T) {
	d := pdfa4Doc(t)
	d.SetInfo(DocumentInfo{Title: "The title", Author: "An author", Keywords: "k1", ModDate: "D:20260102030405Z", Custom: map[string]string{"Dept Code": "42"}})
	d.catalog["/Version"] = pdfName("/1.7")
	rep, err := d.ConvertToPDFA(PDFA4)
	if err != nil {
		t.Fatal(err)
	}
	if !rep.Conformant {
		t.Fatalf("not conformant: %+v", rep.Issues)
	}
	data := saved(t, d)
	if !bytes.HasPrefix(data, []byte("%PDF-2.0\n")) {
		t.Errorf("header %q", data[:9])
	}
	if bytes.Contains(data[bytes.LastIndex(data, []byte("trailer")):], []byte("/Info")) {
		t.Error("the trailer still names /Info")
	}
	if bytes.Contains(data, []byte("/Version")) {
		t.Error("the catalog still has a PDF 1.x /Version")
	}
	reopened, err := OpenStream(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	meta, err := reopened.XMP()
	if err != nil {
		t.Fatal(err)
	}
	if meta.Title != "The title" || len(meta.Authors) != 1 || meta.Authors[0] != "An author" {
		t.Errorf("metadata not kept in XMP: %+v", meta)
	}
	raw, _ := reopened.XMPRaw()
	for _, want := range []string{`Dept_Code`, `2020`} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("XMP lacks %q", want)
		}
	}
	if strings.Contains(string(raw), "pdfaid:conformance") {
		t.Error("plain PDF/A-4 must not state a conformance level")
	}
	if rep := reopened.ValidatePDFA(PDFA4); !rep.Conformant {
		t.Errorf("reopened not conformant: %+v", rep.Issues)
	}
	// Saved again, without any conversion, it is still a PDF 2.0 file.
	if again := saved(t, reopened); !bytes.HasPrefix(again, []byte("%PDF-2.0\n")) {
		t.Errorf("header after a second save %q", again[:9])
	}
}

func TestConvertToPDFA4KeepsModDateWithPieceInfo(t *testing.T) {
	d := pdfa4Doc(t)
	d.SetInfo(DocumentInfo{Title: "T", ModDate: "D:20260102030405Z"})
	d.catalog["/PieceInfo"] = pdfDict{}
	if rep, err := d.ConvertToPDFA(PDFA4); err != nil || !rep.Conformant {
		t.Fatalf("%v %+v", err, rep)
	}
	if len(d.info) != 1 || d.info["/ModDate"] == nil {
		t.Errorf("info %v", d.info)
	}
	d.info["/Title"] = "again"
	if !pdfaHasRule(d.ValidatePDFA(PDFA4), "INFO_FORBIDDEN") {
		t.Error("/Info with more than /ModDate not reported")
	}
}

func TestValidatePDFA4Identification(t *testing.T) {
	d := pdfa4Doc(t)
	if _, err := d.ConvertToPDFA(PDFA4F); err != nil {
		t.Fatal(err)
	}
	if !pdfaHasRule(d.ValidatePDFA(PDFA4), "XMP_CONFORMANCE_MISMATCH") {
		t.Error("a 4f identifier passed as plain 4")
	}
	raw, _ := d.XMPRaw()
	if err := d.SetXMPRaw(bytes.ReplaceAll(raw, []byte("2020"), []byte("2019"))); err != nil {
		t.Fatal(err)
	}
	if !pdfaHasRule(d.ValidatePDFA(PDFA4F), "XMP_REV_MISMATCH") {
		t.Error("a wrong pdfaid:rev not reported")
	}
}

func TestValidatePDFA4InfoAndVersion(t *testing.T) {
	d := pdfa4Doc(t)
	if _, err := d.ConvertToPDFA(PDFA4); err != nil {
		t.Fatal(err)
	}
	d.SetInfo(DocumentInfo{Title: "back"})
	if !pdfaHasRule(d.ValidatePDFA(PDFA4), "INFO_FORBIDDEN") {
		t.Error("/Info not reported")
	}
	d.ClearInfo()
	d.catalog["/Version"] = pdfName("/1.7")
	if !pdfaHasRule(d.ValidatePDFA(PDFA4), "CATALOG_VERSION") {
		t.Error("catalog /Version 1.7 not reported")
	}
	d.catalog["/Version"] = pdfName("/2.0")
	if pdfaHasRule(d.ValidatePDFA(PDFA4), "CATALOG_VERSION") {
		t.Error("catalog /Version 2.0 reported")
	}
}

func TestPDFA4KeepsJavaScriptAndRemovesWhatItForbids(t *testing.T) {
	js := pdfDict{"/S": pdfName("/JavaScript"), "/JS": "app.alert(1)"}
	launch := pdfDict{"/S": pdfName("/Launch")}
	build := func() *Document {
		d := pdfa4Doc(t)
		d.catalog["/OpenAction"] = js
		d.catalog["/Names"] = pdfDict{"/JavaScript": pdfDict{"/Names": pdfArray{}}}
		d.catalog["/AA"] = pdfDict{"/WC": js}
		page := d.pages[0].Value.(pdfDict)
		page["/AA"] = pdfDict{"/O": js, "/E": js, "/X": launch}
		return d
	}
	d := build()
	rep, err := d.ConvertToPDFA(PDFA4)
	if err != nil {
		t.Fatal(err)
	}
	if !rep.Conformant {
		t.Fatalf("not conformant: %+v", rep.Issues)
	}
	if d.catalog["/OpenAction"] == nil {
		t.Error("PDF/A-4 allows JavaScript; the open action went")
	}
	if d.catalog["/AA"] != nil {
		t.Error("a catalog trigger PDF/A-4 forbids stayed")
	}
	if names, _ := d.catalog["/Names"].(pdfDict); names["/JavaScript"] == nil {
		t.Error("PDF/A-4 allows JavaScript; the document's scripts went")
	}
	aa, _ := d.pages[0].Value.(pdfDict)["/AA"].(pdfDict)
	if len(aa) != 1 || aa["/E"] == nil {
		t.Errorf("page /AA: %v", aa)
	}

	d = build()
	if _, err := d.ConvertToPDFA(PDFA2B); err != nil {
		t.Fatal(err)
	}
	if d.catalog["/OpenAction"] != nil || d.pages[0].Value.(pdfDict)["/AA"] != nil {
		t.Error("PDF/A-2 must still lose JavaScript and every /AA")
	}
}

func TestValidatePDFA4Actions(t *testing.T) {
	d := pdfa4Doc(t)
	if _, err := d.ConvertToPDFA(PDFA4); err != nil {
		t.Fatal(err)
	}
	page := d.pages[0].Value.(pdfDict)
	page["/AA"] = pdfDict{"/O": pdfDict{"/S": pdfName("/GoTo")}}
	if !pdfaHasRule(d.ValidatePDFA(PDFA4), "ADDITIONAL_ACTIONS") {
		t.Error("trigger /O not reported")
	}
	page["/AA"] = pdfDict{"/E": pdfDict{"/S": pdfName("/Launch")}}
	if !pdfaHasRule(d.ValidatePDFA(PDFA4), "FORBIDDEN_ACTION") {
		t.Error("a forbidden additional action not reported")
	}
	delete(page, "/AA")
	d.catalog["/OpenAction"] = pdfDict{"/S": pdfName("/SetOCGState")}
	if !pdfaHasRule(d.ValidatePDFA(PDFA4), "FORBIDDEN_ACTION") {
		t.Error("SetOCGState not reported for plain 4")
	}
	d2 := pdfa4Doc(t)
	if _, err := d2.ConvertToPDFA(PDFA4E); err != nil {
		t.Fatal(err)
	}
	d2.catalog["/OpenAction"] = pdfDict{"/S": pdfName("/SetOCGState")}
	if pdfaHasRule(d2.ValidatePDFA(PDFA4E), "FORBIDDEN_ACTION") {
		t.Error("PDF/A-4e allows SetOCGState")
	}
}

func TestPDFA4Attachments(t *testing.T) {
	d := pdfa4Doc(t)
	if rep, _ := d.ConvertToPDFA(PDFA4F); !pdfaHasRule(rep, "EMBEDDED_FILES_REQUIRED") {
		t.Error("PDF/A-4f with no attachment accepted")
	}

	d = pdfa4Doc(t)
	pdfa4Attach(t, d)
	rep, err := d.ConvertToPDFA(PDFA4F)
	if err != nil {
		t.Fatal(err)
	}
	if !rep.Conformant {
		t.Fatalf("4f not conformant: %+v", rep.Issues)
	}

	d = pdfa4Doc(t)
	pdfa4Attach(t, d)
	if rep, _ := d.ConvertToPDFA(PDFA4); !pdfaHasRule(rep, "EMBEDDED_FILE_NOT_PDFA") {
		t.Error("a plain text attachment accepted in plain PDF/A-4")
	}

	d = pdfa4Doc(t)
	pdfa4Attach(t, d)
	if rep, _ := d.ConvertToPDFA(PDFA4E); pdfaHasRule(rep, "EMBEDDED_FILE_NOT_PDFA") {
		t.Error("PDF/A-4e has no rule on what an attachment is")
	}

	// An attachment that is itself PDF/A-4 is fine in plain PDF/A-4.
	inner := pdfa4Doc(t)
	if _, err := inner.ConvertToPDFA(PDFA4); err != nil {
		t.Fatal(err)
	}
	d = pdfa4Doc(t)
	if _, err := d.EmbeddedFiles().AddFromStream("inner.pdf", bytes.NewReader(saved(t, inner))); err != nil {
		t.Fatal(err)
	}
	if rep, _ := d.ConvertToPDFA(PDFA4); !rep.Conformant {
		t.Errorf("attached PDF/A-4 refused: %+v", rep.Issues)
	}

	// Without the relationship and the MIME type it is reported.
	d = pdfa4Doc(t)
	pdfa4Attach(t, d)
	if _, err := d.ConvertToPDFA(PDFA4F); err != nil {
		t.Fatal(err)
	}
	f := d.EmbeddedFiles().All()[0]
	delete(d.objects[f.ref.Num].Value.(pdfDict), "/AFRelationship")
	delete(f.stream().Dict, "/Subtype")
	rep = d.ValidatePDFA(PDFA4F)
	if !pdfaHasRule(rep, "EMBEDDED_FILE_NOT_ASSOCIATED") || !pdfaHasRule(rep, "EMBEDDED_FILE_NO_MIME") {
		t.Errorf("missing relationship or MIME not reported: %+v", rep.Issues)
	}
}

func TestPDFA4AnnotationTypes(t *testing.T) {
	for _, c := range []struct {
		format PDFAFormat
		sub    string
		ok     bool
	}{
		{PDFA4, "/FileAttachment", false},
		{PDFA4E, "/FileAttachment", false},
		{PDFA4F, "/FileAttachment", true},
		{PDFA4, "/3D", false},
		{PDFA4F, "/RichMedia", false},
		{PDFA4E, "/3D", true},
		{PDFA4E, "/RichMedia", true},
		{PDFA4, "/Projection", true},
		{PDFA2B, "/Projection", false},
		{PDFA3B, "/FileAttachment", true},
		{PDFA4, "/Text", true},
		{PDFA4, "/Sound", false},
	} {
		if got := pdfaAnnotAllowed(c.format, c.sub); got != c.ok {
			t.Errorf("%v %s: %v", c.format, c.sub, got)
		}
	}
}

func TestValidatePDFA4WhatItNoLongerAsks(t *testing.T) {
	d := pdfa4Doc(t)
	if _, err := d.ConvertToPDFA(PDFA4); err != nil {
		t.Fatal(err)
	}
	// An integer outside 32 bits, an undeclared XMP property, a broken XMP date and
	// layers with /AS are all fine here and not in PDF/A-2.
	d.addObject(pdfDict{"/Big": int64(1) << 40})
	meta, _ := d.XMP()
	meta.Custom = append(meta.Custom, XMPProperty{Namespace: "http://example.com/x/", Prefix: "x1", Name: "Thing", Value: "v"})
	meta.CreateDate = "yesterday"
	if err := d.SetXMP(meta); err != nil {
		t.Fatal(err)
	}
	if rep := d.ValidatePDFA(PDFA4); !rep.Conformant {
		t.Errorf("PDF/A-4 refused what it allows: %+v", rep.Issues)
	}
}

func TestConvertToPDFA4KeepsUndeclaredXMP(t *testing.T) {
	d := pdfa4Doc(t)
	meta := XMPMetadata{Title: "T", Custom: []XMPProperty{{Namespace: "http://example.com/x/", Prefix: "x1", Name: "Thing", Value: "kept"}}}
	if err := d.SetXMP(meta); err != nil {
		t.Fatal(err)
	}
	if _, err := d.ConvertToPDFA(PDFA4); err != nil {
		t.Fatal(err)
	}
	raw, _ := d.XMPRaw()
	if !strings.Contains(string(raw), "kept") {
		t.Error("PDF/A-4 does not require a schema; the property should stay")
	}
}

func TestValidatePDFA4Content(t *testing.T) {
	check := func(name, rule string, change func(d *Document)) {
		t.Helper()
		d := pdfa4Doc(t)
		if _, err := d.ConvertToPDFA(PDFA4); err != nil {
			t.Fatal(err)
		}
		if pdfaHasRule(d.ValidatePDFA(PDFA4), rule) {
			t.Fatalf("%s: %s before the change", name, rule)
		}
		change(d)
		if !pdfaHasRule(d.ValidatePDFA(PDFA4), rule) {
			t.Errorf("%s: %s not reported", name, rule)
		}
	}
	check("undefined font", "UNDEFINED_RESOURCE", func(d *Document) {
		if err := d.Pages()[0].appendToContentStream([]byte("BT /NoSuch 12 Tf ET\n")); err != nil {
			t.Fatal(err)
		}
	})
	check("HTO", "EXTGSTATE_HTO", func(d *Document) {
		d.catalog["/Extra"] = pdfDict{"/Type": pdfName("/ExtGState"), "/HTO": pdfArray{0, 0}}
	})
	check("structure ActualText", "ACTUALTEXT_PRIVATE_USE", func(d *Document) {
		d.catalog["/Extra"] = pdfDict{"/ActualText": encodeFormString("ab")}
	})
	check("marked-content ActualText", "ACTUALTEXT_PRIVATE_USE", func(d *Document) {
		var utf16 strings.Builder
		for _, b := range []byte{0xFE, 0xFF, 0xE0, 0x00} {
			utf16.WriteString(string([]byte{"0123456789ABCDEF"[b>>4], "0123456789ABCDEF"[b&15]}))
		}
		if err := d.Pages()[0].appendToContentStream([]byte("/Span <</ActualText <" + utf16.String() + ">>> BDC EMC\n")); err != nil {
			t.Fatal(err)
		}
	})
	check("profile reference", "OUTPUT_INTENT_PROFILE_REF", func(d *Document) {
		arr, _ := resolveRefToArray(d.objects, d.catalog["/OutputIntents"])
		oi, _ := resolveRefToDict(d.objects, arr[0])
		oi["/DestOutputProfileRef"] = pdfDict{}
	})
}

func TestConvertToPDFA4RemovesHTO(t *testing.T) {
	d := pdfa4Doc(t)
	d.catalog["/Extra"] = pdfDict{"/Type": pdfName("/ExtGState"), "/HTO": pdfArray{0, 0}}
	if rep, err := d.ConvertToPDFA(PDFA4); err != nil || pdfaHasRule(rep, "EXTGSTATE_HTO") {
		t.Fatalf("%v %+v", err, rep)
	}
}

func TestPDFA4RejectsLZW(t *testing.T) {
	d := pdfa4Doc(t)
	if _, err := d.ConvertToPDFA(PDFA4); err != nil {
		t.Fatal(err)
	}
	d.addObject(&pdfStream{Dict: pdfDict{"/Filter": pdfName("/LZWDecode")}, Data: []byte("x")})
	if !pdfaHasRule(d.ValidatePDFA(PDFA4), "LZW_FILTER") {
		t.Error("LZWDecode not reported for PDF/A-4")
	}
	if pdfaHasRule(d.ValidatePDFA(PDFA2B), "LZW_FILTER") {
		t.Error("LZWDecode reported for PDF/A-2")
	}
}

func TestTTFHasCmap(t *testing.T) {
	font := make([]byte, 0, 64)
	// sfnt header with one table, then the directory entry and the cmap itself.
	font = append(font, 0, 1, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0)
	font = append(font, 'c', 'm', 'a', 'p', 0, 0, 0, 0, 0, 0, 0, 28, 0, 0, 0, 20)
	font = append(font, 0, 0, 0, 2, 0, 3, 0, 1, 0, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0)
	if !ttfHasCmap(font, 3, 1) || !ttfHasCmap(font, 1, 0) {
		t.Error("present records not found")
	}
	if ttfHasCmap(font, 3, 0) {
		t.Error("absent record found")
	}
}

func TestConvertToPDFA4DoesNotRepeatAnXMPProperty(t *testing.T) {
	d := pdfa4Doc(t)
	d.SetInfo(DocumentInfo{Title: "T", Custom: map[string]string{"SourceModified": "D:2020"}})
	meta := XMPMetadata{Title: "T", Custom: []XMPProperty{{Namespace: nsPDFX, Prefix: "pdfx", Name: "SourceModified", Value: "D:2020"}}}
	if err := d.SetXMP(meta); err != nil {
		t.Fatal(err)
	}
	rep, err := d.ConvertToPDFA(PDFA4)
	if err != nil {
		t.Fatal(err)
	}
	if !rep.Conformant {
		t.Fatalf("not conformant: %+v", rep.Issues)
	}
	raw, _ := d.XMPRaw()
	if n := strings.Count(string(raw), "<pdfx:SourceModified>"); n != 1 {
		t.Errorf("stated %d times", n)
	}
}

func TestValidatePDFA4DuplicateXMPProperty(t *testing.T) {
	d := pdfa4Doc(t)
	if _, err := d.ConvertToPDFA(PDFA4); err != nil {
		t.Fatal(err)
	}
	raw, _ := d.XMPRaw()
	twice := strings.Replace(string(raw), "<pdfaid:part>", "<pdfx:A>1</pdfx:A><pdfx:A>1</pdfx:A><pdfaid:part>", 1)
	twice = strings.Replace(twice, `xmlns:pdfaid=`, `xmlns:pdfx="`+nsPDFX+`" xmlns:pdfaid=`, 1)
	if err := d.SetXMPRaw([]byte(twice)); err != nil {
		t.Fatal(err)
	}
	if !pdfaHasRule(d.ValidatePDFA(PDFA4), "XMP_MALFORMED") {
		t.Error("a repeated property not reported")
	}
}

func TestValidatePDFA4ResourceReferringToNothing(t *testing.T) {
	d := pdfa4Doc(t)
	if _, err := d.ConvertToPDFA(PDFA4); err != nil {
		t.Fatal(err)
	}
	p := d.Pages()[0]
	p.pageResources()["/XObject"] = pdfDict{"/Gone": pdfRef{Num: 99999}}
	if err := p.appendToContentStream([]byte("/Gone Do\n")); err != nil {
		t.Fatal(err)
	}
	if !pdfaHasRule(d.ValidatePDFA(PDFA4), "UNDEFINED_RESOURCE") {
		t.Error("a name referring to no object not reported")
	}
}

func TestValidatePDFA4XObjectWithNoKnownSubtype(t *testing.T) {
	d := pdfa4Doc(t)
	if _, err := d.ConvertToPDFA(PDFA4); err != nil {
		t.Fatal(err)
	}
	p := d.Pages()[0]
	id := d.addObject(&pdfStream{Dict: pdfDict{"/Subtype": pdfName("/A")}, Data: []byte("x"), Decoded: true})
	p.pageResources()["/XObject"] = pdfDict{"/Odd": pdfRef{Num: id}}
	if err := p.appendToContentStream([]byte("/Odd Do\n")); err != nil {
		t.Fatal(err)
	}
	if !pdfaHasRule(d.ValidatePDFA(PDFA4), "UNDEFINED_RESOURCE") {
		t.Error("an XObject with an unknown /Subtype not reported")
	}
}

func TestPDFA4KeepsFormFieldTriggers(t *testing.T) {
	d := pdfa4Doc(t)
	field := pdfDict{"/Subtype": pdfName("/Widget"), "/AA": pdfDict{"/K": pdfDict{"/S": pdfName("/JavaScript")}}}
	note := pdfDict{"/Subtype": pdfName("/Text"), "/T": "author", "/AA": pdfDict{"/K": pdfDict{"/S": pdfName("/JavaScript")}, "/E": pdfDict{"/S": pdfName("/URI")}}}
	d.catalog["/Extra"] = pdfArray{field, note}
	d.removeForbiddenActions(PDFA4)
	if aa, _ := field["/AA"].(pdfDict); aa["/K"] == nil {
		t.Error("a widget's trigger went; PDF/A-4 leaves form fields alone")
	}
	aa, _ := note["/AA"].(pdfDict)
	if aa["/K"] != nil || aa["/E"] == nil {
		t.Errorf("annotation /AA: %v", aa)
	}
}

func TestPDFA4KeepsLayerAS(t *testing.T) {
	build := func() (*Document, pdfDict) {
		d := pdfa4Doc(t)
		d.AddLayer("L")
		ocp, _ := resolveRefToDict(d.objects, d.catalog["/OCProperties"])
		cfg, _ := resolveRefToDict(d.objects, ocp["/D"])
		cfg["/AS"] = pdfArray{}
		return d, cfg
	}
	d, cfg := build()
	if rep, _ := d.ConvertToPDFA(PDFA4); pdfaHasRule(rep, "LAYER_CONFIG") || cfg["/AS"] == nil {
		t.Errorf("PDF/A-4 allows /AS: %+v, %v", rep.Issues, cfg)
	}
	d, cfg = build()
	if _, err := d.ConvertToPDFA(PDFA2B); err != nil || cfg["/AS"] != nil {
		t.Errorf("PDF/A-2 must lose /AS: %v %v", err, cfg)
	}
}

func TestPDFA4HasNoSignatureRule(t *testing.T) {
	d := pdfa4Doc(t)
	d.catalog["/Extra"] = pdfDict{"/ByteRange": pdfArray{0, 1, 2, 3}, "/Contents": "x"}
	if _, err := d.ConvertToPDFA(PDFA4); err != nil {
		t.Fatal(err)
	}
	d.signaturesRewritten = true
	if pdfaHasRule(d.ValidatePDFA(PDFA4), "SIGNATURE_STALE") {
		t.Error("PDF/A-4 has no rule on signatures")
	}
	if !pdfaHasRule(d.ValidatePDFA(PDFA2B), "SIGNATURE_STALE") {
		t.Error("PDF/A-2 still has one")
	}
}

func TestPDFA4UnicodeRules(t *testing.T) {
	text := "BT /F1 12 Tf (A) Tj ET"
	if !hasPDFARule(unicodeFontDoc(t, "0000", text).ValidatePDFA(PDFA4), "INVALID_UNICODE_VALUE") {
		t.Error("/ToUnicode to 0 not reported")
	}
	if rep := unicodeFontDoc(t, "", text).ValidatePDFA(PDFA4); hasPDFARule(rep, "NO_UNICODE_MAPPING") {
		t.Error("PDF/A-4 does not require a Unicode mapping")
	}
}

func TestPDFA4TrueTypeCmaps(t *testing.T) {
	font := func(flags int, records ...uint16) (*Document, pdfDict) {
		data := []byte{0, 1, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 'c', 'm', 'a', 'p', 0, 0, 0, 0, 0, 0, 0, 28}
		n := len(records) / 2
		length := 4 + 8*n
		data = append(data, byte(length>>24), byte(length>>16), byte(length>>8), byte(length), 0, 0, byte(n>>8), byte(n))
		for i := 0; i < n; i++ {
			data = append(data, byte(records[2*i]>>8), byte(records[2*i]), byte(records[2*i+1]>>8), byte(records[2*i+1]), 0, 0, 0, 0)
		}
		d := pdfa4Doc(t)
		return d, pdfDict{"/Subtype": pdfName("/TrueType"), "/BaseFont": pdfName("/F"), "/FontDescriptor": pdfDict{
			"/Flags": flags, "/FontFile2": &pdfStream{Dict: pdfDict{}, Data: data, Decoded: true}}}
	}
	const nonsymbolic, symbolic = 32, 4
	for _, c := range []struct {
		flags   int
		records []uint16
		bad     bool
	}{
		{nonsymbolic, []uint16{3, 1}, false},
		{nonsymbolic, []uint16{1, 0}, false},
		{nonsymbolic, []uint16{0, 3}, true},
		{nonsymbolic, []uint16{3, 0}, true},
		{symbolic, []uint16{3, 0}, false},
		{symbolic, []uint16{1, 0}, false},
		{symbolic, []uint16{3, 1}, true},
	} {
		d, f := font(c.flags, c.records...)
		if got := fontIssueRules(d.simpleFontIssues(f, 4))["TRUETYPE_CMAPS"]; got != c.bad {
			t.Errorf("flags %d, cmaps %v: reported %v", c.flags, c.records, got)
		}
	}
}

func TestPDFA4UndefinedFontNamesNothing(t *testing.T) {
	d := pdfa4Doc(t)
	if _, err := d.ConvertToPDFA(PDFA4); err != nil {
		t.Fatal(err)
	}
	p := d.Pages()[0]
	fonts, _ := resolveRefToDict(d.objects, p.pageResources()["/Font"])
	fonts["/Gone"] = pdfRef{Num: 99999}
	if err := p.appendToContentStream([]byte("BT /Gone 12 Tf ET\n")); err != nil {
		t.Fatal(err)
	}
	if !pdfaHasRule(d.ValidatePDFA(PDFA4), "UNDEFINED_RESOURCE") {
		t.Error("a font name referring to no object not reported")
	}
}

func TestPDFA4NoCIDSetOrCharSetRule(t *testing.T) {
	d, font := cidFontDoc(t, "/ABCDEF+DejaVu")
	desc, _ := resolveRefToDict(d.objects, font["/FontDescriptor"])
	desc["/CIDSet"] = pdfRef{Num: d.addObject(&pdfStream{Dict: pdfDict{}, Data: []byte{0x01}, Decoded: true})}
	if hasPDFARule(d.ValidatePDFA(PDFA4), "CIDSET_WRONG") {
		t.Error("PDF/A-4 has no rule on /CIDSet")
	}
	d2, _ := type1SubsetDoc(t, "/nonesuch")
	if fontIssueRules(d2.simpleFontIssues(firstFontDict(t, d2), 4))["CHARSET_INCOMPLETE"] {
		t.Error("PDF/A-4 has no rule on /CharSet")
	}
}

func TestPDFA4ProjectionNeedsNoAppearance(t *testing.T) {
	d := pdfa4Doc(t)
	if _, err := d.ConvertToPDFA(PDFA4); err != nil {
		t.Fatal(err)
	}
	page := d.pages[0].Value.(pdfDict)
	page["/Annots"] = pdfArray{pdfDict{"/Subtype": pdfName("/Projection"), "/F": 4, "/Rect": pdfArray{0, 0, 10, 10}}}
	if pdfaHasRule(d.ValidatePDFA(PDFA4), "ANNOTATION_NO_AP") {
		t.Error("a Projection annotation needs no appearance")
	}
}
