// SPDX-License-Identifier: MIT

package asposepdf

import (
	"crypto/sha1"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"fmt"
	"math/big"
	"strings"
	"time"
)

// OCSP (RFC 6960): asking a certificate's issuer whether it was revoked. The
// structures are hand-rolled on encoding/asn1 beside the CMS and RFC 3161
// code, for the same reason — the standard library has no OCSP and this
// library takes no dependencies.

// oidSHA1 identifies the hash OCSP uses for the issuer hashes in a CertID.
// SHA-1 is not a security choice here: the hashes are identifiers, and every
// responder in the field expects them under this OID.
var oidSHA1 = asn1.ObjectIdentifier{1, 3, 14, 3, 2, 26}

// oidOCSPBasic identifies a BasicOCSPResponse inside an OCSPResponse.
var oidOCSPBasic = asn1.ObjectIdentifier{1, 3, 6, 1, 5, 5, 7, 48, 1, 1}

// oidKeyUsageOCSPSigning marks a certificate the issuer delegated to answer
// OCSP on its behalf (RFC 6960 §4.2.2.2).
var oidKeyUsageOCSPSigning = asn1.ObjectIdentifier{1, 3, 6, 1, 5, 5, 7, 3, 9}

// RevocationState is what a responder or a CRL says about a certificate.
type RevocationState int

const (
	// RevocationUnknown means no usable answer was obtained — no responder,
	// an unparseable or stale response, a signature that did not check out.
	RevocationUnknown RevocationState = iota
	// RevocationGood means the issuer states the certificate is not revoked.
	RevocationGood
	// RevocationRevoked means the issuer states it is revoked.
	RevocationRevoked
)

// String names the state, for messages and test failures.
func (s RevocationState) String() string {
	switch s {
	case RevocationGood:
		return "good"
	case RevocationRevoked:
		return "revoked"
	}
	return "unknown"
}

// ocspCertID names the certificate being asked about, by hashes of its issuer
// plus its serial (RFC 6960 §4.1.1).
type ocspCertID struct {
	HashAlgorithm  algorithmIdentifier
	IssuerNameHash []byte
	IssuerKeyHash  []byte
	SerialNumber   *big.Int
}

type ocspRequestEntry struct {
	ReqCert ocspCertID
}

type ocspTBSRequest struct {
	Version     int `asn1:"optional,explicit,tag:0,default:0"`
	RequestList []ocspRequestEntry
}

type ocspRequest struct {
	TBSRequest ocspTBSRequest
}

// buildOCSPCertID computes the CertID for cert as issued by issuer.
func buildOCSPCertID(cert, issuer *x509.Certificate) (ocspCertID, error) {
	if cert == nil || issuer == nil {
		return ocspCertID{}, fmt.Errorf("ocsp: nil certificate or issuer")
	}
	if err := cert.CheckSignatureFrom(issuer); err != nil {
		return ocspCertID{}, fmt.Errorf("ocsp: %s did not issue %s: %w",
			issuer.Subject.CommonName, cert.Subject.CommonName, err)
	}
	var spki struct {
		Algorithm pkix.AlgorithmIdentifier
		PublicKey asn1.BitString
	}
	if _, err := asn1.Unmarshal(issuer.RawSubjectPublicKeyInfo, &spki); err != nil {
		return ocspCertID{}, fmt.Errorf("ocsp: parse issuer public key: %w", err)
	}
	nameHash := sha1.Sum(issuer.RawSubject)
	keyHash := sha1.Sum(spki.PublicKey.RightAlign())
	return ocspCertID{
		HashAlgorithm:  algorithmIdentifier{Algorithm: oidSHA1, Parameters: asn1NULL()},
		IssuerNameHash: nameHash[:],
		IssuerKeyHash:  keyHash[:],
		SerialNumber:   cert.SerialNumber,
	}, nil
}

// buildOCSPRequest returns the DER of a single-certificate OCSPRequest. No
// nonce: it would have to be matched in the response, and a cached responder
// answer without one is still an answer about the certificate.
func buildOCSPRequest(cert, issuer *x509.Certificate) ([]byte, error) {
	id, err := buildOCSPCertID(cert, issuer)
	if err != nil {
		return nil, err
	}
	return asn1.Marshal(ocspRequest{
		TBSRequest: ocspTBSRequest{RequestList: []ocspRequestEntry{{ReqCert: id}}},
	})
}

type ocspResponseBytes struct {
	ResponseType asn1.ObjectIdentifier
	Response     []byte
}

