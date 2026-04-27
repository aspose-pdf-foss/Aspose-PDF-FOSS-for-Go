package asposepdf_test

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	pdf "github.com/aspose/pdf-for-go"
)

// makeEncryptedDoc builds a single-page PDF, draws a known string, encrypts
// it with the given user/owner passwords, and returns the encoded bytes plus
// the original text for assertion.
func makeEncryptedDoc(t *testing.T, userPwd, ownerPwd string) (data []byte, originalText string) {
	t.Helper()
	doc := pdf.NewDocument(595, 842)
	page, err := doc.Page(1)
	if err != nil {
		t.Fatalf("Page(1): %v", err)
	}
	originalText = "Hello, encrypted world!"
	if err := page.AddText(originalText, pdf.TextStyle{
		Font: pdf.FontHelvetica, Size: 14,
	}, pdf.Rectangle{LLX: 50, LLY: 700, URX: 545, URY: 750}); err != nil {
		t.Fatalf("AddText: %v", err)
	}
	doc.SetPassword(userPwd, ownerPwd)

	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatalf("WriteTo: %v", err)
	}
	return buf.Bytes(), originalText
}

// TestOpenWithPasswordRoundTrip is the central correctness test for the
// password-aware Open path: encrypt a document, reopen it with the same
// password, extract its text, compare against the original. Anything broken
// in /Encrypt parsing, password verification, key derivation, or per-object
// stream/string decryption fails here.
func TestOpenWithPasswordRoundTrip(t *testing.T) {
	data, want := makeEncryptedDoc(t, "secret", "owner")

	doc, err := pdf.OpenStreamWithPassword(bytes.NewReader(data), "secret")
	if err != nil {
		t.Fatalf("OpenStreamWithPassword: %v", err)
	}
	page, err := doc.Page(1)
	if err != nil {
		t.Fatalf("Page(1): %v", err)
	}
	got, err := page.ExtractText()
	if err != nil {
		t.Fatalf("ExtractText: %v", err)
	}
	if !strings.Contains(got, want) {
		t.Errorf("decrypted text = %q, want it to contain %q", got, want)
	}
}

// TestOpenWithOwnerPassword verifies the owner password also unlocks.
func TestOpenWithOwnerPassword(t *testing.T) {
	data, want := makeEncryptedDoc(t, "user-pw", "owner-pw")

	doc, err := pdf.OpenStreamWithPassword(bytes.NewReader(data), "owner-pw")
	if err != nil {
		t.Fatalf("OpenStreamWithPassword(owner): %v", err)
	}
	page, _ := doc.Page(1)
	got, err := page.ExtractText()
	if err != nil {
		t.Fatalf("ExtractText: %v", err)
	}
	if !strings.Contains(got, want) {
		t.Errorf("text = %q, want it to contain %q", got, want)
	}
}

// TestOpenEncryptedReturnsErrEncrypted: plain Open on an encrypted file must
// return a sentinel ErrEncrypted error so callers can switch to the
// password-aware path. Replaces the previous TestOpenEncryptedReturnsError
// which only checked the error message contained "encrypted".
func TestOpenEncryptedReturnsErrEncrypted(t *testing.T) {
	data, _ := makeEncryptedDoc(t, "secret", "")

	_, err := pdf.OpenStream(bytes.NewReader(data))
	if err == nil {
		t.Fatal("expected error opening encrypted PDF without password, got nil")
	}
	if !errors.Is(err, pdf.ErrEncrypted) {
		t.Errorf("expected errors.Is(err, ErrEncrypted), got: %v", err)
	}
}

// TestOpenWithWrongPassword: a wrong password must fail clearly.
func TestOpenWithWrongPassword(t *testing.T) {
	data, _ := makeEncryptedDoc(t, "secret", "")

	_, err := pdf.OpenStreamWithPassword(bytes.NewReader(data), "wrong")
	if err == nil {
		t.Fatal("expected error for wrong password, got nil")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "password") {
		t.Errorf("error should mention password, got: %v", err)
	}
}

// TestOpenWithPasswordOnPlainPDF: opening an unencrypted file with a password
// is allowed and equivalent to plain Open. The password is silently ignored.
// This makes OpenWithPassword a safe drop-in default for code that doesn't
// know up front whether a file is encrypted.
func TestOpenWithPasswordOnPlainPDF(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	_ = page.AddText("plain", pdf.TextStyle{Size: 12},
		pdf.Rectangle{LLX: 50, LLY: 700, URX: 545, URY: 750})
	var buf bytes.Buffer
	_, _ = doc.WriteTo(&buf)

	_, err := pdf.OpenStreamWithPassword(bytes.NewReader(buf.Bytes()), "anything")
	if err != nil {
		t.Errorf("OpenStreamWithPassword on plain PDF should succeed, got: %v", err)
	}
}
