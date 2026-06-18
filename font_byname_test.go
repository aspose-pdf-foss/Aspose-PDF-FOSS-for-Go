// SPDX-License-Identifier: MIT

package asposepdf_test

import (
	"bytes"
	"image"
	"strings"
	"testing"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

func nonWhitePixels(img image.Image) int {
	n := 0
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bl, _ := img.At(x, y).RGBA()
			if r < 0xe000 || g < 0xe000 || bl < 0xe000 {
				n++
			}
		}
	}
	return n
}

// TestLoadFontByNameRegisteredTTF resolves a registered .ttf by its family name
// (not a file path), embeds it, and renders it — the core pdf-go-ha1 path. The
// literal family must win over the Standard-14 expansion ("DejaVu Sans" contains
// "sans" but must not resolve to Arial).
func TestLoadFontByNameRegisteredTTF(t *testing.T) {
	pdf.AddFontFile("testdata/DejaVuSans.ttf")
	defer pdf.ClearFontSources()

	doc := pdf.NewDocument(420, 120)
	f, err := doc.LoadFontByName("DejaVu Sans", false, false)
	if err != nil {
		t.Fatalf("LoadFontByName: %v", err)
	}
	if !f.IsEmbedded() {
		t.Error("font is not embedded")
	}
	if !strings.Contains(strings.ToLower(f.BaseFont()), "dejavu") {
		t.Errorf("BaseFont = %q, want a DejaVu face (literal family must beat the Arial alias)", f.BaseFont())
	}
	p, _ := doc.Page(1)
	if err := p.AddText("DejaVu by name", pdf.TextStyle{Font: f, Size: 28, Color: &pdf.Color{A: 1}},
		pdf.Rectangle{LLX: 15, LLY: 40, URX: 405, URY: 90}); err != nil {
		t.Fatalf("AddText: %v", err)
	}
	if _, err := doc.SubsetFonts(); err != nil { // the by-name font feeds the subset pipeline
		t.Fatalf("SubsetFonts: %v", err)
	}
	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatalf("WriteTo: %v", err)
	}
	out, err := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	img, err := out.RenderImage(1, pdf.RenderOptions{DPI: 150})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if n := nonWhitePixels(img); n < 200 {
		t.Errorf("by-name embedded font rendered almost nothing (%d px)", n)
	}
}

// TestLoadFontByNameRegisteredOTF resolves a registered OpenType-CFF (.otf) by
// family name and embeds it (via the /FontFile3 path).
func TestLoadFontByNameRegisteredOTF(t *testing.T) {
	pdf.AddFontFile("testdata/SourceSerif4-Regular.otf")
	defer pdf.ClearFontSources()

	doc := pdf.NewDocument(200, 100)
	f, err := doc.LoadFontByName("Source Serif 4", false, false)
	if err != nil {
		t.Fatalf("LoadFontByName(.otf): %v", err)
	}
	if !f.IsEmbedded() || !strings.Contains(strings.ToLower(f.BaseFont()), "sourceserif") {
		t.Errorf("BaseFont = %q, embedded=%v", f.BaseFont(), f.IsEmbedded())
	}
}

func TestLoadFontByNameNotFound(t *testing.T) {
	pdf.ClearFontSources()
	doc := pdf.NewDocument(200, 200)
	if _, err := doc.LoadFontByName("No Such Font Family ZZQ", false, false); err == nil {
		t.Error("LoadFontByName for an unknown family = nil error, want an error")
	}
}
