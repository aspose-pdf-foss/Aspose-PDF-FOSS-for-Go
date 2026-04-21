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

func TestParseTTF_CmapLatin(t *testing.T) {
	f, err := parseTTF(loadDejaVu(t))
	if err != nil {
		t.Fatal(err)
	}
	if g := f.glyphID('A'); g == 0 {
		t.Error("glyphID('A') = 0, want non-zero")
	}
	if g := f.glyphID(' '); g == 0 {
		t.Error("glyphID(' ') = 0, want non-zero")
	}
}

func TestParseTTF_CmapCyrillic(t *testing.T) {
	f, err := parseTTF(loadDejaVu(t))
	if err != nil {
		t.Fatal(err)
	}
	if g := f.glyphID('Я'); g == 0 {
		t.Error("glyphID('Я') = 0, want non-zero (DejaVu covers Cyrillic)")
	}
	if g := f.glyphID('ж'); g == 0 {
		t.Error("glyphID('ж') = 0, want non-zero")
	}
}

func TestParseTTF_CmapMissing(t *testing.T) {
	f, err := parseTTF(loadDejaVu(t))
	if err != nil {
		t.Fatal(err)
	}
	// DejaVuSans does not cover CJK.
	if g := f.glyphID('日'); g != 0 {
		t.Errorf("glyphID('日') = %d, want 0 (CJK not in DejaVuSans)", g)
	}
}

func TestParseTTF_AdvanceKnown(t *testing.T) {
	f, err := parseTTF(loadDejaVu(t))
	if err != nil {
		t.Fatal(err)
	}
	gid := f.glyphID('A')
	if gid == 0 {
		t.Fatal("glyphID('A') = 0")
	}
	advA := f.glyphWidths[gid]
	if advA == 0 {
		t.Errorf("advance for 'A' = 0")
	}
	// In DejaVuSans 'A' is wider than ' '.
	gidSp := f.glyphID(' ')
	if f.glyphWidths[gidSp] >= advA {
		t.Errorf("' ' advance (%d) >= 'A' advance (%d), unexpected for DejaVuSans",
			f.glyphWidths[gidSp], advA)
	}
}

func TestParseTTF_OS2(t *testing.T) {
	f, err := parseTTF(loadDejaVu(t))
	if err != nil {
		t.Fatal(err)
	}
	if f.weight == 0 {
		t.Error("weight not populated")
	}
	// DejaVuSans is regular (not bold/italic).
	if f.flagsBold {
		t.Error("DejaVuSans flagged Bold (wrong)")
	}
	if f.flagsItalic {
		t.Error("DejaVuSans flagged Italic (wrong)")
	}
	// capHeight may be 0 if OS/2 version doesn't include it (v0/v1).
	// Just check that it's not negative (which would indicate parse error).
	if f.capHeight < 0 {
		t.Errorf("capHeight = %d, want non-negative", f.capHeight)
	}
}

func TestParseTTF_Post(t *testing.T) {
	f, err := parseTTF(loadDejaVu(t))
	if err != nil {
		t.Fatal(err)
	}
	// DejaVuSans is proportional, italic angle 0.
	if f.italicAngle != 0 {
		t.Errorf("italicAngle = %g, want 0", f.italicAngle)
	}
	if f.isFixedPitch {
		t.Error("DejaVuSans flagged FixedPitch (wrong)")
	}
}

func TestParseTTF_Name(t *testing.T) {
	f, err := parseTTF(loadDejaVu(t))
	if err != nil {
		t.Fatal(err)
	}
	if f.postScriptName != "DejaVuSans" {
		t.Errorf("postScriptName = %q, want DejaVuSans", f.postScriptName)
	}
}
