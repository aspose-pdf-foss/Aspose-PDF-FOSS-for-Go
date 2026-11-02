// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"os"
	"strings"
	"testing"
	"time"
)

const glyphTestFont = "testdata/DejaVuSans.ttf"

// composite returns a reopened document drawing through an embedded Type0 font,
// with that font's descendant and the CID of "A", and the content it draws.
func glyphDoc(t *testing.T) (*Document, pdfDict, uint16) {
	t.Helper()
	d := NewDocument(300, 200)
	font, err := d.LoadFont(glyphTestFont)
	if err != nil {
		t.Skipf("font unavailable: %v", err)
	}
	p, _ := d.Page(1)
	if err := p.AddTextAt("A", TextStyle{Font: font, Size: 12}, 20, 100); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if _, err := d.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	re, err := OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	fonts := re.glyphFontsInUse()
	if len(fonts) != 1 || fonts[0].rf == nil {
		t.Fatalf("fonts = %+v", fonts)
	}
	_, cid := fontDescriptorOf(re.objects, fonts[0].dict)
	var code uint16
	for c := range fonts[0].seen {
		code = uint16(c)
	}
	return re, cid, code
}

func glyphRules(d *Document) map[string]bool {
	out := map[string]bool{}
	for _, is := range d.fontGlyphIssues(true) {
		out[is.rule] = true
	}
	return out
}

func TestFontWidthRule(t *testing.T) {
	for _, tc := range []struct {
		delta float64
		tr3   bool
		want  bool
	}{
		{0, false, false},
		{0.9, false, false}, // within the tolerance
		{-0.9, false, false},
		{1.5, false, true},
		{-1.5, false, true},
		{50, true, false}, // invisible text is not measured
	} {
		d, cid, code := glyphDoc(t)
		pw, ok := d.glyphFontsInUse()[0].rf.programWidth(d.glyphFontsInUse()[0].rf.gid(uint32(code)))
		if !ok {
			t.Fatal("no program width")
		}
		cid["/W"] = pdfArray{int(code), pdfArray{pw + tc.delta}}
		if tc.tr3 {
			p, _ := d.Page(1)
			data, _ := p.contentStreams()
			pd := p.pageObj().Value.(pdfDict)
			pd["/Contents"] = pdfRef{Num: d.addObject(buildFlateStream(append([]byte("3 Tr\n"), data...)))}
		}
		if got := glyphRules(d)["FONT_WIDTH_MISMATCH"]; got != tc.want {
			t.Errorf("%+v: reported = %v", tc, got)
		}
	}
}

// Tr is part of the graphics state, so Q takes it back.
func TestFontWidthRuleTrRestoredByQ(t *testing.T) {
	d, cid, code := glyphDoc(t)
	cid["/W"] = pdfArray{int(code), pdfArray{5}}
	p, _ := d.Page(1)
	data, _ := p.contentStreams()
	pd := p.pageObj().Value.(pdfDict)
	pd["/Contents"] = pdfRef{Num: d.addObject(buildFlateStream(append([]byte("q 3 Tr Q\n"), data...)))}
	if !glyphRules(d)["FONT_WIDTH_MISMATCH"] {
		t.Error("text after the Q is visible and should be measured")
	}
}

func drawOn(t *testing.T, d *Document, content string) {
	t.Helper()
	p, _ := d.Page(1)
	pd := p.pageObj().Value.(pdfDict)
	res, _ := resolveRefToDict(d.objects, pd["/Resources"])
	fonts, _ := resolveRefToDict(d.objects, res["/Font"])
	var name string
	for k := range fonts {
		name = k
	}
	content = strings.ReplaceAll(content, "/F", name)
	pd["/Contents"] = pdfRef{Num: d.addObject(buildFlateStream([]byte(content)))}
}

func TestCompositeGlyphRules(t *testing.T) {
	d, _, _ := glyphDoc(t)
	drawOn(t, d, "BT /F 12 Tf <0000> Tj ET")
	r := glyphRules(d)
	if !r["NOTDEF_GLYPH_USED"] || !r["GLYPH_MISSING"] {
		t.Errorf("CID 0: %v", r)
	}
	if rep := d.ValidatePDFA(PDFA1B); pdfaHasRule(rep, "NOTDEF_GLYPH_USED") {
		t.Error("PDF/A-1 has no rule against .notdef itself")
	}
	if rep := d.ValidatePDFA(PDFA2B); !pdfaHasRule(rep, "NOTDEF_GLYPH_USED") {
		t.Error("PDF/A-2 forbids .notdef")
	}
	// The .notdef rule does not exempt invisible text; the others do.
	drawOn(t, d, "3 Tr BT /F 12 Tf <0000> Tj ET")
	if r := glyphRules(d); !r["NOTDEF_GLYPH_USED"] || r["GLYPH_MISSING"] {
		t.Errorf("invisible CID 0: %v", r)
	}
	// A CID past the end of the program.
	d, _, _ = glyphDoc(t)
	drawOn(t, d, "BT /F 12 Tf <FFF0> Tj ET")
	if r := glyphRules(d); !r["GLYPH_MISSING"] {
		t.Errorf("CID past the program: %v", r)
	}
}

// simpleFontDoc draws through an embedded simple TrueType font.
func simpleFontDoc(t *testing.T, encoding pdfValue, content string) *Document {
	t.Helper()
	data, err := os.ReadFile(glyphTestFont)
	if err != nil {
		t.Skipf("font unavailable: %v", err)
	}
	d := NewDocument(300, 200)
	file := d.addObject(buildFlateStream(data))
	desc := d.addObject(pdfDict{"/Type": pdfName("/FontDescriptor"), "/FontName": pdfName("/DejaVuSans"), "/Flags": 32, "/FontFile2": pdfRef{Num: file}})
	font := pdfDict{"/Type": pdfName("/Font"), "/Subtype": pdfName("/TrueType"), "/BaseFont": pdfName("/DejaVuSans"), "/FontDescriptor": pdfRef{Num: desc}}
	if encoding != nil {
		font["/Encoding"] = encoding
	}
	id := d.addObject(font)
	p, _ := d.Page(1)
	pd := p.pageObj().Value.(pdfDict)
	pd["/Resources"] = pdfDict{"/Font": pdfDict{"/F1": pdfRef{Num: id}}}
	pd["/Contents"] = pdfRef{Num: d.addObject(buildFlateStream([]byte(content)))}
	return d
}

