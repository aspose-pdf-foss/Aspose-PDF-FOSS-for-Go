// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"image"
	"os"
	"strings"
	"testing"
)

func ttRender(t *testing.T, p *Page) *image.RGBA {
	t.Helper()
	img, err := p.RenderImage(RenderOptions{DPI: 72})
	if err != nil {
		t.Fatal(err)
	}
	return img.(*image.RGBA)
}

// ttPageFont returns the only font of the page's resources.
func ttPageFont(t *testing.T, d *Document, p *Page) pdfDict {
	t.Helper()
	fonts, ok := resolveRefToDict(d.objects, p.pageResources()["/Font"])
	if !ok || len(fonts) != 1 {
		t.Fatalf("fonts %v", fonts)
	}
	for _, v := range fonts {
		font, ok := resolveRefToDict(d.objects, v)
		if !ok {
			t.Fatalf("font %v", v)
		}
		return font
	}
	return nil
}

func ttDifferences() pdfDict {
	return pdfDict{"/BaseEncoding": pdfName("/WinAnsiEncoding"), "/Differences": pdfArray{65, pdfName("/Z")}}
}

func ttTextDoc(t *testing.T, text string, enc pdfValue) (*Document, *Page) {
	t.Helper()
	d := NewDocument(300, 200)
	p, _ := d.Page(1)
	if err := p.AddText(text, TextStyle{Font: FontHelvetica, Size: 30}, Rectangle{LLX: 20, LLY: 100, URX: 280, URY: 180}); err != nil {
		t.Fatal(err)
	}
	ttPageFont(t, d, p)["/Encoding"] = enc
	return d, p
}

// A Differences encoding is out in PDF/A-1, whatever the font draws: the font turns
// symbolic and keeps drawing and meaning the same.
func TestPDFA1TrueTypeDifferences(t *testing.T) {
	d, p := ttTextDoc(t, "ABAB", ttDifferences())
	d.embedStandardFonts() // what the page looks like is the substitute's business
	before := ttRender(t, p)
	textBefore, _ := p.ExtractText()
	if !strings.Contains(textBefore, "ZBZB") {
		t.Fatalf("fixture text %q", textBefore)
	}

	rep, err := d.ConvertToPDFA(PDFA1B)
	if err != nil {
		t.Fatal(err)
	}
	for _, is := range rep.Issues {
		if strings.HasPrefix(is.Rule, "TRUETYPE") {
			t.Errorf("%s: %s", is.Rule, is.Message)
		}
	}
	font := ttPageFont(t, d, p)
	if _, has := font["/Encoding"]; has {
		t.Errorf("/Encoding stayed: %v", font["/Encoding"])
	}
	descriptor, _ := resolveRefToDict(d.objects, font["/FontDescriptor"])
	if f := dictGetInt(descriptor, "/Flags"); f&4 == 0 || f&32 != 0 {
		t.Errorf("flags %d", f)
	}
	program, _ := resolveRef(d.objects, descriptor["/FontFile2"]).(*pdfStream)
	if n := ttfCmapCount(decodedStreamData(program)); n != 1 {
		t.Errorf("%d cmaps", n)
	}
	// In the 0xF000 page, where a reader looks in a (3,0) cmap.
	prog, err := parseTTF(decodedStreamData(program))
	if err != nil {
		t.Fatal(err)
	}
	if prog.codeToGlyph[0xF000|'A'] == 0 {
		t.Errorf("code A reaches glyph %d", prog.codeToGlyph[0xF000|'A'])
	}

	check := func(what string, p *Page) {
		t.Helper()
		if !bytes.Equal(ttRender(t, p).Pix, before.Pix) {
			t.Errorf("%s: the page draws differently", what)
		}
		if text, _ := p.ExtractText(); text != textBefore {
			t.Errorf("%s: text %q, was %q", what, text, textBefore)
		}
	}
	check("converted", p)
	var buf bytes.Buffer
	if _, err := d.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	d2, err := OpenStream(&buf)
	if err != nil {
		t.Fatal(err)
	}
	p2, _ := d2.Page(1)
	check("reopened", p2)
}

// A plain encoding is fine as it is, and PDF/A-2 does not ask for any of this.
func TestPDFATrueTypeLeftAlone(t *testing.T) {
	d, p := ttTextDoc(t, "AB", pdfName("/WinAnsiEncoding"))
	if _, err := d.ConvertToPDFA(PDFA1B); err != nil {
		t.Fatal(err)
	}
	if ttPageFont(t, d, p)["/Encoding"] != pdfName("/WinAnsiEncoding") {
		t.Error("a plain encoding should stay")
	}
	d, p = ttTextDoc(t, "AB", ttDifferences())
	if _, err := d.ConvertToPDFA(PDFA2B); err != nil {
		t.Fatal(err)
	}
	if _, has := ttPageFont(t, d, p)["/Encoding"]; !has {
		t.Error("PDF/A-2 allows /Differences")
	}
}

