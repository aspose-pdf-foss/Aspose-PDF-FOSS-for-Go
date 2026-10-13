// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"compress/zlib"
	"io"
	"strings"
	"testing"
)

func TestBuildFontFile2Stream(t *testing.T) {
	f, err := parseTTF(loadDejaVu(t))
	if err != nil {
		t.Fatal(err)
	}
	stream := buildFontFile2Stream(f)
	if stream == nil {
		t.Fatal("buildFontFile2Stream returned nil")
	}
	if stream.Dict["/Length1"] != len(f.data) {
		t.Errorf("/Length1 = %v, want %d", stream.Dict["/Length1"], len(f.data))
	}
	if stream.Dict["/Filter"] != pdfName("/FlateDecode") {
		t.Errorf("/Filter = %v, want /FlateDecode", stream.Dict["/Filter"])
	}
	// Round-trip: inflate the stream data and compare with original.
	r, err := zlib.NewReader(bytes.NewReader(stream.Data))
	if err != nil {
		t.Fatalf("zlib reader: %v", err)
	}
	decompressed, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("decompress: %v", err)
	}
	if !bytes.Equal(decompressed, f.data) {
		t.Errorf("decompressed bytes do not match original (got %d bytes, want %d)",
			len(decompressed), len(f.data))
	}
}

func TestBuildFontDescriptor(t *testing.T) {
	f, err := parseTTF(loadDejaVu(t))
	if err != nil {
		t.Fatal(err)
	}
	fontFileID := 42
	desc := buildFontDescriptor(f, "/FontFile2", fontFileID, true)

	if desc["/Type"] != pdfName("/FontDescriptor") {
		t.Errorf("/Type = %v", desc["/Type"])
	}
	if desc["/FontName"] != pdfName("/DejaVuSans") {
		t.Errorf("/FontName = %v, want /DejaVuSans", desc["/FontName"])
	}
	ref, ok := desc["/FontFile2"].(pdfRef)
	if !ok || ref.Num != fontFileID {
		t.Errorf("/FontFile2 = %v, want pdfRef{%d}", desc["/FontFile2"], fontFileID)
	}
	// Flags: symbolic=true -> Symbolic (bit 3, value 4) set, Nonsymbolic
	// (bit 6, value 32) clear -- the composite-font embedding path (no
	// /Encoding dict to conflict with it).
	flags, _ := desc["/Flags"].(int)
	if flags&0x4 == 0 {
		t.Errorf("/Flags = %d, Symbolic bit not set", flags)
	}
	if flags&0x20 != 0 {
		t.Errorf("/Flags = %d, Nonsymbolic bit set alongside Symbolic", flags)
	}
}

// TestBuildFontDescriptorNonsymbolic pins GitHub issue #12: a simple font
// descriptor built with symbolic=false (the PDF/A Standard-14 substitution
// path, which always writes /Encoding) must set Nonsymbolic, not Symbolic --
// a symbolic TrueType font with /Encoding violates PDF/A (veraPDF
// 6.2.11.4.1-2 / 6.2.11.6-3/-4).
func TestBuildFontDescriptorNonsymbolic(t *testing.T) {
	f, err := parseTTF(loadDejaVu(t))
	if err != nil {
		t.Fatal(err)
	}
	desc := buildFontDescriptor(f, "/FontFile2", 42, false)
	flags, _ := desc["/Flags"].(int)
	if flags&0x20 == 0 {
		t.Errorf("/Flags = %d, Nonsymbolic bit not set", flags)
	}
	if flags&0x4 != 0 {
		t.Errorf("/Flags = %d, Symbolic bit set alongside Nonsymbolic", flags)
	}
}

func TestBuildWArray(t *testing.T) {
	f, err := parseTTF(loadDejaVu(t))
	if err != nil {
		t.Fatal(err)
	}
	arr := buildWArray(f)
	// Round-trip through the existing parseCIDWidthArray.
	widths := make(map[uint16]float64)
	parseCIDWidthArray(nil, arr, widths)

	// 'A' has a known non-default width.
	gidA := f.glyphID('A')
	want := float64(f.glyphWidths[gidA]) * 1000.0 / float64(f.unitsPerEm)
	got := widths[gidA]
	// Default width 500 is skipped from /W, so if advance scaled equals 500 it's absent.
	if got == 0 && want != 500 {
		t.Errorf("/W round-trip for gid %d: got 0, want %g", gidA, want)
	}
	if got != 0 {
		// Round to nearest int because packing stores ints.
		if int(got) != int(want) && int(got) != int(want+0.5) {
			t.Errorf("/W width for gid %d = %g, want %g", gidA, got, want)
		}
	}
}

func TestBuildToUnicodeCMap(t *testing.T) {
	f, err := parseTTF(loadDejaVu(t))
	if err != nil {
		t.Fatal(err)
	}
	stream := buildToUnicodeCMap(f, nil)
	content := string(stream.Data)
	if !strings.Contains(content, "beginbfchar") {
		t.Error("CMap missing beginbfchar block")
	}
	if !strings.Contains(content, "begincmap") || !strings.Contains(content, "endcmap") {
		t.Error("CMap missing begincmap/endcmap")
	}
	// Round-trip: reuse the existing parseCMap reader.
	decoded := parseCMap(stream.Data)
	gidA := f.glyphID('A')
	if r, ok := decoded[gidA]; !ok || r != 'A' {
		t.Errorf("CMap[gid(A)] = (%q, %v), want ('A', true)", r, ok)
	}
}

func TestLoadFontCreatesFiveObjects(t *testing.T) {
	doc := NewDocument(595, 842)
	beforeCount := len(doc.objects)

	f, err := doc.LoadFont("testdata/DejaVuSans.ttf")
	if err != nil {
		t.Fatal(err)
	}
	afterCount := len(doc.objects)

	if afterCount-beforeCount < 5 {
		t.Errorf("LoadFont added %d objects, want >= 5 (Type0, CIDFontType2, FontDescriptor, FontFile2, ToUnicode)",
			afterCount-beforeCount)
	}

	ef, ok := f.(*embeddedFont)
	if !ok {
		t.Fatalf("LoadFont returned %T, want *embeddedFont", f)
	}
	if ef.fontObjectID == 0 {
		t.Error("fontObjectID not set")
	}

	// Walk: Type0 -> DescendantFonts[0] -> CIDFontType2 -> FontDescriptor -> FontFile2
	type0 := doc.objects[ef.fontObjectID].Value.(pdfDict)
	if type0["/Subtype"] != pdfName("/Type0") {
		t.Errorf("Type0 /Subtype = %v", type0["/Subtype"])
	}
	if type0["/Encoding"] != pdfName("/Identity-H") {
		t.Errorf("Type0 /Encoding = %v, want /Identity-H", type0["/Encoding"])
	}
	desc := type0["/DescendantFonts"].(pdfArray)
	if len(desc) != 1 {
		t.Fatalf("DescendantFonts length = %d, want 1", len(desc))
	}
	cidRef := desc[0].(pdfRef)
	cid := doc.objects[cidRef.Num].Value.(pdfDict)
	if cid["/Subtype"] != pdfName("/CIDFontType2") {
		t.Errorf("CIDFont /Subtype = %v", cid["/Subtype"])
	}
	if cid["/CIDToGIDMap"] != pdfName("/Identity") {
		t.Errorf("CIDToGIDMap = %v, want /Identity", cid["/CIDToGIDMap"])
	}

	tuRef := type0["/ToUnicode"].(pdfRef)
	if _, ok := doc.objects[tuRef.Num].Value.(*pdfStream); !ok {
		t.Error("ToUnicode is not a stream")
	}
}
