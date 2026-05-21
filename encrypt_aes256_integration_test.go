package asposepdf_test

import (
	"bytes"
	"strings"
	"testing"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

// TestSetEncryptionAES256_WithFileAttachment verifies that AES-256
// encryption interoperates with FileAttachment annotations: an embedded
// file survives a Write/Open roundtrip and its bytes come back intact.
func TestSetEncryptionAES256_WithFileAttachment(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	fa := pdf.NewFileAttachmentAnnotation(page, pdf.Point{X: 50, Y: 700})
	fa.SetIcon(pdf.FileAttachmentIconPushPin)
	if err := fa.SetFileFromStream(strings.NewReader("attached data 256"), "data.txt"); err != nil {
		t.Fatal(err)
	}
	page.Annotations().Add(fa)
	doc.SetEncryption(pdf.EncryptionOptions{UserPassword: "x", Algorithm: pdf.EncryptionAlgAES256})

	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatalf("WriteTo: %v", err)
	}

	doc2, err := pdf.OpenStreamWithPassword(bytes.NewReader(buf.Bytes()), "x")
	if err != nil {
		t.Fatalf("OpenStreamWithPassword: %v", err)
	}
	page2 := doc2.Pages()[0]
	fa2, ok := page2.Annotations().At(0).(*pdf.FileAttachmentAnnotation)
	if !ok {
		t.Fatal("first annotation is not a FileAttachmentAnnotation")
	}
	if got := string(fa2.FileBytes()); got != "attached data 256" {
		t.Errorf("file bytes after AES-256 roundtrip = %q, want %q", got, "attached data 256")
	}
}

// TestSetEncryptionAES256_WithAcroForm verifies that AES-256 encryption
// interoperates with AcroForm fields: field values in widget annotations
// and /V dictionary entries survive encryption roundtrip.
func TestSetEncryptionAES256_WithAcroForm(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	form := doc.Form()

	// Add text field.
	tb, err := form.AddTextField(1, pdf.Rectangle{LLX: 50, LLY: 700, URX: 200, URY: 720}, "Name")
	if err != nil {
		t.Fatalf("AddTextField: %v", err)
	}
	tb.SetValue("Bob")

	// Encrypt with AES-256.
	doc.SetEncryption(pdf.EncryptionOptions{
		UserPassword: "x",
		Algorithm:    pdf.EncryptionAlgAES256,
	})

	// Serialize and decrypt.
	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatalf("WriteTo: %v", err)
	}
	doc2, err := pdf.OpenStreamWithPassword(bytes.NewReader(buf.Bytes()), "x")
	if err != nil {
		t.Fatalf("OpenStreamWithPassword: %v", err)
	}

	// Verify field survived roundtrip.
	field := doc2.Form().Field("Name")
	if field == nil {
		t.Fatal("field Name not found after roundtrip")
	}
	if v := field.Value(); v != "Bob" {
		t.Errorf("Name value after AES-256 roundtrip = %q, want %q", v, "Bob")
	}
}

// TestSetEncryptionAES256_MultiPage verifies that AES-256 encryption
// works correctly with multi-page documents: all pages and their content
// survive encryption roundtrip.
func TestSetEncryptionAES256_MultiPage(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	doc.AddBlankPage(595, 842)
	doc.AddBlankPage(595, 842)

	// Add text to each page.
	for n := 1; n <= 3; n++ {
		page, _ := doc.Page(n)
		pageNum := string(rune('0' + n))
		if err := page.AddText("Page "+pageNum, pdf.TextStyle{
			Font: pdf.FontHelvetica,
			Size: 12,
		}, pdf.Rectangle{LLX: 50, LLY: 700, URX: 200, URY: 720}); err != nil {
			t.Fatalf("AddText on page %d: %v", n, err)
		}
	}

	// Encrypt with AES-256.
	doc.SetEncryption(pdf.EncryptionOptions{
		UserPassword: "x",
		Algorithm:    pdf.EncryptionAlgAES256,
	})

	// Serialize and decrypt.
	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatalf("WriteTo: %v", err)
	}
	doc2, err := pdf.OpenStreamWithPassword(bytes.NewReader(buf.Bytes()), "x")
	if err != nil {
		t.Fatalf("OpenStreamWithPassword: %v", err)
	}

	// Verify page structure survived roundtrip.
	if doc2.PageCount() != 3 {
		t.Errorf("PageCount = %d, want 3", doc2.PageCount())
	}

	// Verify page content survived roundtrip.
	text, err := doc2.ExtractText()
	if err != nil {
		t.Fatalf("ExtractText: %v", err)
	}
	if len(text) != 3 {
		t.Fatalf("extracted text length = %d, want 3", len(text))
	}
	for n, pageText := range text {
		pageNum := string(rune('0' + n + 1))
		wantSubstr := "Page " + pageNum
		if !strings.Contains(pageText, wantSubstr) {
			t.Errorf("page %d missing %q: got %q", n+1, wantSubstr, pageText)
		}
	}
}
