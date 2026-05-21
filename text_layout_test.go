// SPDX-License-Identifier: MIT

package asposepdf

import (
	"strings"
	"testing"
)

func TestGroupFragmentsIntoLines(t *testing.T) {
	frags := []textFragment{
		{x: 100, y: 50, endX: 150, fontName: "/Helvetica", fontSize: 12},   // footer
		{x: 100, y: 700, endX: 160, fontName: "/Helvetica", fontSize: 12},  // line 1
		{x: 170, y: 700, endX: 230, fontName: "/Helvetica", fontSize: 12},  // line 1 continued
		{x: 100, y: 680, endX: 180, fontName: "/Helvetica", fontSize: 12},  // line 2
	}
	frags[0].text.WriteString("Footer")
	frags[1].text.WriteString("Hello")
	frags[2].text.WriteString("World")
	frags[3].text.WriteString("Second line")

	lines := groupFragmentsIntoLines(frags)

	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d", len(lines))
	}
	// First line should be y=700 (top of page).
	if !strings.Contains(lines[0].Text, "Hello") {
		t.Errorf("line 0: %q, expected Hello", lines[0].Text)
	}
	if !strings.Contains(lines[0].Text, "World") {
		t.Errorf("line 0: %q, expected World", lines[0].Text)
	}
	// Second line y=680.
	if !strings.Contains(lines[1].Text, "Second line") {
		t.Errorf("line 1: %q, expected 'Second line'", lines[1].Text)
	}
	// Last line is footer y=50.
	if !strings.Contains(lines[2].Text, "Footer") {
		t.Errorf("line 2: %q, expected Footer", lines[2].Text)
	}
}

func TestGroupFragmentsEmpty(t *testing.T) {
	lines := groupFragmentsIntoLines(nil)
	if len(lines) != 0 {
		t.Errorf("expected 0 lines, got %d", len(lines))
	}
}

func TestGroupFragmentsSpaceInsertion(t *testing.T) {
	// Two fragments on same line with a gap — should get a space.
	frags := []textFragment{
		{x: 100, y: 700, endX: 140, fontName: "/Helvetica", fontSize: 12},
		{x: 150, y: 700, endX: 200, fontName: "/Helvetica", fontSize: 12},
	}
	frags[0].text.WriteString("Hello")
	frags[1].text.WriteString("World")

	lines := groupFragmentsIntoLines(frags)
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(lines))
	}
	if lines[0].Text != "Hello World" {
		t.Errorf("text=%q, want 'Hello World'", lines[0].Text)
	}
}

func TestGroupFragmentsNoSpuriousSpace(t *testing.T) {
	// Two fragments on same line with no gap — no space.
	frags := []textFragment{
		{x: 100, y: 700, endX: 140, fontName: "/Helvetica", fontSize: 12},
		{x: 140, y: 700, endX: 180, fontName: "/Helvetica", fontSize: 12},
	}
	frags[0].text.WriteString("Hel")
	frags[1].text.WriteString("lo")

	lines := groupFragmentsIntoLines(frags)
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(lines))
	}
	if lines[0].Text != "Hello" {
		t.Errorf("text=%q, want 'Hello'", lines[0].Text)
	}
}

func TestActualTextLigature(t *testing.T) {
	// Simulate a content stream with /ActualText for a ligature:
	//   /Span <</ActualText (ffi)>> BDC
	//     (x) Tj          ← single glyph for the ffi ligature
	//   EMC
	objects := map[int]*pdfObject{}
	fonts := map[string]fontInfo{
		"/F1": {
			name:     "/F1",
			encoding: defaultEncoding(),
			widths:   defaultWidths(),
		},
	}

	ext := newTextExtractor(objects, fonts)
	ops := []contentOp{
		{Operator: "BT"},
		{Operator: "Tf", Operands: []pdfValue{pdfName("/F1"), 12}},
		{Operator: "Td", Operands: []pdfValue{100, 700}},
		// Show "e" before the ligature.
		{Operator: "Tj", Operands: []pdfValue{"e"}},
		// BDC with inline ActualText dict — replace ligature glyph with "ffi".
		{Operator: "BDC", Operands: []pdfValue{
			pdfName("/Span"),
			pdfDict{"/ActualText": "ffi"},
		}},
		{Operator: "Tj", Operands: []pdfValue{"x"}}, // suppressed glyph
		{Operator: "EMC"},
		// Show "cient" after the ligature.
		{Operator: "Tj", Operands: []pdfValue{"cient"}},
		{Operator: "ET"},
	}
	ext.process(ops, nil)
	got := ext.text()
	if !strings.Contains(got, "efficient") {
		t.Errorf("expected text containing 'efficient', got %q", got)
	}
}

func TestActualTextUTF16(t *testing.T) {
	// /ActualText with UTF-16BE BOM encoding.
	objects := map[int]*pdfObject{}
	fonts := map[string]fontInfo{
		"/F1": {
			name:     "/F1",
			encoding: defaultEncoding(),
			widths:   defaultWidths(),
		},
	}

	// UTF-16BE for "fi": BOM + 0066 + 0069
	utf16 := "\xFE\xFF\x00\x66\x00\x69"

	ext := newTextExtractor(objects, fonts)
	ops := []contentOp{
		{Operator: "BT"},
		{Operator: "Tf", Operands: []pdfValue{pdfName("/F1"), 12}},
		{Operator: "Td", Operands: []pdfValue{100, 700}},
		{Operator: "BDC", Operands: []pdfValue{
			pdfName("/Span"),
			pdfDict{"/ActualText": utf16},
		}},
		{Operator: "Tj", Operands: []pdfValue{"X"}}, // suppressed
		{Operator: "EMC"},
		{Operator: "ET"},
	}
	ext.process(ops, nil)
	got := ext.text()
	if got != "fi" {
		t.Errorf("expected 'fi', got %q", got)
	}
}

