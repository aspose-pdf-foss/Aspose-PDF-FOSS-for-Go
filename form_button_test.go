// SPDX-License-Identifier: MIT

package asposepdf_test

import (
	"bytes"
	"testing"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

// TestButtonSetAppearance verifies a push button's rich appearance:
// /MK carries the rollover/down captions and icon position, three /AP
// states (N/R/D) are generated as Form XObjects, and the icon is embedded
// as an Image XObject.
func TestButtonSetAppearance(t *testing.T) {
	doc := pdf.NewDocument(400, 200)
	form := doc.Form()
	btn, err := form.AddPushButton(1, pdf.Rectangle{LLX: 120, LLY: 80, URX: 280, URY: 140}, "go", "Submit")
	if err != nil {
		t.Fatalf("AddPushButton: %v", err)
	}
	err = btn.SetAppearance(pdf.ButtonAppearance{
		Caption:      "Submit",
		RolloverText: "Click me!",
		DownText:     "Sending…",
		IconPath:     "testdata/aspose-logo.png",
		IconPosition: pdf.ButtonIconAboveCaption,
		TextColor:    &pdf.Color{R: 1, G: 1, B: 1, A: 1},
		FaceColor:    &pdf.Color{R: 0.15, G: 0.35, B: 0.75, A: 1},
		BorderColor:  &pdf.Color{R: 0.10, G: 0.22, B: 0.5, A: 1},
	})
	if err != nil {
		t.Fatalf("SetAppearance: %v", err)
	}

	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatalf("WriteTo: %v", err)
	}
	out := buf.Bytes()

	// /MK characteristics (uncompressed dict bytes).
	for _, want := range []string{"/RC", "/AC", "/TP"} {
		if !bytes.Contains(out, []byte(want)) {
			t.Errorf("output missing /MK entry %q", want)
		}
	}
	// Three /AP states are Form XObjects; the icon is an Image XObject.
	if n := bytes.Count(out, []byte("/Subtype /Form")); n < 3 {
		t.Errorf("expected >= 3 Form XObjects (N/R/D), got %d", n)
	}
	if !bytes.Contains(out, []byte("/Subtype /Image")) {
		t.Error("expected the icon embedded as an Image XObject")
	}

	// Reopen to confirm the styled document is structurally valid.
	if _, err := pdf.OpenStream(bytes.NewReader(out)); err != nil {
		t.Fatalf("OpenStream after SetAppearance: %v", err)
	}
}

// TestButtonSetAppearanceCaptionOnly works without an icon and falls back
// to the normal caption for rollover/down when those are unset.
func TestButtonSetAppearanceCaptionOnly(t *testing.T) {
	doc := pdf.NewDocument(400, 200)
	btn, err := doc.Form().AddPushButton(1, pdf.Rectangle{LLX: 120, LLY: 80, URX: 280, URY: 130}, "ok", "OK")
	if err != nil {
		t.Fatalf("AddPushButton: %v", err)
	}
	if err := btn.SetAppearance(pdf.ButtonAppearance{Caption: "OK"}); err != nil {
		t.Fatalf("SetAppearance: %v", err)
	}
	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatalf("WriteTo: %v", err)
	}
	if n := bytes.Count(buf.Bytes(), []byte("/Subtype /Form")); n < 3 {
		t.Errorf("expected >= 3 Form XObjects (N/R/D), got %d", n)
	}
}
