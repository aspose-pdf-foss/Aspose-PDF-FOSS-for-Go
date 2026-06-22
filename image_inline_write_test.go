// SPDX-License-Identifier: MIT

package asposepdf_test

import (
	"bytes"
	"os"
	"testing"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

// TestAddInlineImage writes a PNG and a JPEG as inline images (BI/ID/EI), then
// confirms both round-trip: the saved document re-opens and extraction finds
// both images.
func TestAddInlineImage(t *testing.T) {
	doc := pdf.NewDocument(300, 200)
	p, err := doc.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.AddInlineImage("testdata/aspose-logo.png", pdf.Rectangle{LLX: 20, LLY: 110, URX: 140, URY: 180}); err != nil {
		t.Fatalf("AddInlineImage png: %v", err)
	}
	if err := p.AddInlineImage("testdata/Koala.jpg", pdf.Rectangle{LLX: 160, LLY: 30, URX: 280, URY: 170}); err != nil {
		t.Fatalf("AddInlineImage jpg: %v", err)
	}

	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatalf("WriteTo: %v", err)
	}

	out, err := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	imgs, err := out.ExtractImages()
	if err != nil {
		t.Fatalf("ExtractImages: %v", err)
	}
	n := 0
	for _, pg := range imgs {
		n += len(pg)
	}
	if n != 2 {
		t.Errorf("got %d inline images after round-trip, want 2", n)
	}
}

// TestAddInlineImageFromStream covers the io.Reader variant.
func TestAddInlineImageFromStream(t *testing.T) {
	data, err := os.ReadFile("testdata/Penguins.png")
	if err != nil {
		t.Skipf("test image unavailable: %v", err)
	}
	doc := pdf.NewDocument(200, 200)
	p, _ := doc.Page(1)
	if err := p.AddInlineImageFromStream(bytes.NewReader(data), pdf.Rectangle{LLX: 10, LLY: 10, URX: 190, URY: 190}); err != nil {
		t.Fatalf("AddInlineImageFromStream: %v", err)
	}
	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatalf("WriteTo: %v", err)
	}
	if _, err := pdf.OpenStream(bytes.NewReader(buf.Bytes())); err != nil {
		t.Fatalf("reopen: %v", err)
	}
}

// TestAddInlineImageErrors checks the rejected inputs.
func TestAddInlineImageErrors(t *testing.T) {
	doc := pdf.NewDocument(200, 200)
	p, _ := doc.Page(1)

	// Empty rectangle.
	if err := p.AddInlineImage("testdata/aspose-logo.png", pdf.Rectangle{}); err == nil {
		t.Error("expected error for empty rect")
	}
	// Not an image.
	if err := p.AddInlineImageFromStream(bytes.NewReader([]byte("not an image")), pdf.Rectangle{LLX: 0, LLY: 0, URX: 10, URY: 10}); err == nil {
		t.Error("expected error for non-image data")
	}
}
