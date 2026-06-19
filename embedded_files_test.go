// SPDX-License-Identifier: MIT

package asposepdf_test

import (
	"bytes"
	"strings"
	"testing"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

// TestEmbeddedFilesRoundTrip attaches files to a document, saves and reopens
// it, and reads the content/metadata back.
func TestEmbeddedFilesRoundTrip(t *testing.T) {
	doc := pdf.NewDocument(300, 200)
	ef := doc.EmbeddedFiles()
	if ef.Count() != 0 {
		t.Fatalf("new document has %d embedded files, want 0", ef.Count())
	}

	f, err := ef.AddFromStream("notes.txt", strings.NewReader("hello world"))
	if err != nil {
		t.Fatalf("AddFromStream: %v", err)
	}
	f.SetDescription("A text note")
	if _, err := ef.AddFromStream("data.json", strings.NewReader(`{"k":1}`)); err != nil {
		t.Fatalf("AddFromStream json: %v", err)
	}
	if ef.Count() != 2 {
		t.Fatalf("Count = %d, want 2", ef.Count())
	}

	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatalf("WriteTo: %v", err)
	}
	out, err := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}

	ef2 := out.EmbeddedFiles()
	if got := ef2.Names(); len(got) != 2 || got[0] != "data.json" || got[1] != "notes.txt" {
		t.Fatalf("Names = %v, want [data.json notes.txt]", got)
	}
	notes := ef2.Get("notes.txt")
	if notes == nil {
		t.Fatal("notes.txt missing after round-trip")
	}
	data, err := notes.Data()
	if err != nil || string(data) != "hello world" {
		t.Errorf("content = %q (%v), want \"hello world\"", data, err)
	}
	if notes.MIMEType() != "text/plain" {
		t.Errorf("MIMEType = %q, want text/plain", notes.MIMEType())
	}
	if notes.Description() != "A text note" {
		t.Errorf("Description = %q, want \"A text note\"", notes.Description())
	}
	if notes.Size() != len("hello world") {
		t.Errorf("Size = %d, want %d", notes.Size(), len("hello world"))
	}
}

// TestEmbeddedFilesRemoveClear covers Remove and Clear.
func TestEmbeddedFilesRemoveClear(t *testing.T) {
	doc := pdf.NewDocument(200, 200)
	ef := doc.EmbeddedFiles()
	ef.AddFromStream("a.bin", strings.NewReader("a"))
	ef.AddFromStream("b.bin", strings.NewReader("b"))

	if !ef.Remove("a.bin") {
		t.Error("Remove(a.bin) = false")
	}
	if ef.Remove("a.bin") {
		t.Error("Remove of an absent file = true")
	}
	if ef.Count() != 1 || !ef.Has("b.bin") {
		t.Errorf("after Remove: count=%d has(b.bin)=%v", ef.Count(), ef.Has("b.bin"))
	}
	ef.Clear()
	if ef.Count() != 0 {
		t.Errorf("after Clear: count=%d, want 0", ef.Count())
	}
}

// TestEmbeddedFilesCoexist verifies attachments share /Catalog/Names with
// named destinations and JavaScript without clobbering them.
func TestEmbeddedFilesCoexist(t *testing.T) {
	doc := pdf.NewDocument(300, 200)
	doc.EmbeddedFiles().AddFromStream("a.txt", strings.NewReader("AAA"))
	p, _ := doc.Page(1)
	if err := doc.NamedDestinations().Add("dest1", pdf.NewDestinationFit(p)); err != nil {
		t.Fatalf("named dest: %v", err)
	}
	if err := doc.JavaScript().Add("onOpen", "app.alert('hi')"); err != nil {
		t.Fatalf("javascript: %v", err)
	}

	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatalf("WriteTo: %v", err)
	}
	out, err := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if out.EmbeddedFiles().Count() != 1 {
		t.Error("embedded file lost")
	}
	if !out.NamedDestinations().Has("dest1") {
		t.Error("named destination lost")
	}
	if !out.JavaScript().Has("onOpen") {
		t.Error("javascript lost")
	}
}
