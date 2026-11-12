// SPDX-License-Identifier: MIT

package asposepdf

import (
	"encoding/binary"
	"testing"
)

// kernSubtable builds a 'kern' format 0 subtable with the given coverage.
func kernSubtable(coverage uint16, pairs [][3]int) []byte {
	b := make([]byte, 14+6*len(pairs))
	binary.BigEndian.PutUint16(b[2:], uint16(len(b)))
	binary.BigEndian.PutUint16(b[4:], coverage)
	binary.BigEndian.PutUint16(b[6:], uint16(len(pairs)))
	for i, p := range pairs {
		at := 14 + 6*i
		binary.BigEndian.PutUint16(b[at:], uint16(p[0]))
		binary.BigEndian.PutUint16(b[at+2:], uint16(p[1]))
		binary.BigEndian.PutUint16(b[at+4:], uint16(int16(p[2])))
	}
	return b
}

func kernTable(subtables ...[]byte) []byte {
	b := make([]byte, 4)
	binary.BigEndian.PutUint16(b[2:], uint16(len(subtables)))
	for _, s := range subtables {
		b = append(b, s...)
	}
	return b
}

// TestParseKernTable: horizontal format 0 subtables are read; vertical and
// cross-stream ones, and Apple's version 1 table, are left out.
func TestParseKernTable(t *testing.T) {
	b := kernTable(
		kernSubtable(0x0001, [][3]int{{1, 2, -55}, {2, 1, -40}}),
		kernSubtable(0x0000, [][3]int{{1, 2, -10}}), // vertical
		kernSubtable(0x0005, [][3]int{{1, 2, -10}}), // cross-stream
	)
	sts := otParseKern(b)
	if len(sts) != 1 {
		t.Fatalf("%d subtables, want 1", len(sts))
	}
	if v := sts[0][1<<16|2]; v != -55 {
		t.Errorf("pair 1,2 = %d, want -55", v)
	}
	if v := sts[0][2<<16|1]; v != -40 {
		t.Errorf("pair 2,1 = %d, want -40", v)
	}
	apple := append([]byte{0, 1, 0, 0}, b[4:]...)
	if sts := otParseKern(apple); sts != nil {
		t.Errorf("version 1 table read: %v", sts)
	}
}

// kernFont is a font with glyphs A=1, V=2 and a combining acute=3, no GPOS
// kern feature for Latin and a 'kern' table kerning A V and V A.
func kernFont(gdefMarks bool) *ttfFont {
	f := &ttfFont{
		runeToGlyph: map[rune]uint16{'A': 1, 'V': 2, '́': 3},
		glyphWidths: []uint16{0, 1228, 1222, 500},
	}
	gpos := &otLayoutTable{
		scripts:  map[string]*otScript{"latn": {defaultLang: &otLangSys{required: -1, features: []int{0}}}},
		features: []otFeature{{tag: "mark"}},
	}
	l := &otLayout{gpos: gpos, kern: []otKernSubtable{{1<<16 | 2: -55, 2<<16 | 1: -40}}}
	if gdefMarks {
		l.gdef = &otGDEF{glyphClass: otClassDef{start: 1, values: []uint16{1, 1, 3}}}
	}
	f.ot = l
	f.otOnce.Do(func() {})
	return f
}

type glyphPos struct{ gid, adv, xOff int32 }

func shapedPositions(f *ttfFont, text string, rtl bool) []glyphPos {
	var out []glyphPos
	for _, g := range shapeRun(f, text, rtl) {
		out = append(out, glyphPos{int32(g.gid), g.advance, g.xOff})
	}
	return out
}

func equalPositions(a, b []glyphPos) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestShapeKernTableFallback: with no GPOS kern feature for the script, the
// 'kern' table kerns as HarfBuzz does — the first glyph takes half the value
// rounded down, the second the rest in advance and offset — marks are stepped
// over and lose their advance, and right-to-left runs pair in visual order
// (pdf-go-u3vk; values match uharfbuzz on Tahoma's A V A).
func TestShapeKernTableFallback(t *testing.T) {
	f := kernFont(true)
	if got, want := shapedPositions(f, "AVA", false), []glyphPos{{1, 1200, 0}, {2, 1222 - 27 - 20, -27}, {1, 1228 - 20, -20}}; !equalPositions(got, want) {
		t.Errorf("AVA = %v, want %v", got, want)
	}
	// The acute between A and V is stepped over, and its advance goes.
	if got, want := shapedPositions(f, "ÁV", false), []glyphPos{{1, 1200, 0}, {3, 0, 0}, {2, 1195, -27}}; !equalPositions(got, want) {
		t.Errorf("A+acute+V = %v, want %v", got, want)
	}
	// Right to left the visual order is V then A: the pair V A kerns.
	if got, want := shapedPositions(f, "AV", true), []glyphPos{{1, 1228 - 20, -20}, {2, 1222 - 20, 0}}; !equalPositions(got, want) {
		t.Errorf("AV rtl = %v, want %v", got, want)
	}
	// A GPOS kern feature for the script takes over from the table.
	f.ot.gpos.features[0].tag = "kern"
	if got, want := shapedPositions(f, "AV", false), []glyphPos{{1, 1228, 0}, {2, 1222, 0}}; !equalPositions(got, want) {
		t.Errorf("AV with GPOS kern = %v, want %v", got, want)
	}
}

// TestShapeZeroMarkAdvances: a mark loses its advance, by GDEF class when
// GDEF classes glyphs and by Unicode otherwise; without GPOS a
// left-to-right mark moves back by the advance it loses.
func TestShapeZeroMarkAdvances(t *testing.T) {
	f := kernFont(false) // no GDEF: the acute is a mark by Unicode
	f.ot.kern = nil
	if got, want := shapedPositions(f, "Á", false), []glyphPos{{1, 1228, 0}, {3, 0, 0}}; !equalPositions(got, want) {
		t.Errorf("GPOS, Unicode mark = %v, want %v", got, want)
	}
	g := kernFont(true)
	g.ot.kern = nil
	g.ot.gdef.glyphClass.values[2] = 1 // GDEF says the acute is a base
	if got, want := shapedPositions(g, "Á", false), []glyphPos{{1, 1228, 0}, {3, 500, 0}}; !equalPositions(got, want) {
		t.Errorf("GDEF base = %v, want %v", got, want)
	}
	h := kernFont(true)
	h.ot.kern, h.ot.gpos = nil, nil
	h.ot.gsub = &otLayoutTable{}
	if got, want := shapedPositions(h, "Á", false), []glyphPos{{1, 1228, 0}, {3, 0, -500}}; !equalPositions(got, want) {
		t.Errorf("no GPOS = %v, want %v", got, want)
	}
}
