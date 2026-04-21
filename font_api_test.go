package asposepdf

import (
	"strings"
	"testing"
)

func TestFindFontExact(t *testing.T) {
	f, err := FindFont("Helvetica")
	if err != nil {
		t.Fatalf("FindFont: %v", err)
	}
	if f.BaseFont() != "Helvetica" {
		t.Errorf("FindFont(\"Helvetica\").BaseFont() = %q, want Helvetica", f.BaseFont())
	}
}

func TestFindFontCaseInsensitive(t *testing.T) {
	cases := []string{"helvetica", "HELVETICA", "HeLvEtIcA"}
	for _, name := range cases {
		f, err := FindFont(name)
		if err != nil {
			t.Fatalf("FindFont(%q): %v", name, err)
		}
		if f.BaseFont() != "Helvetica" {
			t.Errorf("FindFont(%q) = %q, want Helvetica", name, f.BaseFont())
		}
	}
}

func TestFindFontAllStandard14(t *testing.T) {
	names := []string{
		"Helvetica", "Helvetica-Bold", "Helvetica-Oblique", "Helvetica-BoldOblique",
		"Times-Roman", "Times-Bold", "Times-Italic", "Times-BoldItalic",
		"Courier", "Courier-Bold", "Courier-Oblique", "Courier-BoldOblique",
		"Symbol", "ZapfDingbats",
	}
	for _, name := range names {
		f, err := FindFont(name)
		if err != nil {
			t.Errorf("FindFont(%q): %v", name, err)
			continue
		}
		if f.BaseFont() != name {
			t.Errorf("FindFont(%q).BaseFont() = %q", name, f.BaseFont())
		}
	}
}

func TestFindFontUnknown(t *testing.T) {
	_, err := FindFont("Arial")
	if err == nil {
		t.Fatal("FindFont(\"Arial\"): expected error, got nil")
	}
	if !strings.Contains(err.Error(), "unknown") {
		t.Errorf("error message = %q, expected to contain \"unknown\"", err.Error())
	}
}

func TestLoadFont_DejaVu(t *testing.T) {
	doc := NewDocument(595, 842)
	f, err := doc.LoadFont("testdata/DejaVuSans.ttf")
	if err != nil {
		t.Fatalf("LoadFont: %v", err)
	}
	if f.BaseFont() != "DejaVuSans" {
		t.Errorf("BaseFont() = %q, want DejaVuSans", f.BaseFont())
	}
	if !f.IsEmbedded() {
		t.Error("IsEmbedded() = false, want true")
	}
}

func TestLoadFont_MissingFile(t *testing.T) {
	doc := NewDocument(595, 842)
	_, err := doc.LoadFont("testdata/does_not_exist.ttf")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
	if !strings.Contains(err.Error(), "load font") {
		t.Errorf("error = %q, want to contain 'load font'", err.Error())
	}
}

func TestLoadFontFromStream_NotTTF(t *testing.T) {
	doc := NewDocument(595, 842)
	_, err := doc.LoadFontFromStream(strings.NewReader("not a font"))
	if err == nil {
		t.Fatal("expected error for non-TTF stream")
	}
}
