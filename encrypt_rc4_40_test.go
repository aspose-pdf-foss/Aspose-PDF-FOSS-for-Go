// SPDX-License-Identifier: MIT

package asposepdf_test

import (
	"bytes"
	"strings"
	"testing"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

// encryptRC4_40 writes a one-page document under the original 40-bit handler.
func encryptRC4_40(t *testing.T, user, owner string) []byte {
	t.Helper()
	doc := pdf.NewDocument(300, 200)
	page, err := doc.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	if err := page.AddText("Forty bits of history", pdf.TextStyle{Font: pdf.FontHelvetica, Size: 14},
		pdf.Rectangle{LLX: 20, LLY: 100, URX: 280, URY: 140}); err != nil {
		t.Fatal(err)
	}
	doc.SetEncryption(pdf.EncryptionOptions{
		UserPassword:  user,
		OwnerPassword: owner,
		Algorithm:     pdf.EncryptionAlgRC4_40,
	})
	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatalf("WriteTo: %v", err)
	}
	return buf.Bytes()
}

// The constant must land after the three that already exist, or every stored
// option value shifts meaning.
func TestEncryptionAlgRC4_40Constant(t *testing.T) {
	if int(pdf.EncryptionAlgRC4_40) != 3 {
		t.Errorf("EncryptionAlgRC4_40 = %d, want 3 (after AES128=0, RC4_128=1, AES256=2)", int(pdf.EncryptionAlgRC4_40))
	}
}

// A 40-bit document announces the original handler: V=1, R=2, 40-bit key.
func TestEncryptRC4_40DictionaryShape(t *testing.T) {
	data := encryptRC4_40(t, "open", "master")
	for _, want := range []string{"/V 1", "/R 2", "/Length 40", "/Filter /Standard"} {
		if !bytes.Contains(data, []byte(want)) {
			t.Errorf("the /Encrypt dictionary does not carry %q", want)
		}
	}
	if bytes.Contains(data, []byte("/CF")) {
		t.Error("V=1 must not carry a crypt filter")
	}
}

// Both passwords open it, and the content survives the round trip.
func TestEncryptRC4_40RoundTrip(t *testing.T) {
	data := encryptRC4_40(t, "open", "master")

	if _, err := pdf.OpenStream(bytes.NewReader(data)); err == nil {
		t.Error("an encrypted document opened without a password")
	}

	for _, pw := range []string{"open", "master"} {
		doc, err := pdf.OpenStreamWithPassword(bytes.NewReader(data), pw)
		if err != nil {
			t.Fatalf("open with %q: %v", pw, err)
		}
		page, err := doc.Page(1)
		if err != nil {
			t.Fatal(err)
		}
		text, err := page.ExtractText()
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(text, "Forty bits of history") {
			t.Errorf("with %q extracted %q", pw, text)
		}
	}

	if _, err := pdf.OpenStreamWithPassword(bytes.NewReader(data), "wrong"); err == nil {
		t.Error("a wrong password was accepted")
	}
}

// Permissions survive the 40-bit handler the same as any other.
func TestEncryptRC4_40Permissions(t *testing.T) {
	doc := pdf.NewDocument(300, 200)
	doc.SetEncryption(pdf.EncryptionOptions{
		UserPassword: "open",
		Algorithm:    pdf.EncryptionAlgRC4_40,
		Permissions:  &pdf.Permissions{AllowPrint: true},
	})
	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	reopened, err := pdf.OpenStreamWithPassword(bytes.NewReader(buf.Bytes()), "open")
	if err != nil {
		t.Fatal(err)
	}
	perms, encrypted := reopened.Permissions()
	if !encrypted {
		t.Fatal("Permissions reports the document as unencrypted")
	}
	if !perms.AllowPrint {
		t.Error("AllowPrint was not preserved")
	}
	if perms.AllowModify {
		t.Error("AllowModify was granted but never asked for")
	}
}
