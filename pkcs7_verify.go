// SPDX-License-Identifier: MIT

package asposepdf

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"fmt"
	"math/big"
)

// CMS / PKCS#7 SignedData verification — the reverse of pkcs7_sign.go.
// Parses the detached SignedData, recovers the signer certificate, checks
// the messageDigest signed attribute against the supplied content, and
// verifies the signature over the (re-tagged) signed attributes.

// parsedSignedData mirrors the SignedData SEQUENCE for unmarshaling. The
// IMPLICIT-tagged fields are captured as raw values and descended into by
// hand, which is more robust than relying on asn1 struct tags for the
// IMPLICIT SET OF forms.
type parsedSignedData struct {
	Version          int
	DigestAlgorithms asn1.RawValue
	ContentInfo      asn1.RawValue
	Certificates     asn1.RawValue      `asn1:"optional,tag:0"`
	SignerInfos      []parsedSignerInfo `asn1:"set"`
}

type parsedSignerInfo struct {
	Version            int
	SID                parsedIssuerSerial
	DigestAlgorithm    pkix.AlgorithmIdentifier
	SignedAttrs        asn1.RawValue `asn1:"optional,tag:0"`
	SignatureAlgorithm pkix.AlgorithmIdentifier
	Signature          []byte
}

type parsedIssuerSerial struct {
	Issuer       asn1.RawValue
	SerialNumber *big.Int
}

// verifyPKCS7Detached parses the CMS SignedData in der, verifies it
// against content (the detached signed bytes), and returns the signer
// certificate (plus any other embedded certs). An error means the
// signature is cryptographically invalid or the content was tampered.
func verifyPKCS7Detached(der, content []byte) (signer *x509.Certificate, certs []*x509.Certificate, err error) {
	var ci struct {
		ContentType asn1.ObjectIdentifier
		Content     asn1.RawValue `asn1:"explicit,tag:0"`
	}
	if _, err = asn1.Unmarshal(der, &ci); err != nil {
		return nil, nil, fmt.Errorf("pkcs7 verify: parse ContentInfo: %w", err)
	}
	if !ci.ContentType.Equal(oidSignedData) {
		return nil, nil, fmt.Errorf("pkcs7 verify: not signedData (%v)", ci.ContentType)
	}
	var sd parsedSignedData
	if _, err = asn1.Unmarshal(ci.Content.Bytes, &sd); err != nil {
		return nil, nil, fmt.Errorf("pkcs7 verify: parse SignedData: %w", err)
	}
	if len(sd.SignerInfos) != 1 {
		return nil, nil, fmt.Errorf("pkcs7 verify: expected 1 SignerInfo, got %d", len(sd.SignerInfos))
	}
	si := sd.SignerInfos[0]

	certs, err = x509.ParseCertificates(sd.Certificates.Bytes)
	if err != nil || len(certs) == 0 {
		return nil, nil, fmt.Errorf("pkcs7 verify: parse certificates: %w", err)
	}
	signer = findSigner(certs, si.SID)
	if signer == nil {
		return nil, nil, fmt.Errorf("pkcs7 verify: signer certificate not found among embedded certs")
	}

	// The messageDigest signed attribute must equal SHA-256 of the content.
	mdAttr, err := signedAttrDigest(si.SignedAttrs.Bytes)
	if err != nil {
		return nil, nil, err
	}
	sum := sha256.Sum256(content)
	if !bytesEqualConst(mdAttr, sum[:]) {
		return nil, nil, fmt.Errorf("pkcs7 verify: messageDigest mismatch (content tampered)")
	}

	// Verify the signature over the signed attributes, re-encoded as a
	// universal SET OF (the form whose digest is signed, RFC 5652 §5.4).
	setOf, err := asn1.Marshal(asn1.RawValue{Class: asn1.ClassUniversal, Tag: asn1.TagSet, IsCompound: true, Bytes: si.SignedAttrs.Bytes})
	if err != nil {
		return nil, nil, err
	}
	digest := sha256.Sum256(setOf)
	if err = verifySignature(signer, digest[:], si.Signature); err != nil {
		return nil, nil, err
	}
	return signer, certs, nil
}

// findSigner returns the certificate matching the signer's issuer+serial.
func findSigner(certs []*x509.Certificate, sid parsedIssuerSerial) *x509.Certificate {
	for _, c := range certs {
		if sid.SerialNumber != nil && c.SerialNumber.Cmp(sid.SerialNumber) == 0 {
			return c
		}
	}
	if len(certs) == 1 {
		return certs[0]
	}
	return nil
}

// signedAttrDigest extracts the messageDigest attribute value from the
// content bytes of the signed-attributes SET.
func signedAttrDigest(setContent []byte) ([]byte, error) {
	rest := setContent
	for len(rest) > 0 {
		var a attribute
		var err error
		rest, err = asn1.Unmarshal(rest, &a)
		if err != nil {
			return nil, fmt.Errorf("pkcs7 verify: parse signed attribute: %w", err)
		}
		if a.Type.Equal(oidAttrMessageDigest) && len(a.Values) == 1 {
			return a.Values[0].Bytes, nil
		}
	}
	return nil, fmt.Errorf("pkcs7 verify: messageDigest attribute not found")
}

// verifySignature checks sig against digest using the certificate's public
// key (RSA PKCS#1 v1.5 or ECDSA).
func verifySignature(cert *x509.Certificate, digest, sig []byte) error {
	switch pub := cert.PublicKey.(type) {
	case *rsa.PublicKey:
		if err := rsa.VerifyPKCS1v15(pub, crypto.SHA256, digest, sig); err != nil {
			return fmt.Errorf("pkcs7 verify: RSA signature invalid: %w", err)
		}
	case *ecdsa.PublicKey:
		if !ecdsa.VerifyASN1(pub, digest, sig) {
			return fmt.Errorf("pkcs7 verify: ECDSA signature invalid")
		}
	default:
		return fmt.Errorf("pkcs7 verify: unsupported public key type %T", pub)
	}
	return nil
}

// bytesEqualConst is a constant-time byte comparison.
func bytesEqualConst(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	var v byte
	for i := range a {
		v |= a[i] ^ b[i]
	}
	return v == 0
}
