// SPDX-License-Identifier: MIT

package asposepdf

import "testing"

func TestEncodedNotdefCodes(t *testing.T) {
	gf := &glyphFont{dict: pdfDict{"/Encoding": pdfName("/WinAnsiEncoding")}, rf: &renderFont{}}
	gf.fi.encoding[1] = 0xFFFD
	gf.fi.encoding[2] = 9
	gf.fi.encoding[3] = 0
	gf.fi.encoding[65] = 'A'
	for code, want := range map[uint32]bool{1: true, 2: true, 3: true, 65: false} {
		if got := gf.encodedNotdef(code); got != want {
			t.Errorf("code %d: %v", code, got)
		}
	}
}

func TestFontWidthRuleDefaultWidth(t *testing.T) {
	d, cid, _ := glyphDoc(t)
	delete(cid, "/W")
	cid["/DW"] = 1
	if !glyphRules(d)["FONT_WIDTH_MISMATCH"] {
		t.Error("/DW disagreeing with the program not reported")
	}
}

func TestToUnicodeGivesTheMapping(t *testing.T) {
	gf := &glyphFont{}
	if gf.hasUnicode(65, 65) {
		t.Fatal("nothing says what 65 means")
	}
	gf.fi.toUnicode = map[uint16]rune{65: 'Ж'}
	if !gf.hasUnicode(65, 65) {
		t.Error("/ToUnicode not consulted")
	}
}

func TestFixFontDictsLeavesWholeFontsAndOtherOrderings(t *testing.T) {
	d, desc := type1SubsetDoc(t, nil)
	firstFontDict(t, d)["/BaseFont"] = pdfName("/T")
	d.fixFontDicts(1)
	if _, has := desc["/CharSet"]; has {
		t.Error("/CharSet written for a whole font")
	}
	// Lowering a supplement says nothing true of a different ordering.
	d, cid := type0FontDoc(t, pdfName("/UniGB-UCS2-H"), 99)
	d.fixFontDicts(2)
	if info, _ := resolveRefToDict(d.objects, cid["/CIDSystemInfo"]); dictGetInt(info, "/Supplement") != 99 {
		t.Errorf("supplement changed: %v", info)
	}
}

func TestConversionsRepairFilespecsAndNumbers(t *testing.T) {
	for name, convert := range map[string]func(d *Document) error{
		"PDF/A":  func(d *Document) error { _, err := d.ConvertToPDFA(PDFA3B); return err },
		"PDF/UA": func(d *Document) error { _, err := d.ConvertToPDFUA(); return err },
	} {
		d := NewDocument(300, 200)
		st := d.addObject(buildFlateStream([]byte("x")))
		fs := pdfDict{"/F": "a.txt", "/EF": pdfDict{"/F": pdfRef{Num: st}}}
		d.addObject(fs)
		num := pdfDict{"/Descent": 4294967076}
		d.addObject(num)
		if err := convert(d); err != nil {
			t.Fatal(err)
		}
		if fs["/UF"] != "a.txt" || num["/Descent"] != -220 {
			t.Errorf("%s: %v %v", name, fs, num)
		}
	}
}

func TestSubstituteWidthEdges(t *testing.T) {
	// Two codes reach one glyph: the lower decides.
	_, re, _ := substituted(t, pdfDict{
		"/Type": pdfName("/Font"), "/Subtype": pdfName("/TrueType"), "/BaseFont": pdfName("/Garamond"),
		"/Encoding":  pdfDict{"/BaseEncoding": pdfName("/WinAnsiEncoding"), "/Differences": pdfArray{65, pdfName("/A"), pdfName("/A")}},
		"/FirstChar": 65, "/LastChar": 66, "/Widths": pdfArray{900, 300},
	}, "BT /F1 12 Tf (AB) Tj ET")
	gf := re.glyphFontsInUse()[0]
	if w, _ := gf.rf.programWidth(gf.rf.gid(65)); w < 899.5 || w > 900.5 {
		t.Errorf("program width = %v, want the lower code's", w)
	}
	// A code below /FirstChar has no entry to take.
	d, _ := dictFontDoc(t, pdfDict{
		"/Type": pdfName("/Font"), "/Subtype": pdfName("/TrueType"), "/BaseFont": pdfName("/Garamond"),
		"/Encoding": pdfName("/WinAnsiEncoding"), "/FirstChar": 70, "/LastChar": 70, "/Widths": pdfArray{900},
	}, "BT /F1 12 Tf (A) Tj ET")
	if _, err := d.ConvertToPDFA(PDFA2B); err != nil {
		t.Fatal(err)
	}
}

func TestConvertToPDFAEmbedsCMapsOnlyForPart1(t *testing.T) {
	for format, want := range map[PDFAFormat]bool{PDFA1B: true, PDFA2B: false} {
		d, _ := type0FontDoc(t, pdfName("/UniJIS-UCS2-H"), 0)
		if _, err := d.ConvertToPDFA(format); err != nil {
			t.Fatal(err)
		}
		_, isStream := resolveRef(d.objects, firstFontDict(t, d)["/Encoding"]).(*pdfStream)
		if isStream != want {
			t.Errorf("%v: embedded = %v", format, isStream)
		}
	}
}

