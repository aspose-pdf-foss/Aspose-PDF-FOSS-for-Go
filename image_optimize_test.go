// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"testing"
)

func TestRawPixelsToImageRGB(t *testing.T) {
	// 2x2 red/green/blue/white image.
	pixels := []byte{
		255, 0, 0, 0, 255, 0,
		0, 0, 255, 255, 255, 255,
	}
	img := rawPixelsToImage(pixels, 2, 2, "/DeviceRGB")
	if img == nil {
		t.Fatal("expected non-nil image")
	}
	bounds := img.Bounds()
	if bounds.Dx() != 2 || bounds.Dy() != 2 {
		t.Fatalf("bounds = %v, want 2x2", bounds)
	}
	r, g, b, a := img.At(0, 0).RGBA()
	if r>>8 != 255 || g>>8 != 0 || b>>8 != 0 || a>>8 != 255 {
		t.Errorf("pixel (0,0) = (%d,%d,%d,%d), want red", r>>8, g>>8, b>>8, a>>8)
	}
}

func TestRawPixelsToImageGray(t *testing.T) {
	pixels := []byte{0, 128, 255, 64}
	img := rawPixelsToImage(pixels, 2, 2, "/DeviceGray")
	if img == nil {
		t.Fatal("expected non-nil image")
	}
	g := img.(*image.Gray)
	if g.GrayAt(0, 0).Y != 0 {
		t.Errorf("pixel (0,0) = %d, want 0", g.GrayAt(0, 0).Y)
	}
	if g.GrayAt(1, 0).Y != 128 {
		t.Errorf("pixel (1,0) = %d, want 128", g.GrayAt(1, 0).Y)
	}
}

func TestRawPixelsToImageUnsupported(t *testing.T) {
	img := rawPixelsToImage([]byte{0, 0, 0, 0}, 1, 1, "/DeviceCMYK")
	if img != nil {
		t.Error("expected nil for unsupported color space")
	}
}

func TestDownscaleImage(t *testing.T) {
	// 4x4 image: top-left quadrant red, top-right green, bottom-left blue, bottom-right white.
	src := image.NewNRGBA(image.Rect(0, 0, 4, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			var c color.NRGBA
			switch {
			case x < 2 && y < 2:
				c = color.NRGBA{R: 255, G: 0, B: 0, A: 255}
			case x >= 2 && y < 2:
				c = color.NRGBA{R: 0, G: 255, B: 0, A: 255}
			case x < 2 && y >= 2:
				c = color.NRGBA{R: 0, G: 0, B: 255, A: 255}
			default:
				c = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
			}
			src.SetNRGBA(x, y, c)
		}
	}

	dst := downscaleImage(src, 2, 2)
	if dst.Bounds().Dx() != 2 || dst.Bounds().Dy() != 2 {
		t.Fatalf("bounds = %v, want 2x2", dst.Bounds())
	}

	// Each output pixel should be the average of a 2x2 block from the source.
	// (0,0) = average of red block = (255, 0, 0)
	r, g, b, _ := dst.At(0, 0).RGBA()
	if r>>8 != 255 || g>>8 != 0 || b>>8 != 0 {
		t.Errorf("pixel (0,0) = (%d,%d,%d), want red", r>>8, g>>8, b>>8)
	}
	// (1,0) = average of green block = (0, 255, 0)
	r, g, b, _ = dst.At(1, 0).RGBA()
	if r>>8 != 0 || g>>8 != 255 || b>>8 != 0 {
		t.Errorf("pixel (1,0) = (%d,%d,%d), want green", r>>8, g>>8, b>>8)
	}
}

func TestDownscaleImagePreservesNonSquare(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 100, 50))
	dst := downscaleImage(src, 50, 25)
	if dst.Bounds().Dx() != 50 || dst.Bounds().Dy() != 25 {
		t.Fatalf("bounds = %v, want 50x25", dst.Bounds())
	}
}

func TestOptimizeImagesNoOp(t *testing.T) {
	// JPEG 10x10 displayed at 10x10 pt → DPI = 10/(10/72) = 72.
	// MaxDPI=150 → no downscale needed, JPEG stays as-is.
	doc := createDocWithImage() // 10x10 JPEG, CTM has 10pt display
	origData := make([]byte, len(doc.objects[1].Value.(*pdfStream).Data))
	copy(origData, doc.objects[1].Value.(*pdfStream).Data)

	count, err := doc.OptimizeImages(OptimizeImageOptions{MaxDPI: 150})
	if err != nil {
		t.Fatalf("OptimizeImages: %v", err)
	}
	if count != 0 {
		t.Errorf("count = %d, want 0 (JPEG already below MaxDPI)", count)
	}

	// Verify data untouched.
	stream := doc.objects[1].Value.(*pdfStream)
	if len(stream.Data) != len(origData) {
		t.Errorf("data length changed: %d → %d", len(origData), len(stream.Data))
	}
}

