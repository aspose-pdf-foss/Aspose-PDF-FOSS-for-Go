// SPDX-License-Identifier: MIT

package ai

import (
	"context"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

// writePNG creates a small solid-color PNG and returns its path.
func writePNG(t *testing.T) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 24, 24))
	for y := 0; y < 24; y++ {
		for x := 0; x < 24; x++ {
			img.Set(x, y, color.RGBA{R: 200, G: 40, B: 40, A: 255})
		}
	}
	path := filepath.Join(t.TempDir(), "fig.png")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	return path
}

// taggedFigureDoc builds a tagged document with one figure that has no
// alternate text.
func taggedFigureDoc(t *testing.T, imgPath string) *pdf.Document {
	t.Helper()
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	tc := doc.TaggedContent()
	tc.SetTitle("Figure test")
	tc.SetLanguage("en")
	flow := doc.NewFlow(pdf.FlowOptions{Tagged: tc})
	flow.AddParagraph("Some introductory text before the figure.", pdf.TextStyle{Size: 12})
	flow.AddImage(imgPath, 120, 120) // no alt text
	if _, err := flow.Render(); err != nil {
		t.Fatal(err)
	}
	return doc
}

func TestImageDescriptionDescribe(t *testing.T) {
	client := &fakeClient{replies: []string{"A solid red square."}}
	cop := NewImageDescriptionCopilot(client, ImageDescriptionOptions{Language: "English", MaxWords: 20})

	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	desc, err := cop.Describe(context.Background(), img)
	if err != nil {
		t.Fatal(err)
	}
	if desc != "A solid red square." {
		t.Errorf("description = %q", desc)
	}
	req := client.requests[0]
	if len(req.Messages) != 2 || len(req.Messages[1].Images) != 1 || req.Messages[1].Images[0].MIME != "image/png" {
		t.Fatalf("request shape wrong: %+v", req.Messages)
	}
	sys := req.Messages[0].Text
	for _, want := range []string{"alternate text", "under 20 words", "in English"} {
		if !strings.Contains(sys, want) {
			t.Errorf("system prompt missing %q: %s", want, sys)
		}
	}
}

func TestFillAltTexts(t *testing.T) {
	doc := taggedFigureDoc(t, writePNG(t))

	// Before: one figure needs alt text, and the UA validator flags it.
	figs, err := doc.FiguresNeedingAltText()
	if err != nil {
		t.Fatal(err)
	}
	if len(figs) != 1 {
		t.Fatalf("figures needing alt = %d; want 1", len(figs))
	}
	if _, ok := figs[0].Image(); !ok {
		t.Fatal("figure image was not resolved via its MCID")
	}
	if !hasUARule(doc.ValidatePDFUA().Issues, "UA_FIGURE_NO_ALT") {
		t.Fatal("expected UA_FIGURE_NO_ALT before filling")
	}

	client := &fakeClient{replies: []string{"A dark red square."}}
	cop := NewImageDescriptionCopilot(client, ImageDescriptionOptions{})
	n, err := cop.FillAltTexts(context.Background(), doc)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("filled = %d; want 1", n)
	}
	// The image was actually sent to the model (JPEG or PNG bytes).
	if len(client.requests) != 1 || len(client.requests[0].Messages[1].Images) != 1 {
		t.Fatalf("image not sent: %+v", client.requests)
	}

	// After: no figure needs alt, and the UA figure rule is satisfied.
	figs, _ = doc.FiguresNeedingAltText()
	if len(figs) != 0 {
		t.Errorf("figures still needing alt = %d; want 0", len(figs))
	}
	if hasUARule(doc.ValidatePDFUA().Issues, "UA_FIGURE_NO_ALT") {
		t.Error("UA_FIGURE_NO_ALT still present after filling")
	}

	// The alt text survives a Save + Open round-trip.
	var sb strings.Builder
	if _, err := doc.WriteTo(&sb); err != nil {
		t.Fatal(err)
	}
	reopened, err := pdf.OpenStream(strings.NewReader(sb.String()))
	if err != nil {
		t.Fatal(err)
	}
	figs, _ = reopened.FiguresNeedingAltText()
	if len(figs) != 0 {
		t.Errorf("after round-trip, figures needing alt = %d; want 0", len(figs))
	}
}

func TestFillAltTextsUntagged(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	figs, err := doc.FiguresNeedingAltText()
	if err != nil {
		t.Fatal(err)
	}
	if figs != nil {
		t.Errorf("untagged document reported %d figures", len(figs))
	}
	n, err := NewImageDescriptionCopilot(&fakeClient{}, ImageDescriptionOptions{}).FillAltTexts(context.Background(), doc)
	if err != nil || n != 0 {
		t.Errorf("FillAltTexts on untagged doc: n=%d err=%v", n, err)
	}
}

func hasUARule(issues []pdf.PDFUAIssue, rule string) bool {
	for _, i := range issues {
		if i.Rule == rule {
			return true
		}
	}
	return false
}
