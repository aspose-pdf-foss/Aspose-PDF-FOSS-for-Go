package asposepdf_test

import (
	"path/filepath"
	"testing"

	asposepdf "github.com/aspose/pdf-for-go"
)

func TestMetadataCustomFieldsRoundTrip(t *testing.T) {
	doc, err := asposepdf.Open(testFile(t))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	meta, err := doc.Metadata()
	if err != nil {
		t.Fatalf("Metadata: %v", err)
	}
	if len(meta.Custom) != 0 {
		t.Errorf("expected no custom fields, got %v", meta.Custom)
	}
}

func TestDocumentMetadataFields(t *testing.T) {
	doc, err := asposepdf.Open(testFile(t))
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
	if meta.Author != "" {
		t.Errorf("Author: expected empty, got %q", meta.Author)
	}
	if meta.Subject != "" {
		t.Errorf("Subject: expected empty, got %q", meta.Subject)
	}
}

func TestDocumentMetadata(t *testing.T) {
	doc, err := asposepdf.Open(testFile(t))
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
	group := testGroups(t)[0]
	doc1, err := asposepdf.Open(group[0])
	if err != nil {
		t.Fatalf("Open doc1: %v", err)
	}
	doc2, err := asposepdf.Open(group[1])
	if err != nil {
		t.Fatalf("Open doc2: %v", err)
	}
	doc1.Append(doc2)

	meta, err := doc1.Metadata()
	if err != nil {
		t.Fatalf("Metadata: %v", err)
	}
	// Should still be doc1's metadata.
	if meta.Title != "Untitled" {
		t.Errorf("Title: got %q, want %q", meta.Title, "Untitled")
	}
}

func TestSetMetadataRoundTrip(t *testing.T) {
	doc, err := asposepdf.Open(testFile(t))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	want := asposepdf.Metadata{
		Title:   "Test Title",
		Author:  "Test Author",
		Subject: "Test Subject",
	}
	doc.SetMetadata(want)

	// Metadata() reads live from doc.info — no save/reload needed.
	got, err := doc.Metadata()
	if err != nil {
		t.Fatalf("Metadata: %v", err)
	}
	if got.Title != want.Title {
		t.Errorf("Title: got %q, want %q", got.Title, want.Title)
	}
	if got.Author != want.Author {
		t.Errorf("Author: got %q, want %q", got.Author, want.Author)
	}
	if got.Subject != want.Subject {
		t.Errorf("Subject: got %q, want %q", got.Subject, want.Subject)
	}
	if got.Keywords != "" {
		t.Errorf("Keywords: expected empty, got %q", got.Keywords)
	}

	// Also verify round-trip through save/reload.
	tmp := filepath.Join(t.TempDir(), "out.pdf")
	if err := doc.Save(tmp); err != nil {
		t.Fatalf("Save: %v", err)
	}
	doc2, err := asposepdf.Open(tmp)
	if err != nil {
		t.Fatalf("Open saved: %v", err)
	}
	got2, err := doc2.Metadata()
	if err != nil {
		t.Fatalf("Metadata after reload: %v", err)
	}
	if got2.Title != want.Title {
		t.Errorf("reloaded Title: got %q, want %q", got2.Title, want.Title)
	}
	if got2.Author != want.Author {
		t.Errorf("reloaded Author: got %q, want %q", got2.Author, want.Author)
	}
}

func TestSetMetadataCustomFields(t *testing.T) {
	doc, err := asposepdf.Open(testFile(t))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	doc.SetMetadata(asposepdf.Metadata{
		Title:  "Doc",
		Custom: map[string]string{"Department": "Legal", "Version": "2.0"},
	})

	tmp := filepath.Join(t.TempDir(), "out.pdf")
	if err := doc.Save(tmp); err != nil {
		t.Fatalf("Save: %v", err)
	}
	doc2, err := asposepdf.Open(tmp)
	if err != nil {
		t.Fatalf("Open saved: %v", err)
	}
	got, err := doc2.Metadata()
	if err != nil {
		t.Fatalf("Metadata: %v", err)
	}
	if got.Custom["Department"] != "Legal" {
		t.Errorf("Department: got %q, want %q", got.Custom["Department"], "Legal")
	}
	if got.Custom["Version"] != "2.0" {
		t.Errorf("Version: got %q, want %q", got.Custom["Version"], "2.0")
	}
}

func TestSetMetadataReplaces(t *testing.T) {
	doc, err := asposepdf.Open(testFile(t))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	doc.SetMetadata(asposepdf.Metadata{Author: "New Author"})

	tmp := filepath.Join(t.TempDir(), "out.pdf")
	if err := doc.Save(tmp); err != nil {
		t.Fatalf("Save: %v", err)
	}
	doc2, err := asposepdf.Open(tmp)
	if err != nil {
		t.Fatalf("Open saved: %v", err)
	}
	got, err := doc2.Metadata()
	if err != nil {
		t.Fatalf("Metadata: %v", err)
	}
	if got.Author != "New Author" {
		t.Errorf("Author: got %q, want %q", got.Author, "New Author")
	}
	// Title from source must NOT appear — SetMetadata is a full replacement.
	if got.Title != "" {
		t.Errorf("Title must be absent after SetMetadata without Title, got %q", got.Title)
	}
}

func TestClearMetadata(t *testing.T) {
	doc, err := asposepdf.Open(testFile(t))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	doc.ClearMetadata()

	tmp := filepath.Join(t.TempDir(), "out.pdf")
	if err := doc.Save(tmp); err != nil {
		t.Fatalf("Save: %v", err)
	}
	doc2, err := asposepdf.Open(tmp)
	if err != nil {
		t.Fatalf("Open saved: %v", err)
	}
	got, err := doc2.Metadata()
	if err != nil {
		t.Fatalf("Metadata: %v", err)
	}
	if got.Title != "" || got.Author != "" || got.Subject != "" ||
		got.Keywords != "" || got.Creator != "" || got.Producer != "" ||
		got.CreationDate != "" || got.ModDate != "" || len(got.Custom) != 0 {
		t.Errorf("expected empty Metadata after ClearMetadata, got %+v", got)
	}
}
