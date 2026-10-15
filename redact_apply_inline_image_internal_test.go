// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"
)

// solidPNG encodes a tiny solid-color PNG for use as an inline image.
func solidPNG(t *testing.T, c color.Color) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			img.Set(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// TestApplyRedactionsDropsInlineImageInRegion pins the inline-image half of
// pdf-go-xp2u: a BI...ID...EI inline image (ISO 32000-1 §8.9.7) placed
// entirely inside a redaction region used to fall through
// rewriteXObjectDoOperatorsInStream's switch into its default case (pass
// through unchanged) -- the "Do" case only ever looked at Image/Form
// XObjects, never an inline image, even though both paint into the same
// unit square under the current CTM. Fixed by giving "BI" the same
// classifyDoBbox-based drop/clip/keep treatment as a non-Form "Do"
// (emitClassifiedImageOp).
//
// Verified structurally rather than by a literal byte marker: an inline
// image is decoded to raw samples, Flate-compressed, then ASCIIHex-wrapped
// before being written back into the content stream
// (image_inline_write.go), so a chosen pixel color does not survive as a
// predictable literal substring the way a passthrough JPEG's raw bytes do
// (contrast TestApplyRedactionsRemovesFullyCoveredImageBytes, pdf-go-ye1z,
// which exploits exactly that passthrough property for an XObject image).
// What IS directly verifiable, and is the actual vulnerability regardless
// of encoding, is whether a "BI" operator -- which carries the image's
// pixel data as its own literal operand -- survives redaction at all: if
// it does not, the pixel data cannot be present anywhere in the saved
// file, full stop, since nothing else in the content stream can carry it.
func TestApplyRedactionsDropsInlineImageInRegion(t *testing.T) {
	png1 := solidPNG(t, color.RGBA{R: 0x13, G: 0x37, B: 0xEE, A: 0xFF})

	d := NewDocumentFromFormat(PageFormatA4)
	p, err := d.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	imgRect := Rectangle{LLX: 50, LLY: 650, URX: 150, URY: 750}
	if err := p.AddInlineImageFromStream(bytes.NewReader(png1), imgRect); err != nil {
		t.Fatal(err)
	}
	if err := p.AddText("visible", TextStyle{Size: 12}, Rectangle{LLX: 50, LLY: 600, URX: 300, URY: 620}); err != nil {
		t.Fatal(err)
	}

	infosBefore, err := p.ImageInfos()
	if err != nil {
		t.Fatal(err)
	}
	if len(infosBefore) != 1 || !infosBefore[0].Inline {
		t.Fatalf("before redaction: got %+v, want exactly one inline image", infosBefore)
	}

	// Redact region strictly covers the inline image's placement rect.
	ra := NewRedactAnnotation(p, Rectangle{LLX: 40, LLY: 640, URX: 160, URY: 760})
	if err := p.Annotations().Add(ra); err != nil {
		t.Fatal(err)
	}

	if err := d.ApplyRedactions(); err != nil {
		t.Fatal(err)
	}

	// Structural check on the in-memory, just-redacted page: no "BI"
	// operator should remain at all (there was only ever the one, and it
	// was fully inside the region).
	data, err := p.contentStreams()
	if err != nil {
		t.Fatal(err)
	}
	ops, err := parseContentStream(data)
	if err != nil {
		t.Fatal(err)
	}
	for _, op := range ops {
		if op.Operator == "BI" {
			t.Fatalf("redacted content stream still contains a BI (inline image) operator")
		}
	}

	var buf bytes.Buffer
	if _, err := d.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	p2, err := reopened.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	infosAfter, err := p2.ImageInfos()
	if err != nil {
		t.Fatal(err)
	}
	if len(infosAfter) != 0 {
		t.Errorf("after redaction+reopen: got %d image(s) on the page, want 0", len(infosAfter))
	}
	txt, err := p2.ExtractText()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(txt, "visible") {
		t.Errorf("unrelated text was lost: got %q", txt)
	}
}
