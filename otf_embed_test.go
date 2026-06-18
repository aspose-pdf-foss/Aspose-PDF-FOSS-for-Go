// SPDX-License-Identifier: MIT

package asposepdf_test

import (
	"bytes"
	"strings"
	"testing"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

// raw returns the serialized PDF bytes as a string, for black-box structural
// assertions.
func raw(t *testing.T, doc *pdf.Document) string {
	t.Helper()
	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatalf("WriteTo: %v", err)
	}
	return buf.String()
}

// TestLoadFontOTFEmbeds checks that an OpenType-CFF (.otf) font is accepted by
// LoadFont and embedded as a CIDFontType0 descendant with an OpenType
// /FontFile3 — and that its text renders and round-trips through extraction.
func TestLoadFontOTFEmbeds(t *testing.T) {
	doc := pdf.NewDocument(420, 160)
	f, err := doc.LoadFont("testdata/SourceSerif4-Regular.otf")
	if err != nil {
		t.Fatalf("LoadFont(.otf): %v", err)
	}
	if !f.IsEmbedded() {
		t.Error("OTF font reports IsEmbedded() = false")
	}
	page, _ := doc.Page(1)
	if err := page.AddText("Source Serif OTF 12345",
		pdf.TextStyle{Font: f, Size: 24, Color: &pdf.Color{A: 1}},
		pdf.Rectangle{LLX: 20, LLY: 70, URX: 400, URY: 120}); err != nil {
		t.Fatalf("AddText: %v", err)
	}

	// Structural: CFF embedding shape, not the TrueType one.
	s := raw(t, doc)
	for _, want := range []string{"/FontFile3", "/CIDFontType0", "/OpenType"} {
		if !strings.Contains(s, want) {
			t.Errorf("serialized PDF missing %s (OTF embed structure)", want)
		}
	}
	if strings.Contains(s, "/FontFile2") || strings.Contains(s, "/CIDFontType2") {
		t.Error("OTF embed unexpectedly produced TrueType (/FontFile2 / CIDFontType2) objects")
	}

	// Round-trip: the text extracts back (validates /ToUnicode). Spaces may
	// come back as U+00A0 depending on the font's space glyph; normalize.
	out, err := pdf.OpenStream(bytes.NewReader([]byte(s)))
	if err != nil {
		t.Fatalf("OpenStream: %v", err)
	}
	txt, err := out.ExtractText()
	if err != nil {
		t.Fatalf("ExtractText: %v", err)
	}
	got := strings.ReplaceAll(txt[0], " ", " ")
	if !strings.Contains(got, "Source Serif OTF 12345") {
		t.Errorf("extracted %q, want it to contain the drawn text", got)
	}

	// The embedded OTF actually renders glyphs (not blank).
	p1, _ := out.Page(1)
	if !hasNonWhitePixel(t, p1) {
		t.Error("embedded OTF rendered blank")
	}
}

// TestLoadFontTTFStillFontFile2 guards that the TrueType embed path is
// unchanged by the OTF addition.
func TestLoadFontTTFStillFontFile2(t *testing.T) {
	doc := pdf.NewDocument(420, 160)
	f, err := doc.LoadFont("testdata/DejaVuSans.ttf")
	if err != nil {
		t.Fatalf("LoadFont(.ttf): %v", err)
	}
	page, _ := doc.Page(1)
	if err := page.AddText("DejaVu TTF",
		pdf.TextStyle{Font: f, Size: 24, Color: &pdf.Color{A: 1}},
		pdf.Rectangle{LLX: 20, LLY: 70, URX: 400, URY: 120}); err != nil {
		t.Fatalf("AddText: %v", err)
	}
	s := raw(t, doc)
	for _, want := range []string{"/FontFile2", "/CIDFontType2"} {
		if !strings.Contains(s, want) {
			t.Errorf("serialized TTF PDF missing %s", want)
		}
	}
	if strings.Contains(s, "/FontFile3") {
		t.Error("TTF embed unexpectedly produced a /FontFile3")
	}
}
