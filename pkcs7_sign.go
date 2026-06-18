// SPDX-License-Identifier: MIT

package asposepdf

import (
	"crypto"
	"crypto/rand"
	"crypto/x509"
	"encoding/asn1"
	"fmt"
	"math/big"
	"time"
)

// CMS / PKCS#7 SignedData construction for PDF digital signatures
// (ISO 32000-1 §12.8.3.3 "adbe.pkcs7.detached"; RFC 5652 CMS).
//
// The standard library has no PKCS#7/CMS package, so the SignedData
// container is assembled here directly from encoding/asn1. The signature
// is "detached": the signed content (the PDF byte-range) is not carried
// inside the CMS — only its digest, via the messageDigest signed
// attribute.

// Object identifiers (RFC 5652, RFC 5480, RFC 8017, NIST).
var (
	oidSignedData        = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 7, 2}
	oidData              = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 7, 1}
	oidAttrContentType   = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 9, 3}
	oidAttrMessageDigest = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 9, 4}
	oidAttrSigningTime   = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 9, 5}
	oidDigestSHA256      = asn1.ObjectIdentifier{2, 16, 840, 1, 101, 3, 4, 2, 1}
	oidRSAEncryption     = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 1, 1}
	oidECDSAWithSHA256   = asn1.ObjectIdentifier{1, 2, 840, 10045, 4, 3, 2}
	oidAttrSigningCertV2 = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 9, 16, 2, 47} // ESS signing-certificate-v2 (PAdES/CAdES)
)

// essCertIDv2 / signingCertificateV2 implement the ESS signing-certificate-v2
// signed attribute (RFC 5035 §5.4), required by PAdES/CAdES. The hash
// algorithm defaults to SHA-256, so it is omitted and only the cert hash is
// carried.
type essCertIDv2 struct {
	CertHash []byte // SHA-256 of the signer certificate DER
}

type signingCertificateV2 struct {
	Certs []essCertIDv2
}

// algorithmIdentifier is the X.509 AlgorithmIdentifier. For digest
// algorithms the parameters are an explicit ASN.1 NULL; for ECDSA they
// are absent. We model both by making Parameters optional+raw.
type algorithmIdentifier struct {
	Algorithm  asn1.ObjectIdentifier
	Parameters asn1.RawValue `asn1:"optional"`
}

// attribute is a CMS signed/unsigned attribute (RFC 5652 §5.3).
type attribute struct {
	Type   asn1.ObjectIdentifier
	Values []asn1.RawValue `asn1:"set"`
}

// issuerAndSerial identifies the signer's certificate (RFC 5652 §5.3).
type issuerAndSerial struct {
	IssuerRaw    asn1.RawValue
	SerialNumber *big.Int
}

// signerInfo is one CMS SignerInfo (RFC 5652 §5.3). SignedAttrs /
// UnsignedAttrs already carry their own [0]/[1] IMPLICIT tags in FullBytes,
// so no struct tag is set; UnsignedAttrs is omitted when empty.
type signerInfo struct {
	Version            int
	SID                issuerAndSerial
	DigestAlgorithm    algorithmIdentifier
	SignedAttrs        asn1.RawValue
	SignatureAlgorithm algorithmIdentifier
	Signature          []byte
	UnsignedAttrs      asn1.RawValue `asn1:"optional"`
}

// signedData is the CMS SignedData (RFC 5652 §5.1).
type signedData struct {
	Version          int
	DigestAlgorithms []algorithmIdentifier `asn1:"set"`
	ContentInfo      encapContentInfo
	Certificates     asn1.RawValue // [0] IMPLICIT SET OF Certificate (tag in FullBytes)
	SignerInfos      []signerInfo  `asn1:"set"`
}

// encapContentInfo carries the content type; for a detached signature
// the eContent itself is omitted.
type encapContentInfo struct {
	EContentType asn1.ObjectIdentifier
}

// contentInfo is the outer CMS wrapper (RFC 5652 §3). Content carries its
// own [0] EXPLICIT tag in the RawValue (a struct tag would be ignored
// because the value is pre-encoded), so no asn1 struct tag is set.
type contentInfo struct {
	ContentType asn1.ObjectIdentifier
	Content     asn1.RawValue
}

