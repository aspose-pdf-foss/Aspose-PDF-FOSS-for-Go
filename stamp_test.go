// SPDX-License-Identifier: MIT

package asposepdf_test

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

// reopen writes doc to memory and opens it back, so tests assert against the
// serialized-then-parsed result (not just in-memory state).
func reopen(t *testing.T, doc *pdf.Document) *pdf.Document {
	t.Helper()
	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatalf("WriteTo: %v", err)
	}
	out, err := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("OpenStream: %v", err)
	}
	return out
}

func TestTextStampExtractable(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	stamp := pdf.NewTextStamp("CONFIDENTIAL", pdf.TextStyle{Font: pdf.FontHelveticaBold, Size: 36})
	stamp.HAlign = pdf.HAlignCenter
	stamp.VAlign = pdf.VAlignMiddle
	stamp.Opacity = 0.5
	stamp.RotateAngle = 45
	stamp.Background = true // behind content
	p, _ := doc.Page(1)
	if err := p.AddStamp(stamp); err != nil {
		t.Fatalf("AddStamp: %v", err)
	}

	out := reopen(t, doc)
	txt, err := out.ExtractText()
	if err != nil {
		t.Fatalf("ExtractText: %v", err)
	}
	if !strings.Contains(txt[0], "CONFIDENTIAL") {
		t.Errorf("page text = %q, want it to contain CONFIDENTIAL", txt[0])
	}
}

func TestPageNumberStampPerPage(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	if err := doc.AddBlankPage(595, 842); err != nil {
		t.Fatal(err)
	}
	if err := doc.AddBlankPage(595, 842); err != nil {
		t.Fatal(err)
	}
	stamp := pdf.NewPageNumberStamp("Page {0} of {1}", pdf.TextStyle{Font: pdf.FontHelvetica, Size: 10})
	stamp.VAlign = pdf.VAlignBottom
	stamp.HAlign = pdf.HAlignCenter
	if err := doc.AddStamp(stamp); err != nil { // all pages
		t.Fatalf("AddStamp: %v", err)
	}

	out := reopen(t, doc)
	txt, err := out.ExtractText()
	if err != nil {
		t.Fatalf("ExtractText: %v", err)
	}
	for i, want := range []string{"Page 1 of 3", "Page 2 of 3", "Page 3 of 3"} {
		if !strings.Contains(txt[i], want) {
			t.Errorf("page %d text = %q, want it to contain %q", i+1, txt[i], want)
		}
	}
}

func TestPageNumberStampStartingNumber(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	stamp := pdf.NewPageNumberStamp("{0}", pdf.TextStyle{Font: pdf.FontHelvetica, Size: 10})
	stamp.StartingNumber = 5
	p, _ := doc.Page(1)
	if err := p.AddStamp(stamp); err != nil {
		t.Fatalf("AddStamp: %v", err)
	}
	out := reopen(t, doc)
	txt, _ := out.ExtractText()
	if !strings.Contains(txt[0], "5") {
		t.Errorf("page text = %q, want it to contain 5 (StartingNumber)", txt[0])
	}
}

func TestImageStamp(t *testing.T) {
	// Build a tiny 4x4 red PNG.
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			img.Set(x, y, color.RGBA{255, 0, 0, 255})
		}
	}
	var pngBuf bytes.Buffer
	if err := png.Encode(&pngBuf, img); err != nil {
		t.Fatal(err)
	}

	doc := pdf.NewDocument(595, 842)
	stamp, err := pdf.NewImageStampFromStream(bytes.NewReader(pngBuf.Bytes()))
	if err != nil {
		t.Fatalf("NewImageStampFromStream: %v", err)
	}
	stamp.Rect = pdf.Rectangle{LLX: 50, LLY: 700, URX: 150, URY: 800}
	stamp.Opacity = 0.7
	stamp.RotateAngle = 30
	p, _ := doc.Page(1)
	if err := p.AddStamp(stamp); err != nil {
		t.Fatalf("AddStamp: %v", err)
	}

	out := reopen(t, doc)
	p2, _ := out.Page(1)
	infos, err := p2.ImageInfos()
	if err != nil {
		t.Fatalf("ImageInfos: %v", err)
	}
	if len(infos) != 1 {
		t.Errorf("image count = %d, want 1 (the stamped image)", len(infos))
	}
}

func TestAddStampNil(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	p, _ := doc.Page(1)
	if err := p.AddStamp(nil); err == nil {
		t.Error("AddStamp(nil) = nil error, want an error")
	}
}