type ocspResponseOuter struct {
	Status        asn1.Enumerated
	ResponseBytes ocspResponseBytes `asn1:"optional,explicit,tag:0"`
}

type ocspBasicResponse struct {
	TBSResponseData    asn1.RawValue
	SignatureAlgorithm pkix.AlgorithmIdentifier
	Signature          asn1.BitString
	Certificates       []asn1.RawValue `asn1:"optional,explicit,tag:0"`
}

type ocspResponseData struct {
	Version            int `asn1:"optional,explicit,tag:0,default:0"`
	RawResponderID     asn1.RawValue
	ProducedAt         time.Time `asn1:"generalized"`
	Responses          []ocspSingleResponse
	ResponseExtensions []pkix.Extension `asn1:"optional,explicit,tag:1"`
}

type ocspSingleResponse struct {
	CertID           ocspCertID
	Status           asn1.RawValue
	ThisUpdate       time.Time        `asn1:"generalized"`
	NextUpdate       time.Time        `asn1:"optional,explicit,tag:0,generalized"`
	SingleExtensions []pkix.Extension `asn1:"optional,explicit,tag:1"`
}

type ocspRevokedInfo struct {
	RevocationTime   time.Time       `asn1:"generalized"`
	RevocationReason asn1.Enumerated `asn1:"optional,explicit,tag:0"`
}

// raw re-encodes the BasicOCSPResponse, which is the form /DSS /OCSPs holds.
func (b *ocspBasicResponse) raw() []byte {
	der, err := asn1.Marshal(*b)
	if err != nil {
		return nil
	}
	return der
}

// parseOCSPResponse unwraps an OCSPResponse down to its BasicOCSPResponse and
// the response data inside it. A responder that answers with an error status
// (tryLater, unauthorized, …) is an error here: it told us nothing about the
// certificate.
func parseOCSPResponse(der []byte) (*ocspBasicResponse, *ocspResponseData, error) {
	var outer ocspResponseOuter
	if _, err := asn1.Unmarshal(der, &outer); err != nil {
		return nil, nil, fmt.Errorf("ocsp: parse response: %w", err)
	}
	if outer.Status != 0 {
		return nil, nil, fmt.Errorf("ocsp: responder returned status %d", int(outer.Status))
	}
	if !outer.ResponseBytes.ResponseType.Equal(oidOCSPBasic) {
		return nil, nil, fmt.Errorf("ocsp: unsupported response type %v", outer.ResponseBytes.ResponseType)
	}
	var basic ocspBasicResponse
	if _, err := asn1.Unmarshal(outer.ResponseBytes.Response, &basic); err != nil {
		return nil, nil, fmt.Errorf("ocsp: parse basic response: %w", err)
	}
	var data ocspResponseData
	if _, err := asn1.Unmarshal(basic.TBSResponseData.FullBytes, &data); err != nil {
		return nil, nil, fmt.Errorf("ocsp: parse response data: %w", err)
	}
	return &basic, &data, nil
}

// ocspSingleFor finds the entry answering about id. A response carrying only
// other certificates says nothing about ours, which is an error rather than an
// "unknown": it means the wrong responder was asked, or the wrong issuer was
// matched.
func ocspSingleFor(data *ocspResponseData, id ocspCertID) (*ocspSingleResponse, error) {
	for i := range data.Responses {
		r := &data.Responses[i]
		if r.CertID.SerialNumber != nil && id.SerialNumber != nil &&
			r.CertID.SerialNumber.Cmp(id.SerialNumber) == 0 &&
			string(r.CertID.IssuerNameHash) == string(id.IssuerNameHash) &&
			string(r.CertID.IssuerKeyHash) == string(id.IssuerKeyHash) {
			return r, nil
		}
	}
	return nil, fmt.Errorf("ocsp: response carries no entry for this certificate")
}

