// SPDX-License-Identifier: MIT

package asposepdf

import (
	"encoding/binary"
	"testing"
)

// extensionLookup builds a GPOS lookup of type 9 whose n extension subtables
// each point at a single-adjustment (type 1, format 1) subtable for glyph 5.
func extensionLookup(n int) []byte {
	u16 := func(b []byte, v int) []byte { return binary.BigEndian.AppendUint16(b, uint16(v)) }
	u32 := func(b []byte, v int) []byte { return binary.BigEndian.AppendUint32(b, uint32(v)) }
	// Single adjustment: format 1, coverage at 8, value format XAdvance, -50;
	// coverage format 1 with one glyph.
	single := u16(u16(u16(u16(nil, 1), 8), 0x0004), 0xFFCE)
	single = u16(u16(u16(single, 1), 1), 5)

	head := 6 + 2*n
	lookup := u16(u16(u16(nil, 9), 0), n)
	var tail []byte
	for i := range n {
		lookup = u16(lookup, head+i*8)
		// Extension subtable: format 1, extended type 1, offset to the
		// subtable from the extension's start (past all n extensions).
		tail = u32(u16(u16(tail, 1), 1), (n-i)*8+i*len(single))
	}
	lookup = append(lookup, tail...)
	for range n {
		lookup = append(lookup, single...)
	}
	return lookup
}

// TestExtensionLookupEverySubtable: every subtable of an extension lookup is
// an extension. The first one used to change the lookup's type, so the others'
// extension headers were read as subtables of the extended type — garbage
// rules that took seconds to parse in fonts such as Arial Bold.
func TestExtensionLookupEverySubtable(t *testing.T) {
	l := otParseLookup(extensionLookup(3), false)
	if l.kind != 1 {
		t.Fatalf("kind %d, want the extended type 1", l.kind)
	}
	if len(l.subtables) != 3 {
		t.Fatalf("%d subtables, want 3", len(l.subtables))
	}
	for i, st := range l.subtables {
		s, ok := st.(*gposSingle)
		if !ok {
			t.Fatalf("subtable %d is %T", i, st)
		}
		if !s.cov.covers(5) || s.value.xAdvance != -50 {
			t.Errorf("subtable %d: coverage %v, value %+v", i, s.cov, s.value)
		}
	}
}
