// SPDX-License-Identifier: MIT

package asposepdf_test

import (
	"bytes"
	"path/filepath"
	"testing"

	asposepdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

func TestMetadataCustomFieldsRoundTrip(t *testing.T) {
	doc, err := asposepdf.Open(testFile(t))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	meta, err := doc.Info()
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
	meta, err := doc.Info()
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
	meta, err := doc.Info()
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

	meta, err := doc1.Info()
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
	want := asposepdf.DocumentInfo{
		Title:   "Test Title",
		Author:  "Test Author",
		Subject: "Test Subject",
	}
	doc.SetInfo(want)

	// Info() reads live from doc.info — no save/reload needed.
	got, err := doc.Info()
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
	got2, err := doc2.Info()
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
	doc.SetInfo(asposepdf.DocumentInfo{
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
	got, err := doc2.Info()
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
	doc.SetInfo(asposepdf.DocumentInfo{Author: "New Author"})

	tmp := filepath.Join(t.TempDir(), "out.pdf")
	if err := doc.Save(tmp); err != nil {
		t.Fatalf("Save: %v", err)
	}
	doc2, err := asposepdf.Open(tmp)
	if err != nil {
		t.Fatalf("Open saved: %v", err)
	}
	got, err := doc2.Info()
	if err != nil {
		t.Fatalf("Metadata: %v", err)
	}
	if got.Author != "New Author" {
		t.Errorf("Author: got %q, want %q", got.Author, "New Author")
	}
	// Title from source must NOT appear — SetInfo is a full replacement.
	if got.Title != "" {
		t.Errorf("Title must be absent after SetInfo without Title, got %q", got.Title)
	}
}

func TestClearMetadata(t *testing.T) {
	doc, err := asposepdf.Open(testFile(t))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	doc.ClearInfo()

	tmp := filepath.Join(t.TempDir(), "out.pdf")
	if err := doc.Save(tmp); err != nil {
		t.Fatalf("Save: %v", err)
	}
	doc2, err := asposepdf.Open(tmp)
	if err != nil {
		t.Fatalf("Open saved: %v", err)
	}
	got, err := doc2.Info()
	if err != nil {
		t.Fatalf("Metadata: %v", err)
	}
	if got.Title != "" || got.Author != "" || got.Subject != "" ||
		got.Keywords != "" || got.Creator != "" || got.Producer != "" ||
		got.CreationDate != "" || got.ModDate != "" || len(got.Custom) != 0 {
		t.Errorf("expected empty Info after ClearInfo, got %+v", got)
	}
}

// TestSetInfoNonASCIIRoundTrip pins GitHub issue #13: a non-ASCII Info
// value must round-trip through Save/Open byte-correctly -- it must also
// be encoded correctly for OTHER readers (PDFDocEncoding/UTF-16BE per ISO
// 32000-1 §7.9.2.2, not raw UTF-8 bytes), which TestInfoNonASCIIUsesTextStringEncoding
// (metadata_internal_test.go) checks directly, since a round-trip within
// this library alone can't tell the two apart (it decodes however it
// encoded).
func TestSetInfoNonASCIIRoundTrip(t *testing.T) {
	doc := asposepdf.NewDocumentFromFormat(asposepdf.PageFormatA4)
	want := asposepdf.DocumentInfo{
		Title:    "Meldergruppenauslösung",
		Author:   "Jane Doe (č)",
		Subject:  "emoji: 😀",
		Keywords: "日本語",
	}
	doc.SetInfo(want)

	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	reopened, err := asposepdf.OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	got, err := reopened.Info()
	if err != nil {
		t.Fatal(err)
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
	if got.Keywords != want.Keywords {
		t.Errorf("Keywords: got %q, want %q", got.Keywords, want.Keywords)
	}
}

// TestSetInfoDatesStayASCII confirms the fix does not touch /CreationDate
// or /ModDate -- they are never ISO 32000-1 §7.9.2.2 text strings, always
// plain ASCII date strings, per GitHub issue #13's own "Dates" consideration.
func TestSetInfoDatesStayASCII(t *testing.T) {
	doc := asposepdf.NewDocumentFromFormat(asposepdf.PageFormatA4)
	want := asposepdf.DocumentInfo{
		CreationDate: "D:20261013120000+03'00'",
		ModDate:      "D:20261013130000+03'00'",
	}
	doc.SetInfo(want)

	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	reopened, err := asposepdf.OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	got, err := reopened.Info()
	if err != nil {
		t.Fatal(err)
	}
	if got.CreationDate != want.CreationDate {
		t.Errorf("CreationDate: got %q, want %q", got.CreationDate, want.CreationDate)
	}
	if got.ModDate != want.ModDate {
		t.Errorf("ModDate: got %q, want %q", got.ModDate, want.ModDate)
	}
	if !bytes.Contains(buf.Bytes(), []byte(want.CreationDate)) {
		t.Error("CreationDate is not stored as a plain ASCII literal in the saved file")
	}
}

// TestSetInfoCustomFieldNonASCIIRoundTrip: custom Info entries get the same
// text-string encoding/decoding as the standard fields.
func TestSetInfoCustomFieldNonASCIIRoundTrip(t *testing.T) {
	doc := asposepdf.NewDocumentFromFormat(asposepdf.PageFormatA4)
	doc.SetInfo(asposepdf.DocumentInfo{Custom: map[string]string{"Department": "Übersetzung"}})

	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	reopened, err := asposepdf.OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	got, err := reopened.Info()
	if err != nil {
		t.Fatal(err)
	}
	if got.Custom["Department"] != "Übersetzung" {
		t.Errorf("Custom[Department] = %q, want %q", got.Custom["Department"], "Übersetzung")
	}
}
