// SPDX-License-Identifier: MIT

package ai

import (
	"bytes"
	"context"
	"image"
	"math"
	"strings"
	"testing"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

// scannedDoc builds a scanned-style PDF: a page whose only content is a
// full-page raster of rendered text (no extractable text objects).
func scannedDoc(t *testing.T) *pdf.Document {
	t.Helper()
	src := textDoc(t, "Invoice 12345 issued to ACME Corp for three hundred dollars.")
	srcPage, err := src.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := srcPage.RenderPNG(&buf, pdf.RenderOptions{DPI: 96}); err != nil {
		t.Fatal(err)
	}

	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	page, err := doc.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	size, err := page.Size()
	if err != nil {
		t.Fatal(err)
	}
	full := pdf.Rectangle{LLX: 0, LLY: 0, URX: size.Width, URY: size.Height}
	if err := page.AddImageFromStream(&buf, full); err != nil {
		t.Fatal(err)
	}
	return doc
}

// fakeEngine returns a scripted OCRResult and records the images it saw.
type fakeEngine struct {
	result *OCRResult
	images []image.Image
}

func (f *fakeEngine) Recognize(_ context.Context, img image.Image) (*OCRResult, error) {
	f.images = append(f.images, img)
	return f.result, nil
}

func TestPageNeedsOCRDetection(t *testing.T) {
	scanned := scannedDoc(t)
	p, err := scanned.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	need, rect, err := pageNeedsOCR(p)
	if err != nil {
		t.Fatal(err)
	}
	if !need || rect == nil {
		t.Errorf("scanned page not detected: need=%v rect=%v", need, rect)
	}

	textual := textDoc(t, "Plain digital text page.")
	tp, err := textual.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	need, _, err = pageNeedsOCR(tp)
	if err != nil {
		t.Fatal(err)
	}
	if need {
		t.Error("text page misdetected as scanned")
	}
}

func TestLLMOCREngine(t *testing.T) {
	client := &fakeClient{replies: []string{"Line one\nLine two\n\n"}}
	engine := NewLLMOCREngine(client, LLMOCROptions{Language: "English"})
	img := image.NewRGBA(image.Rect(0, 0, 10, 10))

	res, err := engine.Recognize(context.Background(), img)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Lines) != 2 || res.Lines[0].Text != "Line one" || res.Lines[1].Text != "Line two" {
		t.Errorf("lines = %+v", res.Lines)
	}
	if res.Lines[0].Box != nil {
		t.Error("LLM engine must not report boxes")
	}
	if res.Text() != "Line one\nLine two" {
		t.Errorf("Text() = %q", res.Text())
	}
	req := client.requests[0]
	if len(req.Messages) != 2 || len(req.Messages[1].Images) != 1 || req.Messages[1].Images[0].MIME != "image/png" {
		t.Fatalf("request shape wrong: %+v", req.Messages)
	}
	if !strings.Contains(req.Messages[0].Text, "The text is in English.") {
		t.Errorf("language hint missing: %s", req.Messages[0].Text)
	}
}

func TestOcrCopilotGetTextRecognition(t *testing.T) {
	doc := scannedDoc(t)
	engine := &fakeEngine{result: &OCRResult{Lines: []OCRLine{{Text: "Invoice 12345"}}}}
	cop := NewOcrCopilot(engine, OcrOptions{Document: doc, DPI: 96})

	results, err := cop.GetTextRecognition(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].PageNumber != 1 || results[0].Text != "Invoice 12345" {
		t.Fatalf("results = %+v", results)
	}
	// The page was rendered at the configured DPI: A4 width 595pt → ~794px.
	if len(engine.images) != 1 {
		t.Fatalf("engine saw %d images", len(engine.images))
	}
	if w := engine.images[0].Bounds().Dx(); w < 780 || w > 810 {
		t.Errorf("rendered width = %dpx; want ≈794 at 96 DPI", w)
	}
	// The document must be untouched.
	p, _ := doc.Page(1)
	if text, _ := p.ExtractText(); strings.TrimSpace(text) != "" {
		t.Errorf("GetTextRecognition modified the document: %q", text)
	}
}

