// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"testing"
)

func TestHasUnicodeSources(t *testing.T) {
	report := func(encoding pdfValue, code string) bool {
		d := simpleFontDoc(t, encoding, "BT /F1 12 Tf ("+code+") Tj ET")
		return hasPDFARule(d.ValidatePDFA(PDFA2U), "NO_UNICODE_MAPPING")
	}
	// ISO 32000-1 Annex D: an unused WinAnsi code above 040 is a bullet; below it, nothing.
	if report(pdfName("/WinAnsiEncoding"), "\x81") {
		t.Error("WinAnsi 0x81 is a bullet")
	}
	if !report(pdfName("/WinAnsiEncoding"), "\x09") {
		t.Error("WinAnsi 0x09 means nothing")
	}
	if report(pdfDict{"/BaseEncoding": pdfName("/WinAnsiEncoding")}, "\x81") {
		t.Error("a WinAnsi base counts as well")
	}
	if !report(pdfName("/MacRomanEncoding"), "\x09") {
		t.Error("MacRoman has no such rule")
	}
	if !report(nil, "\x81") {
		t.Error("no encoding, no bullet")
	}
	// An /Encoding held in an object of its own.
	d := simpleFontDoc(t, nil, "BT /F1 12 Tf (\x81) Tj ET")
	ref := pdfRef{Num: d.addObject(pdfDict{"/BaseEncoding": pdfName("/WinAnsiEncoding")})}
	for _, obj := range d.objects {
		if dict, ok := obj.Value.(pdfDict); ok && dictGetName(dict, "/Type") == "/Font" {
			dict["/Encoding"] = ref
		}
	}
	if hasPDFARule(d.ValidatePDFA(PDFA2U), "NO_UNICODE_MAPPING") {
		t.Error("an indirect WinAnsi base not read")
	}
}

// Latin codes through a predefined CJK CMap reach Unicode as the renderer's do, though
// the ordering table says nothing of them.
func TestHasUnicodeCJKLatin(t *testing.T) {
	cm := predefinedCMap("90ms-RKSJ-H")
	if cm == nil {
		t.Fatal("no CMap")
	}
	table := cidToUnicodeForOrdering("Japan1")
	_, cid, n := cm.next([]byte{'3'})
	if n != 1 || table[cid] != 0 {
		t.Skipf("the table now maps it: cid %d", cid)
	}
	gf := &glyphFont{fi: fontInfo{isType0: true, cidCMap: cm, cidToUni: table}}
	if gf.hasUnicode('3', uint32(cid)) {
		t.Fatal("without the fallback there is no mapping")
	}
	gf.latin = cjkASCIIFallback(cm, table)
	if !gf.hasUnicode('3', uint32(cid)) {
		t.Error("the Latin fallback gives the mapping")
	}
	if got := newGlyphFont(map[int]*pdfObject{}, 0, pdfDict{}); got.latin != nil {
		t.Errorf("no CMap, no fallback: %v", got.latin)
	}
}

func TestPDFAWidgetAction(t *testing.T) {
	build := func(subtype string) (*Document, pdfDict) {
		d, p := conformDoc(t)
		ad := pdfDict{"/Type": pdfName("/Annot"), "/Subtype": pdfName(subtype), "/Rect": pdfArray{0, 0, 10, 10}, "/F": 4, "/A": pdfDict{"/S": pdfName("/SubmitForm"), "/F": "http://x"}}
		p.pageDict()["/Annots"] = pdfArray{pdfRef{Num: d.addObject(ad)}}
		return d, ad
	}
	for _, format := range []PDFAFormat{PDFA1B, PDFA1A, PDFA2B, PDFA2A, PDFA2U, PDFA3B, PDFA3A, PDFA3U} {
		d, _ := build("/Widget")
		if !hasPDFARule(d.ValidatePDFA(format), "WIDGET_ACTION") {
			t.Errorf("%v: not reported", format)
		}
		d, ad := build("/Widget")
		if rep := convertA(t, d, format); hasPDFARule(rep, "WIDGET_ACTION") {
			t.Errorf("%v: still reported: %+v", format, rep.Issues)
		}
		if _, has := ad["/A"]; has {
			t.Errorf("%v: the action stayed", format)
		}
	}
	// A link keeps its own action.
	d, ad := build("/Link")
	if hasPDFARule(convertA(t, d, PDFA2B), "WIDGET_ACTION") {
		t.Error("a link reported")
	}
	if _, has := ad["/A"]; !has {
		t.Error("a link lost its action")
	}
}

