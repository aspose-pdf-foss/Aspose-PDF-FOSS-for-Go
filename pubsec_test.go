// SPDX-License-Identifier: MIT

package asposepdf_test

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"strings"
	"testing"
	"time"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

// newRecipientKey generates a throwaway RSA key plus a self-signed
// certificate for it, in memory — no key material ever touches the repo.
func newRecipientKey(t *testing.T, cn string) (*rsa.PrivateKey, *x509.Certificate) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	tmpl := x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano() % 1_000_000_000),
		Subject:      pkix.Name{CommonName: cn, Organization: []string{"Aspose FOSS"}},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().AddDate(1, 0, 0),
		KeyUsage:     x509.KeyUsageKeyEncipherment | x509.KeyUsageDataEncipherment,
	}
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, key.Public(), key)
	if err != nil {
		t.Fatalf("create certificate: %v", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("parse certificate: %v", err)
	}
	return key, cert
}

// encryptedForRecipients builds a one-page document with known text and
// encrypts it for the given recipients.
func encryptedForRecipients(t *testing.T, alg pdf.EncryptionAlgorithm, recips ...pdf.Recipient) []byte {
	t.Helper()
	doc := pdf.NewDocument(400, 200)
	p, _ := doc.Page(1)
	if err := p.AddText("Certificate secrets", pdf.TextStyle{Font: pdf.FontHelvetica, Size: 18},
		pdf.Rectangle{LLX: 20, LLY: 100, URX: 380, URY: 140}); err != nil {
		t.Fatal(err)
	}
	doc.SetEncryption(pdf.EncryptionOptions{Recipients: recips, Algorithm: alg})
	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatalf("write encrypted: %v", err)
	}
	return buf.Bytes()
}

// A certificate-encrypted document opens for its recipient and carries its
// content through, under both AES sizes.
func TestPubSecRoundTrip(t *testing.T) {
	for _, tc := range []struct {
		name string
		alg  pdf.EncryptionAlgorithm
	}{
		{"AES256", pdf.EncryptionAlgAES256},
		{"AES128", pdf.EncryptionAlgAES128},
	} {
		t.Run(tc.name, func(t *testing.T) {
			key, cert := newRecipientKey(t, "Alice")
			data := encryptedForRecipients(t, tc.alg, pdf.Recipient{Certificate: cert})

			// It really is encrypted: the plain text is not in the bytes, and
			// opening without credentials reports so.
			if bytes.Contains(data, []byte("Certificate secrets")) {
				t.Error("document content is not encrypted")
			}
			if _, err := pdf.OpenStream(bytes.NewReader(data)); err == nil {
				t.Error("OpenStream accepted an encrypted document")
			}

			doc, err := pdf.OpenStreamWithCertificate(bytes.NewReader(data), cert, key)
			if err != nil {
				t.Fatalf("open with certificate: %v", err)
			}
			page, err := doc.Page(1)
			if err != nil {
				t.Fatal(err)
			}
			txt, err := page.ExtractText()
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(txt, "Certificate secrets") {
				t.Errorf("decrypted text = %q", txt)
			}
		})
	}
}

// Every recipient of a multi-recipient document can open it; a stranger
// cannot.
func TestPubSecMultipleRecipients(t *testing.T) {
	aliceKey, aliceCert := newRecipientKey(t, "Alice")
	bobKey, bobCert := newRecipientKey(t, "Bob")
	eveKey, eveCert := newRecipientKey(t, "Eve")

	data := encryptedForRecipients(t, pdf.EncryptionAlgAES256,
		pdf.Recipient{Certificate: aliceCert},
		pdf.Recipient{Certificate: bobCert})

	for _, r := range []struct {
		name string
		cert *x509.Certificate
		key  *rsa.PrivateKey
	}{{"alice", aliceCert, aliceKey}, {"bob", bobCert, bobKey}} {
		doc, err := pdf.OpenStreamWithCertificate(bytes.NewReader(data), r.cert, r.key)
		if err != nil {
			t.Fatalf("%s could not open the document: %v", r.name, err)
		}
		page, _ := doc.Page(1)
		txt, _ := page.ExtractText()
		if !strings.Contains(txt, "Certificate secrets") {
			t.Errorf("%s decrypted garbage: %q", r.name, txt)
		}
	}

	if _, err := pdf.OpenStreamWithCertificate(bytes.NewReader(data), eveCert, eveKey); err == nil {
		t.Error("a non-recipient opened the document")
	}
}

// The wrong private key for a listed certificate fails rather than producing
// garbage.
func TestPubSecWrongKey(t *testing.T) {
	_, cert := newRecipientKey(t, "Alice")
	otherKey, _ := newRecipientKey(t, "Mallory")
	data := encryptedForRecipients(t, pdf.EncryptionAlgAES256, pdf.Recipient{Certificate: cert})

	if _, err := pdf.OpenStreamWithCertificate(bytes.NewReader(data), cert, otherKey); err == nil {
		t.Error("a mismatched private key unlocked the document")
	}
}