// A simple font's glyph is .notdef when its encoding names none; with no /Encoding
// the program decides, and a code it does not map is merely absent.
func TestSimpleFontNotdefComesFromTheEncoding(t *testing.T) {
	const draw = "BT /F1 12 Tf <01> Tj ET"
	r := glyphRules(simpleFontDoc(t, pdfName("/WinAnsiEncoding"), draw))
	if !r["NOTDEF_GLYPH_USED"] || !r["GLYPH_MISSING"] {
		t.Errorf("with an encoding: %v", r)
	}
	r = glyphRules(simpleFontDoc(t, nil, draw))
	if r["NOTDEF_GLYPH_USED"] || !r["GLYPH_MISSING"] {
		t.Errorf("with none: %v", r)
	}
	r = glyphRules(simpleFontDoc(t, pdfDict{"/Differences": pdfArray{65, pdfName("/.notdef")}}, "BT /F1 12 Tf (A) Tj ET"))
	if !r["NOTDEF_GLYPH_USED"] {
		t.Errorf("named .notdef: %v", r)
	}
	if r := glyphRules(simpleFontDoc(t, pdfName("/WinAnsiEncoding"), "BT /F1 12 Tf (A) Tj ET")); r["NOTDEF_GLYPH_USED"] || r["GLYPH_MISSING"] {
		t.Errorf("an ordinary glyph: %v", r)
	}
}

func TestFontWidthRuleSimpleFont(t *testing.T) {
	d := simpleFontDoc(t, pdfName("/WinAnsiEncoding"), "BT /F1 12 Tf (A) Tj ET")
	gf := d.glyphFontsInUse()[0]
	pw, _ := gf.rf.programWidth(gf.rf.gid(65))
	if glyphRules(d)["FONT_WIDTH_MISMATCH"] {
		t.Error("no /Widths: nothing to compare")
	}
	gf.dict["/FirstChar"], gf.dict["/LastChar"], gf.dict["/Widths"] = 65, 65, pdfArray{pw + 10}
	if !glyphRules(d)["FONT_WIDTH_MISMATCH"] {
		t.Error("/Widths disagreeing with the program not reported")
	}
	gf.dict["/Widths"] = pdfArray{pw}
	if glyphRules(d)["FONT_WIDTH_MISMATCH"] {
		t.Error("agreeing /Widths reported")
	}
	// Past /Widths, /MissingWidth speaks for the code.
	gf.dict["/FirstChar"], gf.dict["/LastChar"] = 70, 70
	if glyphRules(d)["FONT_WIDTH_MISMATCH"] {
		t.Error("outside /Widths with no /MissingWidth: nothing to compare")
	}
	desc, _ := resolveRefToDict(d.objects, gf.dict["/FontDescriptor"])
	desc["/MissingWidth"] = pw + 10
	if !glyphRules(d)["FONT_WIDTH_MISMATCH"] {
		t.Error("/MissingWidth disagreeing with the program not reported")
	}
}

func TestProgramAdvances(t *testing.T) {
	f := &cffFont{unitsPerEm: 1000, widthX: cffWidths{def: 500, nominal: 600}, charStrings: [][]byte{
		{14},                    // endchar: no width stated, so the default
		{149, 14},               // 10 endchar: the nominal plus what it states
		{144, 149, 159, 1, 14},  // 5 10 20 hstem endchar: an odd count starts with the width
		{149, 159, 1, 14},       // 10 20 hstem endchar: even, no width
		{149, 149, 149, 21, 14}, // 10 10 10 rmoveto endchar
		{149, 149, 21, 14},      // 10 10 rmoveto endchar
		{149, 22, 14},           // 10 hmoveto endchar
		{149, 149, 22, 14},      // 10 10 hmoveto endchar
	}}
	for gid, want := range []float64{500, 610, 605, 500, 610, 500, 500, 610} {
		if got, ok := f.glyphAdvance(uint16(gid)); !ok || got != want {
			t.Errorf("gid %d: %v %v, want %v", gid, got, ok, want)
		}
	}
	if _, ok := f.glyphAdvance(99); ok {
		t.Error("glyph past the end")
	}
	cid := &cffFont{unitsPerEm: 1000, isCID: true, fdSelect: []uint8{0, 1}, fdWidths: []cffWidths{{def: 100}, {def: 200}}, charStrings: [][]byte{{14}, {14}}}
	if w, _ := cid.glyphAdvance(1); w != 200 {
		t.Errorf("a CID font's width comes from the glyph's own FD: %v", w)
	}
	t1 := &type1Font{glyphNames: []string{"a", "b"}, charstrings: map[string][]byte{"a": {139, 248, 136, 13, 14}}} // 0 500 hsbw endchar
	if w, ok := t1.glyphAdvance(0); !ok || w != 500 {
		t.Errorf("Type1: %v %v", w, ok)
	}
	if _, ok := t1.glyphAdvance(1); ok {
		t.Error("Type1 glyph with no charstring")
	}
	if _, ok := t1.glyphAdvance(5); ok {
		t.Error("Type1 glyph past the end")
	}
}

func TestType1GlyphState(t *testing.T) {
	var fi fontInfo
	fi.glyphNames = map[byte]string{65: "a", 66: "z", 67: ".notdef"}
	rf := &renderFont{fi: fi, t1: &type1Font{charstrings: map[string][]byte{"a": {14}, ".notdef": {14}}, nameToGID: map[string]uint16{".notdef": 0, "a": 1}}}
	if _, present, notdef := rf.glyphState(65); !present || notdef {
		t.Errorf("a: %v %v", present, notdef)
	}
	if _, present, notdef := rf.glyphState(66); present || notdef {
		t.Errorf("z, which the program lacks: %v %v", present, notdef)
	}
	if _, _, notdef := rf.glyphState(67); !notdef {
		t.Error(".notdef")
	}
}

func dictFontDoc(t *testing.T, font pdfDict, content string) (*Document, int) {
	t.Helper()
	d := NewDocument(300, 200)
	id := d.addObject(font)
	p, _ := d.Page(1)
	pd := p.pageObj().Value.(pdfDict)
	pd["/Resources"] = pdfDict{"/Font": pdfDict{"/F1": pdfRef{Num: id}}}
	pd["/Contents"] = pdfRef{Num: d.addObject(buildFlateStream([]byte(content)))}
	return d, id
}

