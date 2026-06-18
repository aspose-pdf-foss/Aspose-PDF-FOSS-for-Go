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
	"os"
	"strings"
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

func TestSignVisibleRenders(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	cert := newSelfSigned(t, key)

	// A blank page: the only thing that can paint non-white is the visible
	// signature block, so a non-white pixel proves it rendered.
	doc := pdf.NewDocument(320, 200)
	if err := doc.Sign(pdf.SignOptions{
		Certificate: cert, PrivateKey: key, Name: "Test Signer", Reason: "Approved",
		Visible: true, Rect: pdf.Rectangle{LLX: 20, LLY: 20, URX: 300, URY: 120},
	}); err != nil {
		t.Fatalf("Sign: %v", err)
	}
	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatalf("WriteTo: %v", err)
	}
	out, err := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("OpenStream: %v", err)
	}
	// Signature still valid with a visible appearance.
	sigs, err := out.VerifySignatures()
	if err != nil || len(sigs) != 1 || !sigs[0].Valid {
		t.Fatalf("VerifySignatures after visible sign: %v / %+v", err, sigs)
	}
	// The appearance actually paints.
	p1, _ := out.Page(1)
	if !hasNonWhitePixel(t, p1) {
		t.Error("visible signature block rendered nothing")
	}
}

func TestSignInvisibleBlankStaysBlank(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	cert := newSelfSigned(t, key)

	doc := pdf.NewDocument(320, 200) // blank
	if err := doc.Sign(pdf.SignOptions{Certificate: cert, PrivateKey: key, Name: "X"}); err != nil {
		t.Fatalf("Sign: %v", err)
	}
	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatalf("WriteTo: %v", err)
	}
	out, _ := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	p1, _ := out.Page(1)
	if hasNonWhitePixel(t, p1) {
		t.Error("invisible signature painted something on a blank page")
	}
}

func TestSignPAdES(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	cert := newSelfSigned(t, key)
	doc := pdf.NewDocument(400, 200)
	if err := doc.Sign(pdf.SignOptions{
		Certificate: cert, PrivateKey: key, Name: "PAdES Signer", PAdES: true,
	}); err != nil {
		t.Fatalf("Sign: %v", err)
	}
	s := raw(t, doc)
	if !strings.Contains(s, "/ETSI.CAdES.detached") {
		t.Error("PAdES signature missing /SubFilter /ETSI.CAdES.detached")
	}
	if strings.Contains(s, "/adbe.pkcs7.detached") {
		t.Error("PAdES signature unexpectedly used /adbe.pkcs7.detached")
	}
	// The ESS attribute is inside the signed set, so verification still holds.
	out, err := pdf.OpenStream(bytes.NewReader([]byte(s)))
	if err != nil {
		t.Fatalf("OpenStream: %v", err)
	}
	sigs, err := out.VerifySignatures()
	if err != nil || len(sigs) != 1 || !sigs[0].Valid {
		t.Fatalf("VerifySignatures (PAdES): %v / %+v", err, sigs)
	}
}

func TestSignCertify(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	cert := newSelfSigned(t, key)
	doc := pdf.NewDocument(400, 200)
	if err := doc.Sign(pdf.SignOptions{
		Certificate: cert, PrivateKey: key, Name: "Author", Certify: pdf.CertifyFillForms,
	}); err != nil {
		t.Fatalf("Sign: %v", err)
	}
	s := raw(t, doc)
	for _, want := range []string{"/DocMDP", "/TransformParams", "/Perms"} {
		if !strings.Contains(s, want) {
			t.Errorf("certified signature missing %s", want)
		}
	}
	out, err := pdf.OpenStream(bytes.NewReader([]byte(s)))
	if err != nil {
		t.Fatalf("OpenStream: %v", err)
	}
	sigs, err := out.VerifySignatures()
	if err != nil || len(sigs) != 1 || !sigs[0].Valid {
		t.Fatalf("VerifySignatures (certified): %v / %+v", err, sigs)
	}
}

