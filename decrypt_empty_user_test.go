// SPDX-License-Identifier: MIT

package asposepdf_test

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

// TestOpenEmptyUserPassword: a file protected only by an owner password needs
// no password to open, so Open opens it as a viewer does (pdf-go-sso1); one
// that needs a user password still returns ErrEncrypted.
func TestOpenEmptyUserPassword(t *testing.T) {
	for _, alg := range []pdf.EncryptionAlgorithm{pdf.EncryptionAlgRC4_128, pdf.EncryptionAlgAES128, pdf.EncryptionAlgAES256} {
		doc := pdf.NewDocument(300, 100)
		p, _ := doc.Page(1)
		if err := p.AddText("restricted text", pdf.TextStyle{}, pdf.Rectangle{LLX: 10, LLY: 40, URX: 290, URY: 90}); err != nil {
			t.Fatal(err)
		}
		doc.SetEncryption(pdf.EncryptionOptions{
			OwnerPassword: "owner", Algorithm: alg,
			Permissions: &pdf.Permissions{AllowPrint: true},
		})
		var buf bytes.Buffer
		if _, err := doc.WriteTo(&buf); err != nil {
			t.Fatal(err)
		}

		got, err := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
		if err != nil {
			t.Fatalf("alg %v: OpenStream: %v", alg, err)
		}
		texts, err := got.ExtractText()
		if err != nil || !strings.Contains(texts[0], "restricted text") {
			t.Errorf("alg %v: text %q, %v", alg, texts, err)
		}
		perms, encrypted := got.Permissions()
		if !encrypted || !perms.AllowPrint || perms.AllowCopy {
			t.Errorf("alg %v: permissions %+v, encrypted %v", alg, perms, encrypted)
		}

		// Saving keeps the encryption: the result opens the same way, and the
		// owner password still unlocks it.
		buf.Reset()
		if _, err := got.WriteTo(&buf); err != nil {
			t.Fatal(err)
		}
		again, err := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
		if err != nil {
			t.Fatalf("alg %v: reopen: %v", alg, err)
		}
		if _, encrypted := again.Permissions(); !encrypted {
			t.Errorf("alg %v: encryption lost on save", alg)
		}
		if _, err := pdf.OpenStreamWithPassword(bytes.NewReader(buf.Bytes()), "owner"); err != nil {
			t.Errorf("alg %v: owner password after save: %v", alg, err)
		}
	}

	// A user password is still needed when there is one.
	doc := pdf.NewDocument(300, 100)
	doc.SetEncryption(pdf.EncryptionOptions{UserPassword: "user", OwnerPassword: "owner"})
	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	if _, err := pdf.OpenStream(bytes.NewReader(buf.Bytes())); !errors.Is(err, pdf.ErrEncrypted) {
		t.Errorf("OpenStream with a user password set: %v, want ErrEncrypted", err)
	}
}
