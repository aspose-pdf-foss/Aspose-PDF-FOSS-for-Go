// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"testing"
)

func TestImageInfoHasPage(t *testing.T) {
	doc := createDocWithImage()
	page, _ := doc.Page(1)
	infos, err := page.ImageInfos()
	if err != nil {
		t.Fatalf("ImageInfos: %v", err)
	}
	if len(infos) == 0 {
		t.Fatal("expected at least 1 image info")
	}
	if infos[0].page == nil {
		t.Error("ImageInfo.page should be set")
	}
}

func TestReplaceImageJPEG(t *testing.T) {
	doc := createDocWithImage()
	page, _ := doc.Page(1)
	infos, _ := page.ImageInfos()

	// Create a different JPEG to replace with.
	newJPEG := buildMinimalJPEG(20, 15, 3)
	tmpFile := t.TempDir() + "/new.jpg"
	os.WriteFile(tmpFile, newJPEG, 0o644)

	err := infos[0].Replace(tmpFile)
	if err != nil {
		t.Fatalf("Replace: %v", err)
	}

	// Verify the stream was updated in place.
	if dictGetInt(infos[0].stream.Dict, "/Width") != 20 {
		t.Errorf("width = %d, want 20", dictGetInt(infos[0].stream.Dict, "/Width"))
	}
	if dictGetInt(infos[0].stream.Dict, "/Height") != 15 {
		t.Errorf("height = %d, want 15", dictGetInt(infos[0].stream.Dict, "/Height"))
	}
	if dictGetName(infos[0].stream.Dict, "/Filter") != "/DCTDecode" {
		t.Errorf("filter = %s, want /DCTDecode", dictGetName(infos[0].stream.Dict, "/Filter"))
	}
}

func TestReplaceImagePNGToJPEG(t *testing.T) {
	// Build doc with PNG image that has SMask.
	doc := createDocWithPNGImage()
	page, _ := doc.Page(1)
	infos, _ := page.ImageInfos()

	if _, hasSMask := infos[0].stream.Dict["/SMask"]; !hasSMask {
		t.Fatal("setup: expected PNG image to have SMask")
	}

	newJPEG := buildMinimalJPEG(20, 15, 3)
	tmpFile := t.TempDir() + "/new.jpg"
	os.WriteFile(tmpFile, newJPEG, 0o644)

	err := infos[0].Replace(tmpFile)
	if err != nil {
		t.Fatalf("Replace: %v", err)
	}

	if _, hasSMask := infos[0].stream.Dict["/SMask"]; hasSMask {
		t.Error("SMask should be removed after replacing with JPEG")
	}
	if dictGetName(infos[0].stream.Dict, "/Filter") != "/DCTDecode" {
		t.Error("filter should be /DCTDecode after JPEG replacement")
	}
}

func TestReplaceImageJPEGToPNGWithAlpha(t *testing.T) {
	doc := createDocWithImage() // has JPEG
	page, _ := doc.Page(1)
	infos, _ := page.ImageInfos()

	pngData := createTestPNGAlpha(8, 8)
	tmpFile := t.TempDir() + "/new.png"
	os.WriteFile(tmpFile, pngData, 0o644)

	err := infos[0].Replace(tmpFile)
	if err != nil {
		t.Fatalf("Replace: %v", err)
	}

	if _, hasSMask := infos[0].stream.Dict["/SMask"]; !hasSMask {
		t.Error("SMask should be added after replacing with PNG with alpha")
	}
	if dictGetName(infos[0].stream.Dict, "/Filter") != "" {
		t.Error("filter should be empty (Decoded=true, writer adds FlateDecode)")
	}
	if !infos[0].stream.Decoded {
		t.Error("stream should be Decoded=true for PNG replacement")
	}
}

func TestReplaceImageInvalidInfo(t *testing.T) {
	info := &ImageInfo{}
	err := info.Replace("testdata/Koala.jpg")
	if err == nil {
		t.Fatal("expected error for nil stream")
	}
}