// Per-recipient permissions: each recipient's envelope carries its own,
// which the password handler cannot express.
func TestPubSecPerRecipientPermissions(t *testing.T) {
	aliceKey, aliceCert := newRecipientKey(t, "Alice")
	bobKey, bobCert := newRecipientKey(t, "Bob")

	readOnly := pdf.Permissions{AllowPrint: true}
	data := encryptedForRecipients(t, pdf.EncryptionAlgAES256,
		pdf.Recipient{Certificate: aliceCert},
		pdf.Recipient{Certificate: bobCert, Permissions: &readOnly})

	alice, err := pdf.OpenStreamWithCertificate(bytes.NewReader(data), aliceCert, aliceKey)
	if err != nil {
		t.Fatal(err)
	}
	bob, err := pdf.OpenStreamWithCertificate(bytes.NewReader(data), bobCert, bobKey)
	if err != nil {
		t.Fatal(err)
	}
	ap, aok := alice.Permissions()
	bp, bok := bob.Permissions()
	if !aok || !bok {
		t.Fatal("Permissions() does not report a certificate-encrypted document as encrypted")
	}
	if !ap.AllowCopy {
		t.Error("the unrestricted recipient lost permissions")
	}
	if bp.AllowCopy || !bp.AllowPrint {
		t.Errorf("restricted recipient permissions = %+v; want print-only", bp)
	}
}

// Editing in place: a certificate-encrypted document re-saves for the same
// recipients, so they can still open the edited file.
func TestPubSecResaveKeepsRecipients(t *testing.T) {
	aliceKey, aliceCert := newRecipientKey(t, "Alice")
	bobKey, bobCert := newRecipientKey(t, "Bob")
	data := encryptedForRecipients(t, pdf.EncryptionAlgAES256,
		pdf.Recipient{Certificate: aliceCert},
		pdf.Recipient{Certificate: bobCert})

	doc, err := pdf.OpenStreamWithCertificate(bytes.NewReader(data), aliceCert, aliceKey)
	if err != nil {
		t.Fatal(err)
	}
	page, _ := doc.Page(1)
	if err := page.AddText("edited", pdf.TextStyle{Font: pdf.FontHelvetica, Size: 12},
		pdf.Rectangle{LLX: 20, LLY: 40, URX: 200, URY: 60}); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if _, err := doc.WriteTo(&out); err != nil {
		t.Fatalf("re-save: %v", err)
	}
	if bytes.Contains(out.Bytes(), []byte("edited")) {
		t.Error("the re-saved document is not encrypted")
	}

	// Both original recipients still open it, and the edit is there.
	for _, r := range []struct {
		name string
		cert *x509.Certificate
		key  *rsa.PrivateKey
	}{{"alice", aliceCert, aliceKey}, {"bob", bobCert, bobKey}} {
		re, err := pdf.OpenStreamWithCertificate(bytes.NewReader(out.Bytes()), r.cert, r.key)
		if err != nil {
			t.Fatalf("%s cannot open the re-saved document: %v", r.name, err)
		}
		p2, _ := re.Page(1)
		txt, _ := p2.ExtractText()
		if !strings.Contains(txt, "edited") || !strings.Contains(txt, "Certificate secrets") {
			t.Errorf("%s read back %q", r.name, txt)
		}
	}
}

// Guard rails: no recipients means no public-key handler, and RC4 is refused.
func TestPubSecErrors(t *testing.T) {
	_, cert := newRecipientKey(t, "Alice")
	doc := pdf.NewDocument(200, 100)
	doc.SetEncryption(pdf.EncryptionOptions{
		Recipients: []pdf.Recipient{{Certificate: cert}},
		Algorithm:  pdf.EncryptionAlgRC4_128,
	})
	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err == nil {
		t.Error("RC4 certificate encryption was accepted")
	}

	// A password-protected document tells the caller to use the password API.
	plain := pdf.NewDocument(200, 100)
	plain.SetEncryption(pdf.EncryptionOptions{UserPassword: "pw"})
	buf.Reset()
	if _, err := plain.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	_, key2 := newRecipientKey(t, "Alice")
	_ = key2
	if _, err := pdf.OpenStreamWithCertificate(bytes.NewReader(buf.Bytes()), cert, nil); err == nil {
		t.Error("a password-protected document opened with a certificate")
	}
}

// With no Algorithm, certificate encryption uses AES-128, like a password.
func TestPubSecDefaultAlgorithm(t *testing.T) {
	_, cert := newRecipientKey(t, "Alice")
	doc := pdf.NewDocument(200, 100)
	doc.SetEncryption(pdf.EncryptionOptions{Recipients: []pdf.Recipient{{Certificate: cert}}})
	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(buf.Bytes(), []byte("/AESV2")) || bytes.Contains(buf.Bytes(), []byte("/AESV3")) {
		t.Error("default certificate encryption is not AES-128 (/AESV2)")
	}
}