func TestOptimizeImagesDownscale(t *testing.T) {
	// Build a doc with a 100x100 JPEG displayed at 50x50 pt.
	// DPI = 100 / (50/72) = 144. MaxDPI=72 → should downscale to ~50x50.

	// Create a valid 100x100 JPEG.
	srcImg := image.NewNRGBA(image.Rect(0, 0, 100, 100))
	for y := 0; y < 100; y++ {
		for x := 0; x < 100; x++ {
			srcImg.SetNRGBA(x, y, color.NRGBA{R: 128, G: 64, B: 32, A: 255})
		}
	}
	var jpegBuf bytes.Buffer
	jpeg.Encode(&jpegBuf, srcImg, &jpeg.Options{Quality: 90})
	jpegData := jpegBuf.Bytes()

	imgStream := &pdfStream{
		Dict: pdfDict{
			"/Type":             pdfName("/XObject"),
			"/Subtype":          pdfName("/Image"),
			"/Width":            100,
			"/Height":           100,
			"/BitsPerComponent": 8,
			"/ColorSpace":       pdfName("/DeviceRGB"),
			"/Filter":           pdfName("/DCTDecode"),
		},
		Data:    jpegData,
		Decoded: false,
	}
	imgObj := &pdfObject{Num: 1, Value: imgStream}

	// CTM: 50 0 0 50 0 0 → 50pt x 50pt display.
	contentData := "q\n50 0 0 50 10 10 cm\n/Im0 Do\nQ\n"
	contentStream := &pdfStream{
		Dict:    pdfDict{},
		Data:    []byte(contentData),
		Decoded: true,
	}
	contentObj := &pdfObject{Num: 2, Value: contentStream}

	pageDict := pdfDict{
		"/Type":     pdfName("/Page"),
		"/MediaBox": pdfArray{0.0, 0.0, 612.0, 792.0},
		"/Resources": pdfDict{
			"/XObject": pdfDict{
				"/Im0": pdfRef{Num: 1},
			},
		},
		"/Contents": pdfRef{Num: 2},
	}
	pageObj := &pdfObject{Num: 3, Value: pageDict}

	doc := &Document{
		objects: map[int]*pdfObject{1: imgObj, 2: contentObj, 3: pageObj},
		pages:   []*pdfObject{pageObj},
		nextID:  4,
	}

	count, err := doc.OptimizeImages(OptimizeImageOptions{MaxDPI: 72})
	if err != nil {
		t.Fatalf("OptimizeImages: %v", err)
	}
	if count != 1 {
		t.Errorf("count = %d, want 1", count)
	}

	// Width and Height should be reduced.
	w := dictGetInt(imgStream.Dict, "/Width")
	h := dictGetInt(imgStream.Dict, "/Height")
	if w >= 100 || h >= 100 {
		t.Errorf("expected downscaled dimensions, got %dx%d", w, h)
	}
	if w != 50 || h != 50 {
		t.Errorf("dimensions = %dx%d, want 50x50", w, h)
	}
}

