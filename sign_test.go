// SPDX-License-Identifier: MIT

package asposepdf_test

import (
	"bytes"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"testing"
	"time"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

// newSelfSigned generates a throwaway self-signed certificate for the given
// key — entirely in memory, so no private key or certificate is ever stored
// in the repository (the standard, scanner-clean way to test signing).
func newSelfSigned(t *testing.T, key crypto.Signer) *x509.Certificate {
	t.Helper()
	tmpl := x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano() % 1_000_000_000),
		Subject:      pkix.Name{CommonName: "Test Signer", Organization: []string{"Aspose FOSS"}},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().AddDate(1, 0, 0),
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, key.Public(), key)
	if err != nil {
		t.Fatalf("create certificate: %v", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("parse certificate: %v", err)
	}
	return cert
}

// signOneDoc builds a one-page document and signs it with key, returning the
// signed PDF bytes.
func signOneDoc(t *testing.T, key crypto.Signer, cert *x509.Certificate) []byte {
	t.Helper()
	doc := pdf.NewDocument(400, 200)
	page, _ := doc.Page(1)
	if err := page.AddText("This document is digitally signed.",
		pdf.TextStyle{Font: pdf.FontHelvetica, Size: 14, Color: &pdf.Color{A: 1}},
		pdf.Rectangle{LLX: 30, LLY: 110, URX: 370, URY: 150}); err != nil {
		t.Fatalf("AddText: %v", err)
	}
	if err := doc.Sign(pdf.SignOptions{
		Certificate: cert, PrivateKey: key,
		Name: "Test Signer", Reason: "Regression test", Location: "CI",
	}); err != nil {
		t.Fatalf("Sign: %v", err)
	}
	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatalf("WriteTo: %v", err)
	}
	return buf.Bytes()
}

func signVerifyRoundTrip(t *testing.T, key crypto.Signer) {
	t.Helper()
	cert := newSelfSigned(t, key)
	signed := signOneDoc(t, key, cert)

	doc, err := pdf.OpenStream(bytes.NewReader(signed))
	if err != nil {
		t.Fatalf("OpenStream signed: %v", err)
	}
	sigs, err := doc.VerifySignatures()
	if err != nil {
		t.Fatalf("VerifySignatures: %v", err)
	}
	if len(sigs) != 1 {
		t.Fatalf("got %d signatures, want 1", len(sigs))
	}
	s := sigs[0]
	if !s.Valid {
		t.Errorf("signature not Valid: %v", s.Err)
	}
	if !s.IntegrityOK {
		t.Error("IntegrityOK = false")
	}
	if !s.CoversWholeDocument {
		t.Error("CoversWholeDocument = false")
	}
	if s.FieldName != "Signature1" {
		t.Errorf("FieldName = %q, want Signature1", s.FieldName)
	}
	if s.SignerName != "Test Signer" {
		t.Errorf("SignerName = %q, want Test Signer", s.SignerName)
	}
	if s.Reason != "Regression test" {
		t.Errorf("Reason = %q", s.Reason)
	}
	if s.Certificate == nil || s.Certificate.Subject.CommonName != "Test Signer" {
		t.Errorf("Certificate = %v", s.Certificate)
	}
}

func TestSignVerifyRSA(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	signVerifyRoundTrip(t, key)
}

func TestSignVerifyECDSA(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signVerifyRoundTrip(t, key)
}

func TestSignTamperDetected(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	cert := newSelfSigned(t, key)
	signed := signOneDoc(t, key, cert)

	// Flip a byte well inside the signed region (the document body).
	tampered := append([]byte(nil), signed...)
	tampered[120] ^= 0x40

	doc, err := pdf.OpenStream(bytes.NewReader(tampered))
	if err != nil {
		// A structural break from the edit also defeats the forgery — fine.
		return
	}
	sigs, err := doc.VerifySignatures()
	if err != nil {
		return
	}
	if len(sigs) == 1 && sigs[0].Valid {
		t.Error("tampered document verified as Valid — integrity check failed to catch it")
	}
}

func TestSignRequiresCertAndKey(t *testing.T) {
	doc := pdf.NewDocument(200, 200)
	if err := doc.Sign(pdf.SignOptions{}); err == nil {
		t.Error("Sign with no certificate/key = nil error, want an error")
	}
}

func TestVerifySignaturesNeedsSource(t *testing.T) {
	// A freshly built (never-opened) document has no source bytes.
	doc := pdf.NewDocument(200, 200)
	if _, err := doc.VerifySignatures(); err == nil {
		t.Error("VerifySignatures on a built document = nil error, want an error")
	}
}
