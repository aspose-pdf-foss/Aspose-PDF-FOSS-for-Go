package asposepdf_test

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	asposepdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

func TestEncryptSetPassword(t *testing.T) {
	doc, err := asposepdf.Open(testFile(t))
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

func TestEncryptContentIsObfuscated(t *testing.T) {
	path := testFile(t)
	plainData, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read source PDF: %v", err)
	}

	doc, err := asposepdf.Open(path)
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

func TestEncryptFunc(t *testing.T) {
	if err := os.MkdirAll(resultDir, 0o755); err != nil {
		t.Fatalf("create result dir: %v", err)
	}
	outputPath := filepath.Join(resultDir, "encrypt_func.pdf")

	if err := asposepdf.Encrypt(testFile(t), outputPath, "user123", "owner456"); err != nil {
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

func TestEncryptEmptyPassword(t *testing.T) {
	doc, err := asposepdf.Open(testFile(t))
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

func TestOpenEncryptedReturnsError(t *testing.T) {
	doc, err := asposepdf.Open(testFile(t))
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

func TestEncryptPreservesPageCount(t *testing.T) {
	doc, err := asposepdf.Open(testFile(t))
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

	if doc.PageCount() != wantPages {
		t.Errorf("expected %d pages after SetPassword, got %d", wantPages, doc.PageCount())
	}
}

// TestSetEncryptionAES128_RoundTrip verifies AES-128 (V=4 R=4) encryption and decryption.
func TestSetEncryptionAES128_RoundTrip(t *testing.T) {
	doc := asposepdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	page.AddText("Secret content", asposepdf.TextStyle{Font: asposepdf.FontHelvetica, Size: 12},
		asposepdf.Rectangle{LLX: 50, LLY: 700, URX: 545, URY: 720})
	doc.SetEncryption(asposepdf.EncryptionOptions{
		UserPassword: "pwd",
		Algorithm:    asposepdf.EncryptionAlgAES128,
	})
	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	// Encrypted — Open without password fails.
	if _, err := asposepdf.OpenStream(bytes.NewReader(buf.Bytes())); !errors.Is(err, asposepdf.ErrEncrypted) {
		t.Errorf("OpenStream without password: err=%v, want ErrEncrypted", err)
	}
	// OpenWithPassword succeeds.
	doc2, err := asposepdf.OpenStreamWithPassword(bytes.NewReader(buf.Bytes()), "pwd")
	if err != nil {
		t.Fatalf("OpenStreamWithPassword: %v", err)
	}
	text, err := doc2.ExtractText()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(text, "\n"), "Secret content") {
		t.Errorf("text not recovered: %q", text)
	}
}

// TestSetEncryptionAES128_DefaultsToAES verifies that EncryptionAlgAES128 is the default (zero value).
func TestSetEncryptionAES128_DefaultsToAES(t *testing.T) {
	doc := asposepdf.NewDocument(595, 842)
	doc.SetEncryption(asposepdf.EncryptionOptions{UserPassword: "x"}) // no Algorithm — zero value
	var buf bytes.Buffer
	doc.WriteTo(&buf)
	s := buf.String()
	if !strings.Contains(s, "/V 4") {
		t.Errorf("expected /V 4 (AES default), missing")
	}
	if !strings.Contains(s, "/CFM /AESV2") {
		t.Errorf("expected /CFM /AESV2, missing")
	}
}

// TestSetEncryptionRC4Explicit_Unchanged verifies that explicit RC4-128 (V=2 R=3) still works.
func TestSetEncryptionRC4Explicit_Unchanged(t *testing.T) {
	doc := asposepdf.NewDocument(595, 842)
	doc.SetEncryption(asposepdf.EncryptionOptions{
		UserPassword: "x",
		Algorithm:    asposepdf.EncryptionAlgRC4_128,
	})
	var buf bytes.Buffer
	doc.WriteTo(&buf)
	s := buf.String()
	if !strings.Contains(s, "/V 2") {
		t.Errorf("expected /V 2 for explicit RC4, missing")
	}
	if strings.Contains(s, "/CFM") {
		t.Errorf("RC4 dict should not contain /CFM, found it")
	}
	// Confirm RC4 roundtrip still works.
	doc2, err := asposepdf.OpenStreamWithPassword(bytes.NewReader(buf.Bytes()), "x")
	if err != nil {
		t.Fatalf("RC4 OpenStreamWithPassword: %v", err)
	}
	_ = doc2
}

// TestSetEncryptionAES128_WrongPassword verifies that wrong password fails to open.
func TestSetEncryptionAES128_WrongPassword(t *testing.T) {
	doc := asposepdf.NewDocument(595, 842)
	doc.SetEncryption(asposepdf.EncryptionOptions{
		UserPassword: "right",
		Algorithm:    asposepdf.EncryptionAlgAES128,
	})
	var buf bytes.Buffer
	doc.WriteTo(&buf)
	if _, err := asposepdf.OpenStreamWithPassword(bytes.NewReader(buf.Bytes()), "wrong"); err == nil {
		t.Error("expected error for wrong password, got nil")
	}
}

// TestSetEncryptionAES128_OwnerPasswordRecovery verifies that owner password also opens the document.
func TestSetEncryptionAES128_OwnerPasswordRecovery(t *testing.T) {
	doc := asposepdf.NewDocument(595, 842)
	doc.SetEncryption(asposepdf.EncryptionOptions{
		UserPassword:  "user",
		OwnerPassword: "owner",
		Algorithm:     asposepdf.EncryptionAlgAES128,
	})
	var buf bytes.Buffer
	doc.WriteTo(&buf)
	// Owner password should also open the document.
	if _, err := asposepdf.OpenStreamWithPassword(bytes.NewReader(buf.Bytes()), "owner"); err != nil {
		t.Errorf("owner password open: %v", err)
	}
}

// TestSetEncryptionAES128_PermissionsRoundTrip verifies that permissions survive encryption roundtrip.
func TestSetEncryptionAES128_PermissionsRoundTrip(t *testing.T) {
	doc := asposepdf.NewDocument(595, 842)
	doc.SetEncryption(asposepdf.EncryptionOptions{
		UserPassword: "x",
		Algorithm:    asposepdf.EncryptionAlgAES128,
		Permissions: &asposepdf.Permissions{
			AllowPrint: true,
			AllowCopy:  true,
		},
	})
	var buf bytes.Buffer
	doc.WriteTo(&buf)
	doc2, err := asposepdf.OpenStreamWithPassword(bytes.NewReader(buf.Bytes()), "x")
	if err != nil {
		t.Fatal(err)
	}
	perms, encrypted := doc2.Permissions()
	if !encrypted {
		t.Fatal("Permissions() reports not encrypted")
	}
	if !perms.AllowPrint || !perms.AllowCopy {
		t.Errorf("permissions lost: %+v", perms)
	}
	if perms.AllowModify {
		t.Errorf("AllowModify should be false: %+v", perms)
	}
}