func TestActualTextNested(t *testing.T) {
	// Nested BDC: outer has no ActualText, inner does.
	objects := map[int]*pdfObject{}
	fonts := map[string]fontInfo{
		"/F1": {
			name:     "/F1",
			encoding: defaultEncoding(),
			widths:   defaultWidths(),
		},
	}

	ext := newTextExtractor(objects, fonts)
	ops := []contentOp{
		{Operator: "BT"},
		{Operator: "Tf", Operands: []pdfValue{pdfName("/F1"), 12}},
		{Operator: "Td", Operands: []pdfValue{100, 700}},
		// Outer BMC — no ActualText.
		{Operator: "BMC", Operands: []pdfValue{pdfName("/P")}},
		{Operator: "Tj", Operands: []pdfValue{"AB"}}, // normal — emitted
		// Inner BDC with ActualText.
		{Operator: "BDC", Operands: []pdfValue{
			pdfName("/Span"),
			pdfDict{"/ActualText": "CD"},
		}},
		{Operator: "Tj", Operands: []pdfValue{"xx"}}, // suppressed
		{Operator: "EMC"}, // pops inner → emits "CD"
		{Operator: "Tj", Operands: []pdfValue{"EF"}}, // normal — emitted
		{Operator: "EMC"}, // pops outer
		{Operator: "ET"},
	}
	ext.process(ops, nil)
	got := ext.text()
	if !strings.Contains(got, "ABCDEF") {
		t.Errorf("expected text containing 'ABCDEF', got %q", got)
	}
}

func TestBMCWithoutActualText(t *testing.T) {
	// BMC without ActualText should pass glyphs through normally.
	objects := map[int]*pdfObject{}
	fonts := map[string]fontInfo{
		"/F1": {
			name:     "/F1",
			encoding: defaultEncoding(),
			widths:   defaultWidths(),
		},
	}

	ext := newTextExtractor(objects, fonts)
	ops := []contentOp{
		{Operator: "BT"},
		{Operator: "Tf", Operands: []pdfValue{pdfName("/F1"), 12}},
		{Operator: "Td", Operands: []pdfValue{100, 700}},
		{Operator: "BMC", Operands: []pdfValue{pdfName("/P")}},
		{Operator: "Tj", Operands: []pdfValue{"Hello"}},
		{Operator: "EMC"},
		{Operator: "ET"},
	}
	ext.process(ops, nil)
	got := ext.text()
	if got != "Hello" {
		t.Errorf("expected 'Hello', got %q", got)
	}
}

func defaultEncoding() [256]rune {
	var enc [256]rune
	for i := 0; i < 256; i++ {
		enc[i] = rune(i)
	}
	return enc
}

func defaultWidths() [256]float64 {
	var w [256]float64
	for i := 0; i < 256; i++ {
		w[i] = 600 // monospace-like
	}
	return w
}

func TestMacExpertEncoding(t *testing.T) {
	// Verify key mappings in MacExpertEncoding.
	tests := []struct {
		code byte
		want rune
		desc string
	}{
		{32, ' ', "space"},
		{49, '0', "zerooldstyle"},
		{50, '1', "oneoldstyle"},
		{58, ':', "colon"},
		{48, '\u2044', "fraction"},
		{72, '\u00BD', "onehalf"},
		{73, '\u00BC', "onequarter"},
		{74, '\u00BE', "threequarters"},
		{81, '\u2070', "zerosuperior"},
		{88, '\u2080', "zeroinferior"},
		{89, '\u2081', "oneinferior"},
		{108, '\u00C6', "AEsmall→Æ"},
		{183, 'A', "Asmall"},
		{210, 'Z', "Zsmall"},
		{229, '\uFB00', "ff ligature"},
		{230, '\uFB01', "fi ligature"},
		{231, '\uFB02', "fl ligature"},
		{232, '\uFB03', "ffi ligature"},
		{233, '\uFB04', "ffl ligature"},
		{248, '\u00B7', "periodcentered"},
	}
	for _, tt := range tests {
		got := macExpertEncoding[tt.code]
		if got != tt.want {
			t.Errorf("macExpertEncoding[%d] (%s) = %U, want %U", tt.code, tt.desc, got, tt.want)
		}
	}
}

func TestMacExpertEncodingLookup(t *testing.T) {
	enc, ok := lookupEncoding("/MacExpertEncoding")
	if !ok {
		t.Fatal("lookupEncoding did not recognize /MacExpertEncoding")
	}
	if enc[230] != '\uFB01' {
		t.Errorf("expected fi ligature at 230, got %U", enc[230])
	}
}

func TestCleanFontName(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"/MCEFGG+Garamond-Bold", "Garamond-Bold"},
		{"/Helvetica", "Helvetica"},
		{"/ABCDEF+Arial-BoldMT", "Arial-BoldMT"},
		{"", ""},
	}
	for _, tt := range tests {
		got := cleanFontName(tt.in)
		if got != tt.want {
			t.Errorf("cleanFontName(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
