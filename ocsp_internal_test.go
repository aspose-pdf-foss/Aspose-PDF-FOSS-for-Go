// SPDX-License-Identifier: MIT

package asposepdf

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"math/big"
	"testing"
	"time"
)

// ocspTestCA returns a CA certificate and its key.
func ocspTestCA(t *testing.T) (*x509.Certificate, *rsa.PrivateKey) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "Revocation Test CA"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().AddDate(1, 0, 0),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, key.Public(), key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return cert, key
}

// ocspTestLeaf issues an end-entity certificate from the CA, optionally naming
// an OCSP responder and a CRL distribution point.
func ocspTestLeaf(t *testing.T, ca *x509.Certificate, caKey *rsa.PrivateKey, serial int64, ocspURL, crlURL string) *x509.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := x509.Certificate{
		SerialNumber: big.NewInt(serial),
		Subject:      pkix.Name{CommonName: "Revocation Test Leaf"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().AddDate(1, 0, 0),
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}
	if ocspURL != "" {
		tmpl.OCSPServer = []string{ocspURL}
	}
	if crlURL != "" {
		tmpl.CRLDistributionPoints = []string{crlURL}
	}
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, ca, key.Public(), caKey)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return cert
}

// ocspDelegatedResponder issues a responder certificate from the CA, with or
// without the extended key usage that makes it legitimate.
func ocspDelegatedResponder(t *testing.T, ca *x509.Certificate, caKey *rsa.PrivateKey, withEKU bool) (*x509.Certificate, *rsa.PrivateKey) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := x509.Certificate{
		SerialNumber: big.NewInt(9001),
		Subject:      pkix.Name{CommonName: "Delegated Responder"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().AddDate(1, 0, 0),
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}
	if withEKU {
		tmpl.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageOCSPSigning}
	}
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, ca, key.Public(), caKey)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return cert, key
}

// ocspTestResponse builds a signed OCSPResponse the way a responder would:
// the CertID names idIssuer (which must have issued leaf), while signer/
// signerKey sign the response and signer's certificate travels in it. status
// is "good", "revoked" or "unknown".
func ocspTestResponse(t *testing.T, idIssuer, signer *x509.Certificate, signerKey *rsa.PrivateKey, leaf *x509.Certificate,
	status string, revokedAt time.Time, reason int, nextUpdate time.Time) []byte {
	t.Helper()
	id, err := buildOCSPCertID(leaf, idIssuer)
	if err != nil {
		t.Fatal(err)
	}

	var certStatus asn1.RawValue
	switch status {
	case "good":
		certStatus = asn1.RawValue{Class: asn1.ClassContextSpecific, Tag: 0, Bytes: []byte{}}
	case "revoked":
		body, err := asn1.Marshal(ocspRevokedInfo{
			RevocationTime:   revokedAt.UTC(),
			RevocationReason: asn1.Enumerated(reason),
		})
		if err != nil {
			t.Fatal(err)
		}
		var seq asn1.RawValue
		if _, err := asn1.Unmarshal(body, &seq); err != nil {
			t.Fatal(err)
		}
		certStatus = asn1.RawValue{Class: asn1.ClassContextSpecific, Tag: 1, IsCompound: true, Bytes: seq.Bytes}
	default:
		certStatus = asn1.RawValue{Class: asn1.ClassContextSpecific, Tag: 2, Bytes: []byte{}}
	}

	single := ocspSingleResponse{
		CertID:     id,
		Status:     certStatus,
		ThisUpdate: time.Now().Add(-time.Minute).UTC(),
	}
	if !nextUpdate.IsZero() {
		single.NextUpdate = nextUpdate.UTC()
	}

	responderID, err := asn1.Marshal(asn1.RawValue{
		Class: asn1.ClassContextSpecific, Tag: 1, IsCompound: true, Bytes: signer.RawSubject,
	})
	if err != nil {
		t.Fatal(err)
	}
	tbs, err := asn1.Marshal(ocspResponseData{
		RawResponderID: asn1.RawValue{FullBytes: responderID},
		ProducedAt:     time.Now().UTC(),
		Responses:      []ocspSingleResponse{single},
	})
	if err != nil {
		t.Fatal(err)
	}
	h := crypto.SHA256.New()
	h.Write(tbs)
	sig, err := signerKey.Sign(rand.Reader, h.Sum(nil), crypto.SHA256)
	if err != nil {
		t.Fatal(err)
	}
	basicDER, err := asn1.Marshal(ocspBasicResponse{
		TBSResponseData: asn1.RawValue{FullBytes: tbs},
		SignatureAlgorithm: pkix.AlgorithmIdentifier{
			Algorithm:  asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 1, 11},
			Parameters: asn1NULL(),
		},
		Signature:    asn1.BitString{Bytes: sig, BitLength: len(sig) * 8},
		Certificates: []asn1.RawValue{{FullBytes: signer.Raw}},
	})
	if err != nil {
		t.Fatal(err)
	}
	der, err := asn1.Marshal(ocspResponseOuter{
		Status:        0,
		ResponseBytes: ocspResponseBytes{ResponseType: oidOCSPBasic, Response: basicDER},
	})
	if err != nil {
		t.Fatal(err)
	}
	return der
}