func TestOcrCopilotSkipsTextPages(t *testing.T) {
	doc := textDoc(t, "Already digital.")
	engine := &fakeEngine{result: &OCRResult{Lines: []OCRLine{{Text: "should not be called"}}}}
	cop := NewOcrCopilot(engine, OcrOptions{Document: doc})

	n, err := cop.MakeSearchable(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 || len(engine.images) != 0 {
		t.Errorf("text page was OCRed: n=%d calls=%d", n, len(engine.images))
	}
}

// TestMakeSearchableBoxed: engine with line boxes → the hidden text lands at
// the mapped position, extraction and search work, pixels stay identical.
func TestMakeSearchableBoxed(t *testing.T) {
	doc := scannedDoc(t)
	before, err := doc.RenderImage(1, pdf.RenderOptions{DPI: 96})
	if err != nil {
		t.Fatal(err)
	}

	// 150 DPI → scale 0.48 pt/px. Box (100,200)-(1100,260) px →
	// user-space rect (48, 717.1)-(528, 745.9) on an 841.89pt-tall page.
	engine := &fakeEngine{result: &OCRResult{Lines: []OCRLine{
		{Text: "Invoice 12345", Box: &OCRBox{Left: 100, Top: 200, Right: 1100, Bottom: 260}},
		{Text: "ACME Corp", Box: &OCRBox{Left: 100, Top: 300, Right: 700, Bottom: 350}},
	}}}
	cop := NewOcrCopilot(engine, OcrOptions{Document: doc, DPI: 150})

	n, err := cop.MakeSearchable(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("processed %d pages; want 1", n)
	}

	p, _ := doc.Page(1)
	text, err := p.ExtractText()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "Invoice 12345") || !strings.Contains(text, "ACME Corp") {
		t.Errorf("hidden layer not extractable: %q", text)
	}

	matches, err := p.SearchText("Invoice 12345")
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 {
		t.Fatalf("SearchText matches = %d; want 1", len(matches))
	}
	m := matches[0].Rect
	if math.Abs(m.LLX-48) > 5 {
		t.Errorf("match LLX = %g; want ≈48", m.LLX)
	}
	if m.LLY < 710 || m.URY > 755 {
		t.Errorf("match rect %+v; want inside the mapped line band (≈717–746)", m)
	}

	// Invisible layer: the raster must be pixel-identical.
	after, err := doc.RenderImage(1, pdf.RenderOptions{DPI: 96})
	if err != nil {
		t.Fatal(err)
	}
	if !imagesEqual(before, after) {
		t.Error("MakeSearchable changed the page's rendered pixels")
	}

	// Round-trip through Save+Open.
	var sb strings.Builder
	if _, err := doc.WriteTo(&sb); err != nil {
		t.Fatal(err)
	}
	reopened, err := pdf.OpenStream(strings.NewReader(sb.String()))
	if err != nil {
		t.Fatal(err)
	}
	rp, err := reopened.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	rtText, err := rp.ExtractText()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rtText, "Invoice 12345") {
		t.Errorf("hidden layer lost after round-trip: %q", rtText)
	}
}

// TestMakeSearchableGrid: the coordinate-less engine falls back to the even
// grid — text extractable in order, pixels identical.
func TestMakeSearchableGrid(t *testing.T) {
	doc := scannedDoc(t)
	before, err := doc.RenderImage(1, pdf.RenderOptions{DPI: 96})
	if err != nil {
		t.Fatal(err)
	}

	engine := &fakeEngine{result: &OCRResult{Lines: []OCRLine{
		{Text: "First recognized line"},
		{Text: "Second recognized line"},
	}}}
	cop := NewOcrCopilot(engine, OcrOptions{Document: doc})

	n, err := cop.MakeSearchable(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("processed %d pages; want 1", n)
	}
	p, _ := doc.Page(1)
	text, err := p.ExtractText()
	if err != nil {
		t.Fatal(err)
	}
	first := strings.Index(text, "First recognized line")
	second := strings.Index(text, "Second recognized line")
	if first < 0 || second < 0 || second < first {
		t.Errorf("grid layer wrong: %q", text)
	}
	after, err := doc.RenderImage(1, pdf.RenderOptions{DPI: 96})
	if err != nil {
		t.Fatal(err)
	}
	if !imagesEqual(before, after) {
		t.Error("grid MakeSearchable changed the page's rendered pixels")
	}
}

func imagesEqual(a, b image.Image) bool {
	if a.Bounds() != b.Bounds() {
		return false
	}
	bo := a.Bounds()
	for y := bo.Min.Y; y < bo.Max.Y; y++ {
		for x := bo.Min.X; x < bo.Max.X; x++ {
			ar, ag, ab, aa := a.At(x, y).RGBA()
			br, bg, bb, ba := b.At(x, y).RGBA()
			if ar != br || ag != bg || ab != bb || aa != ba {
				return false
			}
		}
	}
	return true
}
