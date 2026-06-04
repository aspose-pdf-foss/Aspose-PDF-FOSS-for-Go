// SPDX-License-Identifier: MIT

package asposepdf_test

import (
	"bytes"
	"testing"

	asposepdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

// TestRenderSubsettedEmbeddedFont guards against the regression where a
// subsetted embedded TTF (SubsetFonts drops cmap/OS-2/name) could not be parsed
// by the renderer, leaving embedded-font text — e.g. the showcase's "Embedded
// TTF (DejaVu Sans) — Unicode" block — blank. Render must draw glyph pixels.
func TestRenderSubsettedEmbeddedFont(t *testing.T) {
	doc := asposepdf.NewDocument(220, 60)
	font, err := doc.LoadFont("testdata/DejaVuSans.ttf")
	if err != nil {
		t.Fatalf("LoadFont: %v", err)
	}
	p, _ := doc.Page(1)
	style := asposepdf.TextStyle{Font: font, Size: 28, Color: &asposepdf.Color{A: 1}}
	if err := p.AddText("Привет Ωμ", style,
		asposepdf.Rectangle{LLX: 8, LLY: 14, URX: 212, URY: 48}); err != nil {
		t.Fatalf("AddText: %v", err)
	}
	if _, err := doc.SubsetFonts(); err != nil {
		t.Fatalf("SubsetFonts: %v", err)
	}
	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatalf("WriteTo: %v", err)
	}

	d2, err := asposepdf.OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("OpenStream: %v", err)
	}
	img, err := d2.RenderImage(1, asposepdf.RenderOptions{DPI: 72})
	if err != nil {
		t.Fatalf("RenderImage: %v", err)
	}
	if !hasNonWhite(img, 0, 0, img.Bounds().Dx(), img.Bounds().Dy()) {
		t.Fatal("subsetted embedded-font text rendered blank")
	}
}