// A used font with no /Type is a font all the same: it was neither embedded nor
// reported, since both looked only at /Type /Font.
func TestFontWithoutTypeIsRepairedAndEmbedded(t *testing.T) {
	d, id := dictFontDoc(t, pdfDict{"/Subtype": pdfName("/TrueType"), "/BaseFont": pdfName("/Arial")}, "BT /F1 12 Tf (A) Tj ET")
	rep := d.ValidatePDFA(PDFA2B)
	if !pdfaHasRule(rep, "FONT_NO_TYPE") || !pdfaHasRule(rep, "FONT_NOT_EMBEDDED") {
		t.Errorf("not reported: %+v", rep.Issues)
	}
	rep, err := d.ConvertToPDFA(PDFA2B)
	if err != nil {
		t.Fatal(err)
	}
	if pdfaHasRule(rep, "FONT_NO_TYPE") || pdfaHasRule(rep, "FONT_NOT_EMBEDDED") {
		t.Errorf("not repaired: %+v", rep.Issues)
	}
	if dictGetName(d.objects[id].Value.(pdfDict), "/Type") != "/Font" {
		t.Error("/Type not set")
	}
}

func type0FontDoc(t *testing.T, encoding pdfValue, supplement int) (*Document, pdfDict) {
	t.Helper()
	d := NewDocument(300, 200)
	cid := pdfDict{"/Type": pdfName("/Font"), "/Subtype": pdfName("/CIDFontType2"), "/BaseFont": pdfName("/X"),
		"/CIDSystemInfo": pdfDict{"/Registry": "Adobe", "/Ordering": "Japan1", "/Supplement": supplement}}
	cidID := d.addObject(cid)
	font := d.addObject(pdfDict{"/Type": pdfName("/Font"), "/Subtype": pdfName("/Type0"), "/BaseFont": pdfName("/X"),
		"/Encoding": encoding, "/DescendantFonts": pdfArray{pdfRef{Num: cidID}}})
	p, _ := d.Page(1)
	pd := p.pageObj().Value.(pdfDict)
	pd["/Resources"] = pdfDict{"/Font": pdfDict{"/F1": pdfRef{Num: font}}}
	pd["/Contents"] = pdfRef{Num: d.addObject(buildFlateStream([]byte("BT /F1 12 Tf <0041> Tj ET")))}
	return d, cid
}

func fontIssueRules(is []fontIssue) map[string]bool {
	out := map[string]bool{}
	for _, i := range is {
		out[i.rule] = true
	}
	return out
}

func TestCIDSystemInfoRule(t *testing.T) {
	cm := predefinedCMap("UniJIS-UCS2-H")
	if cm == nil || cm.ordering != "Japan1" || cm.registry != "Adobe" {
		t.Fatalf("CMap system info: %+v", cm)
	}
	d, _ := type0FontDoc(t, pdfName("/UniJIS-UCS2-H"), cm.supplement)
	if fontIssueRules(d.fontDictIssues(2))["CIDSYSTEMINFO_MISMATCH"] {
		t.Error("equal supplements reported")
	}
	// A later supplement than the CMap's fails PDF/A-2 only, and is repairable.
	d, cid := type0FontDoc(t, pdfName("/UniJIS-UCS2-H"), cm.supplement+1)
	if !fontIssueRules(d.fontDictIssues(2))["CIDSYSTEMINFO_MISMATCH"] {
		t.Error("later supplement not reported for part 2")
	}
	if fontIssueRules(d.fontDictIssues(1))["CIDSYSTEMINFO_MISMATCH"] {
		t.Error("PDF/A-1 does not compare supplements")
	}
	d.fixFontDicts(2)
	info, _ := resolveRefToDict(d.objects, cid["/CIDSystemInfo"])
	if dictGetInt(info, "/Supplement") != cm.supplement {
		t.Errorf("supplement not lowered: %v", info)
	}
	if fontIssueRules(d.fontDictIssues(2))["CIDSYSTEMINFO_MISMATCH"] {
		t.Error("still reported after the repair")
	}
	// A different ordering is a real mismatch, which nothing here can repair.
	d, cid = type0FontDoc(t, pdfName("/UniGB-UCS2-H"), 0)
	if !fontIssueRules(d.fontDictIssues(1))["CIDSYSTEMINFO_MISMATCH"] {
		t.Error("different ordering not reported")
	}
	d.fixFontDicts(2)
	if info, _ := resolveRefToDict(d.objects, cid["/CIDSystemInfo"]); info["/Ordering"] != "Japan1" {
		t.Errorf("ordering changed: %v", info)
	}
	// Identity matches anything.
	d, _ = type0FontDoc(t, pdfName("/Identity-H"), 9)
	if r := fontIssueRules(d.fontDictIssues(1)); r["CIDSYSTEMINFO_MISMATCH"] || r["CMAP_NOT_EMBEDDED"] {
		t.Errorf("Identity reported: %v", r)
	}
}

func TestPDFA1EmbedsPredefinedCMaps(t *testing.T) {
	// A -HW-H CMap builds on its -H sibling, which has to come along.
	d, _ := type0FontDoc(t, pdfName("/UniJIS-UCS2-HW-H"), 0)
	if !fontIssueRules(d.fontDictIssues(1))["CMAP_NOT_EMBEDDED"] {
		t.Fatal("not reported")
	}
	if fontIssueRules(d.fontDictIssues(2))["CMAP_NOT_EMBEDDED"] {
		t.Error("PDF/A-2 allows predefined CMaps")
	}
	before := resolveFontOfFirst(t, d).cidCMap
	_, wantCID, _ := before.next([]byte{0x30, 0x42})
	d.embedPredefinedCMaps()
	if fontIssueRules(d.fontDictIssues(1))["CMAP_NOT_EMBEDDED"] {
		t.Error("still reported")
	}
	var buf bytes.Buffer
	if _, err := d.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	re, err := OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	font := firstFontDict(t, re)
	st, ok := resolveRef(re.objects, font["/Encoding"]).(*pdfStream)
	if !ok {
		t.Fatalf("/Encoding = %v", font["/Encoding"])
	}
	if dictGetName(st.Dict, "/CMapName") != "/UniJIS-UCS2-HW-H" || dictGetName(st.Dict, "/Type") != "/CMap" {
		t.Errorf("CMap dictionary: %v", st.Dict)
	}
	parent, ok := resolveRef(re.objects, st.Dict["/UseCMap"]).(*pdfStream)
	if !ok || dictGetName(parent.Dict, "/CMapName") != "/UniJIS-UCS2-H" {
		t.Errorf("/UseCMap = %v", st.Dict["/UseCMap"])
	}
	info, _ := resolveRefToDict(re.objects, st.Dict["/CIDSystemInfo"])
	if info["/Ordering"] != "Japan1" {
		t.Errorf("/CIDSystemInfo = %v", info)
	}
	// Reading it back gives the same CIDs.
	if _, got, _ := resolveFontOfFirst(t, re).cidCMap.next([]byte{0x30, 0x42}); got != wantCID || wantCID == 0 {
		t.Errorf("CID %d, want %d", got, wantCID)
	}
}