// buildMinimalJPEG constructs a minimal JPEG with a SOF0 marker declaring the given dimensions.
func buildMinimalJPEG(width, height, components int) []byte {
	var buf []byte
	buf = append(buf, 0xFF, 0xD8) // SOI
	// SOF0: FF C0, length, precision, height, width, components
	buf = append(buf, 0xFF, 0xC0)
	segLen := 8 + components*3
	buf = append(buf, byte(segLen>>8), byte(segLen))
	buf = append(buf, 0x08) // precision
	buf = append(buf, byte(height>>8), byte(height))
	buf = append(buf, byte(width>>8), byte(width))
	buf = append(buf, byte(components))
	for i := 0; i < components; i++ {
		buf = append(buf, byte(i+1), 0x22, 0x00)
	}
	buf = append(buf, 0xFF, 0xD9) // EOI
	return buf
}

// createDocWithPNGImage builds a Document with one page containing a PNG XObject with SMask.
func createDocWithPNGImage() *Document {
	pngData := createTestPNGAlpha(4, 4)
	imgStream, smaskStream, _ := createImageXObject(pngData, ImageFormatPNG)

	smaskObj := &pdfObject{Num: 1, Value: smaskStream}
	imgStream.Dict["/SMask"] = pdfRef{Num: 1}
	imgObj := &pdfObject{Num: 2, Value: imgStream}

	contentData := "q\n10 0 0 10 50 50 cm\n/Im0 Do\nQ\n"
	contentStream := &pdfStream{
		Dict:    pdfDict{},
		Data:    []byte(contentData),
		Decoded: true,
	}
	contentObj := &pdfObject{Num: 3, Value: contentStream}

	pageDict := pdfDict{
		"/Type":     pdfName("/Page"),
		"/MediaBox": pdfArray{0.0, 0.0, 200.0, 300.0},
		"/Resources": pdfDict{
			"/XObject": pdfDict{
				"/Im0": pdfRef{Num: 2},
			},
		},
		"/Contents": pdfRef{Num: 3},
	}
	pageObj := &pdfObject{Num: 4, Value: pageDict}

	return &Document{
		objects: map[int]*pdfObject{1: smaskObj, 2: imgObj, 3: contentObj, 4: pageObj},
		pages:   []*pdfObject{pageObj},
		nextID:  5,
	}
}

// createTestPNGAlpha creates a small PNG with alpha channel.
func createTestPNGAlpha(w, h int) []byte {
	var buf bytes.Buffer
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: 255, G: 0, B: 0, A: 128})
		}
	}
	png.Encode(&buf, img)
	return buf.Bytes()
}

// createDocWithImage builds a Document with one page containing a JPEG XObject.
func createDocWithImage() *Document {
	jpegData := []byte{
		0xFF, 0xD8,
		0xFF, 0xC0, 0x00, 0x0B, 0x08,
		0x00, 0x0A, 0x00, 0x0A, 0x03,
		0x01, 0x22, 0x00, 0x02, 0x11, 0x01, 0x03, 0x11, 0x01,
		0xFF, 0xD9,
	}

	imgStream := &pdfStream{
		Dict: pdfDict{
			"/Type":             pdfName("/XObject"),
			"/Subtype":          pdfName("/Image"),
			"/Width":            10,
			"/Height":           10,
			"/BitsPerComponent": 8,
			"/ColorSpace":       pdfName("/DeviceRGB"),
			"/Filter":           pdfName("/DCTDecode"),
		},
		Data:    jpegData,
		Decoded: false,
	}
	imgObj := &pdfObject{Num: 1, Value: imgStream}

	contentData := "q\n10 0 0 10 50 50 cm\n/Im0 Do\nQ\n"
	contentStream := &pdfStream{
		Dict:    pdfDict{},
		Data:    []byte(contentData),
		Decoded: true,
	}
	contentObj := &pdfObject{Num: 2, Value: contentStream}

	pageDict := pdfDict{
		"/Type":     pdfName("/Page"),
		"/MediaBox": pdfArray{0.0, 0.0, 200.0, 300.0},
		"/Resources": pdfDict{
			"/XObject": pdfDict{
				"/Im0": pdfRef{Num: 1},
			},
		},
		"/Contents": pdfRef{Num: 2},
	}
	pageObj := &pdfObject{Num: 3, Value: pageDict}

	return &Document{
		objects: map[int]*pdfObject{1: imgObj, 2: contentObj, 3: pageObj},
		pages:   []*pdfObject{pageObj},
		nextID:  4,
	}
}
