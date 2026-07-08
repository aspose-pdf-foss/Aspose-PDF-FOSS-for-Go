// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"io"
	"testing"
)

// cmap4Lookup resolves one rune through a cmap table built by
// buildCmapFormat4 (single (3,1) format-4 subtable, pure-delta segments).
func cmap4Lookup(table []byte, r rune) uint16 {
	sub := table[12:] // one encoding record at offset 12
	segCount := int(binary.BigEndian.Uint16(sub[6:8])) / 2
	ends := sub[14:]
	starts := sub[16+segCount*2:]
	deltas := sub[16+segCount*4:]
	for i := 0; i < segCount; i++ {
		end := binary.BigEndian.Uint16(ends[i*2:])
		if uint16(r) > end {
			continue
		}
		start := binary.BigEndian.Uint16(starts[i*2:])
		if uint16(r) < start {
			return 0
		}
		delta := binary.BigEndian.Uint16(deltas[i*2:])
		return uint16(r) + delta
	}
	return 0
}

// TestBuildCmapFormat4: every mapped rune resolves to its GID; unmapped
// runes resolve to 0; contiguous and scattered mappings both work.
func TestBuildCmapFormat4(t *testing.T) {
	m := map[rune]uint16{
		'A': 36, 'B': 37, 'C': 38, // contiguous run
		'Z': 61,
		'г': 500, 'д': 700, // adjacent runes, non-contiguous GIDs
	}
	table := buildCmapFormat4(m)

	if got := binary.BigEndian.Uint16(table[4:6]); got != 3 {
		t.Fatalf("platform = %d, want 3", got)
	}
	for r, want := range m {
		if got := cmap4Lookup(table, r); got != want {
			t.Errorf("lookup %q = %d, want %d", r, got, want)
		}
	}
	for _, r := range []rune{'D', ' ', 'я'} {
		if got := cmap4Lookup(table, r); got != 0 {
			t.Errorf("unmapped %q = %d, want 0", r, got)
		}
	}
}

// TestWOFFEncodeRoundTrip: the WOFF header carries the right sfnt geometry
// and every table decompresses back to the original bytes.
func TestWOFFEncodeRoundTrip(t *testing.T) {
	sfnt := assembleSFNT(map[string][]byte{
		"head": make([]byte, 54),
		"glyf": bytes.Repeat([]byte("outline data "), 100), // compressible
		"cvt ": {1, 2, 3},
	})
	// Expectation = the assembled sfnt's own tables (head carries the
	// checkSumAdjustment assembleSFNT patched in).
	_, tbls, err := sfntReadTables(sfnt)
	if err != nil {
		t.Fatal(err)
	}
	woff, err := woffEncode(sfnt)
	if err != nil {
		t.Fatal(err)
	}

	if string(woff[0:4]) != "wOFF" {
		t.Fatalf("signature = %q", woff[0:4])
	}
	if got := binary.BigEndian.Uint32(woff[4:8]); got != 0x00010000 {
		t.Errorf("flavor = 0x%08X, want 0x00010000", got)
	}
	if got := binary.BigEndian.Uint32(woff[8:12]); int(got) != len(woff) {
		t.Errorf("length field = %d, want %d", got, len(woff))
	}
	numTables := int(binary.BigEndian.Uint16(woff[12:14]))
	if numTables != len(tbls) {
		t.Fatalf("numTables = %d, want %d", numTables, len(tbls))
	}
	if got := binary.BigEndian.Uint32(woff[16:20]); int(got) != len(sfnt) {
		t.Errorf("totalSfntSize = %d, want %d", got, len(sfnt))
	}

	for i := 0; i < numTables; i++ {
		rec := woff[44+i*20:]
		tag := string(rec[0:4])
		off := int(binary.BigEndian.Uint32(rec[4:8]))
		compLen := int(binary.BigEndian.Uint32(rec[8:12]))
		origLen := int(binary.BigEndian.Uint32(rec[12:16]))
		want, ok := tbls[tag]
		if !ok {
			t.Fatalf("unexpected table %q", tag)
		}
		if origLen != len(want) {
			t.Fatalf("%s: origLength = %d, want %d", tag, origLen, len(want))
		}
		data := woff[off : off+compLen]
		if compLen < origLen {
			zr, err := zlib.NewReader(bytes.NewReader(data))
			if err != nil {
				t.Fatalf("%s: zlib: %v", tag, err)
			}
			if data, err = io.ReadAll(zr); err != nil {
				t.Fatalf("%s: inflate: %v", tag, err)
			}
		}
		if !bytes.Equal(data, want) {
			t.Errorf("%s: table bytes do not round-trip", tag)
		}
	}
}

// TestSFNTReadTables: assembleSFNTFlavor output parses back losslessly,
// including the OTTO flavor; garbage is rejected.
func TestSFNTReadTables(t *testing.T) {
	tbls := map[string][]byte{"CFF ": {1, 2, 3, 4, 5}, "priv": {9, 8}}
	sfnt := assembleSFNTFlavor(0x4F54544F, tbls)
	flavor, got, err := sfntReadTables(sfnt)
	if err != nil {
		t.Fatal(err)
	}
	if flavor != 0x4F54544F {
		t.Errorf("flavor = 0x%08X, want OTTO", flavor)
	}
	for tag, want := range tbls {
		if !bytes.Equal(got[tag], want) {
			t.Errorf("table %q does not round-trip", tag)
		}
	}
	if _, _, err := sfntReadTables([]byte("not a font at all")); err == nil {
		t.Error("garbage accepted")
	}
}