// TestSignWithTimestamp signs with a real RFC 3161 TSA and confirms the
// result still verifies. It is network-dependent, so it skips gracefully when
// the TSA is unreachable (e.g. offline CI). Override the TSA with TSA_URL.
func TestSignWithTimestamp(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping network TSA test in -short mode")
	}
	tsa := os.Getenv("TSA_URL")
	if tsa == "" {
		tsa = "http://timestamp.digicert.com"
	}
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	cert := newSelfSigned(t, key)
	doc := pdf.NewDocument(400, 200)
	if err := doc.Sign(pdf.SignOptions{
		Certificate: cert, PrivateKey: key, Name: "TSA Signer",
		PAdES: true, TimestampURL: tsa,
	}); err != nil {
		t.Skipf("TSA %s unreachable (%v) — skipping", tsa, err)
	}
	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatalf("WriteTo: %v", err)
	}
	out, err := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("OpenStream: %v", err)
	}
	sigs, err := out.VerifySignatures()
	if err != nil || len(sigs) != 1 || !sigs[0].Valid {
		t.Fatalf("VerifySignatures (timestamped): %v / %+v", err, sigs)
	}
}

func TestSignMultiple(t *testing.T) {
	k1, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	c1 := newSelfSigned(t, k1)
	k2, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	c2 := newSelfSigned(t, k2)

	// First signature — ordinary full-rewrite.
	doc := pdf.NewDocument(400, 300)
	if err := doc.Sign(pdf.SignOptions{Certificate: c1, PrivateKey: k1, Name: "Alice"}); err != nil {
		t.Fatalf("first Sign: %v", err)
	}
	var b1 bytes.Buffer
	if _, err := doc.WriteTo(&b1); err != nil {
		t.Fatalf("first WriteTo: %v", err)
	}
	first := b1.Bytes()

	// Second signature — auto-incremental, must preserve the first.
	doc2, err := pdf.OpenStream(bytes.NewReader(first))
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if err := doc2.Sign(pdf.SignOptions{Certificate: c2, PrivateKey: k2, Name: "Bob"}); err != nil {
		t.Fatalf("second Sign: %v", err)
	}
	var b2 bytes.Buffer
	if _, err := doc2.WriteTo(&b2); err != nil {
		t.Fatalf("second WriteTo: %v", err)
	}
	second := b2.Bytes()

	// The incremental update appends — the original bytes are a verbatim
	// prefix, which is exactly why the first signature stays valid.
	if len(second) <= len(first) || !bytes.Equal(second[:len(first)], first) {
		t.Fatal("incremental signature did not preserve the original bytes verbatim")
	}

	out, err := pdf.OpenStream(bytes.NewReader(second))
	if err != nil {
		t.Fatalf("reopen signed: %v", err)
	}
	sigs, err := out.VerifySignatures()
	if err != nil {
		t.Fatalf("VerifySignatures: %v", err)
	}
	if len(sigs) != 2 {
		t.Fatalf("got %d signatures, want 2", len(sigs))
	}
	for _, s := range sigs {
		if !s.Valid {
			t.Errorf("signature %s not valid: %v", s.FieldName, s.Err)
		}
	}
	if sigs[0].FieldName != "Signature1" || sigs[1].FieldName != "Signature2" {
		t.Errorf("field names = %q, %q; want Signature1, Signature2", sigs[0].FieldName, sigs[1].FieldName)
	}
	// The first signature covers only its own revision; the second covers all.
	if sigs[0].CoversWholeDocument {
		t.Error("first signature should not cover the whole (extended) document")
	}
	if !sigs[1].CoversWholeDocument {
		t.Error("second signature should cover the whole document")
	}
}

func TestSignIncrementalRequiresSource(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	cert := newSelfSigned(t, key)
	doc := pdf.NewDocument(200, 200) // built in memory, no source bytes
	if err := doc.Sign(pdf.SignOptions{Certificate: cert, PrivateKey: key, Incremental: true}); err == nil {
		t.Error("Incremental sign on a built document = nil error, want an error")
	}
}

func TestSignVisibleRequiresRect(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	cert := newSelfSigned(t, key)
	doc := pdf.NewDocument(320, 200)
	if err := doc.Sign(pdf.SignOptions{Certificate: cert, PrivateKey: key, Visible: true}); err == nil {
		t.Error("Visible sign with empty Rect = nil error, want an error")
	}
}
