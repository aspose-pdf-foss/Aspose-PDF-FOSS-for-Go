// SPDX-License-Identifier: MIT

package asposepdf_test

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"fmt"
	"testing"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

// encryptedDoc builds a one-page document with an /Info title, encrypted with
// alg, and returns its bytes.
func encryptedDoc(t *testing.T, alg pdf.EncryptionAlgorithm) []byte {
	t.Helper()
	doc := pdf.NewDocument(300, 300)
	page, err := doc.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	if err := page.AddText("hello", pdf.TextStyle{Font: pdf.FontHelvetica, Size: 12},
		pdf.Rectangle{LLX: 10, LLY: 10, URX: 200, URY: 100}); err != nil {
		t.Fatal(err)
	}
	doc.SetInfo(pdf.DocumentInfo{Title: "Original title"})
	doc.SetEncryption(pdf.EncryptionOptions{UserPassword: "u", OwnerPassword: "o", Algorithm: alg})
	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// A document opened from an encrypted file and saved again must keep the
// file's security handler: same algorithm, same key length, same header, and
// it must open again with the same password. RC4-40 used to come back
// labelled RC4-128 (so the password stopped working) and AES-256 lost its
// PDF 2.0 header.
func TestEncryptedResaveKeepsHandler(t *testing.T) {
	for _, tc := range []struct {
		alg    pdf.EncryptionAlgorithm
		header string
	}{
		{pdf.EncryptionAlgRC4_40, "%PDF-1.4"},
		{pdf.EncryptionAlgRC4_128, "%PDF-1.4"},
		{pdf.EncryptionAlgAES128, "%PDF-1.4"},
		{pdf.EncryptionAlgAES256, "%PDF-2.0"},
	} {
		t.Run(fmt.Sprint(tc.alg), func(t *testing.T) {
			first := encryptedDoc(t, tc.alg)
			doc, err := pdf.OpenStreamWithPassword(bytes.NewReader(first), "u")
			if err != nil {
				t.Fatalf("open: %v", err)
			}
			var buf bytes.Buffer
			if _, err := doc.WriteTo(&buf); err != nil {
				t.Fatal(err)
			}
			second := buf.Bytes()
			if !bytes.HasPrefix(second, []byte(tc.header)) {
				t.Errorf("re-saved header = %q, want %q", second[:8], tc.header)
			}
			reopened, err := pdf.OpenStreamWithPassword(bytes.NewReader(second), "u")
			if err != nil {
				t.Fatalf("the re-saved file does not open with its password: %v", err)
			}
			texts, err := reopened.ExtractText()
			if err != nil {
				t.Fatal(err)
			}
			if len(texts) != 1 || texts[0] != "hello" {
				t.Errorf("text after re-save = %q, want hello", texts)
			}
			if _, err := pdf.OpenStreamWithPassword(bytes.NewReader(second), "wrong"); err == nil {
				t.Error("the re-saved file opens with a wrong password")
			}
		})
	}
}

// An incremental revision (a signature appended to an existing file) must
// name the document information dictionary again in its trailer: readers
// consult only the newest trailer, so without it the title vanished.
func TestIncrementalSignatureKeepsInfo(t *testing.T) {
	src := encryptedDoc(t, pdf.EncryptionAlgAES128)
	doc, err := pdf.OpenStreamWithPassword(bytes.NewReader(src), "u")
	if err != nil {
		t.Fatal(err)
	}
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	if err := doc.Sign(pdf.SignOptions{Certificate: newSelfSigned(t, key), PrivateKey: key}); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(buf.Bytes(), src) {
		t.Fatal("the signature was not appended as an incremental revision")
	}
	signed, err := pdf.OpenStreamWithPassword(bytes.NewReader(buf.Bytes()), "u")
	if err != nil {
		t.Fatal(err)
	}
	info, err := signed.Info()
	if err != nil {
		t.Fatal(err)
	}
	if info.Title != "Original title" {
		t.Errorf("title after an incremental signature = %q, want %q", info.Title, "Original title")
	}
	sigs, err := signed.VerifySignatures()
	if err != nil || len(sigs) != 1 || !sigs[0].Valid {
		t.Fatalf("signature does not hold: %v %+v", err, sigs)
	}
}