// A symbolic font may have one cmap only; the code keeps reaching the same glyph, and
// a font sharing the descriptor or program does not change with it.
func TestPDFA1TrueTypeSymbolicCmaps(t *testing.T) {
	data, err := os.ReadFile(glyphTestFont)
	if err != nil {
		t.Skip(err)
	}
	if n := ttfCmapCount(data); n < 2 {
		t.Skipf("test font has %d cmaps", n)
	}
	d := NewDocument(300, 200)
	p, _ := d.Page(1)
	progID := d.addObject(buildFontFile2StreamBytes(data))
	descriptor := pdfDict{"/Type": pdfName("/FontDescriptor"), "/FontName": pdfName("/T"), "/Flags": 4,
		"/FontBBox": pdfArray{0, 0, 1000, 1000}, "/ItalicAngle": 0, "/Ascent": 800, "/Descent": -200, "/CapHeight": 700, "/StemV": 80,
		"/FontFile2": pdfRef{Num: progID}}
	descID := d.addObject(descriptor)
	font := func() pdfRef {
		widths := make(pdfArray, 256)
		for i := range widths {
			widths[i] = 600
		}
		return pdfRef{Num: d.addObject(pdfDict{"/Type": pdfName("/Font"), "/Subtype": pdfName("/TrueType"), "/BaseFont": pdfName("/T"),
			"/FirstChar": 0, "/LastChar": 255, "/Widths": widths, "/FontDescriptor": pdfRef{Num: descID}})}
	}
	used, unused := font(), font()
	p.pageDict()["/Resources"] = pdfDict{"/Font": pdfDict{"/F": used}, "/Unused": unused}
	p.pageDict()["/Contents"] = pdfRef{Num: d.addObject(buildFlateStream([]byte("BT /F 30 Tf 20 100 Td (AB) Tj ET")))}

	before := ttRender(t, p)
	d.fixTrueTypeEncodings()
	if !bytes.Equal(ttRender(t, p).Pix, before.Pix) {
		t.Error("the page draws differently")
	}
	got, _ := resolveRefToDict(d.objects, ttPageFont(t, d, p)["/FontDescriptor"])
	program, _ := resolveRef(d.objects, got["/FontFile2"]).(*pdfStream)
	if n := ttfCmapCount(decodedStreamData(program)); n != 1 {
		t.Errorf("%d cmaps", n)
	}
	// The other font still points at what it had, which so stays in the document.
	other, _ := resolveRefToDict(d.objects, unused)
	if other["/FontDescriptor"] != (pdfRef{Num: descID}) || d.objects[progID] == nil {
		t.Errorf("the other font changed: %v", other["/FontDescriptor"])
	}
	if got["/FontFile2"] == (pdfRef{Num: progID}) {
		t.Error("the shared program was reused")
	}
	if dictGetInt(descriptor, "/Flags") != 4 || descriptor["/FontFile2"] != (pdfRef{Num: progID}) {
		t.Errorf("the shared descriptor changed: %v", descriptor)
	}
}

// With nothing else pointing at it, the old program goes: it is large, and of no use.
func TestPDFA1TrueTypeDropsOldProgram(t *testing.T) {
	d, p := ttTextDoc(t, "AB", ttDifferences())
	d.embedStandardFonts()
	descriptor, _ := resolveRefToDict(d.objects, ttPageFont(t, d, p)["/FontDescriptor"])
	old := descriptor["/FontFile2"].(pdfRef).Num
	d.fixTrueTypeEncodings()
	if d.objects[old] != nil {
		t.Error("the replaced program is still in the document")
	}
}

func TestSimpleToUnicodeMeanings(t *testing.T) {
	var fi fontInfo
	fi.encoding['A'] = 'a'
	fi.encoding['B'] = 'b'
	fi.encoding['C'] = 'c'
	fi.encoding['D'] = 0xFFFD
	fi.encoding['E'] = 'e'
	fi.toUnicode = map[uint16]rune{'B': 'x', 'E': 'y'}
	fi.toUnicodeSeq = map[uint16][]rune{'C': {'f', 'i'}, 'F': {0x1F600}}
	st := simpleToUnicode(fi)
	if st == nil {
		t.Fatal("no CMap")
	}
	got, seq := parseCMapFull(decodedStreamData(st))
	for c, r := range map[uint16]rune{'A': 'a', 'B': 'x', 'E': 'y', 'F': 0x1F600} {
		if got[c] != r {
			t.Errorf("code %q: %q, want %q", rune(c), got[c], r)
		}
	}
	if string(seq['C']) != "fi" {
		t.Errorf("sequence %q", string(seq['C']))
	}
	if _, has := got['D']; has {
		t.Error("U+FFFD means nothing and gets no entry")
	}
}

// Only a TrueType font is held to this; a program under another subtype is another
// rule's business.
func TestPDFA1TrueTypeOtherSubtype(t *testing.T) {
	d, p := ttTextDoc(t, "AB", ttDifferences())
	d.embedStandardFonts()
	font := ttPageFont(t, d, p)
	font["/Subtype"] = pdfName("/Type1")
	d.fixTrueTypeEncodings()
	if _, has := font["/Encoding"]; !has {
		t.Error("the encoding of a Type1 font was removed")
	}
}
