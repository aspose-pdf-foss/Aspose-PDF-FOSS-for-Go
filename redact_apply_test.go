package asposepdf_test

import (
	"bytes"
	"strings"
	"testing"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

func TestValidateRedactionsEmpty(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	if err := doc.ValidateRedactions(); err != nil {
		t.Errorf("ValidateRedactions on empty doc: %v", err)
	}
}

func TestValidateRedactionsWithRedact(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	page.AddText("Confidential", pdf.TextStyle{Font: pdf.FontHelvetica, Size: 12},
		pdf.Rectangle{LLX: 50, LLY: 700, URX: 300, URY: 720})
	ra := pdf.NewRedactAnnotation(page, pdf.Rectangle{LLX: 100, LLY: 700, URX: 250, URY: 720})
	if err := page.Annotations().Add(ra); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := doc.ValidateRedactions(); err != nil {
		t.Errorf("ValidateRedactions: %v", err)
	}
}

func TestValidateRedactionsNoPagesWithRedact(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	page.AddText("Hello", pdf.TextStyle{Font: pdf.FontHelvetica, Size: 12},
		pdf.Rectangle{LLX: 50, LLY: 700, URX: 200, URY: 720})
	// No redact annotations — Validate should still succeed (no-op).
	if err := doc.ValidateRedactions(); err != nil {
		t.Errorf("ValidateRedactions: %v", err)
	}
}

func TestApplyRedactionsStubReturnsNil(t *testing.T) {
	// Stub for Task 8 — full implementation in Task 12.
	doc := pdf.NewDocument(595, 842)
	if err := doc.ApplyRedactions(); err != nil {
		t.Errorf("ApplyRedactions stub: %v", err)
	}
}

func TestValidateRedactionsRoundTripParseability(t *testing.T) {
	// After writing + reading, Validate should still pass on the parsed doc.
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	page.AddText("Test", pdf.TextStyle{Font: pdf.FontHelvetica, Size: 12},
		pdf.Rectangle{LLX: 50, LLY: 700, URX: 200, URY: 720})
	ra := pdf.NewRedactAnnotation(page, pdf.Rectangle{LLX: 50, LLY: 700, URX: 200, URY: 720})
	page.Annotations().Add(ra)
	var buf bytes.Buffer
	doc.WriteTo(&buf)
	doc2, _ := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	if err := doc2.ValidateRedactions(); err != nil {
		t.Errorf("ValidateRedactions after roundtrip: %v", err)
	}
}

func TestApplyRedactionsRemovesText(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	page.AddText("Hello world",
		pdf.TextStyle{Font: pdf.FontHelvetica, Size: 12},
		pdf.Rectangle{LLX: 50, LLY: 700, URX: 300, URY: 720})
	// Redact the "world" portion. "Hello " in Helvetica 12pt advances ~30pt;
	// LLX=80 is chosen conservatively to cover "world" while leaving "Hello".
	ra := pdf.NewRedactAnnotation(page, pdf.Rectangle{LLX: 80, LLY: 698, URX: 300, URY: 722})
	page.Annotations().Add(ra)

	if err := doc.ApplyRedactions(); err != nil {
		t.Fatalf("ApplyRedactions: %v", err)
	}

	var buf bytes.Buffer
	doc.WriteTo(&buf)
	doc2, _ := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	text, err := doc2.ExtractText()
	if err != nil {
		t.Fatal(err)
	}
	pageText := strings.Join(text, "\n")
	if strings.Contains(pageText, "world") {
		t.Errorf("expected 'world' redacted, got %q", pageText)
	}
}

func TestApplyRedactionsPreservesNonRedactedText(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	page.AddText("Public Hidden",
		pdf.TextStyle{Font: pdf.FontHelvetica, Size: 12},
		pdf.Rectangle{LLX: 50, LLY: 700, URX: 300, URY: 720})
	// Redact only the right half — "Hidden" portion.
	// "Public " in Helvetica 12pt advances ~42pt from x=50, so LLX=100 covers "Hidden".
	ra := pdf.NewRedactAnnotation(page, pdf.Rectangle{LLX: 100, LLY: 698, URX: 300, URY: 722})
	page.Annotations().Add(ra)

	if err := doc.ApplyRedactions(); err != nil {
		t.Fatalf("ApplyRedactions: %v", err)
	}

	var buf bytes.Buffer
	doc.WriteTo(&buf)
	doc2, _ := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	text, _ := doc2.ExtractText()
	pageText := strings.Join(text, "\n")
	if !strings.Contains(pageText, "Public") {
		t.Errorf("expected 'Public' preserved, got %q", pageText)
	}
}

func TestApplyRedactionsAcrossMultiplePages(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	doc.AddBlankPage(595, 842)
	doc.AddBlankPage(595, 842)
	doc.AddBlankPage(595, 842)

	// Add text + redact on pages 1 and 3 (1-based).
	for _, n := range []int{1, 3} {
		page, _ := doc.Page(n)
		page.AddText("Secret",
			pdf.TextStyle{Font: pdf.FontHelvetica, Size: 12},
			pdf.Rectangle{LLX: 50, LLY: 700, URX: 300, URY: 720})
		ra := pdf.NewRedactAnnotation(page, pdf.Rectangle{LLX: 0, LLY: 698, URX: 595, URY: 722})
		page.Annotations().Add(ra)
	}

	if err := doc.ApplyRedactions(); err != nil {
		t.Fatalf("ApplyRedactions: %v", err)
	}

	var buf bytes.Buffer
	doc.WriteTo(&buf)
	doc2, _ := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	text, _ := doc2.ExtractText()
	for i, pageText := range text {
		if strings.Contains(pageText, "Secret") {
			t.Errorf("page %d (1-based %d) still contains 'Secret': %q", i, i+1, pageText)
		}
	}
}

func TestApplyRedactionsRemovesAnnotation(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	ra := pdf.NewRedactAnnotation(page, pdf.Rectangle{LLX: 50, LLY: 700, URX: 300, URY: 720})
	page.Annotations().Add(ra)
	if got := page.Annotations().Count(); got != 1 {
		t.Fatalf("before Apply: Count = %d", got)
	}

	if err := doc.ApplyRedactions(); err != nil {
		t.Fatalf("ApplyRedactions: %v", err)
	}

	if got := page.Annotations().Count(); got != 0 {
		t.Errorf("after Apply: redact still in collection, Count = %d", got)
	}
}

func TestApplyRedactionsCoexistsWithOtherAnnotations(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)

	link := pdf.NewLinkAnnotation(page, pdf.Rectangle{LLX: 50, LLY: 600, URX: 200, URY: 620})
	page.Annotations().Add(link)

	hl := pdf.NewHighlightAnnotation(page, pdf.Rectangle{LLX: 50, LLY: 500, URX: 200, URY: 520})
	page.Annotations().Add(hl)

	ra := pdf.NewRedactAnnotation(page, pdf.Rectangle{LLX: 50, LLY: 700, URX: 200, URY: 720})
	page.Annotations().Add(ra)

	if err := doc.ApplyRedactions(); err != nil {
		t.Fatalf("ApplyRedactions: %v", err)
	}

	counts := map[pdf.AnnotationType]int{}
	for _, a := range page.Annotations().All() {
		counts[a.AnnotationType()]++
	}
	if counts[pdf.AnnotationTypeRedact] != 0 {
		t.Errorf("Redact should be gone, count = %d", counts[pdf.AnnotationTypeRedact])
	}
	if counts[pdf.AnnotationTypeLink] != 1 {
		t.Errorf("Link should survive, count = %d", counts[pdf.AnnotationTypeLink])
	}
	if counts[pdf.AnnotationTypeHighlight] != 1 {
		t.Errorf("Highlight should survive, count = %d", counts[pdf.AnnotationTypeHighlight])
	}
}

