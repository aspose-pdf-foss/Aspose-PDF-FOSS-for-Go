// SPDX-License-Identifier: MIT

package asposepdf_test

import (
	"bytes"
	"testing"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

// TestChangePassword re-encrypts an encrypted document with a new password,
// keeping the original algorithm and permissions: the old password must stop
// working, the new one must open it, and the content/permissions survive.
func TestChangePassword(t *testing.T) {
	algs := []struct {
		alg  pdf.EncryptionAlgorithm
		name string
	}{
		{pdf.EncryptionAlgRC4_128, "RC4-128"},
		{pdf.EncryptionAlgAES128, "AES-128"},
		{pdf.EncryptionAlgAES256, "AES-256"},
	}
	for _, a := range algs {
		t.Run(a.name, func(t *testing.T) {
			// Encrypted source: old password, print-only permissions.
			doc := pdf.NewDocument(320, 200)
			p, _ := doc.Page(1)
			if err := p.AddText("classified", pdf.TextStyle{Font: pdf.FontHelvetica, Size: 20, Color: &pdf.Color{A: 1}},
				pdf.Rectangle{LLX: 20, LLY: 120, URX: 300, URY: 160}); err != nil {
				t.Fatalf("AddText: %v", err)
			}
			doc.SetEncryption(pdf.EncryptionOptions{
				UserPassword: "oldpw", OwnerPassword: "owner", Algorithm: a.alg,
				Permissions: &pdf.Permissions{AllowPrint: true},
			})
			var src bytes.Buffer
			if _, err := doc.WriteTo(&src); err != nil {
				t.Fatalf("WriteTo: %v", err)
			}

			// Open with the old password and change it.
			d2, err := pdf.OpenStreamWithPassword(bytes.NewReader(src.Bytes()), "oldpw")
			if err != nil {
				t.Fatalf("open with old password: %v", err)
			}
			if err := d2.ChangePassword("newpw", ""); err != nil {
				t.Fatalf("ChangePassword: %v", err)
			}
			var out bytes.Buffer
			if _, err := d2.WriteTo(&out); err != nil {
				t.Fatalf("WriteTo after change: %v", err)
			}

			// The old password must no longer open it.
			if _, err := pdf.OpenStreamWithPassword(bytes.NewReader(out.Bytes()), "oldpw"); err == nil {
				t.Error("old password still opens the document after ChangePassword")
			}
			// The new password must open it, with content and permissions intact.
			d3, err := pdf.OpenStreamWithPassword(bytes.NewReader(out.Bytes()), "newpw")
			if err != nil {
				t.Fatalf("open with new password: %v", err)
			}
			perms, enc := d3.Permissions()
			if !enc {
				t.Error("document is no longer encrypted after ChangePassword")
			}
			if !perms.AllowPrint || perms.AllowModify {
				t.Errorf("permissions not preserved: %+v", perms)
			}
			txt, err := d3.ExtractText()
			if err != nil || len(txt) == 0 || txt[0] != "classified" {
				t.Errorf("content not preserved: %q (%v)", txt, err)
			}
		})
	}
}

// TestChangePasswordPlaintext returns an error on a document that is not
// encrypted (there is no password to change).
func TestChangePasswordPlaintext(t *testing.T) {
	doc := pdf.NewDocument(200, 200)
	if err := doc.ChangePassword("x", "y"); err == nil {
		t.Error("ChangePassword on a plaintext document = nil error, want an error")
	}
}
