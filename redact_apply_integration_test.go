// SPDX-License-Identifier: MIT

package asposepdf_test

import (
	"bytes"
	"strings"
	"testing"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

func TestApplyRedactionsValidate(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	page.AddText("Confidential", pdf.TextStyle{Font: pdf.FontHelvetica, Size: 12},
		pdf.Rectangle{LLX: 100, LLY: 700, URX: 300, URY: 720})
	ra := pdf.NewRedactAnnotation(page, pdf.Rectangle{LLX: 100, LLY: 700, URX: 300, URY: 720})
	page.Annotations().Add(ra)
	if err := doc.ValidateRedactions(); err != nil {
		t.Errorf("ValidateRedactions returned error: %v", err)
	}
}

func TestApplyRedactionsEmpty(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	if err := doc.ApplyRedactions(); err != nil {
		t.Errorf("ApplyRedactions on empty doc returned error: %v", err)
	}
}

func TestApplyRedactionsTextExtractionPostApply(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	page.AddText("Public Hidden",
		pdf.TextStyle{Font: pdf.FontHelvetica, Size: 12},
		pdf.Rectangle{LLX: 50, LLY: 700, URX: 300, URY: 720})
	// Add redact over "Hidden" portion. Helvetica 12pt — "Public " is ~42pt from x=50.
	// LLX=100 covers the "Hidden" text while preserving "Public ".
	ra := pdf.NewRedactAnnotation(page, pdf.Rectangle{LLX: 100, LLY: 698, URX: 300, URY: 722})
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
	if strings.Contains(pageText, "Hidden") {
		t.Errorf("ExtractText returned redacted content: %q", pageText)
	}
	if !strings.Contains(pageText, "Public") {
		t.Errorf("ExtractText missing non-redacted content: %q", pageText)
	}
}

func TestSubepic4FilterByType(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)

	fa := pdf.NewFileAttachmentAnnotation(page, pdf.Point{X: 50, Y: 700})
	page.Annotations().Add(fa)

	ra := pdf.NewRedactAnnotation(page, pdf.Rectangle{LLX: 100, LLY: 600, URX: 300, URY: 650})
	page.Annotations().Add(ra)

	var buf bytes.Buffer
	doc.WriteTo(&buf)
	doc2, _ := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	page2, _ := doc2.Page(1)

	counts := map[pdf.AnnotationType]int{}
	for _, a := range page2.Annotations().All() {
		counts[a.AnnotationType()]++
	}
	if counts[pdf.AnnotationTypeFileAttachment] != 1 {
		t.Errorf("FileAttachment count = %d, want 1", counts[pdf.AnnotationTypeFileAttachment])
	}
	if counts[pdf.AnnotationTypeRedact] != 1 {
		t.Errorf("Redact count = %d, want 1", counts[pdf.AnnotationTypeRedact])
	}
}

func TestSubepic4RegenerateAppearance(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)

	fa := pdf.NewFileAttachmentAnnotation(page, pdf.Point{X: 50, Y: 700})
	page.Annotations().Add(fa)
	fa.RegenerateAppearance() // no-op for FileAttachment

	ra := pdf.NewRedactAnnotation(page, pdf.Rectangle{LLX: 100, LLY: 600, URX: 300, URY: 650})
	page.Annotations().Add(ra)
	ra.RegenerateAppearance() // mark-mode visual rebuild
}
