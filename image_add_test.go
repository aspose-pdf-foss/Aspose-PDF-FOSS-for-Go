package asposepdf

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"strings"
	"testing"
)

func TestDetectImageFormat(t *testing.T) {
	tests := []struct {
		name    string
		data    []byte
		want    ImageFormat
		wantErr bool
	}{
		{"JPEG", []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10}, ImageFormatJPEG, false},
		{"PNG", []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}, ImageFormatPNG, false},
		{"unknown", []byte{0x00, 0x01, 0x02, 0x03}, 0, true},
		{"too short", []byte{0xFF}, 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := detectImageFormat(tt.data)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %d, want %d", got, tt.want)
			}
		})
	}
}

func TestParseJPEGHeader(t *testing.T) {
	// Build minimal JPEG: SOI + SOF0 marker with 100x80, 3 components (RGB).
	sof := []byte{
		0xFF, 0xD8, // SOI
		0xFF, 0xC0, // SOF0
		0x00, 0x0B, // length = 11
		0x08,       // precision = 8
		0x00, 0x50, // height = 80
		0x00, 0x64, // width = 100
		0x03,       // 3 components = RGB
		0x01, 0x22, 0x00, // component 1
		0x02, 0x11, 0x01, // component 2
		0x03, 0x11, 0x01, // component 3
	}
	info, err := parseJPEGHeader(bytes.NewReader(sof))
	if err != nil {
		t.Fatalf("parseJPEGHeader: %v", err)
	}
	if info.width != 100 || info.height != 80 {
		t.Errorf("dimensions = %dx%d, want 100x80", info.width, info.height)
	}
	if info.components != 3 {
		t.Errorf("components = %d, want 3", info.components)
	}
}

func TestParseJPEGHeaderGray(t *testing.T) {
	sof := []byte{
		0xFF, 0xD8, // SOI
		0xFF, 0xC0, // SOF0
		0x00, 0x08, // length = 8
		0x08,       // precision = 8
		0x00, 0x20, // height = 32
		0x00, 0x40, // width = 64
		0x01,       // 1 component = Gray
		0x01, 0x11, 0x00,
	}
	info, err := parseJPEGHeader(bytes.NewReader(sof))
	if err != nil {
		t.Fatalf("parseJPEGHeader: %v", err)
	}
	if info.width != 64 || info.height != 32 {
		t.Errorf("dimensions = %dx%d, want 64x32", info.width, info.height)
	}
	if info.components != 1 {
		t.Errorf("components = %d, want 1", info.components)
	}
}

func TestCreateJPEGXObject(t *testing.T) {
	// Minimal JPEG with SOF0: 100x80, 3 components.
	jpegData := []byte{
		0xFF, 0xD8,
		0xFF, 0xC0, 0x00, 0x0B, 0x08,
		0x00, 0x50, 0x00, 0x64, 0x03,
		0x01, 0x22, 0x00, 0x02, 0x11, 0x01, 0x03, 0x11, 0x01,
		0xFF, 0xD9,
	}

	stream, smask, err := createImageXObject(jpegData, ImageFormatJPEG)
	if err != nil {
		t.Fatalf("createImageXObject: %v", err)
	}
	if smask != nil {
		t.Error("expected nil smask for JPEG")
	}
	if dictGetName(stream.Dict, "/Subtype") != "/Image" {
		t.Error("expected /Subtype /Image")
	}
	if dictGetName(stream.Dict, "/Filter") != "/DCTDecode" {
		t.Error("expected /Filter /DCTDecode")
	}
	if dictGetInt(stream.Dict, "/Width") != 100 {
		t.Errorf("width = %d, want 100", dictGetInt(stream.Dict, "/Width"))
	}
	if dictGetInt(stream.Dict, "/Height") != 80 {
		t.Errorf("height = %d, want 80", dictGetInt(stream.Dict, "/Height"))
	}
	if stream.Decoded {
		t.Error("JPEG stream should have Decoded=false")
	}
	if !bytes.Equal(stream.Data, jpegData) {
		t.Error("JPEG data should be stored as-is")
	}
}

func TestCreatePNGXObject(t *testing.T) {
	pngData := createTestPNG(2, 2, false)
	stream, smask, err := createImageXObject(pngData, ImageFormatPNG)
	if err != nil {
		t.Fatalf("createImageXObject: %v", err)
	}
	if smask != nil {
		t.Error("expected nil smask for opaque PNG")
	}
	if dictGetName(stream.Dict, "/Subtype") != "/Image" {
		t.Error("expected /Subtype /Image")
	}
	if dictGetInt(stream.Dict, "/Width") != 2 {
		t.Errorf("width = %d, want 2", dictGetInt(stream.Dict, "/Width"))
	}
	if dictGetInt(stream.Dict, "/Height") != 2 {
		t.Errorf("height = %d, want 2", dictGetInt(stream.Dict, "/Height"))
	}
	if !stream.Decoded {
		t.Error("PNG stream should have Decoded=true")
	}
	// 2x2 RGB = 12 bytes of pixel data.
	if len(stream.Data) != 12 {
		t.Errorf("data len = %d, want 12", len(stream.Data))
	}
}

