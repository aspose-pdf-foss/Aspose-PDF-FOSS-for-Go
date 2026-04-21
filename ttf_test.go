package asposepdf

import (
	"os"
	"strings"
	"testing"
)

func loadDejaVu(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile("testdata/DejaVuSans.ttf")
	if err != nil {
		t.Fatalf("read DejaVuSans.ttf: %v", err)
	}
	return data
}

func TestParseTTF_NotTTF(t *testing.T) {
	_, err := parseTTF([]byte("not a font file, just garbage"))
	if err == nil {
		t.Fatal("expected error for non-TTF input")
	}
	if !strings.Contains(err.Error(), "TrueType") {
		t.Errorf("error = %q, want to mention TrueType", err.Error())
	}
}

func TestParseTTF_TooSmall(t *testing.T) {
	_, err := parseTTF([]byte{0x00, 0x01, 0x00, 0x00})
	if err == nil {
		t.Fatal("expected error for truncated file")
	}
}

func TestParseTTF_DejaVuBasic(t *testing.T) {
	f, err := parseTTF(loadDejaVu(t))
	if err != nil {
		t.Fatalf("parseTTF: %v", err)
	}
	if f == nil {
		t.Fatal("parseTTF returned nil font")
	}
	if len(f.data) == 0 {
		t.Error("ttfFont.data is empty")
	}
}

func TestParseTTF_Head(t *testing.T) {
	f, err := parseTTF(loadDejaVu(t))
	if err != nil {
		t.Fatal(err)
	}
	if f.unitsPerEm != 2048 {
		t.Errorf("unitsPerEm = %d, want 2048", f.unitsPerEm)
	}
	if f.xMin == 0 && f.yMin == 0 && f.xMax == 0 && f.yMax == 0 {
		t.Error("font bbox not populated")
	}
}

func TestParseTTF_Hhea(t *testing.T) {
	f, err := parseTTF(loadDejaVu(t))
	if err != nil {
		t.Fatal(err)
	}
	if f.ascent <= 0 {
		t.Errorf("ascent = %d, want positive", f.ascent)
	}
	if f.descent >= 0 {
		t.Errorf("descent = %d, want negative", f.descent)
	}
	if f.numOfLongHorMetrics == 0 {
		t.Error("numOfLongHorMetrics = 0")
	}
}

func TestParseTTF_Maxp(t *testing.T) {
	f, err := parseTTF(loadDejaVu(t))
	if err != nil {
		t.Fatal(err)
	}
	if f.numGlyphs < 256 {
		t.Errorf("numGlyphs = %d, want >= 256 for DejaVuSans", f.numGlyphs)
	}
}

func TestParseTTF_Hmtx(t *testing.T) {
	f, err := parseTTF(loadDejaVu(t))
	if err != nil {
		t.Fatal(err)
	}
	if len(f.glyphWidths) != int(f.numGlyphs) {
		t.Errorf("len(glyphWidths) = %d, want numGlyphs %d", len(f.glyphWidths), f.numGlyphs)
	}
	// glyphID 0 is always .notdef, should still have a width.
	if f.glyphWidths[0] == 0 {
		t.Error("glyphWidths[0] (.notdef) is zero — likely parse error")
	}
}