// The CertID identifies the certificate by hashes of its issuer, not by the
// issuer certificate itself — get those wrong and every responder answers
// "unknown" for a certificate it knows perfectly well.
func TestBuildOCSPCertID(t *testing.T) {
	ca, caKey := ocspTestCA(t)
	leaf := ocspTestLeaf(t, ca, caKey, 42, "", "")

	id, err := buildOCSPCertID(leaf, ca)
	if err != nil {
		t.Fatal(err)
	}
	wantName := sha1.Sum(ca.RawSubject)
	if string(id.IssuerNameHash) != string(wantName[:]) {
		t.Error("IssuerNameHash is not SHA-1 of the issuer's raw subject")
	}
	var spki struct {
		Algorithm pkix.AlgorithmIdentifier
		PublicKey asn1.BitString
	}
	if _, err := asn1.Unmarshal(ca.RawSubjectPublicKeyInfo, &spki); err != nil {
		t.Fatal(err)
	}
	wantKey := sha1.Sum(spki.PublicKey.RightAlign())
	if string(id.IssuerKeyHash) != string(wantKey[:]) {
		t.Error("IssuerKeyHash is not SHA-1 of the issuer's public key bits")
	}
	if id.SerialNumber.Cmp(leaf.SerialNumber) != 0 {
		t.Errorf("SerialNumber = %v, want %v", id.SerialNumber, leaf.SerialNumber)
	}
	if !id.HashAlgorithm.Algorithm.Equal(oidSHA1) {
		t.Errorf("HashAlgorithm = %v, want SHA-1", id.HashAlgorithm.Algorithm)
	}
}

func TestBuildOCSPRequest(t *testing.T) {
	ca, caKey := ocspTestCA(t)
	leaf := ocspTestLeaf(t, ca, caKey, 7, "", "")

	der, err := buildOCSPRequest(leaf, ca)
	if err != nil {
		t.Fatal(err)
	}
	var req ocspRequest
	rest, err := asn1.Unmarshal(der, &req)
	if err != nil {
		t.Fatalf("the request does not parse as an OCSPRequest: %v", err)
	}
	if len(rest) != 0 {
		t.Errorf("%d trailing bytes after the request", len(rest))
	}
	if n := len(req.TBSRequest.RequestList); n != 1 {
		t.Fatalf("request carries %d entries, want 1", n)
	}
	if req.TBSRequest.RequestList[0].ReqCert.SerialNumber.Cmp(leaf.SerialNumber) != 0 {
		t.Error("the request asks about the wrong serial")
	}
}

func TestBuildOCSPRequestRejectsWrongIssuer(t *testing.T) {
	ca, caKey := ocspTestCA(t)
	other, _ := ocspTestCA(t)
	leaf := ocspTestLeaf(t, ca, caKey, 9, "", "")

	if _, err := buildOCSPRequest(leaf, other); err == nil {
		t.Fatal("a request was built against an issuer that did not sign the certificate")
	}
}

func TestParseOCSPResponseGood(t *testing.T) {
	ca, caKey := ocspTestCA(t)
	leaf := ocspTestLeaf(t, ca, caKey, 11, "", "")
	der := ocspTestResponse(t, ca, ca, caKey, leaf, "good", time.Time{}, 0, time.Now().Add(time.Hour))

	_, data, err := parseOCSPResponse(der)
	if err != nil {
		t.Fatal(err)
	}
	id, err := buildOCSPCertID(leaf, ca)
	if err != nil {
		t.Fatal(err)
	}
	single, err := ocspSingleFor(data, id)
	if err != nil {
		t.Fatal(err)
	}
	state, _, _, err := ocspStatusOf(single)
	if err != nil {
		t.Fatal(err)
	}
	if state != RevocationGood {
		t.Errorf("state = %v, want good", state)
	}
}

func TestParseOCSPResponseRevoked(t *testing.T) {
	ca, caKey := ocspTestCA(t)
	leaf := ocspTestLeaf(t, ca, caKey, 12, "", "")
	when := time.Now().Add(-48 * time.Hour).Truncate(time.Second)
	der := ocspTestResponse(t, ca, ca, caKey, leaf, "revoked", when, 1 /* keyCompromise */, time.Now().Add(time.Hour))

	_, data, err := parseOCSPResponse(der)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := buildOCSPCertID(leaf, ca)
	single, err := ocspSingleFor(data, id)
	if err != nil {
		t.Fatal(err)
	}
	state, revokedAt, reason, err := ocspStatusOf(single)
	if err != nil {
		t.Fatal(err)
	}
	if state != RevocationRevoked {
		t.Fatalf("state = %v, want revoked", state)
	}
	if !revokedAt.Equal(when.UTC()) {
		t.Errorf("revokedAt = %v, want %v", revokedAt, when.UTC())
	}
	if reason != 1 {
		t.Errorf("reason = %d, want 1 (keyCompromise)", reason)
	}
}

