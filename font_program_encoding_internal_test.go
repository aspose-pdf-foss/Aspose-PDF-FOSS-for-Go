// SPDX-License-Identifier: MIT

package asposepdf

import (
	"os"
	"strings"
	"testing"
)

// symbolicTrueTypeDoc builds a page drawing text with an embedded TrueType
// font whose dictionary has neither /Encoding nor /ToUnicode, as Word writes
// its subsets (pdf-go-nw0h).
func symbolicTrueTypeDoc(t *testing.T, text string) *Document {
	t.Helper()
	ttf, err := os.ReadFile("testdata/DejaVuSans.ttf")
	if err != nil {
		t.Fatal(err)
	}
	d := NewDocument(300, 100)
	file := d.addObject(&pdfStream{Dict: pdfDict{"/Length1": len(ttf)}, Data: ttf, Decoded: true})
	desc := d.addObject(pdfDict{
		"/Type": pdfName("/FontDescriptor"), "/FontName": pdfName("/DejaVuSans"), "/Flags": 4,
		"/FontBBox": pdfArray{0, -200, 1000, 900}, "/ItalicAngle": 0, "/Ascent": 900, "/Descent": -200,
		"/CapHeight": 700, "/StemV": 80, "/FontFile2": pdfRef{Num: file},
	})
	widths := pdfArray{}
	for range 256 {
		widths = append(widths, 600)
	}
	font := d.addObject(pdfDict{
		"/Type": pdfName("/Font"), "/Subtype": pdfName("/TrueType"), "/BaseFont": pdfName("/DejaVuSans"),
		"/FirstChar": 0, "/LastChar": 255, "/Widths": widths, "/FontDescriptor": pdfRef{Num: desc},
	})
	p, _ := d.Page(1)
	p.pageDict()["/Resources"] = pdfDict{"/Font": pdfDict{"/F9": pdfRef{Num: font}}}
	if err := p.appendToContentStream([]byte("BT /F9 12 Tf 10 50 Td (" + text + ") Tj ET")); err != nil {
		t.Fatal(err)
	}
	return d
}

// TestProgramEncodingTrueTypeExtracts: the codes reach glyphs through the
// program's (1,0) cmap, and the glyphs reach Unicode through its (3,1) cmap.
func TestProgramEncodingTrueTypeExtracts(t *testing.T) {
	d := symbolicTrueTypeDoc(t, "Hello world")
	p, _ := d.Page(1)
	got, err := p.ExtractText()
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(got) != "Hello world" {
		t.Errorf("ExtractText = %q, want %q", got, "Hello world")
	}
	// PDF/A does not count a TrueType program's cmap as a Unicode mapping
	// (veraPDF agrees): reading text back from it must not change that.
	if !hasPDFARule(d.ValidatePDFA(PDFA2U), "NO_UNICODE_MAPPING") {
		t.Error("NO_UNICODE_MAPPING no longer reported")
	}
	// The font now counts as known, so text in it can be replaced.
	if n, err := p.ReplaceText("world", "there"); err != nil || n != 1 {
		t.Fatalf("ReplaceText = %d, %v", n, err)
	}
	got, _ = p.ExtractText()
	if !strings.Contains(got, "there") || strings.Contains(got, "world") {
		t.Errorf("after ReplaceText: %q", got)
	}
}

// TestProgramEncodingTrueTypeRules covers the TrueType fallbacks on their own.
func TestProgramEncodingTrueTypeRules(t *testing.T) {
	prog := &ttfFont{
		codeToGlyph: map[uint16]uint16{
			0x41: 5, 0xC4: 6, // (1,0) keys: A, and MacRoman 0xC4 (florin)
			0xF042: 7, 0xF0E9: 8, // (3,0) keys: printable ASCII B, and 0xE9
			0x43: 9,  // a key whose glyph a Unicode cmap names
			0x44: 10, // a glyph the Unicode cmap gives only a private-use code
		},
		runeToGlyph: map[rune]uint16{'Ç': 9, 'C': 9, 0xF043: 9, 0xF044: 10},
	}
	enc, ok := trueTypeProgramEncoding(prog)
	if !ok {
		t.Fatal("no encoding")
	}
	for code, want := range map[int]rune{0x41: 'A', 0xC4: 'ƒ', 0x42: 'B', 0xE9: 0, 0x43: 'C', 0x44: 'D'} {
		if got := enc[code]; got != want {
			t.Errorf("code %#x = %q, want %q", code, got, want)
		}
	}
	if _, ok := trueTypeProgramEncoding(&ttfFont{runeToGlyph: map[rune]uint16{'A': 1}}); ok {
		t.Error("a program with no code cmap gave an encoding")
	}
}

// TestProgramEncodingCFFAndType1: the built-in encoding names each glyph.
func TestProgramEncodingCFFAndType1(t *testing.T) {
	// CFF standard strings: SID 34 = "A", 66 = "a"; a custom SID names a
	// uniXXXX glyph.
	cff := &cffFont{
		simpleGID: map[uint16]uint16{0x41: 1, 0x61: 2, 0x80: 3},
		charset:   []uint16{0, 34, 66, uint16(len(cffStdStrings))},
		strings:   [][]byte{[]byte("uni20AC")},
	}
	enc, ok := encodingFromProgram(&renderFont{cff: cff})
	if !ok || enc[0x41] != 'A' || enc[0x61] != 'a' || enc[0x80] != '€' || enc[0x42] != '�' {
		t.Errorf("CFF: ok=%v A=%q a=%q 0x80=%q 0x42=%q", ok, enc[0x41], enc[0x61], enc[0x80], enc[0x42])
	}
	if _, ok := encodingFromProgram(&renderFont{cff: &cffFont{isCID: true}}); ok {
		t.Error("a CID-keyed CFF gave a simple encoding")
	}

	t1 := &type1Font{}
	t1.builtinEnc[0x31] = "one"
	t1.builtinEnc[0x32] = ".notdef"
	enc, ok = encodingFromProgram(&renderFont{t1: t1})
	if !ok || enc[0x31] != '1' || enc[0x32] != '�' {
		t.Errorf("Type1: ok=%v 0x31=%q 0x32=%q", ok, enc[0x31], enc[0x32])
	}
}