func TestOptimizeImagesPNGToJPEG(t *testing.T) {
	// Build doc with opaque PNG (no SMask, Decoded=true, no /Filter).
	pixels := make([]byte, 10*10*3) // 10x10 RGB
	for i := range pixels {
		pixels[i] = 128
	}
	imgStream := &pdfStream{
		Dict: pdfDict{
			"/Type":             pdfName("/XObject"),
			"/Subtype":          pdfName("/Image"),
			"/Width":            10,
			"/Height":           10,
			"/BitsPerComponent": 8,
			"/ColorSpace":       pdfName("/DeviceRGB"),
		},
		Data:    pixels,
		Decoded: true,
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

	doc := &Document{
		objects: map[int]*pdfObject{1: imgObj, 2: contentObj, 3: pageObj},
		pages:   []*pdfObject{pageObj},
		nextID:  4,
	}

	count, err := doc.OptimizeImages(OptimizeImageOptions{ConvertPNGToJPEG: true})
	if err != nil {
		t.Fatalf("OptimizeImages: %v", err)
	}
	if count != 1 {
		t.Errorf("count = %d, want 1", count)
	}

	// Verify filter changed to DCTDecode.
	filter := dictGetName(imgStream.Dict, "/Filter")
	if filter != "/DCTDecode" {
		t.Errorf("filter = %s, want /DCTDecode", filter)
	}
	if imgStream.Decoded {
		t.Error("stream should be Decoded=false after JPEG encoding")
	}
}

func TestOptimizeImagesPNGWithAlphaNotConverted(t *testing.T) {
	// PNG with SMask (alpha) — must NOT be converted to JPEG.
	pixels := make([]byte, 4*4*3)
	imgStream := &pdfStream{
		Dict: pdfDict{
			"/Type":             pdfName("/XObject"),
			"/Subtype":          pdfName("/Image"),
			"/Width":            4,
			"/Height":           4,
			"/BitsPerComponent": 8,
			"/ColorSpace":       pdfName("/DeviceRGB"),
			"/SMask":            pdfRef{Num: 10}, // has alpha
		},
		Data:    pixels,
		Decoded: true,
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

	smaskStream := &pdfStream{
		Dict:    pdfDict{"/Type": pdfName("/XObject"), "/Subtype": pdfName("/Image")},
		Data:    make([]byte, 16),
		Decoded: true,
	}
	smaskObj := &pdfObject{Num: 10, Value: smaskStream}

	doc := &Document{
		objects: map[int]*pdfObject{1: imgObj, 2: contentObj, 3: pageObj, 10: smaskObj},
		pages:   []*pdfObject{pageObj},
		nextID:  11,
	}

	count, err := doc.OptimizeImages(OptimizeImageOptions{ConvertPNGToJPEG: true})
	if err != nil {
		t.Fatalf("OptimizeImages: %v", err)
	}
	if count != 0 {
		t.Errorf("count = %d, want 0 (PNG with alpha should not be converted)", count)
	}

	// Filter should still be empty (PNG).
	filter := dictGetName(imgStream.Dict, "/Filter")
	if filter != "" {
		t.Errorf("filter = %s, want empty (PNG preserved)", filter)
	}
}

func TestOptimizeImagesSharedXObject(t *testing.T) {
	// One image XObject shared by two pages — should be optimized once.
	pixels := make([]byte, 10*10*3)
	for i := range pixels {
		pixels[i] = 200
	}
	imgStream := &pdfStream{
		Dict: pdfDict{
			"/Type":             pdfName("/XObject"),
			"/Subtype":          pdfName("/Image"),
			"/Width":            10,
			"/Height":           10,
			"/BitsPerComponent": 8,
			"/ColorSpace":       pdfName("/DeviceRGB"),
		},
		Data:    pixels,
		Decoded: true,
	}
	imgObj := &pdfObject{Num: 1, Value: imgStream}

	cs1 := &pdfStream{Dict: pdfDict{}, Data: []byte("q\n10 0 0 10 50 50 cm\n/Im0 Do\nQ\n"), Decoded: true}
	cs2 := &pdfStream{Dict: pdfDict{}, Data: []byte("q\n10 0 0 10 50 50 cm\n/Im0 Do\nQ\n"), Decoded: true}
	csObj1 := &pdfObject{Num: 2, Value: cs1}
	csObj2 := &pdfObject{Num: 4, Value: cs2}

	page1Dict := pdfDict{
		"/Type":     pdfName("/Page"),
		"/MediaBox": pdfArray{0.0, 0.0, 200.0, 300.0},
		"/Resources": pdfDict{"/XObject": pdfDict{"/Im0": pdfRef{Num: 1}}},
		"/Contents": pdfRef{Num: 2},
	}
	page1Obj := &pdfObject{Num: 3, Value: page1Dict}

	page2Dict := pdfDict{
		"/Type":     pdfName("/Page"),
		"/MediaBox": pdfArray{0.0, 0.0, 200.0, 300.0},
		"/Resources": pdfDict{"/XObject": pdfDict{"/Im0": pdfRef{Num: 1}}},
		"/Contents": pdfRef{Num: 4},
	}
	page2Obj := &pdfObject{Num: 5, Value: page2Dict}

	doc := &Document{
		objects: map[int]*pdfObject{
			1: imgObj,
			2: csObj1, 3: page1Obj,
			4: csObj2, 5: page2Obj,
		},
		pages:  []*pdfObject{page1Obj, page2Obj},
		nextID: 6,
	}

	count, err := doc.OptimizeImages(OptimizeImageOptions{ConvertPNGToJPEG: true})
	if err != nil {
		t.Fatalf("OptimizeImages: %v", err)
	}
	if count != 1 {
		t.Errorf("count = %d, want 1 (shared XObject optimized once)", count)
	}
}

func TestOptimizeImagesInvalidQuality(t *testing.T) {
	doc := createDocWithImage()
	_, err := doc.OptimizeImages(OptimizeImageOptions{JPEGQuality: 101})
	if err == nil {
		t.Fatal("expected error for JPEGQuality=101")
	}
	_, err = doc.OptimizeImages(OptimizeImageOptions{JPEGQuality: -1})
	if err == nil {
		t.Fatal("expected error for JPEGQuality=-1")
	}
}