func firstFontDict(t *testing.T, d *Document) pdfDict {
	t.Helper()
	p, _ := d.Page(1)
	res := p.pageResources()
	fonts, _ := resolveRefToDict(d.objects, res["/Font"])
	font, ok := resolveRefToDict(d.objects, fonts["/F1"])
	if !ok {
		t.Fatal("no font")
	}
	return font
}

func resolveFontOfFirst(t *testing.T, d *Document) fontInfo {
	t.Helper()
	return resolveFont(d.objects, firstFontDict(t, d))
}

func TestTrueTypeEncodingRules(t *testing.T) {
	font := func(flags int, enc pdfValue) []fontIssue {
		d := simpleFontDoc(t, enc, "BT /F1 12 Tf (A) Tj ET")
		f := firstFontDict(t, d)
		desc, _ := resolveRefToDict(d.objects, f["/FontDescriptor"])
		desc["/Flags"] = flags
		return d.simpleFontIssues(f, 1)
	}
	win := pdfName("/WinAnsiEncoding")
	if r := fontIssueRules(font(32, win)); r["TRUETYPE_ENCODING"] {
		t.Error("a plain encoding reported")
	}
	if r := fontIssueRules(font(32, pdfDict{"/BaseEncoding": win})); r["TRUETYPE_ENCODING"] {
		t.Error("a base encoding with no differences reported")
	}
	if r := fontIssueRules(font(32, pdfDict{"/BaseEncoding": win, "/Differences": pdfArray{65, pdfName("/B")}})); !r["TRUETYPE_ENCODING"] {
		t.Error("/Differences not reported for PDF/A-1")
	}
	if r := fontIssueRules(font(32, nil)); !r["TRUETYPE_ENCODING"] {
		t.Error("no encoding on a non-symbolic font not reported")
	}
	if r := fontIssueRules(font(4, win)); !r["TRUETYPE_ENCODING"] {
		t.Error("an encoding on a symbolic font not reported")
	}
	// Only PDF/A-1 asks for this much of a non-symbolic font.
	d := simpleFontDoc(t, pdfDict{"/BaseEncoding": win, "/Differences": pdfArray{65, pdfName("/B")}}, "BT /F1 12 Tf (A) Tj ET")
	if r := fontIssueRules(d.simpleFontIssues(firstFontDict(t, d), 2)); r["TRUETYPE_ENCODING"] {
		t.Error("part 2 reported /Differences")
	}
	// DejaVu has several cmap subtables, too many for a symbolic font in PDF/A-1.
	if r := fontIssueRules(font(4, nil)); !r["TRUETYPE_CMAPS"] || r["TRUETYPE_ENCODING"] {
		t.Errorf("symbolic with no encoding: %v", r)
	}
	if r := fontIssueRules(font(32, win)); r["TRUETYPE_CMAPS"] {
		t.Error("the cmap count is only asked of a symbolic font")
	}
}

func type1SubsetDoc(t *testing.T, charset any) (*Document, pdfDict) {
	t.Helper()
	d := NewDocument(300, 200)
	file := d.addObject(buildFlateStream(buildMinimalType1()))
	desc := pdfDict{"/Type": pdfName("/FontDescriptor"), "/FontName": pdfName("/ABCDEF+T"), "/Flags": 32, "/FontFile": pdfRef{Num: file}}
	if charset != nil {
		desc["/CharSet"] = charset
	}
	descID := d.addObject(desc)
	font := d.addObject(pdfDict{"/Type": pdfName("/Font"), "/Subtype": pdfName("/Type1"), "/BaseFont": pdfName("/ABCDEF+T"), "/FontDescriptor": pdfRef{Num: descID}})
	p, _ := d.Page(1)
	pd := p.pageObj().Value.(pdfDict)
	pd["/Resources"] = pdfDict{"/Font": pdfDict{"/F1": pdfRef{Num: font}}}
	pd["/Contents"] = pdfRef{Num: d.addObject(buildFlateStream([]byte("BT /F1 12 Tf (A) Tj ET")))}
	return d, desc
}

func TestCharSetRule(t *testing.T) {
	names := func(d *Document, desc pdfDict) []string { return d.programGlyphNames(desc) }
	d, desc := type1SubsetDoc(t, nil)
	all := names(d, desc)
	if len(all) == 0 {
		t.Fatal("no glyph names from the program")
	}
	if !fontIssueRules(d.simpleFontIssues(firstFontDict(t, d), 1))["CHARSET_INCOMPLETE"] {
		t.Error("PDF/A-1 asks a subset for /CharSet")
	}
	if fontIssueRules(d.simpleFontIssues(firstFontDict(t, d), 2))["CHARSET_INCOMPLETE"] {
		t.Error("later parts do not ask for one")
	}
	d.fixFontDicts(1)
	cs, _ := desc["/CharSet"].(string)
	if cs != "/"+strings.Join(all, "/") {
		t.Errorf("/CharSet = %q", cs)
	}
	if fontIssueRules(d.simpleFontIssues(firstFontDict(t, d), 1))["CHARSET_INCOMPLETE"] {
		t.Error("still reported")
	}
	// One that is present but wrong is wrong in any part, and repaired in any.
	d, desc = type1SubsetDoc(t, "/nonesuch")
	if !fontIssueRules(d.simpleFontIssues(firstFontDict(t, d), 2))["CHARSET_INCOMPLETE"] {
		t.Error("a wrong /CharSet not reported")
	}
	d.fixFontDicts(2)
	if desc["/CharSet"] == "/nonesuch" {
		t.Error("a wrong /CharSet not repaired")
	}
	// Part 2 adds none to a subset that has none.
	d, desc = type1SubsetDoc(t, nil)
	d.fixFontDicts(2)
	if _, has := desc["/CharSet"]; has {
		t.Error("part 2 needs no /CharSet")
	}
	// A font that is not a subset is not asked.
	d, _ = type1SubsetDoc(t, nil)
	f := firstFontDict(t, d)
	f["/BaseFont"] = pdfName("/T")
	if fontIssueRules(d.simpleFontIssues(f, 1))["CHARSET_INCOMPLETE"] {
		t.Error("a whole font reported")
	}
}

func selfSigned(t *testing.T) (*x509.Certificate, *ecdsa.PrivateKey) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "t"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour)}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return cert, key
}