// A file specification counts however it is reached, as veraPDF scans every object.
func TestPDFA3LooseFilespec(t *testing.T) {
	build := func() (*Document, pdfDict, *pdfStream) {
		d, _ := conformDoc(t)
		s := &pdfStream{Dict: pdfDict{"/Type": pdfName("/EmbeddedFile")}, Data: []byte("x"), Decoded: true}
		fs := pdfDict{"/Type": pdfName("/Filespec"), "/F": "a.bin", "/UF": "a.bin", "/EF": pdfDict{"/F": pdfRef{Num: d.addObject(s)}}}
		if d.catalog == nil {
			d.catalog = pdfDict{}
		}
		d.catalog["/Holder"] = pdfRef{Num: d.addObject(fs)}
		return d, fs, s
	}
	d, _, _ := build()
	if !hasPDFARule(d.ValidatePDFA(PDFA3B), "EMBEDDED_FILE_NOT_ASSOCIATED") {
		t.Error("loose file specification not reported")
	}
	if !hasPDFARule(d.ValidatePDFA(PDFA3U), "EMBEDDED_FILE_NO_MIME") {
		t.Error("missing MIME type not reported")
	}
	for _, rule := range []string{"EMBEDDED_FILE_NOT_ASSOCIATED", "EMBEDDED_FILE_NO_MIME"} {
		if hasPDFARule(d.ValidatePDFA(PDFA2B), rule) {
			t.Errorf("%s is a PDF/A-3 rule", rule)
		}
	}
	d, fs, s := build()
	rep := convertA(t, d, PDFA3U)
	if hasPDFARule(rep, "EMBEDDED_FILE_NOT_ASSOCIATED") || hasPDFARule(rep, "EMBEDDED_FILE_NO_MIME") {
		t.Errorf("not repaired: %+v", rep.Issues)
	}
	if fs["/AFRelationship"] != pdfName("/Unspecified") {
		t.Errorf("relationship %v", fs["/AFRelationship"])
	}
	if s.Dict["/Subtype"] != pdfName("/application/octet-stream") {
		t.Errorf("type %v", s.Dict["/Subtype"])
	}
	// A relationship the file already states, and a good type, stay.
	d, fs, s = build()
	fs["/AFRelationship"] = pdfName("/Data")
	s.Dict["/Subtype"] = pdfName("/text/xml")
	if rep := convertA(t, d, PDFA3B); hasPDFARule(rep, "EMBEDDED_FILE_NOT_ASSOCIATED") || hasPDFARule(rep, "EMBEDDED_FILE_NO_MIME") {
		t.Errorf("%+v", rep.Issues)
	}
	if fs["/AFRelationship"] != pdfName("/Data") || s.Dict["/Subtype"] != pdfName("/text/xml") {
		t.Errorf("%v %v", fs["/AFRelationship"], s.Dict["/Subtype"])
	}
	// A relationship alone is not enough: the catalog has to list it too.
	d, fs, _ = build()
	fs["/AFRelationship"] = pdfName("/Data")
	if !hasPDFARule(d.ValidatePDFA(PDFA3B), "EMBEDDED_FILE_NOT_ASSOCIATED") {
		t.Error("a file specification missing from /AF not reported")
	}
	// A value that is not a MIME type is replaced.
	d, _, s = build()
	s.Dict["/Subtype"] = pdfName("/notamime")
	if !hasPDFARule(d.ValidatePDFA(PDFA3B), "EMBEDDED_FILE_NO_MIME") {
		t.Error("a bad type not reported")
	}
	convertA(t, d, PDFA3B)
	if s.Dict["/Subtype"] != pdfName("/application/octet-stream") {
		t.Errorf("type %v", s.Dict["/Subtype"])
	}
}

