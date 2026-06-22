// SPDX-License-Identifier: MIT

package asposepdf

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestValidateImageColorSpace checks the §8.9.5 diagnostic: an image XObject
// missing /ColorSpace or /BitsPerComponent is reported, while a well-formed
// image is not.
func TestValidateImageColorSpace(t *testing.T) {
	dir := t.TempDir()

	build := func() *Document {
		doc := NewDocument(200, 200)
		p, _ := doc.Page(1)
		if err := p.AddImage("testdata/aspose-logo.png", Rectangle{LLX: 20, LLY: 20, URX: 180, URY: 180}); err != nil {
			t.Skipf("test image unavailable: %v", err)
		}
		return doc
	}

	countCSIssues := func(rep *ValidationReport) (cs, bpc int) {
		for _, is := range rep.Issues {
			if is.Code != "OBJECT_ERROR" {
				continue
			}
			if strings.Contains(is.Message, "/ColorSpace") {
				cs++
			}
			if strings.Contains(is.Message, "/BitsPerComponent") {
				bpc++
			}
		}
		return
	}

	// Well-formed image: no missing-colour-space diagnostics.
	ok := build()
	okPath := filepath.Join(dir, "ok.pdf")
	if err := ok.Save(okPath); err != nil {
		t.Fatalf("save ok: %v", err)
	}
	repOK, err := Validate(okPath)
	if err != nil {
		t.Fatalf("Validate ok: %v", err)
	}
	if cs, bpc := countCSIssues(repOK); cs != 0 || bpc != 0 {
		t.Errorf("well-formed image flagged (cs=%d, bpc=%d)", cs, bpc)
	}

	// Strip the required entries from every image XObject — they must be flagged.
	bad := build()
	stripped := 0
	for _, o := range bad.objects {
		if s, ok := o.Value.(*pdfStream); ok && dictGetName(s.Dict, "/Subtype") == "/Image" {
			delete(s.Dict, "/ColorSpace")
			delete(s.Dict, "/BitsPerComponent")
			stripped++
		}
	}
	if stripped == 0 {
		t.Skip("no image XObject to strip")
	}
	badPath := filepath.Join(dir, "bad.pdf")
	if err := bad.Save(badPath); err != nil {
		t.Fatalf("save bad: %v", err)
	}
	repBad, err := Validate(badPath)
	if err != nil {
		t.Fatalf("Validate bad: %v", err)
	}
	if cs, bpc := countCSIssues(repBad); cs == 0 || bpc == 0 {
		t.Errorf("stripped image not flagged (cs=%d, bpc=%d)", cs, bpc)
	}
	if repBad.Valid {
		t.Error("report.Valid = true despite missing /ColorSpace")
	}
	_ = os.Remove(badPath)
}

// TestStreamHasFilter covers the filter-name lookup helper (name and array).
func TestStreamHasFilter(t *testing.T) {
	if !streamHasFilter(pdfDict{"/Filter": pdfName("/JPXDecode")}, "/JPXDecode") {
		t.Error("single-name /Filter not matched")
	}
	if !streamHasFilter(pdfDict{"/Filter": pdfArray{pdfName("/FlateDecode"), pdfName("/DCTDecode")}}, "/DCTDecode") {
		t.Error("array /Filter not matched")
	}
	if streamHasFilter(pdfDict{"/Filter": pdfName("/FlateDecode")}, "/JPXDecode") {
		t.Error("false positive on a non-matching filter")
	}
	if streamHasFilter(pdfDict{}, "/JPXDecode") {
		t.Error("false positive on a missing /Filter")
	}
}