func TestSignatureStaleRule(t *testing.T) {
	d := NewDocument(300, 200)
	cert, key := selfSigned(t)
	if err := d.Sign(SignOptions{Certificate: cert, PrivateKey: key}); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if _, err := d.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	re, err := OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if pdfaHasRule(re.ValidatePDFA(PDFA2B), "SIGNATURE_STALE") {
		t.Error("a signature that covers its file reported")
	}
	// A revision appended after it leaves the signature short of the file's end.
	grown, err := OpenStream(bytes.NewReader(append(buf.Bytes(), "\n% later\n"...)))
	if err != nil {
		t.Fatal(err)
	}
	if !pdfaHasRule(grown.ValidatePDFA(PDFA2B), "SIGNATURE_STALE") {
		t.Error("a signature short of the end of the file not reported")
	}
	rep, err := re.ConvertToPDFA(PDFA2B)
	if err != nil {
		t.Fatal(err)
	}
	if !pdfaHasRule(rep, "SIGNATURE_STALE") {
		t.Errorf("a conversion breaks the signature, which went unreported: %+v", rep.Issues)
	}
	if pdfaHasRule(re.ValidatePDFA(PDFA1B), "SIGNATURE_STALE") {
		t.Error("PDF/A-1 has no such rule")
	}
	if re.ValidatePDFA(PDFA2B).Conformant {
		t.Error("not conformant with a stale signature")
	}
}

func TestFilespecNames(t *testing.T) {
	d := NewDocument(300, 200)
	st := d.addObject(buildFlateStream([]byte("x")))
	fs := pdfDict{"/Type": pdfName("/Filespec"), "/F": "a.txt", "/EF": pdfDict{"/F": pdfRef{Num: st}}}
	d.addObject(fs)
	if rule, _ := d.sharedConformanceIssues(); rule != "FILESPEC_NO_UF" {
		t.Fatalf("rule = %q", rule)
	}
	d.fixFilespecNames()
	if fs["/UF"] != "a.txt" {
		t.Errorf("/UF = %v", fs["/UF"])
	}
	if rule, _ := d.sharedConformanceIssues(); rule == "FILESPEC_NO_UF" {
		t.Error("still reported")
	}
	other := pdfDict{"/UF": "b.txt", "/EF": pdfDict{"/F": pdfRef{Num: st}}}
	d.addObject(other)
	d.fixFilespecNames()
	if other["/F"] != "b.txt" {
		t.Errorf("/F = %v", other["/F"])
	}
	// A specification embedding nothing is not asked.
	d = NewDocument(300, 200)
	d.addObject(pdfDict{"/Type": pdfName("/Filespec"), "/F": "a.txt"})
	if rule, _ := d.sharedConformanceIssues(); rule == "FILESPEC_NO_UF" {
		t.Error("a specification with no /EF reported")
	}
}

func TestAttachedFileMustBePDFAInPart2(t *testing.T) {
	d := NewDocument(300, 200)
	if _, err := d.EmbeddedFiles().AddFromStream("a.txt", strings.NewReader("x")); err != nil {
		t.Fatal(err)
	}
	if !pdfaHasRule(d.ValidatePDFA(PDFA2B), "EMBEDDED_FILE_NOT_PDFA") {
		t.Error("a text attachment not reported for PDF/A-2")
	}
	if pdfaHasRule(d.ValidatePDFA(PDFA3B), "EMBEDDED_FILE_NOT_PDFA") {
		t.Error("PDF/A-3 allows any file")
	}
	inner := NewDocument(300, 200)
	if _, err := inner.ConvertToPDFA(PDFA2B); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if _, err := inner.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	d = NewDocument(300, 200)
	if _, err := d.EmbeddedFiles().AddFromStream("a.pdf", bytes.NewReader(buf.Bytes())); err != nil {
		t.Fatal(err)
	}
	if pdfaHasRule(d.ValidatePDFA(PDFA2B), "EMBEDDED_FILE_NOT_PDFA") {
		t.Error("an attached PDF/A-2 reported")
	}
	if attachmentClaimsPDFA12(mustBytes(t, NewDocument(300, 200))) {
		t.Error("a plain PDF taken for PDF/A")
	}
}