// ocspStatusOf reads the certificate status out of a single response, and
// refuses one whose validity window has passed: a stale answer is not an
// answer.
func ocspStatusOf(sr *ocspSingleResponse) (RevocationState, time.Time, int, error) {
	now := time.Now()
	if !sr.NextUpdate.IsZero() && now.After(sr.NextUpdate) {
		return RevocationUnknown, time.Time{}, 0,
			fmt.Errorf("ocsp: response expired at %s", sr.NextUpdate.Format(time.RFC3339))
	}
	if sr.ThisUpdate.After(now.Add(5 * time.Minute)) {
		return RevocationUnknown, time.Time{}, 0,
			fmt.Errorf("ocsp: response is dated in the future (%s)", sr.ThisUpdate.Format(time.RFC3339))
	}
	switch sr.Status.Tag {
	case 0:
		return RevocationGood, time.Time{}, 0, nil
	case 1:
		// The status is [1] IMPLICIT RevokedInfo, so its body carries a
		// SEQUENCE's contents under a context tag; re-wrap it as the
		// universal SEQUENCE the parser expects.
		seq, err := asn1.Marshal(asn1.RawValue{
			Class: asn1.ClassUniversal, Tag: asn1.TagSequence, IsCompound: true, Bytes: sr.Status.Bytes,
		})
		if err != nil {
			return RevocationUnknown, time.Time{}, 0, fmt.Errorf("ocsp: parse revoked info: %w", err)
		}
		var info ocspRevokedInfo
		if _, err := asn1.Unmarshal(seq, &info); err != nil {
			return RevocationUnknown, time.Time{}, 0, fmt.Errorf("ocsp: parse revoked info: %w", err)
		}
		return RevocationRevoked, info.RevocationTime, int(info.RevocationReason), nil
	case 2:
		return RevocationUnknown, time.Time{}, 0, fmt.Errorf("ocsp: responder does not know this certificate")
	}
	return RevocationUnknown, time.Time{}, 0,
		fmt.Errorf("ocsp: unrecognised certificate status tag %d", sr.Status.Tag)
}

// ocspSigAlgs maps the signature algorithm identifiers a responder may use to
// the x509 algorithm that checks them. Anything outside this table is refused
// rather than guessed at.
var ocspSigAlgs = map[string]x509.SignatureAlgorithm{
	"1.2.840.113549.1.1.5":  x509.SHA1WithRSA,
	"1.2.840.113549.1.1.11": x509.SHA256WithRSA,
	"1.2.840.113549.1.1.12": x509.SHA384WithRSA,
	"1.2.840.113549.1.1.13": x509.SHA512WithRSA,
	"1.2.840.10045.4.3.2":   x509.ECDSAWithSHA256,
	"1.2.840.10045.4.3.3":   x509.ECDSAWithSHA384,
	"1.2.840.10045.4.3.4":   x509.ECDSAWithSHA512,
}

// verifyOCSPResponse checks who signed the response and that the signature is
// good. The signer is either the issuer itself or a responder certificate the
// issuer signed that carries the OCSPSigning extended key usage; anything else
// is refused, because an unverified response is not evidence.
func verifyOCSPResponse(basic *ocspBasicResponse, issuer *x509.Certificate) error {
	if issuer == nil {
		return fmt.Errorf("ocsp: no issuer certificate to check the response against")
	}
	alg, ok := ocspSigAlgs[basic.SignatureAlgorithm.Algorithm.String()]
	if !ok {
		return fmt.Errorf("ocsp: unsupported response signature algorithm %v", basic.SignatureAlgorithm.Algorithm)
	}
	sig := basic.Signature.RightAlign()

	// The issuer itself.
	if err := issuer.CheckSignature(alg, basic.TBSResponseData.FullBytes, sig); err == nil {
		return nil
	}

	// A responder the issuer delegated to, carried in the response.
	for _, raw := range basic.Certificates {
		responder, err := x509.ParseCertificate(raw.FullBytes)
		if err != nil {
			continue
		}
		if err := responder.CheckSignatureFrom(issuer); err != nil {
			continue
		}
		if !hasOCSPSigningEKU(responder) {
			continue
		}
		if err := responder.CheckSignature(alg, basic.TBSResponseData.FullBytes, sig); err == nil {
			return nil
		}
	}
	return fmt.Errorf("ocsp: response is not signed by the issuer or a delegated responder")
}

// hasOCSPSigningEKU reports whether the certificate is allowed to answer OCSP
// for its issuer.
func hasOCSPSigningEKU(cert *x509.Certificate) bool {
	for _, eku := range cert.ExtKeyUsage {
		if eku == x509.ExtKeyUsageOCSPSigning {
			return true
		}
	}
	for _, oid := range cert.UnknownExtKeyUsage {
		if oid.Equal(oidKeyUsageOCSPSigning) {
			return true
		}
	}
	return false
}

// certName is a readable name for a certificate in an error message.
func certName(cert *x509.Certificate) string {
	if cert == nil {
		return "the certificate"
	}
	if n := strings.TrimSpace(cert.Subject.CommonName); n != "" {
		return n
	}
	if cert.SerialNumber != nil {
		return "serial " + cert.SerialNumber.String()
	}
	return "the certificate"
}