// A response about a different certificate answers nothing about ours.
func TestOCSPSingleForWrongCertificate(t *testing.T) {
	ca, caKey := ocspTestCA(t)
	asked := ocspTestLeaf(t, ca, caKey, 13, "", "")
	other := ocspTestLeaf(t, ca, caKey, 14, "", "")
	der := ocspTestResponse(t, ca, ca, caKey, other, "good", time.Time{}, 0, time.Now().Add(time.Hour))

	_, data, err := parseOCSPResponse(der)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := buildOCSPCertID(asked, ca)
	if _, err := ocspSingleFor(data, id); err == nil {
		t.Fatal("a response about another certificate was accepted as ours")
	}
}

// A stale answer is not an answer.
func TestOCSPStatusExpired(t *testing.T) {
	ca, caKey := ocspTestCA(t)
	leaf := ocspTestLeaf(t, ca, caKey, 15, "", "")
	der := ocspTestResponse(t, ca, ca, caKey, leaf, "good", time.Time{}, 0, time.Now().Add(-time.Hour))

	_, data, err := parseOCSPResponse(der)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := buildOCSPCertID(leaf, ca)
	single, err := ocspSingleFor(data, id)
	if err != nil {
		t.Fatal(err)
	}
	state, _, _, err := ocspStatusOf(single)
	if err == nil {
		t.Fatal("an expired response was accepted")
	}
	if state != RevocationUnknown {
		t.Errorf("state = %v, want unknown", state)
	}
}

func TestParseOCSPResponseErrorStatus(t *testing.T) {
	der, err := asn1.Marshal(ocspResponseOuter{Status: 6 /* unauthorized */})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := parseOCSPResponse(der); err == nil {
		t.Fatal("an unauthorized response was parsed as successful")
	}
}

func TestVerifyOCSPResponseFromIssuer(t *testing.T) {
	ca, caKey := ocspTestCA(t)
	leaf := ocspTestLeaf(t, ca, caKey, 21, "", "")
	der := ocspTestResponse(t, ca, ca, caKey, leaf, "good", time.Time{}, 0, time.Now().Add(time.Hour))

	basic, _, err := parseOCSPResponse(der)
	if err != nil {
		t.Fatal(err)
	}
	if err := verifyOCSPResponse(basic, ca); err != nil {
		t.Errorf("a response signed by the issuer was rejected: %v", err)
	}
}

// A response signed by somebody else is worthless, however well-formed.
func TestVerifyOCSPResponseWrongSigner(t *testing.T) {
	ca, caKey := ocspTestCA(t)
	impostor, impostorKey := ocspTestCA(t)
	leaf := ocspTestLeaf(t, ca, caKey, 22, "", "")
	der := ocspTestResponse(t, ca, impostor, impostorKey, leaf, "good", time.Time{}, 0, time.Now().Add(time.Hour))

	basic, _, err := parseOCSPResponse(der)
	if err != nil {
		t.Fatal(err)
	}
	if err := verifyOCSPResponse(basic, ca); err == nil {
		t.Fatal("a response signed by another key was accepted")
	}
}

func TestVerifyOCSPResponseTampered(t *testing.T) {
	ca, caKey := ocspTestCA(t)
	leaf := ocspTestLeaf(t, ca, caKey, 23, "", "")
	der := ocspTestResponse(t, ca, ca, caKey, leaf, "good", time.Time{}, 0, time.Now().Add(time.Hour))

	basic, _, err := parseOCSPResponse(der)
	if err != nil {
		t.Fatal(err)
	}
	basic.Signature.Bytes[10] ^= 0xFF
	if err := verifyOCSPResponse(basic, ca); err == nil {
		t.Fatal("a tampered signature was accepted")
	}
}

// A delegated responder counts only when the issuer gave it the OCSPSigning
// usage; without it, anyone the CA ever issued a certificate to could answer.
func TestVerifyOCSPResponseDelegatedResponder(t *testing.T) {
	ca, caKey := ocspTestCA(t)
	leaf := ocspTestLeaf(t, ca, caKey, 24, "", "")

	for _, tc := range []struct {
		name    string
		withEKU bool
		wantOK  bool
	}{
		{"with OCSPSigning", true, true},
		{"without OCSPSigning", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			responder, responderKey := ocspDelegatedResponder(t, ca, caKey, tc.withEKU)
			der := ocspTestResponse(t, ca, responder, responderKey, leaf, "good", time.Time{}, 0, time.Now().Add(time.Hour))
			basic, _, err := parseOCSPResponse(der)
			if err != nil {
				t.Fatal(err)
			}
			err = verifyOCSPResponse(basic, ca)
			if tc.wantOK && err != nil {
				t.Errorf("a delegated responder with the EKU was rejected: %v", err)
			}
			if !tc.wantOK && err == nil {
				t.Error("a responder without the OCSPSigning EKU was accepted")
			}
		})
	}
}
