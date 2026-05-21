// SPDX-License-Identifier: MIT

package asposepdf

import (
	"testing"
)

func TestExtractXObjectImageJPEGPassthrough(t *testing.T) {
	// Minimal synthetic JPEG data (just the SOI and EOI markers).
	jpegData := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x02, 0xFF, 0xD9}

	imgStream := &pdfStream{
		Dict: pdfDict{
			"/Subtype":          pdfName("/Image"),
			"/Width":            100,
			"/Height":           80,
			"/BitsPerComponent":  8,
			"/ColorSpace":       pdfName("/DeviceRGB"),
			"/Filter":           pdfName("/DCTDecode"),
		},
		Data:    jpegData,
		Decoded: false,
	}

	objects := map[int]*pdfObject{
		1: {Value: imgStream},
	}
	resources := pdfDict{
		"/XObject": pdfDict{
			"/Im0": pdfRef{Num: 1},
		},
	}

	ctm := identityMatrix()
	ctm[4] = 72
	ctm[5] = 500
	ctm[0] = 200
	ctm[3] = 160

	info, ok := xobjectImageInfo(objects, resources, "/Im0", ctm)
	if !ok {
		t.Fatal("xobjectImageInfo returned false for JPEG image")
	}
	img, err := info.Extract()
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}
	if img.Format != ImageFormatJPEG {
		t.Errorf("format = %d, want ImageFormatJPEG", img.Format)
	}
	if img.Width != 100 || img.Height != 80 {
		t.Errorf("dimensions = %dx%d, want 100x80", img.Width, img.Height)
	}
	if img.BPC != 8 {
		t.Errorf("BPC = %d, want 8", img.BPC)
	}
	if img.ColorSpace != ColorSpaceDeviceRGB {
		t.Errorf("colorSpace = %d, want DeviceRGB", img.ColorSpace)
	}
	if len(img.Data) != len(jpegData) {
		t.Errorf("data len = %d, want %d", len(img.Data), len(jpegData))
	}
	if img.X != 72 || img.Y != 500 {
		t.Errorf("position = (%g, %g), want (72, 500)", img.X, img.Y)
	}
	if img.PageWidth != 200 || img.PageHeight != 160 {
		t.Errorf("page size = (%g, %g), want (200, 160)", img.PageWidth, img.PageHeight)
	}
}

func TestExtractXObjectImageSkipsNonImage(t *testing.T) {
	formStream := &pdfStream{
		Dict: pdfDict{
			"/Subtype": pdfName("/Form"),
			"/BBox":    pdfArray{0, 0, 100, 100},
		},
		Data: []byte{},
	}

	objects := map[int]*pdfObject{
		1: {Value: formStream},
	}
	resources := pdfDict{
		"/XObject": pdfDict{
			"/Fm0": pdfRef{Num: 1},
		},
	}

	_, ok := xobjectImageInfo(objects, resources, "/Fm0", identityMatrix())
	if ok {
		t.Error("expected false for Form XObject, got true")
	}
}

func TestExtractXObjectImagePNGFlateDecode(t *testing.T) {
	// FlateDecode image with pre-decoded RGB pixels should produce a PNG.
	pixels := make([]byte, 10*10*3) // 10x10 RGB
	imgStream := &pdfStream{
		Dict: pdfDict{
			"/Subtype":         pdfName("/Image"),
			"/Width":           10,
			"/Height":          10,
			"/BitsPerComponent": 8,
			"/ColorSpace":      pdfName("/DeviceRGB"),
			"/Filter":          pdfName("/FlateDecode"),
		},
		Data:    pixels,
		Decoded: true,
	}

	objects := map[int]*pdfObject{
		1: {Value: imgStream},
	}
	resources := pdfDict{
		"/XObject": pdfDict{
			"/Im0": pdfRef{Num: 1},
		},
	}

	info, ok := xobjectImageInfo(objects, resources, "/Im0", identityMatrix())
	if !ok {
		t.Fatal("expected true for FlateDecode image, got false")
	}
	img, err := info.Extract()
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}
	if img.Format != ImageFormatPNG {
		t.Errorf("format = %d, want ImageFormatPNG", img.Format)
	}
	if img.Width != 10 || img.Height != 10 {
		t.Errorf("dimensions = %dx%d, want 10x10", img.Width, img.Height)
	}
	if len(img.Data) == 0 {
		t.Error("expected non-empty PNG data")
	}
}

