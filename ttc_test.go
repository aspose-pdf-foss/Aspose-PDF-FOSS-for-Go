// SPDX-License-Identifier: MIT

package asposepdf

import (
	"encoding/binary"
	"testing"
)

// makeTTC wraps a single sfnt font into a one-font TrueType Collection: it moves
// the sfnt to byte offset 16 (after the ttcf header) and shifts every table
// record offset by +16 so the file-absolute offsets stay valid. This lets the
// TTC path be exercised deterministically, without depending on OS fonts.
func makeTTC(sfnt []byte) []byte {
	const hdr = 16
	patched := append([]byte(nil), sfnt...)
	numTables := int(binary.BigEndian.Uint16(patched[4:6]))
	for i := 0; i < numTables; i++ {
		rec := 12 + i*16
		off := binary.BigEndian.Uint32(patched[rec+8 : rec+12])
		binary.BigEndian.PutUint32(patched[rec+8:rec+12], off+hdr)
	}
	out := make([]byte, hdr+len(patched))
	copy(out[0:4], "ttcf")
	binary.BigEndian.PutUint16(out[4:6], 1)     // major version
	binary.BigEndian.PutUint16(out[6:8], 0)     // minor version
	binary.BigEndian.PutUint32(out[8:12], 1)    // numFonts
	binary.BigEndian.PutUint32(out[12:16], hdr) // offset to font 0
	copy(out[hdr:], patched)
	return out
}

func TestParseFontCollectionTTC(t *testing.T) {
	sfnt, err := stdFontsFS.ReadFile("fonts/Arimo-Regular.ttf")
	if err != nil {
		t.Fatal(err)
	}
	ttc := makeTTC(sfnt)

	// parseTTF must reject a collection — it cannot be embedded as one FontFile2.
	if _, err := parseTTF(ttc); err == nil {
		t.Error("parseTTF accepted a TTC; expected rejection")
	}

	fonts, err := parseFontCollection(ttc)
	if err != nil {
		t.Fatalf("parseFontCollection: %v", err)
	}
	if len(fonts) != 1 {
		t.Fatalf("got %d sub-fonts, want 1", len(fonts))
	}
	f := fonts[0]
	if f.family == "" {
		t.Error("sub-font family name empty")
	}
	// Glyph outlines must resolve through the sub-font's shifted table directory,
	// not the file head — this is what regressed before f.tables was captured.
	if len(f.glyphContours(f.glyphID('A'))) == 0 {
		t.Error("glyph 'A' has no contours — TTC table offsets not resolving")
	}
}

func TestParseFontCollectionSingle(t *testing.T) {
	sfnt, err := stdFontsFS.ReadFile("fonts/Tinos-Regular.ttf")
	if err != nil {
		t.Fatal(err)
	}
	fonts, err := parseFontCollection(sfnt)
	if err != nil {
		t.Fatalf("parseFontCollection on plain sfnt: %v", err)
	}
	if len(fonts) != 1 {
		t.Fatalf("got %d fonts from a single sfnt, want 1", len(fonts))
	}
}
