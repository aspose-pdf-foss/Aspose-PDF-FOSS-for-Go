// SPDX-License-Identifier: MIT

package asposepdf_test

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

func TestSetEncryptionAES256_RoundTrip(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	page.AddText("Strong-encrypted content",
		pdf.TextStyle{Font: pdf.FontHelvetica, Size: 12},
		pdf.Rectangle{LLX: 50, LLY: 700, URX: 545, URY: 720})
	doc.SetEncryption(pdf.EncryptionOptions{
		UserPassword: "pwd",
		Algorithm:    pdf.EncryptionAlgAES256,
	})
	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	if _, err := pdf.OpenStream(bytes.NewReader(buf.Bytes())); !errors.Is(err, pdf.ErrEncrypted) {
		t.Errorf("OpenStream without password: %v, want ErrEncrypted", err)
	}
	doc2, err := pdf.OpenStreamWithPassword(bytes.NewReader(buf.Bytes()), "pwd")
	if err != nil {
		t.Fatalf("OpenStreamWithPassword: %v", err)
	}
	text, _ := doc2.ExtractText()
	if !strings.Contains(strings.Join(text, "\n"), "Strong-encrypted") {
		t.Errorf("text not recovered: %q", text)
	}
}

func TestSetEncryptionAES256_OutputV5R6Shape(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	doc.SetEncryption(pdf.EncryptionOptions{UserPassword: "x", Algorithm: pdf.EncryptionAlgAES256})
	var buf bytes.Buffer
	doc.WriteTo(&buf)
	s := buf.String()
	for _, want := range []string{"/V 5", "/R 6", "/Length 256", "/CFM /AESV3", "/EncryptMetadata", "/UE ", "/OE ", "/Perms "} {
		if !strings.Contains(s, want) {
			t.Errorf("encrypted output missing %q", want)
		}
	}
}

func TestSetEncryptionAES256_HeaderIsPDF20(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	doc.SetEncryption(pdf.EncryptionOptions{UserPassword: "x", Algorithm: pdf.EncryptionAlgAES256})
	var buf bytes.Buffer
	doc.WriteTo(&buf)
	if !bytes.HasPrefix(buf.Bytes(), []byte("%PDF-2.0")) {
		t.Errorf("PDF header should start with %%PDF-2.0, got: %q", buf.Bytes()[:16])
	}
}

func TestSetEncryptionAES256_DefaultIsStillAES128(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	doc.SetEncryption(pdf.EncryptionOptions{UserPassword: "x"}) // no Algorithm → zero value
	var buf bytes.Buffer
	doc.WriteTo(&buf)
	s := buf.String()
	if !strings.Contains(s, "/V 4") {
		t.Errorf("default should be V=4 (AES-128), missing in output")
	}
	if strings.Contains(s, "/V 5") {
		t.Error("default should NOT produce V=5 output")
	}
	if !bytes.HasPrefix(buf.Bytes(), []byte("%PDF-1.4")) {
		t.Errorf("AES-128 header should start with %%PDF-1.4, got: %q", buf.Bytes()[:16])
	}
}

func TestSetEncryptionAES256_RC4Unchanged(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	doc.SetEncryption(pdf.EncryptionOptions{UserPassword: "x", Algorithm: pdf.EncryptionAlgRC4_128})
	var buf bytes.Buffer
	doc.WriteTo(&buf)
	s := buf.String()
	if !strings.Contains(s, "/V 2") {
		t.Error("explicit RC4 should still produce V=2 output")
	}
	if strings.Contains(s, "/CFM") {
		t.Error("RC4 dict should not contain /CFM")
	}
}

func TestSetEncryptionAES256_WrongPassword(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	doc.SetEncryption(pdf.EncryptionOptions{UserPassword: "right", Algorithm: pdf.EncryptionAlgAES256})
	var buf bytes.Buffer
	doc.WriteTo(&buf)
	if _, err := pdf.OpenStreamWithPassword(bytes.NewReader(buf.Bytes()), "wrong"); err == nil {
		t.Error("expected error for wrong password")
	}
}

func TestSetEncryptionAES256_OwnerPasswordRecovery(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	doc.SetEncryption(pdf.EncryptionOptions{
		UserPassword:  "user",
		OwnerPassword: "owner",
		Algorithm:     pdf.EncryptionAlgAES256,
	})
	var buf bytes.Buffer
	doc.WriteTo(&buf)
	if _, err := pdf.OpenStreamWithPassword(bytes.NewReader(buf.Bytes()), "owner"); err != nil {
		t.Errorf("owner password open: %v", err)
	}
}

func TestSetEncryptionAES256_PermissionsRoundTrip(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	doc.SetEncryption(pdf.EncryptionOptions{
		UserPassword: "x",
		Algorithm:    pdf.EncryptionAlgAES256,
		Permissions: &pdf.Permissions{
			AllowPrint: true,
			AllowCopy:  true,
		},
	})
	var buf bytes.Buffer
	doc.WriteTo(&buf)
	doc2, err := pdf.OpenStreamWithPassword(bytes.NewReader(buf.Bytes()), "x")
	if err != nil {
		t.Fatal(err)
	}
	perms, encrypted := doc2.Permissions()
	if !encrypted {
		t.Fatal("Permissions reports not encrypted")
	}
	if !perms.AllowPrint || !perms.AllowCopy {
		t.Errorf("permissions lost: %+v", perms)
	}
	if perms.AllowModify {
		t.Errorf("AllowModify should be false: %+v", perms)
	}
}

func TestSetEncryptionAES256_PermsTamperDetection(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	doc.SetEncryption(pdf.EncryptionOptions{
		UserPassword: "x",
		Algorithm:    pdf.EncryptionAlgAES256,
		Permissions: &pdf.Permissions{AllowPrint: false},
	})
	var buf bytes.Buffer
	doc.WriteTo(&buf)
	raw := buf.Bytes()

	// Tamper attack: locate the /Perms <...> hex literal in the output
	// and flip one hex digit. This corrupts the encrypted /Perms block,
	// so the 'adb' marker will not appear after AES decryption, and
	// OpenStreamWithPassword must return an error.
	permsIdx := bytes.Index(raw, []byte("/Perms <"))
	if permsIdx < 0 {
		t.Skip("/Perms hex literal not found — output shape changed")
	}
	// Find the first hex digit after "/Perms <" and flip it.
	for i := permsIdx + 8; i < len(raw); i++ {
		c := raw[i]
		if (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F') {
			if c == 'f' || c == 'F' {
				raw[i] = '0'
			} else {
				raw[i]++
			}
			break
		}
	}
	if _, err := pdf.OpenStreamWithPassword(bytes.NewReader(raw), "x"); err == nil {
		t.Error("expected error after /Perms tampering; /Perms tamper-detection should fire")
	}
}