// buildPKCS7Detached produces a DER-encoded CMS SignedData over content
// (detached), signed by key for cert. chain is included alongside the
// signer cert so verifiers can build the path. SHA-256 throughout. When
// padES is set, the ESS signing-certificate-v2 signed attribute is added,
// making the signature CAdES/PAdES-conformant.
func buildPKCS7Detached(content []byte, cert *x509.Certificate, key crypto.Signer, chain []*x509.Certificate, signingTime time.Time, padES bool, tsaURL string) ([]byte, error) {
	if cert == nil || key == nil {
		return nil, fmt.Errorf("pkcs7: nil certificate or key")
	}
	h := crypto.SHA256.New()
	h.Write(content)
	msgDigest := h.Sum(nil)

	// Signed attributes: contentType, signingTime, messageDigest (+ ESS
	// signing-certificate-v2 for PAdES).
	signedAttrs, err := buildSignedAttrs(msgDigest, signingTime, cert, padES)
	if err != nil {
		return nil, err
	}
	// The signature is computed over the DER of the signed attributes
	// re-encoded with an explicit SET OF tag (0x31), per RFC 5652 §5.4.
	attrsForSigning, err := marshalSetOf(signedAttrs)
	if err != nil {
		return nil, err
	}
	ah := crypto.SHA256.New()
	ah.Write(attrsForSigning)
	signature, err := key.Sign(rand.Reader, ah.Sum(nil), crypto.SHA256)
	if err != nil {
		return nil, fmt.Errorf("pkcs7: sign: %w", err)
	}
	sigAlg, err := signatureAlgorithmFor(cert)
	if err != nil {
		return nil, err
	}

	// SignedAttrs inside the SignerInfo keep the [0] IMPLICIT tag.
	signedAttrsImplicit, err := marshalImplicitSet(signedAttrs)
	if err != nil {
		return nil, err
	}

	si := signerInfo{
		Version: 1,
		SID: issuerAndSerial{
			IssuerRaw:    asn1.RawValue{FullBytes: cert.RawIssuer},
			SerialNumber: cert.SerialNumber,
		},
		DigestAlgorithm:    algorithmIdentifier{Algorithm: oidDigestSHA256, Parameters: asn1NULL()},
		SignedAttrs:        asn1.RawValue{FullBytes: signedAttrsImplicit},
		SignatureAlgorithm: sigAlg,
		Signature:          signature,
	}

	// RFC 3161 timestamp: fetch a token over the signature and attach it as
	// the signature-time-stamp unsigned attribute ([1] IMPLICIT SET).
	if tsaURL != "" {
		token, err := requestTimestamp(tsaURL, signature)
		if err != nil {
			return nil, err
		}
		ua, err := marshalTimestampUnsignedAttr(token)
		if err != nil {
			return nil, err
		}
		si.UnsignedAttrs = asn1.RawValue{FullBytes: ua}
	}

	certsDER, err := marshalCertSet(cert, chain)
	if err != nil {
		return nil, err
	}

	sd := signedData{
		Version:          1,
		DigestAlgorithms: []algorithmIdentifier{{Algorithm: oidDigestSHA256, Parameters: asn1NULL()}},
		ContentInfo:      encapContentInfo{EContentType: oidData},
		Certificates:     asn1.RawValue{FullBytes: certsDER},
		SignerInfos:      []signerInfo{si},
	}
	sdDER, err := asn1.Marshal(sd)
	if err != nil {
		return nil, fmt.Errorf("pkcs7: marshal SignedData: %w", err)
	}

	ci := contentInfo{
		ContentType: oidSignedData,
		// [0] EXPLICIT wrapping the SignedData SEQUENCE.
		Content: asn1.RawValue{Class: asn1.ClassContextSpecific, Tag: 0, IsCompound: true, Bytes: sdDER},
	}
	out, err := asn1.Marshal(ci)
	if err != nil {
		return nil, fmt.Errorf("pkcs7: marshal ContentInfo: %w", err)
	}
	return out, nil
}