func mustBytes(t *testing.T, d *Document) []byte {
	t.Helper()
	var buf bytes.Buffer
	if _, err := d.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestNumberLimits(t *testing.T) {
	d := NewDocument(300, 200)
	nested := pdfArray{4294967076}
	desc := pdfDict{"/Descent": 4294967076, "/Nested": nested, "/Ok": 7}
	d.addObject(desc)
	huge := pdfDict{"/V": int64(1) << 40}
	d.addObject(huge)
	if !pdfaHasRule(d.ValidatePDFA(PDFA2B), "NUMBER_OUT_OF_RANGE") {
		t.Fatal("not reported")
	}
	d.fixOutOfRangeIntegers()
	if desc["/Descent"] != -220 || nested[0] != -220 || desc["/Ok"] != 7 {
		t.Errorf("%v", desc)
	}
	if huge["/V"] != int64(1)<<40 {
		t.Error("a value that is no unsigned 32-bit number left alone")
	}
	if !pdfaHasRule(d.ValidatePDFA(PDFA2B), "NUMBER_OUT_OF_RANGE") {
		t.Error("the rest still counts")
	}
	delete(huge, "/V")
	if pdfaHasRule(d.ValidatePDFA(PDFA2B), "NUMBER_OUT_OF_RANGE") {
		t.Error("in-range numbers reported")
	}
	// Reals: PDF/A-1 only, in dictionaries and content alike.
	d = NewDocument(300, 200)
	real := pdfDict{"/R": 40000.5}
	id := d.addObject(real)
	if !pdfaHasRule(d.ValidatePDFA(PDFA1B), "NUMBER_OUT_OF_RANGE") {
		t.Error("a real in a dictionary not reported")
	}
	if pdfaHasRule(d.ValidatePDFA(PDFA2B), "NUMBER_OUT_OF_RANGE") {
		t.Error("PDF/A-2 has a far higher limit for reals")
	}
	delete(d.objects, id)
	p, _ := d.Page(1)
	if err := p.appendToContentStream([]byte("0 0 m 40000.5 0 l S\n")); err != nil {
		t.Fatal(err)
	}
	if !pdfaHasRule(d.ValidatePDFA(PDFA1B), "NUMBER_OUT_OF_RANGE") {
		t.Error("a real in content not reported")
	}
}

func TestNameNotUTF8(t *testing.T) {
	d := NewDocument(300, 200)
	p, _ := d.Page(1)
	pd := p.pageObj().Value.(pdfDict)
	// Elsewhere a Shift-JIS name is common and veraPDF does not mind.
	d.addObject(pdfDict{"/FontName": pdfName("/\x82l\x82r")})
	if pdfaHasRule(d.ValidatePDFA(PDFA2B), "NAME_NOT_UTF8") {
		t.Error("a name no content stream refers to reported")
	}
	pd["/Resources"] = pdfDict{"/Font": pdfDict{"/\x82l": pdfDict{}}}
	if !pdfaHasRule(d.ValidatePDFA(PDFA2B), "NAME_NOT_UTF8") {
		t.Error("a resource name not reported")
	}
	if pdfaHasRule(d.ValidatePDFA(PDFA1B), "NAME_NOT_UTF8") {
		t.Error("PDF/A-1 has no such rule")
	}
	font := d.addObject(pdfDict{"/Type": pdfName("/Font"), "/Subtype": pdfName("/TrueType"), "/BaseFont": pdfName("/\x82l\x82r")})
	pd["/Resources"] = pdfDict{"/Font": pdfDict{"/F1": pdfRef{Num: font}}}
	pd["/Contents"] = pdfRef{Num: d.addObject(buildFlateStream([]byte("BT /F1 12 Tf (A) Tj ET")))}
	if !pdfaHasRule(d.ValidatePDFA(PDFA2B), "NAME_NOT_UTF8") {
		t.Error("the name of a font in use not reported")
	}
}

func TestAnnotationTypeRule(t *testing.T) {
	annot := func(sub string) *Document {
		d := NewDocument(300, 200)
		p, _ := d.Page(1)
		pd := p.pageObj().Value.(pdfDict)
		pd["/Annots"] = pdfArray{pdfDict{"/Subtype": pdfName(sub), "/Rect": pdfArray{0, 0, 1, 1}}}
		return d
	}
	for _, tc := range []struct {
		sub    string
		format PDFAFormat
		want   bool
	}{
		{"/FileAttachment", PDFA1B, true},
		{"/FileAttachment", PDFA2B, false},
		{"/Polygon", PDFA1B, true},
		{"/Polygon", PDFA2B, false},
		{"/Square", PDFA1B, false},
		{"/Widget", PDFA1B, false},
		{"/Movie", PDFA2B, true},
		{"/Sound", PDFA3B, true},
		{"/3D", PDFA2B, true},
		{"/Screen", PDFA2B, true},
	} {
		if got := pdfaHasRule(annot(tc.sub).ValidatePDFA(tc.format), "ANNOTATION_TYPE_FORBIDDEN"); got != tc.want {
			t.Errorf("%s in %v: %v", tc.sub, tc.format, got)
		}
	}
}

// An ExtGState written straight into a page's resources is still transparency, for
// the rule and for the flattening that repairs it.
func TestDirectExtGStateTransparency(t *testing.T) {
	d := NewDocument(300, 200)
	p, _ := d.Page(1)
	pd := p.pageObj().Value.(pdfDict)
	pd["/Resources"] = pdfDict{"/ExtGState": pdfDict{"/G0": pdfDict{"/CA": 0}}}
	if !pdfaHasRule(d.ValidatePDFA(PDFA1B), "TRANSPARENCY") {
		t.Error("not reported")
	}
	need, err := pageNeedsTransparencyFlatten(d.objects, pd)
	if err != nil || !need {
		t.Errorf("flatten: %v %v", need, err)
	}
	pd["/Resources"] = pdfDict{"/ExtGState": pdfDict{"/G0": pdfArray{pdfDict{"/ca": 0.5}}}}
	if !pdfaValueHasTransparency(d.objects, pd["/Resources"]) {
		t.Error("inside an array")
	}
	pd["/Resources"] = pdfDict{"/ExtGState": pdfDict{"/G0": pdfDict{"/CA": 1}}}
	if pdfaHasRule(d.ValidatePDFA(PDFA1B), "TRANSPARENCY") {
		t.Error("opaque reported")
	}
}

// substituted returns a non-embedded simple font's dictionary after conversion.
func substituted(t *testing.T, font pdfDict, content string) (*Document, *Document, pdfDict) {
	t.Helper()
	d, _ := dictFontDoc(t, font, content)
	before := mustBytes(t, d)
	if _, err := d.ConvertToPDFA(PDFA2B); err != nil {
		t.Fatal(err)
	}
	re, err := OpenStream(bytes.NewReader(mustBytes(t, d)))
	if err != nil {
		t.Fatal(err)
	}
	orig, err := OpenStream(bytes.NewReader(before))
	if err != nil {
		t.Fatal(err)
	}
	return orig, re, firstFontDict(t, re)
}

// Text is laid out by /Widths. Replacing them with the substitute's own moved it:
// a font wider than Times ran into what came after it.
func TestSubstituteKeepsTheDocumentsWidths(t *testing.T) {
	_, re, font := substituted(t, pdfDict{
		"/Type": pdfName("/Font"), "/Subtype": pdfName("/TrueType"), "/BaseFont": pdfName("/Garamond"),
		"/Encoding": pdfName("/WinAnsiEncoding"), "/FirstChar": 65, "/LastChar": 66, "/Widths": pdfArray{900, 300},
	}, "BT /F1 12 Tf (AB) Tj ET")
	arr, _ := resolveRefToArray(re.objects, font["/Widths"])
	if w := operandFloat(arr[65]); w != 900 {
		t.Errorf("/Widths for A = %v, want 900", w)
	}
	if w := operandFloat(arr[66]); w != 300 {
		t.Errorf("/Widths for B = %v, want 300", w)
	}
	// The program agrees, as PDF/A requires, so the outlines keep their places too.
	if r := glyphRules(re); r["FONT_WIDTH_MISMATCH"] || r["GLYPH_MISSING"] {
		t.Errorf("%v", r)
	}
	gf := re.glyphFontsInUse()[0]
	if w, _ := gf.rf.programWidth(gf.rf.gid(65)); w < 899.5 || w > 900.5 {
		t.Errorf("program width for A = %v", w)
	}
}

// Only what is drawn binds the program: a width for a code nothing shows has no say.
func TestSubstituteWidthsOnlyForCodesDrawn(t *testing.T) {
	_, re, _ := substituted(t, pdfDict{
		"/Type": pdfName("/Font"), "/Subtype": pdfName("/TrueType"), "/BaseFont": pdfName("/Garamond"),
		"/Encoding": pdfName("/WinAnsiEncoding"), "/FirstChar": 65, "/LastChar": 66, "/Widths": pdfArray{900, 300},
	}, "BT /F1 12 Tf (A) Tj ET")
	gf := re.glyphFontsInUse()[0]
	if w, _ := gf.rf.programWidth(gf.rf.prog.glyphID('B')); w == 300 {
		t.Error("B's width was taken though nothing draws it")
	}
}

func TestSetAdvancesKeepsEverythingElse(t *testing.T) {
	data, err := os.ReadFile(glyphTestFont)
	if err != nil {
		t.Skip(err)
	}
	f, err := parseTTF(data)
	if err != nil {
		t.Fatal(err)
	}
	gid := f.glyphID('A')
	last := uint16(len(f.glyphWidths) - 1) // past numberOfHMetrics in many fonts
	fitted := fitAdvances(f, map[uint16]float64{gid: 123, last: 77})
	if fitted == f {
		t.Fatal("nothing changed")
	}
	near := func(a uint16, w float64) bool {
		got := float64(a) * 1000 / float64(fitted.unitsPerEm)
		return got > w-0.5 && got < w+0.5
	}
	if !near(fitted.glyphWidths[gid], 123) || !near(fitted.glyphWidths[last], 77) {
		t.Errorf("advances: %d %d", fitted.glyphWidths[gid], fitted.glyphWidths[last])
	}
	for _, r := range []rune{'B', 'z', ' '} {
		if g := f.glyphID(r); fitted.glyphWidths[g] != f.glyphWidths[g] || fitted.glyphID(r) != g {
			t.Errorf("%c changed", r)
		}
	}
	if fitted.postScriptName != f.postScriptName || len(fitted.glyphWidths) != len(f.glyphWidths) {
		t.Errorf("font changed: %q %d", fitted.postScriptName, len(fitted.glyphWidths))
	}
	if len(fitted.glyphContours(gid)) != len(f.glyphContours(gid)) {
		t.Error("outlines changed")
	}
	// The left side bearings stay with their glyphs.
	lsb := func(tt *ttfFont, g uint16) [2]byte {
		dir := tableDir(tt.data)
		b := tableSlice(tt.data, dir, "hmtx")
		n := int(tt.numOfLongHorMetrics)
		if int(g) < n {
			return [2]byte{b[int(g)*4+2], b[int(g)*4+3]}
		}
		return [2]byte{b[n*4+(int(g)-n)*2], b[n*4+(int(g)-n)*2+1]}
	}
	if lsb(f, gid) != lsb(fitted, gid) || lsb(f, last) != lsb(fitted, last) {
		t.Error("left side bearings moved")
	}
	if fitAdvances(f, map[uint16]float64{gid: float64(f.glyphWidths[gid]) * 1000 / float64(f.unitsPerEm)}) != f {
		t.Error("an advance that already agrees rebuilt the font")
	}
}

// With no /Encoding a Latin font means StandardEncoding, where 0xAE is "fi" and the
// quote at 0x27 curls. Writing WinAnsi changed what those codes draw.
func TestSubstituteKeepsStandardEncoding(t *testing.T) {
	orig, re, font := substituted(t, pdfDict{
		"/Type": pdfName("/Font"), "/Subtype": pdfName("/Type1"), "/BaseFont": pdfName("/Helvetica"),
	}, "BT /F1 12 Tf <AE> Tj ET")
	enc, ok := resolveRefToDict(re.objects, font["/Encoding"])
	if !ok {
		t.Fatalf("/Encoding = %v", font["/Encoding"])
	}
	if diffs, _ := enc["/Differences"].(pdfArray); len(diffs) != 2 || diffs[1] != pdfName("/fi") {
		t.Errorf("/Differences = %v", enc["/Differences"])
	}
	p1, _ := orig.Page(1)
	p2, _ := re.Page(1)
	a, _ := p1.ExtractText()
	b, _ := p2.ExtractText()
	if strings.TrimSpace(a) != "ﬁ" || strings.TrimSpace(b) != "ﬁ" {
		t.Errorf("text %q -> %q", a, b)
	}
	// Codes that mean the same in WinAnsi need no /Differences, which PDF/A-1 would
	// not accept on a TrueType font.
	_, _, font = substituted(t, pdfDict{
		"/Type": pdfName("/Font"), "/Subtype": pdfName("/Type1"), "/BaseFont": pdfName("/Helvetica"),
	}, "BT /F1 12 Tf (Hello) Tj ET")
	if font["/Encoding"] != pdfName("/WinAnsiEncoding") {
		t.Errorf("/Encoding = %v", font["/Encoding"])
	}
}

// /ToUnicode may be all that says what a font's codes mean.
func TestSubstituteKeepsToUnicode(t *testing.T) {
	cmap := `begincmap
1 begincodespacerange
<00> <FF>
endcodespacerange
1 beginbfchar
<41> <0416>
endbfchar
endcmap
`
	d := NewDocument(300, 200)
	tu := d.addObject(buildFlateStream([]byte(cmap)))
	font := d.addObject(pdfDict{"/Type": pdfName("/Font"), "/Subtype": pdfName("/TrueType"), "/BaseFont": pdfName("/Garamond"),
		"/Encoding": pdfName("/WinAnsiEncoding"), "/ToUnicode": pdfRef{Num: tu}})
	p, _ := d.Page(1)
	pd := p.pageObj().Value.(pdfDict)
	pd["/Resources"] = pdfDict{"/Font": pdfDict{"/F1": pdfRef{Num: font}}}
	pd["/Contents"] = pdfRef{Num: d.addObject(buildFlateStream([]byte("BT /F1 12 Tf (A) Tj ET")))}
	if _, err := d.ConvertToPDFA(PDFA2B); err != nil {
		t.Fatal(err)
	}
	re, err := OpenStream(bytes.NewReader(mustBytes(t, d)))
	if err != nil {
		t.Fatal(err)
	}
	rp, _ := re.Page(1)
	if text, _ := rp.ExtractText(); strings.TrimSpace(text) != "Ж" {
		t.Errorf("text = %q", text)
	}
}

// A symbol font embedded from an installed face loses its name's standard encoding,
// so it carries what that encoding said.
func TestSimpleToUnicode(t *testing.T) {
	var fi fontInfo
	if simpleToUnicode(fi) != nil {
		t.Error("an empty encoding says nothing")
	}
	fi.encoding[0xB7] = '•'
	fi.encoding[0x41] = 0xFFFD
	st := simpleToUnicode(fi)
	if st == nil {
		t.Fatal("nil")
	}
	toU, _ := parseCMapFull(decodedStreamData(st))
	if toU[0xB7] != '•' {
		t.Errorf("0xB7 -> %q", toU[0xB7])
	}
	if _, has := toU[0x41]; has {
		t.Error("U+FFFD written")
	}
	// A section holds at most 100 entries.
	for c := 0x20; c < 0xF0; c++ {
		fi.encoding[c] = rune(0x400 + c)
	}
	toU, _ = parseCMapFull(decodedStreamData(simpleToUnicode(fi)))
	if len(toU) != 0xF0-0x20 {
		t.Errorf("%d entries", len(toU))
	}
	if !strings.Contains(string(decodedStreamData(simpleToUnicode(fi))), "100 beginbfchar") {
		t.Error("sections not split")
	}
}

// Latin codes in a CJK font go through CIDs the ordering table leaves without
// Unicode: dropped from the subset, every one of them vanished from the page.
func TestConvertToPDFACompositeKeepsLatin(t *testing.T) {
	if fontRepo.findCJK(fontInfo{name: "/MS-Gothic"}, "Japan1") == nil {
		t.Skip("no CJK face installed")
	}
	d, _ := dictFontDoc(t, pdfDict{}, "")
	cid := d.addObject(pdfDict{"/Type": pdfName("/Font"), "/Subtype": pdfName("/CIDFontType2"), "/BaseFont": pdfName("/MS-Gothic"),
		"/CIDSystemInfo": pdfDict{"/Registry": "Adobe", "/Ordering": "Japan1", "/Supplement": 2}, "/DW": 1000})
	font := d.addObject(pdfDict{"/Type": pdfName("/Font"), "/Subtype": pdfName("/Type0"), "/BaseFont": pdfName("/MS-Gothic"),
		"/Encoding": pdfName("/90ms-RKSJ-H"), "/DescendantFonts": pdfArray{pdfRef{Num: cid}}})
	p, _ := d.Page(1)
	pd := p.pageObj().Value.(pdfDict)
	pd["/Resources"] = pdfDict{"/Font": pdfDict{"/F1": pdfRef{Num: font}}}
	pd["/Contents"] = pdfRef{Num: d.addObject(buildFlateStream([]byte("BT /F1 12 Tf (Latin) Tj ET")))}
	rep, err := d.ConvertToPDFA(PDFA2B)
	if err != nil {
		t.Fatal(err)
	}
	if pdfaHasRule(rep, "NOTDEF_GLYPH_USED") || pdfaHasRule(rep, "GLYPH_MISSING") {
		t.Errorf("Latin lost its glyphs: %+v", rep.Issues)
	}
}

// A composite substitute keeps the document's /W, as a simple one keeps /Widths.
func TestConvertToPDFACompositeKeepsWidths(t *testing.T) {
	if fontRepo.findCJK(fontInfo{name: "/SimSun"}, "GB1") == nil {
		t.Skip("no CJK face installed")
	}
	d := NewDocument(300, 200)
	cm := predefinedCMap("UniGB-UCS2-H")
	_, cidOf, _ := cm.next([]byte{0x4F, 0x60})
	cid := d.addObject(pdfDict{"/Type": pdfName("/Font"), "/Subtype": pdfName("/CIDFontType2"), "/BaseFont": pdfName("/SimSun"),
		"/CIDSystemInfo": pdfDict{"/Registry": "Adobe", "/Ordering": "GB1", "/Supplement": 2},
		"/W":             pdfArray{int(cidOf), pdfArray{700}}})
	font := d.addObject(pdfDict{"/Type": pdfName("/Font"), "/Subtype": pdfName("/Type0"), "/BaseFont": pdfName("/SimSun"),
		"/Encoding": pdfName("/UniGB-UCS2-H"), "/DescendantFonts": pdfArray{pdfRef{Num: cid}}})
	p, _ := d.Page(1)
	pd := p.pageObj().Value.(pdfDict)
	pd["/Resources"] = pdfDict{"/Font": pdfDict{"/F1": pdfRef{Num: font}}}
	pd["/Contents"] = pdfRef{Num: d.addObject(buildFlateStream([]byte("BT /F1 12 Tf <4F60> Tj ET")))}
	rep, err := d.ConvertToPDFA(PDFA2B)
	if err != nil {
		t.Fatal(err)
	}
	if pdfaHasRule(rep, "FONT_WIDTH_MISMATCH") {
		t.Errorf("%+v", rep.Issues)
	}
	re, err := OpenStream(bytes.NewReader(mustBytes(t, d)))
	if err != nil {
		t.Fatal(err)
	}
	fi := resolveFontOfFirst(t, re)
	if fi.cidWidths[cidOf] != 700 {
		t.Errorf("/W for the CID = %v, want 700", fi.cidWidths[cidOf])
	}
}

func TestNoUnicodeMappingRule(t *testing.T) {
	// A symbolic font with no encoding and no /ToUnicode says nothing.
	d := simpleFontDoc(t, nil, "BT /F1 12 Tf (A) Tj ET")
	gf := d.glyphFontsInUse()[0]
	if len(gf.unmapped) != 1 {
		t.Errorf("unmapped = %v", gf.unmapped)
	}
	for _, is := range d.fontGlyphIssues(false) {
		if is.rule == "NO_UNICODE_MAPPING" {
			t.Error("PDF/A does not ask for this")
		}
	}
	if !glyphRules(d)["NO_UNICODE_MAPPING"] {
		t.Error("not reported for PDF/UA")
	}
	if r := glyphRules(simpleFontDoc(t, pdfName("/WinAnsiEncoding"), "BT /F1 12 Tf (A) Tj ET")); r["NO_UNICODE_MAPPING"] {
		t.Error("an encoding gives the mapping")
	}
	if r := glyphRules(simpleFontDoc(t, pdfDict{"/Differences": pdfArray{65, pdfName("/Zhe")}}, "BT /F1 12 Tf (A) Tj ET")); r["NO_UNICODE_MAPPING"] && cffGlyphNameRune("Zhe") != 0 {
		t.Error("a glyph name gives the mapping")
	}
	// A Type1 program's own glyph name does as well.
	d, _ = type1SubsetDoc(t, nil)
	if d.glyphFontsInUse()[0].rf == nil {
		t.Fatal("no program")
	}
	if r := glyphRules(d); r["NO_UNICODE_MAPPING"] {
		t.Errorf("Type1 built-in encoding: %v", r)
	}
}

// A CMap this library does not carry stays what the report says it is.
func TestPDFA1CannotEmbedAnUnknownCMap(t *testing.T) {
	d, _ := type0FontDoc(t, pdfName("/NoSuch-H"), 0)
	d.embedPredefinedCMaps()
	if !fontIssueRules(d.fontDictIssues(1))["CMAP_NOT_EMBEDDED"] {
		t.Error("not reported")
	}
}
