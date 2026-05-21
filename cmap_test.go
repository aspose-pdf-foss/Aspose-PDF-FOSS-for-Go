// SPDX-License-Identifier: MIT

package asposepdf

import (
	"testing"
)

func TestParseCMapBfchar(t *testing.T) {
	data := []byte(`/CIDInit /ProcSet findresource begin
12 dict begin
begincmap
/CMapName /Adobe-Identity-UCS def
/CMapType 2 def
1 begincodespacerange
<0000> <FFFF>
endcodespacerange
3 beginbfchar
<0003> <0020>
<0004> <0041>
<015E> <0410>
endbfchar
endcmap`)
	m := parseCMap(data)
	if m[0x0003] != 0x0020 {
		t.Errorf("0x0003: got %U, want U+0020", m[0x0003])
	}
	if m[0x0004] != 0x0041 {
		t.Errorf("0x0004: got %U, want U+0041 (A)", m[0x0004])
	}
	if m[0x015E] != 0x0410 {
		t.Errorf("0x015E: got %U, want U+0410 (Cyrillic A)", m[0x015E])
	}
}

func TestParseCMapBfrange(t *testing.T) {
	data := []byte(`begincmap
1 begincodespacerange
<0000> <FFFF>
endcodespacerange
1 beginbfrange
<0041> <0043> <0061>
endbfrange
endcmap`)
	m := parseCMap(data)
	// 0x0041 -> 'a', 0x0042 -> 'b', 0x0043 -> 'c'
	if m[0x0041] != 'a' {
		t.Errorf("0x0041: got %U, want U+0061 (a)", m[0x0041])
	}
	if m[0x0042] != 'b' {
		t.Errorf("0x0042: got %U, want U+0062 (b)", m[0x0042])
	}
	if m[0x0043] != 'c' {
		t.Errorf("0x0043: got %U, want U+0063 (c)", m[0x0043])
	}
}

func TestParseCMapBfrangeArray(t *testing.T) {
	data := []byte(`begincmap
1 begincodespacerange
<0000> <FFFF>
endcodespacerange
1 beginbfrange
<0100> <0102> [<0041> <0042> <0043>]
endbfrange
endcmap`)
	m := parseCMap(data)
	if m[0x0100] != 'A' {
		t.Errorf("0x0100: got %U, want U+0041 (A)", m[0x0100])
	}
	if m[0x0101] != 'B' {
		t.Errorf("0x0101: got %U, want U+0042 (B)", m[0x0101])
	}
	if m[0x0102] != 'C' {
		t.Errorf("0x0102: got %U, want U+0043 (C)", m[0x0102])
	}
}

func TestParseCMapEmpty(t *testing.T) {
	m := parseCMap([]byte{})
	if len(m) != 0 {
		t.Errorf("expected empty map, got %d entries", len(m))
	}
}
