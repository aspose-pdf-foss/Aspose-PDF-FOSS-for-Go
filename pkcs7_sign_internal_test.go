// SPDX-License-Identifier: MIT

package asposepdf

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"math/big"
	"testing"
	"time"
)

// pkcs7TestSigner returns a throwaway key and its self-signed certificate,
// generated in memory so nothing secret is ever written to the repository.
func pkcs7TestSigner(t *testing.T, ec bool) (crypto.Signer, *x509.Certificate) {
	t.Helper()
	var key crypto.Signer
	var err error
	if ec {
		key, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	} else {
		key, err = rsa.GenerateKey(rand.Reader, 2048)
	}
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	tmpl := x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano() % 1_000_000_000),
		Subject:      pkix.Name{CommonName: "Digest Test"},
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
	return key, cert
}

// signerInfoOf parses a detached CMS and returns its single SignerInfo, so a
// test can check which algorithms were actually written into it.
func signerInfoOf(t *testing.T, der []byte) parsedSignerInfo {
	t.Helper()
	var ci struct {
		ContentType asn1.ObjectIdentifier
		Content     asn1.RawValue `asn1:"explicit,tag:0"`
	}
	if _, err := asn1.Unmarshal(der, &ci); err != nil {
		t.Fatalf("parse ContentInfo: %v", err)
	}
	var sd parsedSignedData
	if _, err := asn1.Unmarshal(ci.Content.Bytes, &sd); err != nil {
		t.Fatalf("parse SignedData: %v", err)
	}
	if len(sd.SignerInfos) != 1 {
		t.Fatalf("got %d SignerInfos, want 1", len(sd.SignerInfos))
	}
	return sd.SignerInfos[0]
}

// Every digest the public API offers must reach the CMS: the SignerInfo has
// to name the digest and the matching signature algorithm, and our own
// verifier has to accept the result — including the SHA-3 family, which is
// what pdf-go-blch asked for.
func TestBuildPKCS7DigestAlgorithms(t *testing.T) {
	content := []byte("the bytes a signature covers")

	cases := []struct {
		digest    DigestAlgorithm
		name      string
		digestOID asn1.ObjectIdentifier
		rsaSigOID asn1.ObjectIdentifier
		ecSigOID  asn1.ObjectIdentifier
	}{
		{DigestSHA256, "SHA-256",
			asn1.ObjectIdentifier{2, 16, 840, 1, 101, 3, 4, 2, 1},
			asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 1, 1},
			asn1.ObjectIdentifier{1, 2, 840, 10045, 4, 3, 2}},
		{DigestSHA384, "SHA-384",
			asn1.ObjectIdentifier{2, 16, 840, 1, 101, 3, 4, 2, 2},
			asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 1, 1},
			asn1.ObjectIdentifier{1, 2, 840, 10045, 4, 3, 3}},
		{DigestSHA512, "SHA-512",
			asn1.ObjectIdentifier{2, 16, 840, 1, 101, 3, 4, 2, 3},
			asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 1, 1},
			asn1.ObjectIdentifier{1, 2, 840, 10045, 4, 3, 4}},
		{DigestSHA3_256, "SHA3-256",
			asn1.ObjectIdentifier{2, 16, 840, 1, 101, 3, 4, 2, 8},
			asn1.ObjectIdentifier{2, 16, 840, 1, 101, 3, 4, 3, 14},
			asn1.ObjectIdentifier{2, 16, 840, 1, 101, 3, 4, 3, 10}},
		{DigestSHA3_384, "SHA3-384",
			asn1.ObjectIdentifier{2, 16, 840, 1, 101, 3, 4, 2, 9},
			asn1.ObjectIdentifier{2, 16, 840, 1, 101, 3, 4, 3, 15},
			asn1.ObjectIdentifier{2, 16, 840, 1, 101, 3, 4, 3, 11}},
		{DigestSHA3_512, "SHA3-512",
			asn1.ObjectIdentifier{2, 16, 840, 1, 101, 3, 4, 2, 10},
			asn1.ObjectIdentifier{2, 16, 840, 1, 101, 3, 4, 3, 16},
			asn1.ObjectIdentifier{2, 16, 840, 1, 101, 3, 4, 3, 12}},
	}

	for _, kind := range []struct {
		name string
		ec   bool
	}{{"RSA", false}, {"ECDSA", true}} {
		key, cert := pkcs7TestSigner(t, kind.ec)
		for _, c := range cases {
			t.Run(kind.name+"/"+c.name, func(t *testing.T) {
				der, err := buildPKCS7Detached(content, cert, key, nil, time.Now(), false, "", c.digest)
				if err != nil {
					t.Fatalf("buildPKCS7Detached: %v", err)
				}
				si := signerInfoOf(t, der)
				if !si.DigestAlgorithm.Algorithm.Equal(c.digestOID) {
					t.Errorf("digest OID = %v, want %v", si.DigestAlgorithm.Algorithm, c.digestOID)
				}
				wantSig := c.rsaSigOID
				if kind.ec {
					wantSig = c.ecSigOID
				}
				if !si.SignatureAlgorithm.Algorithm.Equal(wantSig) {
					t.Errorf("signature OID = %v, want %v", si.SignatureAlgorithm.Algorithm, wantSig)
				}
				if _, _, err := verifyPKCS7Detached(der, content); err != nil {
					t.Errorf("verifyPKCS7Detached: %v", err)
				}
				if _, _, err := verifyPKCS7Detached(der, []byte("tampered")); err == nil {
					t.Error("verification accepted tampered content")
				}
			})
		}
	}
}

// An unknown enum value must be refused rather than silently signing with
// whatever crypto.Hash(0) happens to mean.
func TestDigestAlgorithmRejectsUnknown(t *testing.T) {
	key, cert := pkcs7TestSigner(t, false)
	if _, err := buildPKCS7Detached([]byte("x"), cert, key, nil, time.Now(), false, "", DigestAlgorithm(99)); err == nil {
		t.Fatal("an unknown digest algorithm was accepted")
	}
}