func TestReadCFFWidths(t *testing.T) {
	// 500 defaultWidthX, 600 nominalWidthX, after a byte the offset has to skip.
	data := []byte{0xFF, 28, 0x01, 0xF4, 20, 28, 0x02, 0x58, 21}
	if w := readCFFWidths(data, []float64{8, 1}); w.def != 500 || w.nominal != 600 {
		t.Errorf("%+v", w)
	}
	if w := readCFFWidths(data, []float64{99, 1}); w != (cffWidths{}) {
		t.Errorf("past the end: %+v", w)
	}
}

func TestPDFUAReportsFontRules(t *testing.T) {
	d, cid, code := glyphDoc(t)
	cid["/W"] = pdfArray{int(code), pdfArray{5}}
	if !hasUARule(d.ValidatePDFUA(), "UA_FONT_WIDTH_MISMATCH") {
		t.Error("glyph rule not reported")
	}
	d, _ = dictFontDoc(t, pdfDict{"/Subtype": pdfName("/TrueType"), "/BaseFont": pdfName("/Arial")}, "BT /F1 12 Tf (A) Tj ET")
	if !hasUARule(d.ValidatePDFUA(), "UA_FONT_NO_TYPE") {
		t.Error("dictionary rule not reported")
	}
}

// The codes this exercises are the point: CIDs the ordering table has no Unicode
// for, which only the CMap's Latin range explains.
func TestConvertToPDFACompositeUnmappedLatin(t *testing.T) {
	cm := predefinedCMap("90ms-RKSJ-H")
	latin := cjkASCIIFallback(cm, cidToUnicodeForOrdering("Japan1"))
	if len(latin) == 0 {
		t.Fatal("the ordering table covers this CMap's Latin; pick another fixture")
	}
	if fontRepo.findCJK(fontInfo{name: "/MS-Gothic"}, "Japan1") == nil {
		t.Skip("no CJK face installed")
	}
	var draw byte
	for code := byte(0x21); code < 0x7F; code++ {
		_, cid, n := cm.next([]byte{code})
		if n == 1 && latin[cid] != 0 {
			draw = code
			break
		}
	}
	if draw == 0 {
		t.Fatal("no single-byte code reaches an unmapped CID")
	}
	d := NewDocument(300, 200)
	cid := d.addObject(pdfDict{"/Type": pdfName("/Font"), "/Subtype": pdfName("/CIDFontType2"), "/BaseFont": pdfName("/MS-Gothic"),
		"/CIDSystemInfo": pdfDict{"/Registry": "Adobe", "/Ordering": "Japan1", "/Supplement": 2}, "/DW": 500})
	font := d.addObject(pdfDict{"/Type": pdfName("/Font"), "/Subtype": pdfName("/Type0"), "/BaseFont": pdfName("/MS-Gothic"),
		"/Encoding": pdfName("/90ms-RKSJ-H"), "/DescendantFonts": pdfArray{pdfRef{Num: cid}}})
	p, _ := d.Page(1)
	pd := p.pageObj().Value.(pdfDict)
	pd["/Resources"] = pdfDict{"/Font": pdfDict{"/F1": pdfRef{Num: font}}}
	pd["/Contents"] = pdfRef{Num: d.addObject(buildFlateStream([]byte("BT /F1 12 Tf <" + testHexByte(draw) + "> Tj ET")))}
	rep, err := d.ConvertToPDFA(PDFA2B)
	if err != nil {
		t.Fatal(err)
	}
	if pdfaHasRule(rep, "NOTDEF_GLYPH_USED") || pdfaHasRule(rep, "GLYPH_MISSING") || pdfaHasRule(rep, "FONT_NOT_EMBEDDED") {
		t.Errorf("Latin lost its glyph: %+v", rep.Issues)
	}
}

// The composite font takes the name of the face actually embedded for it.
func TestConvertToPDFACompositeTakesTheEmbeddedName(t *testing.T) {
	d := NewDocument(300, 200)
	cid := d.addObject(pdfDict{"/Type": pdfName("/Font"), "/Subtype": pdfName("/CIDFontType2"), "/BaseFont": pdfName("/NotInstalled"),
		"/CIDSystemInfo": pdfDict{"/Registry": "Adobe", "/Ordering": "GB1", "/Supplement": 2}, "/DW": 1000})
	font := d.addObject(pdfDict{"/Type": pdfName("/Font"), "/Subtype": pdfName("/Type0"), "/BaseFont": pdfName("/NotInstalled"),
		"/Encoding": pdfName("/UniGB-UCS2-H"), "/DescendantFonts": pdfArray{pdfRef{Num: cid}}})
	if fontRepo.findCJK(resolveFont(d.objects, d.objects[font].Value.(pdfDict)), "GB1") == nil {
		t.Skip("no CJK face installed")
	}
	p, _ := d.Page(1)
	pd := p.pageObj().Value.(pdfDict)
	pd["/Resources"] = pdfDict{"/Font": pdfDict{"/F1": pdfRef{Num: font}}}
	pd["/Contents"] = pdfRef{Num: d.addObject(buildFlateStream([]byte("BT /F1 12 Tf <4F60> Tj ET")))}
	if _, err := d.ConvertToPDFA(PDFA2B); err != nil {
		t.Fatal(err)
	}
	name := dictGetName(d.objects[font].Value.(pdfDict), "/BaseFont")
	if name == "/NotInstalled" || name != dictGetName(d.objects[cid].Value.(pdfDict), "/BaseFont") {
		t.Errorf("/BaseFont = %s", name)
	}
}

func testHexByte(b byte) string {
	const digits = "0123456789ABCDEF"
	return string([]byte{digits[b>>4], digits[b&15]})
}
