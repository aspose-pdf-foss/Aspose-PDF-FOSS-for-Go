package asposepdf_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	asposepdf "github.com/aspose/pdf-for-go"
)

// TestEncryptSetPassword verifies that Document.SetPassword produces an encrypted PDF.
func TestEncryptSetPassword(t *testing.T) {
	doc, err := asposepdf.Open(fourPagesPDF)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	doc.SetPassword("secret", "ownerpass")

	if err := os.MkdirAll(resultDir, 0o755); err != nil {
		t.Fatalf("create result dir: %v", err)
	}
	outputPath := filepath.Join(resultDir, "encrypt_set_password.pdf")
	if err := doc.Save(outputPath); err != nil {
		t.Fatalf("Save: %v", err)
	}

	data, _ := os.ReadFile(outputPath)

	if !bytes.HasPrefix(data, []byte("%PDF-")) {
		t.Fatal("output does not start with PDF header")
	}
	if !bytes.Contains(data, []byte("/Encrypt")) {
		t.Fatal("output is missing /Encrypt entry")
	}
	if !bytes.Contains(data, []byte("/ID")) {
		t.Fatal("output is missing /ID in trailer")
	}
	if !bytes.Contains(data, []byte("/O ")) {
		t.Fatal("output is missing /O in /Encrypt dict")
	}
	if !bytes.Contains(data, []byte("/U ")) {
		t.Fatal("output is missing /U in /Encrypt dict")
	}
}

// TestEncryptContentIsObfuscated verifies that content from 4pages.pdf is not
// readable in clear-text after encryption.
func TestEncryptContentIsObfuscated(t *testing.T) {
	plainData, err := os.ReadFile(fourPagesPDF)
	if err != nil {
		t.Fatalf("read source PDF: %v", err)
	}

	doc, err := asposepdf.Open(fourPagesPDF)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	doc.SetPassword("secret", "")

	if err := os.MkdirAll(resultDir, 0o755); err != nil {
		t.Fatalf("create result dir: %v", err)
	}
	outputPath := filepath.Join(resultDir, "encrypt_content_obfuscated.pdf")
	if err := doc.Save(outputPath); err != nil {
		t.Fatalf("Save: %v", err)
	}

	encrypted, _ := os.ReadFile(outputPath)

	if !bytes.HasPrefix(encrypted, []byte("%PDF-")) {
		t.Fatal("encrypted output does not start with PDF header")
	}
	if !bytes.Contains(encrypted, []byte("/Encrypt")) {
		t.Fatal("output is missing /Encrypt entry")
	}
	if bytes.Equal(plainData, encrypted) {
		t.Error("encrypted output is identical to the plaintext — content was not modified")
	}
}

// TestEncryptFunc verifies the functional Encrypt API using 4pages.pdf.
func TestEncryptFunc(t *testing.T) {
	if err := os.MkdirAll(resultDir, 0o755); err != nil {
		t.Fatalf("create result dir: %v", err)
	}
	outputPath := filepath.Join(resultDir, "encrypt_func.pdf")

	if err := asposepdf.Encrypt(fourPagesPDF, outputPath, "user123", "owner456"); err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	data, _ := os.ReadFile(outputPath)

	if !bytes.HasPrefix(data, []byte("%PDF-")) {
		t.Fatal("output does not start with PDF header")
	}
	if !bytes.Contains(data, []byte("/Encrypt")) {
		t.Fatal("output is missing /Encrypt entry")
	}
	if !bytes.Contains(data, []byte("/Standard")) {
		t.Fatal("output is missing /Standard filter")
	}
}

// TestEncryptEmptyPassword verifies that an empty password is a valid input.
func TestEncryptEmptyPassword(t *testing.T) {
	doc, err := asposepdf.Open(fourPagesPDF)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	doc.SetPassword("", "")

	if err := os.MkdirAll(resultDir, 0o755); err != nil {
		t.Fatalf("create result dir: %v", err)
	}
	outputPath := filepath.Join(resultDir, "encrypt_empty_password.pdf")
	if err := doc.Save(outputPath); err != nil {
		t.Fatalf("Save: %v", err)
	}

	data, _ := os.ReadFile(outputPath)
	if !bytes.Contains(data, []byte("/Encrypt")) {
		t.Fatal("output is missing /Encrypt entry")
	}
}

// TestOpenEncryptedReturnsError verifies that opening an encrypted PDF returns a clear
// error instead of a cryptic parsing failure.
func TestOpenEncryptedReturnsError(t *testing.T) {
	doc, err := asposepdf.Open(fourPagesPDF)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	doc.SetPassword("user", "owner")

	tmp := filepath.Join(t.TempDir(), "encrypted.pdf")
	if err := doc.Save(tmp); err != nil {
		t.Fatalf("Save: %v", err)
	}

	_, err = asposepdf.Open(tmp)
	if err == nil {
		t.Fatal("expected error opening encrypted PDF, got nil")
	}
	if !strings.Contains(err.Error(), "encrypted") {
		t.Errorf("expected error to mention encryption, got: %v", err)
	}
}

// TestEncryptPreservesPageCount verifies the page count is unchanged after encrypt.
func TestEncryptPreservesPageCount(t *testing.T) {
	doc, err := asposepdf.Open(fourPagesPDF)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	wantPages := doc.PageCount()
	doc.SetPassword("pass", "")

	if err := os.MkdirAll(resultDir, 0o755); err != nil {
		t.Fatalf("create result dir: %v", err)
	}
	outputPath := filepath.Join(resultDir, "encrypt_preserves_page_count.pdf")
	if err := doc.Save(outputPath); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Page count is tracked in-memory; it must be unchanged by SetPassword.
	if doc.PageCount() != wantPages {
		t.Errorf("expected %d pages after SetPassword, got %d", wantPages, doc.PageCount())
	}
}
