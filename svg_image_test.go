// SPDX-License-Identifier: MIT

package asposepdf

import (
	"encoding/base64"
	"testing"
)

func TestDecodeSVGDataURI_PNG(t *testing.T) {
	raw := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A} // PNG magic
	uri := "data:image/png;base64," + base64.StdEncoding.EncodeToString(raw)
	data, format, ok := decodeSVGDataURI(uri)
	if !ok {
		t.Fatal("decode failed")
	}
	if format != ImageFormatPNG {
		t.Errorf("format = %v", format)
	}
	if len(data) != len(raw) {
		t.Errorf("len(data) = %d, want %d", len(data), len(raw))
	}
}

func TestDecodeSVGDataURI_JPEG(t *testing.T) {
	raw := []byte{0xFF, 0xD8, 0xFF, 0xE0} // JPEG magic
	for _, mime := range []string{"image/jpeg", "image/jpg"} {
		uri := "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(raw)
		_, format, ok := decodeSVGDataURI(uri)
		if !ok || format != ImageFormatJPEG {
			t.Errorf("%s: format = %v ok=%v", mime, format, ok)
		}
	}
}

func TestDecodeSVGDataURI_NotData(t *testing.T) {
	_, _, ok := decodeSVGDataURI("https://example.com/foo.png")
	if ok {
		t.Error("expected failure for non-data URI")
	}
}

func TestDecodeSVGDataURI_MalformedBase64(t *testing.T) {
	_, _, ok := decodeSVGDataURI("data:image/png;base64,!@#$%")
	if ok {
		t.Error("expected failure for malformed base64")
	}
}

func TestDecodeSVGDataURI_UnsupportedMIME(t *testing.T) {
	_, _, ok := decodeSVGDataURI("data:image/gif;base64,AAAA")
	if ok {
		t.Error("expected failure for unsupported MIME")
	}
}

func TestDecodeSVGDataURI_MissingBase64Marker(t *testing.T) {
	_, _, ok := decodeSVGDataURI("data:image/png,rawdata")
	if ok {
		t.Error("expected failure for URI without base64 marker (raw URL-encoded data not supported)")
	}
}