func TestCreatePNGXObjectWithAlpha(t *testing.T) {
	pngData := createTestPNG(2, 2, true)
	stream, smask, err := createImageXObject(pngData, ImageFormatPNG)
	if err != nil {
		t.Fatalf("createImageXObject: %v", err)
	}
	if stream == nil {
		t.Fatal("expected non-nil stream")
	}
	if smask == nil {
		t.Fatal("expected non-nil smask for RGBA PNG")
	}
	if dictGetName(smask.Dict, "/Subtype") != "/Image" {
		t.Error("smask should be /Image")
	}
	if dictGetName(smask.Dict, "/ColorSpace") != "/DeviceGray" {
		t.Error("smask should be DeviceGray")
	}
	if dictGetInt(smask.Dict, "/Width") != 2 || dictGetInt(smask.Dict, "/Height") != 2 {
		t.Error("smask dimensions should match image")
	}
	if len(smask.Data) != 4 {
		t.Errorf("smask data len = %d, want 4", len(smask.Data))
	}
}

// createTestPNG generates a minimal PNG file as bytes.
func createTestPNG(w, h int, withAlpha bool) []byte {
	var buf bytes.Buffer
	if withAlpha {
		img := image.NewNRGBA(image.Rect(0, 0, w, h))
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				img.SetNRGBA(x, y, color.NRGBA{R: 255, G: 0, B: 0, A: 128})
			}
		}
		png.Encode(&buf, img)
	} else {
		img := image.NewNRGBA(image.Rect(0, 0, w, h))
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				img.SetNRGBA(x, y, color.NRGBA{R: 0, G: 128, B: 255, A: 255})
			}
		}
		png.Encode(&buf, img)
	}
	return buf.Bytes()
}

func TestAddImageToPage(t *testing.T) {
	doc := createBlankDocument(200, 300)

	jpegData := []byte{
		0xFF, 0xD8,
		0xFF, 0xC0, 0x00, 0x0B, 0x08,
		0x00, 0x0A, 0x00, 0x0A, 0x03,
		0x01, 0x22, 0x00, 0x02, 0x11, 0x01, 0x03, 0x11, 0x01,
		0xFF, 0xD9,
	}
	tmpFile := t.TempDir() + "/test.jpg"
	os.WriteFile(tmpFile, jpegData, 0o644)

	page, _ := doc.Page(1)
	err := page.AddImage(tmpFile, Rectangle{LLX: 10, LLY: 10, URX: 110, URY: 110})
	if err != nil {
		t.Fatalf("AddImage: %v", err)
	}

	contentData, err := page.contentStreams()
	if err != nil {
		t.Fatalf("contentStreams: %v", err)
	}
	content := string(contentData)
	if !strings.Contains(content, "Do") {
		t.Error("content stream should contain Do operator")
	}
	if !strings.Contains(content, "cm") {
		t.Error("content stream should contain cm operator")
	}

	res := page.pageResources()
	if res == nil {
		t.Fatal("page should have resources")
	}
	xobj, ok := res["/XObject"]
	if !ok {
		t.Fatal("resources should have /XObject")
	}
	xobjDict, ok := xobj.(pdfDict)
	if !ok {
		t.Fatal("/XObject should be a dict")
	}
	if len(xobjDict) != 1 {
		t.Errorf("expected 1 XObject entry, got %d", len(xobjDict))
	}
}

func TestAddImageInvalidRect(t *testing.T) {
	doc := createBlankDocument(200, 300)
	page, _ := doc.Page(1)

	tmpFile := t.TempDir() + "/test.jpg"
	os.WriteFile(tmpFile, []byte{0xFF, 0xD8, 0xFF, 0xC0, 0x00, 0x0B, 0x08, 0x00, 0x0A, 0x00, 0x0A, 0x03, 0x01, 0x22, 0x00, 0x02, 0x11, 0x01, 0x03, 0x11, 0x01, 0xFF, 0xD9}, 0o644)

	err := page.AddImage(tmpFile, Rectangle{LLX: 100, LLY: 10, URX: 50, URY: 110})
	if err == nil {
		t.Fatal("expected error for invalid rectangle")
	}
}

func createBlankDocument(width, height float64) *Document {
	contentStream := &pdfStream{
		Dict:    pdfDict{},
		Data:    []byte{},
		Decoded: true,
	}
	contentObj := &pdfObject{Num: 1, Value: contentStream}

	pageDict := pdfDict{
		"/Type":      pdfName("/Page"),
		"/MediaBox":  pdfArray{0.0, 0.0, width, height},
		"/Resources": pdfDict{},
		"/Contents":  pdfRef{Num: 1},
	}
	pageObj := &pdfObject{Num: 2, Value: pageDict}

	return &Document{
		objects: map[int]*pdfObject{1: contentObj, 2: pageObj},
		pages:   []*pdfObject{pageObj},
		nextID:  3,
	}
}
