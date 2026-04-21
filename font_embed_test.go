package asposepdf

import (
	"bytes"
	"compress/zlib"
	"io"
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
	desc := buildFontDescriptor(f, fontFileID)

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
	// Flags: Symbolic (bit 3, value 4) is always set for embedded TTF.
	flags, _ := desc["/Flags"].(int)
	if flags&0x4 == 0 {
		t.Errorf("/Flags = %d, Symbolic bit not set", flags)
	}
}