func TestResolveColorSpaceVariants(t *testing.T) {
	objects := map[int]*pdfObject{}

	tests := []struct {
		name string
		dict pdfDict
		want ImageColorSpace
	}{
		{"no key", pdfDict{}, ColorSpaceDeviceRGB},
		{"DeviceRGB", pdfDict{"/ColorSpace": pdfName("/DeviceRGB")}, ColorSpaceDeviceRGB},
		{"DeviceGray", pdfDict{"/ColorSpace": pdfName("/DeviceGray")}, ColorSpaceDeviceGray},
		{"DeviceCMYK", pdfDict{"/ColorSpace": pdfName("/DeviceCMYK")}, ColorSpaceDeviceCMYK},
		{"ICCBased array", pdfDict{"/ColorSpace": pdfArray{pdfName("/ICCBased"), pdfRef{Num: 99}}}, ColorSpaceICCBased},
		{"Indexed array", pdfDict{"/ColorSpace": pdfArray{pdfName("/Indexed"), pdfName("/DeviceRGB"), 255, "palette"}}, ColorSpaceIndexed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveColorSpace(objects, tt.dict)
			if got != tt.want {
				t.Errorf("resolveColorSpace = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestImageInfoMetadata(t *testing.T) {
	jpegData := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x02, 0xFF, 0xD9}

	imgStream := &pdfStream{
		Dict: pdfDict{
			"/Subtype":         pdfName("/Image"),
			"/Width":           100,
			"/Height":          80,
			"/BitsPerComponent": 8,
			"/ColorSpace":      pdfName("/DeviceRGB"),
			"/Filter":          pdfName("/DCTDecode"),
		},
		Data:    jpegData,
		Decoded: false,
	}

	objects := map[int]*pdfObject{
		1: {Value: imgStream},
	}
	resources := pdfDict{
		"/XObject": pdfDict{
			"/Im0": pdfRef{Num: 1},
		},
	}

	// Build a content stream: q 200 0 0 160 72 500 cm /Im0 Do Q
	ops := []contentOp{
		{Operator: "q"},
		{Operator: "cm", Operands: []pdfValue{200.0, 0.0, 0.0, 160.0, 72.0, 500.0}},
		{Operator: "Do", Operands: []pdfValue{pdfName("/Im0")}},
		{Operator: "Q"},
	}

	infos := collectImageInfos(objects, ops, resources)
	if len(infos) != 1 {
		t.Fatalf("got %d infos, want 1", len(infos))
	}
	info := infos[0]
	if info.Width != 100 || info.Height != 80 {
		t.Errorf("dimensions = %dx%d, want 100x80", info.Width, info.Height)
	}
	if info.BPC != 8 {
		t.Errorf("BPC = %d, want 8", info.BPC)
	}
	if info.ColorSpace != ColorSpaceDeviceRGB {
		t.Errorf("colorSpace = %d, want DeviceRGB", info.ColorSpace)
	}
	if info.Format != ImageFormatJPEG {
		t.Errorf("format = %d, want ImageFormatJPEG", info.Format)
	}
	if info.Name != "/Im0" {
		t.Errorf("name = %q, want /Im0", info.Name)
	}
	if info.X != 72 || info.Y != 500 {
		t.Errorf("position = (%g, %g), want (72, 500)", info.X, info.Y)
	}
	if info.PageWidth != 200 || info.PageHeight != 160 {
		t.Errorf("page size = (%g, %g), want (200, 160)", info.PageWidth, info.PageHeight)
	}
	if info.Inline {
		t.Error("expected Inline=false")
	}
}

func TestImageInfoFlateDecode(t *testing.T) {
	pixels := make([]byte, 10*10*3)
	imgStream := &pdfStream{
		Dict: pdfDict{
			"/Subtype":         pdfName("/Image"),
			"/Width":           10,
			"/Height":          10,
			"/BitsPerComponent": 8,
			"/ColorSpace":      pdfName("/DeviceRGB"),
			"/Filter":          pdfName("/FlateDecode"),
		},
		Data:    pixels,
		Decoded: true,
	}

	objects := map[int]*pdfObject{
		1: {Value: imgStream},
	}
	resources := pdfDict{
		"/XObject": pdfDict{
			"/Im1": pdfRef{Num: 1},
		},
	}

	ops := []contentOp{
		{Operator: "Do", Operands: []pdfValue{pdfName("/Im1")}},
	}

	infos := collectImageInfos(objects, ops, resources)
	if len(infos) != 1 {
		t.Fatalf("got %d infos, want 1", len(infos))
	}
	if infos[0].Format != ImageFormatPNG {
		t.Errorf("format = %d, want ImageFormatPNG", infos[0].Format)
	}
	if infos[0].Name != "/Im1" {
		t.Errorf("name = %q, want /Im1", infos[0].Name)
	}
}

func TestImageInfoExtract(t *testing.T) {
	jpegData := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x02, 0xFF, 0xD9}

	imgStream := &pdfStream{
		Dict: pdfDict{
			"/Subtype":          pdfName("/Image"),
			"/Width":            100,
			"/Height":           80,
			"/BitsPerComponent": 8,
			"/ColorSpace":       pdfName("/DeviceRGB"),
			"/Filter":           pdfName("/DCTDecode"),
		},
		Data:    jpegData,
		Decoded: false,
	}

	objects := map[int]*pdfObject{
		1: {Value: imgStream},
	}
	resources := pdfDict{
		"/XObject": pdfDict{
			"/Im0": pdfRef{Num: 1},
		},
	}

	ops := []contentOp{
		{Operator: "q"},
		{Operator: "cm", Operands: []pdfValue{200.0, 0.0, 0.0, 160.0, 72.0, 500.0}},
		{Operator: "Do", Operands: []pdfValue{pdfName("/Im0")}},
		{Operator: "Q"},
	}

	infos := collectImageInfos(objects, ops, resources)
	if len(infos) != 1 {
		t.Fatalf("got %d infos, want 1", len(infos))
	}

	img, err := infos[0].Extract()
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}
	if img.Format != ImageFormatJPEG {
		t.Errorf("format = %d, want ImageFormatJPEG", img.Format)
	}
	if len(img.Data) != len(jpegData) {
		t.Errorf("data len = %d, want %d", len(img.Data), len(jpegData))
	}
	if img.Width != 100 || img.Height != 80 {
		t.Errorf("dimensions = %dx%d, want 100x80", img.Width, img.Height)
	}
	if img.X != 72 || img.Y != 500 {
		t.Errorf("position = (%g, %g), want (72, 500)", img.X, img.Y)
	}
}

func TestImageInfoExtractPNG(t *testing.T) {
	pixels := make([]byte, 10*10*3) // 10x10 RGB
	imgStream := &pdfStream{
		Dict: pdfDict{
			"/Subtype":          pdfName("/Image"),
			"/Width":            10,
			"/Height":           10,
			"/BitsPerComponent": 8,
			"/ColorSpace":       pdfName("/DeviceRGB"),
			"/Filter":           pdfName("/FlateDecode"),
		},
		Data:    pixels,
		Decoded: true,
	}

	objects := map[int]*pdfObject{
		1: {Value: imgStream},
	}
	resources := pdfDict{
		"/XObject": pdfDict{
			"/Im0": pdfRef{Num: 1},
		},
	}

	ops := []contentOp{
		{Operator: "Do", Operands: []pdfValue{pdfName("/Im0")}},
	}

	infos := collectImageInfos(objects, ops, resources)
	if len(infos) != 1 {
		t.Fatalf("got %d infos, want 1", len(infos))
	}

	img, err := infos[0].Extract()
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}
	if img.Format != ImageFormatPNG {
		t.Errorf("format = %d, want ImageFormatPNG", img.Format)
	}
	if len(img.Data) == 0 {
		t.Error("expected non-empty PNG data")
	}
	if img.Width != 10 || img.Height != 10 {
		t.Errorf("dimensions = %dx%d, want 10x10", img.Width, img.Height)
	}
}

func TestPrimaryFilter(t *testing.T) {
	tests := []struct {
		name string
		dict pdfDict
		want string
	}{
		{"no filter", pdfDict{}, ""},
		{"single name", pdfDict{"/Filter": pdfName("/DCTDecode")}, "/DCTDecode"},
		{"array", pdfDict{"/Filter": pdfArray{pdfName("/FlateDecode"), pdfName("/ASCII85Decode")}}, "/FlateDecode"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := primaryFilter(tt.dict)
			if got != tt.want {
				t.Errorf("primaryFilter = %q, want %q", got, tt.want)
			}
		})
	}
}