// A name is escaped once, by the writer: what other readers see is what was meant.
func TestNamesEscapedOnceInTheFile(t *testing.T) {
	d, _ := conformDoc(t)
	if _, err := d.AttachInvoice(ciiInvoice("urn:cen.eu:en16931:2017")); err != nil {
		t.Fatal(err)
	}
	col := d.Collection().Schema()
	col.Add("My Field", "Mine", CollectionFieldText)
	d.EmbeddedFiles().All()[0].CollectionItem().SetText("My Field", "v")
	var buf bytes.Buffer
	if _, err := d.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	raw := buf.Bytes()
	if !bytes.Contains(raw, []byte("/text#2Fxml")) {
		t.Error("the MIME type is not /text#2Fxml")
	}
	if !bytes.Contains(raw, []byte("/My#20Field")) {
		t.Error("the column key is not /My#20Field")
	}
	if bytes.Contains(raw, []byte("#23")) {
		t.Error("a name escaped twice")
	}
	reopened, err := OpenStream(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	f := reopened.EmbeddedFiles().All()[0]
	if f.MIMEType() != "text/xml" {
		t.Errorf("MIME type %q", f.MIMEType())
	}
	if v := f.CollectionItem().Text("My Field"); v != "v" {
		t.Errorf("column value %q", v)
	}
	if rep := reopened.ValidatePDFA(PDFA3B); hasPDFARule(rep, "EMBEDDED_FILE_NO_MIME") {
		t.Errorf("%+v", rep.Issues)
	}
	// A file saved twice-escaped by an earlier version comes out as it was meant.
	d, _ = conformDoc(t)
	s := &pdfStream{Dict: pdfDict{"/Subtype": pdfName("/text#2Fxml")}, Data: []byte("x"), Decoded: true}
	if d.catalog == nil {
		d.catalog = pdfDict{}
	}
	d.catalog["/Holder"] = pdfRef{Num: d.addObject(pdfDict{"/F": "a", "/UF": "a", "/EF": pdfDict{"/F": pdfRef{Num: d.addObject(s)}}})}
	if !hasPDFARule(d.ValidatePDFA(PDFA3B), "EMBEDDED_FILE_NO_MIME") {
		t.Error("twice-escaped type not reported")
	}
	convertA(t, d, PDFA3B)
	if s.Dict["/Subtype"] != pdfName("/text/xml") {
		t.Errorf("type %v", s.Dict["/Subtype"])
	}
}

// An ordinary attachment too, with no conversion to repair it afterwards.
func TestAttachmentMIMEEscapedOnce(t *testing.T) {
	d, _ := conformDoc(t)
	if _, err := d.EmbeddedFiles().AddFromStream("a.pdf", bytes.NewReader([]byte("x"))); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if _, err := d.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(buf.Bytes(), []byte("/application#2Fpdf")) || bytes.Contains(buf.Bytes(), []byte("#232F")) {
		t.Error("the MIME type is not escaped once")
	}
}

// A file specification with no file counts too. One nothing refers to goes, as after
// a removed Launch action; one in use stays and gets its relationship.
func TestPDFA3FilespecWithoutFile(t *testing.T) {
	d, p := conformDoc(t)
	orphan := d.addObject(pdfDict{"/Type": pdfName("/Filespec"), "/F": "a.pdf", "/UF": "a.pdf"})
	used := pdfDict{"/Type": pdfName("/Filespec"), "/F": "b.pdf", "/UF": "b.pdf"}
	usedID := d.addObject(used)
	p.pageDict()["/Annots"] = pdfArray{pdfDict{
		"/Subtype": pdfName("/Link"), "/Rect": pdfArray{0, 0, 9, 9}, "/F": 4,
		"/A": pdfDict{"/S": pdfName("/GoToR"), "/D": pdfArray{0, pdfName("/Fit")}, "/F": pdfRef{Num: usedID}},
	}}
	if !hasPDFARule(d.ValidatePDFA(PDFA3B), "EMBEDDED_FILE_NOT_ASSOCIATED") {
		t.Error("not reported")
	}
	rep := convertA(t, d, PDFA3B)
	if hasPDFARule(rep, "EMBEDDED_FILE_NOT_ASSOCIATED") {
		t.Errorf("%+v", rep.Issues)
	}
	if _, kept := d.objects[orphan]; kept {
		t.Error("the unreferenced file specification stayed")
	}
	if used["/AFRelationship"] != pdfName("/Unspecified") || !d.isAssociatedFile(pdfRef{Num: usedID}) {
		t.Errorf("%v", used)
	}
}