// buildSignedAttrs returns the signed attributes: contentType, signingTime,
// messageDigest, and — when padES is set — the ESS signing-certificate-v2
// attribute binding the signer certificate (PAdES/CAdES). The conventional
// order is kept stable (the same bytes are hashed and embedded).
func buildSignedAttrs(messageDigest []byte, signingTime time.Time, cert *x509.Certificate, padES bool) ([]attribute, error) {
	ctVal, err := asn1.Marshal(oidData)
	if err != nil {
		return nil, err
	}
	mdVal, err := asn1.Marshal(messageDigest)
	if err != nil {
		return nil, err
	}
	stVal, err := asn1.Marshal(signingTime.UTC())
	if err != nil {
		return nil, err
	}
	attrs := []attribute{
		{Type: oidAttrContentType, Values: []asn1.RawValue{{FullBytes: ctVal}}},
		{Type: oidAttrSigningTime, Values: []asn1.RawValue{{FullBytes: stVal}}},
		{Type: oidAttrMessageDigest, Values: []asn1.RawValue{{FullBytes: mdVal}}},
	}
	if padES {
		certHash := sha256Sum(cert.Raw)
		scVal, err := asn1.Marshal(signingCertificateV2{Certs: []essCertIDv2{{CertHash: certHash}}})
		if err != nil {
			return nil, err
		}
		attrs = append(attrs, attribute{Type: oidAttrSigningCertV2, Values: []asn1.RawValue{{FullBytes: scVal}}})
	}
	return attrs, nil
}

// sha256Sum returns SHA-256(b).
func sha256Sum(b []byte) []byte {
	h := crypto.SHA256.New()
	h.Write(b)
	return h.Sum(nil)
}

// marshalSetOf encodes the attributes as a universal SET OF (tag 0x31) —
// the form whose digest is signed.
func marshalSetOf(attrs []attribute) ([]byte, error) {
	var wrapper struct {
		Attrs []attribute `asn1:"set"`
	}
	wrapper.Attrs = attrs
	full, err := asn1.Marshal(wrapper)
	if err != nil {
		return nil, err
	}
	// Unwrap the outer SEQUENCE to get the inner SET bytes.
	var raw asn1.RawValue
	if _, err := asn1.Unmarshal(full, &raw); err != nil {
		return nil, err
	}
	return raw.Bytes, nil
}

// marshalImplicitSet encodes the attributes as [0] IMPLICIT SET OF
// (tag 0xA0) — the form carried inside the SignerInfo.
func marshalImplicitSet(attrs []attribute) ([]byte, error) {
	return marshalImplicitSetTag(0, attrs)
}

// marshalImplicitSetTag encodes the attributes as a [tag] IMPLICIT SET OF —
// tag 0 for signedAttrs, tag 1 for unsignedAttrs (RFC 5652 §5.3).
func marshalImplicitSetTag(tag int, attrs []attribute) ([]byte, error) {
	setBytes, err := marshalSetOf(attrs)
	if err != nil {
		return nil, err
	}
	var raw asn1.RawValue
	if _, err := asn1.Unmarshal(setBytes, &raw); err != nil {
		return nil, err
	}
	out := asn1.RawValue{Class: asn1.ClassContextSpecific, Tag: tag, IsCompound: true, Bytes: raw.Bytes}
	return asn1.Marshal(out)
}

// marshalCertSet encodes the signer cert (+ chain) as a [0] IMPLICIT SET
// OF Certificate.
func marshalCertSet(cert *x509.Certificate, chain []*x509.Certificate) ([]byte, error) {
	var body []byte
	body = append(body, cert.Raw...)
	for _, c := range chain {
		body = append(body, c.Raw...)
	}
	out := asn1.RawValue{Class: asn1.ClassContextSpecific, Tag: 0, IsCompound: true, Bytes: body}
	return asn1.Marshal(out)
}

// signatureAlgorithmFor picks the SignerInfo signatureAlgorithm from the
// certificate's public key type.
func signatureAlgorithmFor(cert *x509.Certificate) (algorithmIdentifier, error) {
	switch cert.PublicKeyAlgorithm {
	case x509.RSA:
		return algorithmIdentifier{Algorithm: oidRSAEncryption, Parameters: asn1NULL()}, nil
	case x509.ECDSA:
		return algorithmIdentifier{Algorithm: oidECDSAWithSHA256}, nil
	default:
		return algorithmIdentifier{}, fmt.Errorf("pkcs7: unsupported key algorithm %v (need RSA or ECDSA)", cert.PublicKeyAlgorithm)
	}
}

// asn1NULL returns the DER for an explicit ASN.1 NULL (0x05 0x00).
func asn1NULL() asn1.RawValue {
	return asn1.RawValue{Tag: asn1.TagNull, FullBytes: []byte{0x05, 0x00}}
}
