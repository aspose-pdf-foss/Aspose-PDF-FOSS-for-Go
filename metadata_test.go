package asposepdf_test

import (
	"testing"

	asposepdf "github.com/aspose/pdf-for-go"
)

func TestGetMetadata(t *testing.T) {
	meta, err := asposepdf.GetMetadata("testdata/split/4pages.pdf")
	if err != nil {
		t.Fatalf("GetMetadata: %v", err)
	}

	if meta.Title != "Untitled" {
		t.Errorf("Title: got %q, want %q", meta.Title, "Untitled")
	}
	if meta.Creator != "Acrobat Editor 9.0" {
		t.Errorf("Creator: got %q, want %q", meta.Creator, "Acrobat Editor 9.0")
	}
	if meta.Producer != "Adobe Acrobat 9.0.0" {
		t.Errorf("Producer: got %q, want %q", meta.Producer, "Adobe Acrobat 9.0.0")
	}
	if meta.CreationDate == "" {
		t.Error("CreationDate should not be empty")
	}
	if meta.ModDate == "" {
		t.Error("ModDate should not be empty")
	}
	// Fields absent in this file must be empty strings.
	if meta.Author != "" {
		t.Errorf("Author: expected empty, got %q", meta.Author)
	}
	if meta.Subject != "" {
		t.Errorf("Subject: expected empty, got %q", meta.Subject)
	}
}

func TestDocumentMetadata(t *testing.T) {
	doc, err := asposepdf.Open("testdata/split/4pages.pdf")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	meta, err := doc.Metadata()
	if err != nil {
		t.Fatalf("Metadata: %v", err)
	}

	if meta.Title != "Untitled" {
		t.Errorf("Title: got %q, want %q", meta.Title, "Untitled")
	}
	if meta.Producer != "Adobe Acrobat 9.0.0" {
		t.Errorf("Producer: got %q, want %q", meta.Producer, "Adobe Acrobat 9.0.0")
	}
}

func TestDocumentMetadataAfterAppend(t *testing.T) {
	// After Append, Metadata returns info from the first (primary) document.
	doc1, err := asposepdf.Open("testdata/split/4pages.pdf")
	if err != nil {
		t.Fatalf("Open doc1: %v", err)
	}
	doc2, err := asposepdf.Open("testdata/split/marketing.pdf")
	if err != nil {
		t.Fatalf("Open doc2: %v", err)
	}
	combined := doc1.Append(doc2)

	meta, err := combined.Metadata()
	if err != nil {
		t.Fatalf("Metadata: %v", err)
	}
	// Should still be doc1's metadata.
	if meta.Title != "Untitled" {
		t.Errorf("Title: got %q, want %q", meta.Title, "Untitled")
	}
}