func TestApplyRedactionsOverlayText(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	page.AddText("Confidential",
		pdf.TextStyle{Font: pdf.FontHelvetica, Size: 14},
		pdf.Rectangle{LLX: 50, LLY: 700, URX: 545, URY: 720})
	ra := pdf.NewRedactAnnotation(page, pdf.Rectangle{LLX: 50, LLY: 700, URX: 545, URY: 720})
	ra.SetInteriorColor(&pdf.Color{R: 0, G: 0, B: 0, A: 1})
	ra.SetOverlayText("REDACTED")
	page.Annotations().Add(ra)

	if err := doc.ApplyRedactions(); err != nil {
		t.Fatalf("ApplyRedactions: %v", err)
	}

	var buf bytes.Buffer
	doc.WriteTo(&buf)
	doc2, _ := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	text, _ := doc2.ExtractText()
	pageText := strings.Join(text, "\n")
	if strings.Contains(pageText, "Confidential") {
		t.Errorf("expected Confidential removed, got %q", pageText)
	}
	if !strings.Contains(pageText, "REDACTED") {
		t.Errorf("expected REDACTED overlay present, got %q", pageText)
	}
}

func TestApplyRedactionsRepeatOverlay(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	page.AddText("Sensitive info to be hidden",
		pdf.TextStyle{Font: pdf.FontHelvetica, Size: 12},
		pdf.Rectangle{LLX: 50, LLY: 700, URX: 545, URY: 720})
	ra := pdf.NewRedactAnnotation(page, pdf.Rectangle{LLX: 50, LLY: 700, URX: 545, URY: 720})
	ra.SetOverlayText("X")
	ra.SetRepeatOverlayText(true)
	page.Annotations().Add(ra)

	if err := doc.ApplyRedactions(); err != nil {
		t.Fatalf("ApplyRedactions: %v", err)
	}

	var buf bytes.Buffer
	doc.WriteTo(&buf)
	doc2, _ := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	text, _ := doc2.ExtractText()
	pageText := strings.Join(text, "\n")
	// Expect multiple X's in extracted text (tiled).
	if count := strings.Count(pageText, "X"); count < 2 {
		t.Errorf("expected >=2 X's tiled, got %d in %q", count, pageText)
	}
}

func TestApplyRedactionsNoOverlayKeepsFill(t *testing.T) {
	// /IC set but no /OverlayText — should still emit a fill block.
	// We can't easily verify the visual but at least confirm Apply succeeds
	// and the redact annotation is removed.
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	page.AddText("Hide me",
		pdf.TextStyle{Font: pdf.FontHelvetica, Size: 12},
		pdf.Rectangle{LLX: 50, LLY: 700, URX: 200, URY: 720})
	ra := pdf.NewRedactAnnotation(page, pdf.Rectangle{LLX: 50, LLY: 700, URX: 200, URY: 720})
	ra.SetInteriorColor(&pdf.Color{R: 1, G: 0, B: 0, A: 1}) // red fill, no overlay text
	page.Annotations().Add(ra)

	if err := doc.ApplyRedactions(); err != nil {
		t.Fatalf("ApplyRedactions: %v", err)
	}

	// Verify redact annotation is gone.
	if got := page.Annotations().Count(); got != 0 {
		t.Errorf("expected annotation removed, Count = %d", got)
	}
}
