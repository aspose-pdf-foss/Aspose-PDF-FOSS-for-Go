# Signature Revocation and LTV Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Report whether a signing certificate was revoked (OCSP and CRL), and embed that evidence in the document so the signature still verifies after the certificate expires (`/DSS`, PAdES B-LT).

**Architecture:** A hand-rolled OCSP client beside the existing CMS and RFC 3161 code, CRLs through `crypto/x509`, both behind a `RevocationFetcher` interface whose default implementation is ours. Phase 1 surfaces the status per signature at verification time; phase 2 writes the same material into the catalog as `/DSS` with `/VRI`, appended as a new revision so existing signatures stay valid.

**Tech Stack:** Go 1.24 (toolchain 1.26), standard library only — `encoding/asn1`, `crypto/x509`, `net/http`. Package `asposepdf` at the repository root. No new dependencies.

**Spec:** `docs/superpowers/specs/2026-09-16-signature-revocation-ltv-design.md`

## Global Constraints

- Every new `.go` file starts with `// SPDX-License-Identifier: MIT` on its own line, then a blank line, then `package asposepdf`.
- Pure Go, standard library only. Adding a module dependency is a task failure.
- **No test may touch the network.** Every fetch goes through `RevocationFetcher`, which tests substitute; the built-in HTTP implementation is exercised only against `httptest`.
- Public API mirrors Aspose.PDF for .NET where a counterpart exists (`ValidationOptions`, `IsLtvEnabled` → `LTVEnabled`); options structs follow the `SearchOptions` idiom — zero value usable, passed variadically, last one wins.
- `SignatureVerification.Valid` keeps its current meaning (the signature is intact and covers the bytes). Revocation never flips it.
- Do not add entries to `testdata/testfiles.json` and do not add PDF or certificate files to `testdata/` — every fixture is generated in memory, as the existing signature tests do.
- `gofmt -l` clean for files you touch; `go vet ./...` silent; `go test ./...` passes; `golangci-lint run` reports 0 issues.
- Commit messages end with a blank line then `Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>`.
- Do not push and do not tag.

## File Structure

| File | Responsibility |
|---|---|
| `ocsp.go` (create) | OCSP request DER, response parsing, response-signature verification |
| `crl.go` (create) | CRL parsing, issuer-signature check, serial lookup |
| `revocation.go` (create) | `RevocationFetcher`, the HTTP implementation, orchestration, per-call cache, public status types |
| `dss.go` (create) | `/DSS`, `/VRI`, `/Extensions`; appending them as an incremental revision |
| `sign_incremental.go` (modify) | Extract the revision-appending tail so `dss.go` can reuse it |
| `sign_verify.go` (modify) | `ValidationOptions` on the two verify entry points; `Revocation` and `LTVEnabled` on the result |
| `sign.go` (modify) | `SignOptions.LTV` |
| `ocsp_internal_test.go`, `crl_internal_test.go`, `revocation_test.go`, `dss_test.go` (create) | Tests per area |
| `CLAUDE.md`, `README.md`, `CHANGELOG.md` (modify) | Documentation, in the last task |

Existing code this plan builds on:

- `algorithmIdentifier` (`pkcs7_sign.go`) — `struct{ Algorithm asn1.ObjectIdentifier; Parameters asn1.RawValue \`asn1:"optional"\` }`.
- `digestOIDs`, `digestAlgorithmFor(oid) (crypto.Hash, bool)` (`pkcs7_sign.go`).
- `SignatureVerification`, `verifyOneSignature(source []byte, fieldName string, sig pdfDict)`, `(*Document).collectSignatureFields() []sigFieldRef` where `sigFieldRef{name string; sig pdfDict}` (`sign_verify.go`).
- `buildIncrementalSignedPDF` (`sign_incremental.go`) with its helpers `lastStartxref(source) (int64, error)`, `writeIncrementalXref(buf, rows)`, `xrefRow{num, gen int; off int64}`, `(*Document).incrementalID() ([]byte, []byte)`, `deepCopyValue`, `writeValue(w, v, remap, encFn)`.
- `contentsBytes(v pdfValue) []byte` (`sign_verify.go`) — the decoded `/Contents` octets.

---

### Task 1: OCSP request

**Files:**
- Create: `ocsp.go`
- Test: `ocsp_internal_test.go`

**Interfaces:**
- Consumes: `algorithmIdentifier` (`pkcs7_sign.go`).
- Produces:
  - `type ocspCertID struct { HashAlgorithm algorithmIdentifier; IssuerNameHash []byte; IssuerKeyHash []byte; SerialNumber *big.Int }`
  - `func buildOCSPCertID(cert, issuer *x509.Certificate) (ocspCertID, error)`
  - `func buildOCSPRequest(cert, issuer *x509.Certificate) ([]byte, error)`

- [ ] **Step 1: Write the failing test**

Create `ocsp_internal_test.go`:

```go
// SPDX-License-Identifier: MIT

package asposepdf

import (
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

// ocspTestLeaf issues a certificate from the CA, optionally naming an OCSP
// responder and a CRL distribution point.
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
	if !id.HashAlgorithm.Algorithm.Equal(asn1.ObjectIdentifier{1, 3, 14, 3, 2, 26}) {
		t.Errorf("HashAlgorithm = %v, want SHA-1", id.HashAlgorithm.Algorithm)
	}
}

// The request must be a well-formed OCSPRequest carrying exactly that CertID.
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
		t.Fatal("a certificate was built against an issuer that did not sign it")
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test -run "TestBuildOCSP" .`
Expected: FAIL — `undefined: buildOCSPCertID`, `undefined: ocspRequest`.

- [ ] **Step 3: Write the implementation**

Create `ocsp.go`:

```go
// SPDX-License-Identifier: MIT

package asposepdf

import (
	"crypto/sha1"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"fmt"
	"math/big"
)

// OCSP (RFC 6960): asking a responder whether one certificate is revoked.
// The structures are hand-rolled on encoding/asn1 beside the CMS and RFC 3161
// code, for the same reason — the standard library has no OCSP and this
// library takes no dependencies.

// oidSHA1 identifies the hash OCSP uses for the issuer hashes in a CertID.
// SHA-1 is not a security choice here: the hashes are identifiers, and every
// responder in the field expects them under this OID.
var oidSHA1 = asn1.ObjectIdentifier{1, 3, 14, 3, 2, 26}

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
// answer without one is still a valid answer for our purposes.
func buildOCSPRequest(cert, issuer *x509.Certificate) ([]byte, error) {
	id, err := buildOCSPCertID(cert, issuer)
	if err != nil {
		return nil, err
	}
	return asn1.Marshal(ocspRequest{
		TBSRequest: ocspTBSRequest{RequestList: []ocspRequestEntry{{ReqCert: id}}},
	})
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test -run "TestBuildOCSP" .`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add ocsp.go ocsp_internal_test.go
git commit -m "$(cat <<'EOF'
feat: OCSP request building (pdf-go-x2s6)

A CertID names a certificate by hashes of its issuer's subject and public
key plus its serial; get those wrong and a responder answers "unknown" for a
certificate it knows. Building one refuses an issuer that did not sign the
certificate, which is the mistake that produces exactly that.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 2: OCSP response parsing

**Files:**
- Modify: `ocsp.go` (append)
- Test: `ocsp_internal_test.go` (append)

**Interfaces:**
- Consumes: `ocspCertID`, `buildOCSPCertID` (Task 1).
- Produces:
  - `type ocspSingleResponse struct { CertID ocspCertID; Status asn1.RawValue; ThisUpdate time.Time; NextUpdate time.Time; SingleExtensions []pkix.Extension }`
  - `type ocspBasicResponse struct { TBSResponseData asn1.RawValue; SignatureAlgorithm pkix.AlgorithmIdentifier; Signature asn1.BitString; Certificates []asn1.RawValue }`
  - `func parseOCSPResponse(der []byte) (*ocspBasicResponse, *ocspResponseData, error)`
  - `func ocspSingleFor(data *ocspResponseData, id ocspCertID) (*ocspSingleResponse, error)`
  - `func ocspStatusOf(sr *ocspSingleResponse) (state RevocationState, revokedAt time.Time, reason int, err error)` — `RevocationState` is defined here as `type RevocationState int` with `RevocationGood`, `RevocationRevoked`, `RevocationUnknown`

- [ ] **Step 1: Write the failing test**

Append to `ocsp_internal_test.go` (add `"crypto"`, `"crypto/rsa"` are already imported; add `"bytes"`):

```go
// ocspTestResponse builds a signed BasicOCSPResponse for one certificate,
// the way a responder would. status is "good", "revoked" or "unknown".
func ocspTestResponse(t *testing.T, ca *x509.Certificate, caKey *rsa.PrivateKey, leaf *x509.Certificate, status string, revokedAt time.Time, reason int, nextUpdate time.Time) []byte {
	t.Helper()
	id, err := buildOCSPCertID(leaf, ca)
	if err != nil {
		t.Fatal(err)
	}

	var certStatus asn1.RawValue
	switch status {
	case "good":
		certStatus = asn1.RawValue{Class: asn1.ClassContextSpecific, Tag: 0, IsCompound: false, Bytes: []byte{}}
	case "revoked":
		info := ocspRevokedInfo{RevocationTime: revokedAt.UTC(), RevocationReason: asn1.Enumerated(reason)}
		body, err := asn1.Marshal(info)
		if err != nil {
			t.Fatal(err)
		}
		// The body of a SEQUENCE, re-tagged as [1] IMPLICIT.
		var seq asn1.RawValue
		if _, err := asn1.Unmarshal(body, &seq); err != nil {
			t.Fatal(err)
		}
		certStatus = asn1.RawValue{Class: asn1.ClassContextSpecific, Tag: 1, IsCompound: true, Bytes: seq.Bytes}
	default:
		certStatus = asn1.RawValue{Class: asn1.ClassContextSpecific, Tag: 2, IsCompound: false, Bytes: []byte{}}
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
		Class: asn1.ClassContextSpecific, Tag: 1, IsCompound: true, Bytes: ca.RawSubject,
	})
	if err != nil {
		t.Fatal(err)
	}
	data := ocspResponseData{
		RawResponderID: asn1.RawValue{FullBytes: responderID},
		ProducedAt:     time.Now().UTC(),
		Responses:      []ocspSingleResponse{single},
	}
	tbs, err := asn1.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	sig, err := signOCSPTBS(t, caKey, tbs)
	if err != nil {
		t.Fatal(err)
	}
	basic := ocspBasicResponse{
		TBSResponseData:    asn1.RawValue{FullBytes: tbs},
		SignatureAlgorithm: pkix.AlgorithmIdentifier{Algorithm: asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 1, 11}, Parameters: asn1.RawValue{Tag: asn1.TagNull, FullBytes: []byte{5, 0}}},
		Signature:          asn1.BitString{Bytes: sig, BitLength: len(sig) * 8},
		Certificates:       []asn1.RawValue{{FullBytes: ca.Raw}},
	}
	basicDER, err := asn1.Marshal(basic)
	if err != nil {
		t.Fatal(err)
	}
	outer := ocspResponseOuter{
		Status: 0, // successful
		ResponseBytes: ocspResponseBytes{
			ResponseType: asn1.ObjectIdentifier{1, 3, 6, 1, 5, 5, 7, 48, 1, 1},
			Response:     basicDER,
		},
	}
	der, err := asn1.Marshal(outer)
	if err != nil {
		t.Fatal(err)
	}
	return der
}

// signOCSPTBS signs the response data with SHA-256/RSA, as a responder does.
func signOCSPTBS(t *testing.T, key *rsa.PrivateKey, tbs []byte) ([]byte, error) {
	t.Helper()
	h := crypto.SHA256.New()
	h.Write(tbs)
	return key.Sign(rand.Reader, h.Sum(nil), crypto.SHA256)
}

func TestParseOCSPResponseGood(t *testing.T) {
	ca, caKey := ocspTestCA(t)
	leaf := ocspTestLeaf(t, ca, caKey, 11, "", "")
	der := ocspTestResponse(t, ca, caKey, leaf, "good", time.Time{}, 0, time.Now().Add(time.Hour))

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
	der := ocspTestResponse(t, ca, caKey, leaf, "revoked", when, 1 /* keyCompromise */, time.Now().Add(time.Hour))

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
	der := ocspTestResponse(t, ca, caKey, other, "good", time.Time{}, 0, time.Now().Add(time.Hour))

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
	der := ocspTestResponse(t, ca, caKey, leaf, "good", time.Time{}, 0, time.Now().Add(-time.Hour))

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

// A responder that reports an error status yields no certificate status.
func TestParseOCSPResponseErrorStatus(t *testing.T) {
	der, err := asn1.Marshal(ocspResponseOuter{Status: 6 /* unauthorized */})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := parseOCSPResponse(der); err == nil {
		t.Fatal("an unauthorized response was parsed as successful")
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test -run "TestParseOCSP|TestOCSPSingle|TestOCSPStatus" .`
Expected: FAIL — `undefined: ocspResponseOuter`, `undefined: parseOCSPResponse`.

- [ ] **Step 3: Write the implementation**

Append to `ocsp.go` (add `"time"` and `"crypto/x509/pkix"` to the imports if not present):

```go
// oidOCSPBasic identifies a BasicOCSPResponse inside an OCSPResponse.
var oidOCSPBasic = asn1.ObjectIdentifier{1, 3, 6, 1, 5, 5, 7, 48, 1, 1}

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

// String names the state for logs and test failures.
func (s RevocationState) String() string {
	switch s {
	case RevocationGood:
		return "good"
	case RevocationRevoked:
		return "revoked"
	}
	return "unknown"
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
// other certificates says nothing about ours, which is an error rather than
// an "unknown": it means we asked the wrong responder or matched the wrong
// issuer.
func ocspSingleFor(data *ocspResponseData, id ocspCertID) (*ocspSingleResponse, error) {
	for i := range data.Responses {
		r := &data.Responses[i]
		if r.CertID.SerialNumber != nil && r.CertID.SerialNumber.Cmp(id.SerialNumber) == 0 &&
			bytesEqualConst(r.CertID.IssuerNameHash, id.IssuerNameHash) &&
			bytesEqualConst(r.CertID.IssuerKeyHash, id.IssuerKeyHash) {
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
		return RevocationUnknown, time.Time{}, 0, fmt.Errorf("ocsp: response expired at %s", sr.NextUpdate.Format(time.RFC3339))
	}
	if sr.ThisUpdate.After(now.Add(5 * time.Minute)) {
		return RevocationUnknown, time.Time{}, 0, fmt.Errorf("ocsp: response is dated in the future (%s)", sr.ThisUpdate.Format(time.RFC3339))
	}
	switch sr.Status.Tag {
	case 0:
		return RevocationGood, time.Time{}, 0, nil
	case 1:
		var info ocspRevokedInfo
		if _, err := asn1.Unmarshal(asn1.RawValue{
			Class: asn1.ClassUniversal, Tag: asn1.TagSequence, IsCompound: true, Bytes: sr.Status.Bytes,
		}.FullBytes, &info); err != nil {
			// Re-marshal the body as a SEQUENCE before parsing: the status is
			// [1] IMPLICIT, so the tag on the wire is not the universal one.
			seq, merr := asn1.Marshal(asn1.RawValue{Class: asn1.ClassUniversal, Tag: asn1.TagSequence, IsCompound: true, Bytes: sr.Status.Bytes})
			if merr != nil {
				return RevocationUnknown, time.Time{}, 0, fmt.Errorf("ocsp: parse revoked info: %w", err)
			}
			if _, err := asn1.Unmarshal(seq, &info); err != nil {
				return RevocationUnknown, time.Time{}, 0, fmt.Errorf("ocsp: parse revoked info: %w", err)
			}
		}
		return RevocationRevoked, info.RevocationTime, int(info.RevocationReason), nil
	case 2:
		return RevocationUnknown, time.Time{}, 0, fmt.Errorf("ocsp: responder does not know this certificate")
	}
	return RevocationUnknown, time.Time{}, 0, fmt.Errorf("ocsp: unrecognised certificate status tag %d", sr.Status.Tag)
}
```

Note on the revoked branch: `asn1.RawValue{…}.FullBytes` is empty for a value built by hand, so the first `Unmarshal` fails and the code falls through to the re-marshalled form. Simplify it to the marshal-then-unmarshal path alone if the first branch proves dead when you run the test — keep exactly one working path, not two.

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test -run "TestParseOCSP|TestOCSPSingle|TestOCSPStatus" .`
Expected: PASS. If the revoked case fails to parse, fix the re-tagging as the note above says and re-run.

- [ ] **Step 5: Commit**

```bash
git add ocsp.go ocsp_internal_test.go
git commit -m "$(cat <<'EOF'
feat: OCSP response parsing (pdf-go-x2s6)

Unwraps an OCSPResponse to its BasicOCSPResponse, finds the entry that
answers about the certificate we asked about — a response about another
certificate is refused rather than read as good — and reads the status,
including the revocation time and reason. A response whose validity window
has passed, or one dated in the future, yields unknown with the reason: a
stale answer is not an answer.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 3: OCSP response signature verification

**Files:**
- Modify: `ocsp.go` (append)
- Test: `ocsp_internal_test.go` (append)

**Interfaces:**
- Consumes: `ocspBasicResponse`, `ocspResponseData` (Task 2).
- Produces: `func verifyOCSPResponse(basic *ocspBasicResponse, data *ocspResponseData, issuer *x509.Certificate) error`

- [ ] **Step 1: Write the failing test**

Append to `ocsp_internal_test.go`:

```go
func TestVerifyOCSPResponseFromIssuer(t *testing.T) {
	ca, caKey := ocspTestCA(t)
	leaf := ocspTestLeaf(t, ca, caKey, 21, "", "")
	der := ocspTestResponse(t, ca, caKey, leaf, "good", time.Time{}, 0, time.Now().Add(time.Hour))

	basic, data, err := parseOCSPResponse(der)
	if err != nil {
		t.Fatal(err)
	}
	if err := verifyOCSPResponse(basic, data, ca); err != nil {
		t.Errorf("a response signed by the issuer was rejected: %v", err)
	}
}

// A response signed by somebody else is worthless, however well-formed.
func TestVerifyOCSPResponseWrongSigner(t *testing.T) {
	ca, caKey := ocspTestCA(t)
	impostor, impostorKey := ocspTestCA(t)
	leaf := ocspTestLeaf(t, ca, caKey, 22, "", "")
	der := ocspTestResponse(t, impostor, impostorKey, leaf, "good", time.Time{}, 0, time.Now().Add(time.Hour))

	basic, data, err := parseOCSPResponse(der)
	if err != nil {
		t.Fatal(err)
	}
	if err := verifyOCSPResponse(basic, data, ca); err == nil {
		t.Fatal("a response signed by another key was accepted")
	}
}

// A tampered response must not verify.
func TestVerifyOCSPResponseTampered(t *testing.T) {
	ca, caKey := ocspTestCA(t)
	leaf := ocspTestLeaf(t, ca, caKey, 23, "", "")
	der := ocspTestResponse(t, ca, caKey, leaf, "good", time.Time{}, 0, time.Now().Add(time.Hour))

	basic, data, err := parseOCSPResponse(der)
	if err != nil {
		t.Fatal(err)
	}
	basic.Signature.Bytes[10] ^= 0xFF
	if err := verifyOCSPResponse(basic, data, ca); err == nil {
		t.Fatal("a tampered signature was accepted")
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test -run TestVerifyOCSPResponse .`
Expected: FAIL — `undefined: verifyOCSPResponse`.

- [ ] **Step 3: Write the implementation**

Append to `ocsp.go`:

```go
// ocspSigAlgs maps the signature algorithm identifiers a responder may use to
// the x509 algorithm that checks them. Anything outside this table is
// refused rather than guessed at.
var ocspSigAlgs = map[string]x509.SignatureAlgorithm{
	"1.2.840.113549.1.1.5":  x509.SHA1WithRSA,
	"1.2.840.113549.1.1.11": x509.SHA256WithRSA,
	"1.2.840.113549.1.1.12": x509.SHA384WithRSA,
	"1.2.840.113549.1.1.13": x509.SHA512WithRSA,
	"1.2.840.10045.4.3.2":   x509.ECDSAWithSHA256,
	"1.2.840.10045.4.3.3":   x509.ECDSAWithSHA384,
	"1.2.840.10045.4.3.4":   x509.ECDSAWithSHA512,
}

// oidKeyUsageOCSPSigning marks a certificate the issuer delegated to answer
// OCSP on its behalf (RFC 6960 §4.2.2.2).
var oidKeyUsageOCSPSigning = asn1.ObjectIdentifier{1, 3, 6, 1, 5, 5, 7, 3, 9}

// verifyOCSPResponse checks who signed the response and that the signature is
// good. The signer is either the issuer itself or a responder certificate the
// issuer signed that carries the OCSPSigning extended key usage; anything
// else is refused, because an unverified response is not evidence.
func verifyOCSPResponse(basic *ocspBasicResponse, data *ocspResponseData, issuer *x509.Certificate) error {
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

	// A delegated responder, carried in the response.
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
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test -run TestVerifyOCSPResponse .`
Expected: PASS.

- [ ] **Step 5: Add the delegated-responder cases**

Append to `ocsp_internal_test.go`:

```go
// ocspDelegatedResponder issues a responder certificate from the CA, with or
// without the extended key usage that makes it legitimate.
func ocspDelegatedResponder(t *testing.T, ca *x509.Certificate, caKey *rsa.PrivateKey, withEKU bool) (*x509.Certificate, *rsa.PrivateKey) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano() % 100000),
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
			der := ocspTestResponse(t, responder, responderKey, leaf, "good", time.Time{}, 0, time.Now().Add(time.Hour))
			// The CertID must still name the real issuer, not the responder.
			fixed := ocspTestResponseForIssuer(t, ca, responder, responderKey, leaf)
			_ = der

			basic, data, err := parseOCSPResponse(fixed)
			if err != nil {
				t.Fatal(err)
			}
			err = verifyOCSPResponse(basic, data, ca)
			if tc.wantOK && err != nil {
				t.Errorf("a delegated responder with the EKU was rejected: %v", err)
			}
			if !tc.wantOK && err == nil {
				t.Error("a responder without the OCSPSigning EKU was accepted")
			}
		})
	}
}
```

This needs one more helper — a response whose CertID names the real issuer but which is signed by the responder. Append it too:

```go
// ocspTestResponseForIssuer builds a response about a certificate issued by
// ca, signed by responder (whose certificate travels in the response).
func ocspTestResponseForIssuer(t *testing.T, ca, responder *x509.Certificate, responderKey *rsa.PrivateKey, leaf *x509.Certificate) []byte {
	t.Helper()
	id, err := buildOCSPCertID(leaf, ca)
	if err != nil {
		t.Fatal(err)
	}
	single := ocspSingleResponse{
		CertID:     id,
		Status:     asn1.RawValue{Class: asn1.ClassContextSpecific, Tag: 0, Bytes: []byte{}},
		ThisUpdate: time.Now().Add(-time.Minute).UTC(),
		NextUpdate: time.Now().Add(time.Hour).UTC(),
	}
	responderID, err := asn1.Marshal(asn1.RawValue{
		Class: asn1.ClassContextSpecific, Tag: 1, IsCompound: true, Bytes: responder.RawSubject,
	})
	if err != nil {
		t.Fatal(err)
	}
	data := ocspResponseData{
		RawResponderID: asn1.RawValue{FullBytes: responderID},
		ProducedAt:     time.Now().UTC(),
		Responses:      []ocspSingleResponse{single},
	}
	tbs, err := asn1.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	sig, err := signOCSPTBS(t, responderKey, tbs)
	if err != nil {
		t.Fatal(err)
	}
	basic := ocspBasicResponse{
		TBSResponseData:    asn1.RawValue{FullBytes: tbs},
		SignatureAlgorithm: pkix.AlgorithmIdentifier{Algorithm: asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 1, 11}, Parameters: asn1.RawValue{Tag: asn1.TagNull, FullBytes: []byte{5, 0}}},
		Signature:          asn1.BitString{Bytes: sig, BitLength: len(sig) * 8},
		Certificates:       []asn1.RawValue{{FullBytes: responder.Raw}},
	}
	basicDER, err := asn1.Marshal(basic)
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
```

Then delete the now-unused `der` variable and the `_ = der` line from the delegated test.

- [ ] **Step 6: Run the tests**

Run: `go test -run TestVerifyOCSPResponse .`
Expected: PASS, both sub-cases.

- [ ] **Step 7: Commit**

```bash
git add ocsp.go ocsp_internal_test.go
git commit -m "$(cat <<'EOF'
feat: verify the signature on an OCSP response (pdf-go-x2s6)

An unverified response is not evidence, so the signer must be the issuer
itself or a responder certificate the issuer signed carrying the OCSPSigning
extended key usage. Anything else — another CA, a delegated certificate
without the usage, a tampered signature — is refused.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 4: CRL checking

**Files:**
- Create: `crl.go`
- Test: `crl_internal_test.go`

**Interfaces:**
- Consumes: `RevocationState` and its constants (Task 2).
- Produces: `func checkCRL(der []byte, cert, issuer *x509.Certificate) (RevocationState, time.Time, int, time.Time, error)` — returns state, revocation time, reason, the CRL's `ThisUpdate`, and an error explaining an Unknown.

- [ ] **Step 1: Write the failing test**

Create `crl_internal_test.go`:

```go
// SPDX-License-Identifier: MIT

package asposepdf

import (
	"crypto/rand"
	"crypto/x509"
	"math/big"
	"testing"
	"time"
)

// testCRL issues a CRL from the CA listing the given serials as revoked.
func testCRL(t *testing.T, ca *x509.Certificate, caKey interface{ Public() crypto.PublicKey }, revoked []x509.RevocationListEntry, nextUpdate time.Time) []byte {
	t.Helper()
	signer, ok := caKey.(crypto.Signer)
	if !ok {
		t.Fatal("CA key is not a signer")
	}
	tmpl := &x509.RevocationList{
		Number:                    big.NewInt(1),
		ThisUpdate:                time.Now().Add(-time.Minute),
		NextUpdate:                nextUpdate,
		RevokedCertificateEntries: revoked,
	}
	der, err := x509.CreateRevocationList(rand.Reader, tmpl, ca, signer)
	if err != nil {
		t.Fatal(err)
	}
	return der
}

func TestCheckCRLNotListed(t *testing.T) {
	ca, caKey := ocspTestCA(t)
	leaf := ocspTestLeaf(t, ca, caKey, 31, "", "")
	der := testCRL(t, ca, caKey, nil, time.Now().Add(24*time.Hour))

	state, _, _, _, err := checkCRL(der, leaf, ca)
	if err != nil {
		t.Fatal(err)
	}
	if state != RevocationGood {
		t.Errorf("state = %v, want good for a serial the CRL does not list", state)
	}
}

func TestCheckCRLListed(t *testing.T) {
	ca, caKey := ocspTestCA(t)
	leaf := ocspTestLeaf(t, ca, caKey, 32, "", "")
	when := time.Now().Add(-72 * time.Hour).Truncate(time.Second)
	der := testCRL(t, ca, caKey, []x509.RevocationListEntry{
		{SerialNumber: leaf.SerialNumber, RevocationTime: when, ReasonCode: 4 /* superseded */},
	}, time.Now().Add(24*time.Hour))

	state, revokedAt, reason, _, err := checkCRL(der, leaf, ca)
	if err != nil {
		t.Fatal(err)
	}
	if state != RevocationRevoked {
		t.Fatalf("state = %v, want revoked", state)
	}
	if !revokedAt.Equal(when.UTC()) {
		t.Errorf("revokedAt = %v, want %v", revokedAt, when.UTC())
	}
	if reason != 4 {
		t.Errorf("reason = %d, want 4 (superseded)", reason)
	}
}

// A CRL from another CA says nothing about this certificate.
func TestCheckCRLWrongIssuer(t *testing.T) {
	ca, caKey := ocspTestCA(t)
	other, otherKey := ocspTestCA(t)
	leaf := ocspTestLeaf(t, ca, caKey, 33, "", "")
	der := testCRL(t, other, otherKey, nil, time.Now().Add(24*time.Hour))

	if _, _, _, _, err := checkCRL(der, leaf, ca); err == nil {
		t.Fatal("a CRL issued by another CA was accepted")
	}
}

// An expired CRL is not evidence either.
func TestCheckCRLExpired(t *testing.T) {
	ca, caKey := ocspTestCA(t)
	leaf := ocspTestLeaf(t, ca, caKey, 34, "", "")
	der := testCRL(t, ca, caKey, nil, time.Now().Add(-time.Hour))

	state, _, _, _, err := checkCRL(der, leaf, ca)
	if err == nil {
		t.Fatal("an expired CRL was accepted")
	}
	if state != RevocationUnknown {
		t.Errorf("state = %v, want unknown", state)
	}
}
```

Add `"crypto"` to the import block of this file.

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test -run TestCheckCRL .`
Expected: FAIL — `undefined: checkCRL`.

- [ ] **Step 3: Write the implementation**

Create `crl.go`:

```go
// SPDX-License-Identifier: MIT

package asposepdf

import (
	"crypto/x509"
	"fmt"
	"time"
)

// Certificate revocation lists (RFC 5280 §5). The parsing is the standard
// library's; what is added here is the part that decides whether a list is
// evidence about a particular certificate: it must be signed by that
// certificate's issuer and it must not have expired.

// checkCRL reports what the list says about cert. The returned time is the
// revocation time (zero unless revoked), the int is the CRL reason code, and
// the last time is the list's ThisUpdate, which callers record as the moment
// the answer was produced.
func checkCRL(der []byte, cert, issuer *x509.Certificate) (RevocationState, time.Time, int, time.Time, error) {
	if cert == nil || issuer == nil {
		return RevocationUnknown, time.Time{}, 0, time.Time{}, fmt.Errorf("crl: nil certificate or issuer")
	}
	list, err := x509.ParseRevocationList(der)
	if err != nil {
		return RevocationUnknown, time.Time{}, 0, time.Time{}, fmt.Errorf("crl: parse: %w", err)
	}
	if err := list.CheckSignatureFrom(issuer); err != nil {
		return RevocationUnknown, time.Time{}, 0, time.Time{}, fmt.Errorf("crl: not issued by %s: %w", issuer.Subject.CommonName, err)
	}
	now := time.Now()
	if !list.NextUpdate.IsZero() && now.After(list.NextUpdate) {
		return RevocationUnknown, time.Time{}, 0, list.ThisUpdate,
			fmt.Errorf("crl: expired at %s", list.NextUpdate.Format(time.RFC3339))
	}
	for _, entry := range list.RevokedCertificateEntries {
		if entry.SerialNumber != nil && entry.SerialNumber.Cmp(cert.SerialNumber) == 0 {
			return RevocationRevoked, entry.RevocationTime, entry.ReasonCode, list.ThisUpdate, nil
		}
	}
	return RevocationGood, time.Time{}, 0, list.ThisUpdate, nil
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test -run TestCheckCRL .`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add crl.go crl_internal_test.go
git commit -m "$(cat <<'EOF'
feat: CRL revocation checking (pdf-go-x2s6)

The parsing is the standard library's; what is added is what makes a list
evidence about a particular certificate — it must be signed by that
certificate's issuer, and it must not have expired. Either failure yields
unknown with the reason rather than a comfortable "not listed, so good".

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 5: Fetcher, orchestration and the public status type

**Files:**
- Create: `revocation.go`
- Test: `revocation_test.go` (package `asposepdf_test`)

**Interfaces:**
- Consumes: `buildOCSPRequest`, `parseOCSPResponse`, `ocspSingleFor`, `ocspStatusOf`, `verifyOCSPResponse`, `buildOCSPCertID` (Tasks 1-3); `checkCRL` (Task 4); `RevocationState` (Task 2).
- Produces:
  - `type RevocationSource int` with `RevocationFromDocument`, `RevocationFromOCSP`, `RevocationFromCRL`
  - `type RevocationStatus struct { Status RevocationState; Source RevocationSource; RevokedAt time.Time; Reason int; ProducedAt time.Time; Err error }`
  - `type RevocationCheck int` with `RevocationNone`, `RevocationOffline`, `RevocationOnline`
  - `type RevocationFetcher interface { FetchOCSP(ctx context.Context, url string, req []byte) ([]byte, error); FetchCRL(ctx context.Context, url string) ([]byte, error) }`
  - `type ValidationOptions struct { Revocation RevocationCheck; Fetcher RevocationFetcher; Timeout time.Duration }`
  - `func checkRevocation(cert, issuer *x509.Certificate, embedded [][]byte, o ValidationOptions) *RevocationStatus`

- [ ] **Step 1: Write the failing test**

Create `revocation_test.go`:

```go
// SPDX-License-Identifier: MIT

package asposepdf_test

import (
	"context"
	"errors"
	"testing"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

// refusingFetcher fails the test if anything reaches for the network.
type refusingFetcher struct{ t *testing.T }

func (f refusingFetcher) FetchOCSP(ctx context.Context, url string, req []byte) ([]byte, error) {
	f.t.Errorf("an OCSP request went out to %s during an offline check", url)
	return nil, errors.New("refused")
}

func (f refusingFetcher) FetchCRL(ctx context.Context, url string) ([]byte, error) {
	f.t.Errorf("a CRL request went out to %s during an offline check", url)
	return nil, errors.New("refused")
}

// The zero ValidationOptions must check nothing, so every existing call keeps
// its behaviour and no library user is surprised by outbound traffic.
func TestValidationOptionsZeroValueChecksNothing(t *testing.T) {
	var o pdf.ValidationOptions
	if o.Revocation != pdf.RevocationNone {
		t.Errorf("zero Revocation = %v, want RevocationNone", o.Revocation)
	}
}

// An offline check must never call the fetcher.
func TestRevocationOfflineDoesNotFetch(t *testing.T) {
	doc, _ := signedTestDocument(t)
	sigs, err := doc.VerifySignatures(pdf.ValidationOptions{
		Revocation: pdf.RevocationOffline,
		Fetcher:    refusingFetcher{t},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(sigs) != 1 {
		t.Fatalf("got %d signatures, want 1", len(sigs))
	}
	if sigs[0].Revocation == nil {
		t.Fatal("no revocation status reported")
	}
	// A self-signed document carries no revocation material, so the honest
	// answer is unknown — with a reason.
	if sigs[0].Revocation.Status != pdf.RevocationUnknown {
		t.Errorf("status = %v, want unknown for a document with no material", sigs[0].Revocation.Status)
	}
	if sigs[0].Revocation.Err == nil {
		t.Error("an unknown status came with no explanation")
	}
	// And it must not have touched Valid.
	if !sigs[0].Valid {
		t.Errorf("revocation checking changed Valid: %v", sigs[0].Err)
	}
}
```

This test needs a signed document. Add the helper to the same file:

```go
// signedTestDocument returns a one-page document signed by a throwaway
// self-signed certificate, reopened from its own bytes.
func signedTestDocument(t *testing.T) (*pdf.Document, *x509.Certificate) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	cert := newSelfSigned(t, key)
	doc := pdf.NewDocument(400, 200)
	page, _ := doc.Page(1)
	if err := page.AddText("Signed", pdf.TextStyle{Font: pdf.FontHelvetica, Size: 14},
		pdf.Rectangle{LLX: 20, LLY: 100, URX: 380, URY: 140}); err != nil {
		t.Fatal(err)
	}
	if err := doc.Sign(pdf.SignOptions{Certificate: cert, PrivateKey: key, Name: "Signer"}); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	signed, err := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	return signed, cert
}
```

Add the imports it needs: `"bytes"`, `"crypto/rand"`, `"crypto/rsa"`, `"crypto/x509"`. `newSelfSigned` already exists in `sign_test.go`, same package.

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test -run "TestValidationOptions|TestRevocationOffline" .`
Expected: FAIL — `undefined: pdf.ValidationOptions`.

- [ ] **Step 3: Write the implementation**

Create `revocation.go`:

```go
// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"context"
	"crypto/x509"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Revocation checking (epic pdf-go-x2s6). Whether a certificate was revoked
// is answered by its issuer, over OCSP (RFC 6960) or through a CRL
// (RFC 5280). Both are reached through RevocationFetcher, which is how tests
// avoid the network and how a caller with a proxy or a cache substitutes its
// own transport.
//
// Network access is opt-in: the zero ValidationOptions checks nothing, and
// RevocationOffline uses only material the document already carries. A
// library that quietly contacted a CA would also leak which documents are
// being checked, and when.

// RevocationCheck selects how far verification goes in establishing whether
// a signing certificate was revoked.
type RevocationCheck int

const (
	// RevocationNone performs no check. The zero value.
	RevocationNone RevocationCheck = iota
	// RevocationOffline uses only revocation material the document carries.
	RevocationOffline
	// RevocationOnline additionally fetches what the document lacks, from the
	// OCSP responder and CRL distribution points named in the certificate.
	RevocationOnline
)

// RevocationSource records where an answer came from.
type RevocationSource int

const (
	// RevocationFromDocument — material embedded in the PDF.
	RevocationFromDocument RevocationSource = iota
	// RevocationFromOCSP — a responder answered.
	RevocationFromOCSP
	// RevocationFromCRL — a revocation list was consulted.
	RevocationFromCRL
)

// RevocationStatus is what the issuer says about a signing certificate.
type RevocationStatus struct {
	Status     RevocationState
	Source     RevocationSource
	RevokedAt  time.Time // set only when Status is RevocationRevoked
	Reason     int       // CRL reason code (RFC 5280 §5.3.1); 0 when unspecified
	ProducedAt time.Time // when the answer was produced
	Err        error     // why the status is unknown
}

// RevocationFetcher retrieves revocation material. Implement it to route
// through a proxy, serve from a cache, or refuse the network entirely.
type RevocationFetcher interface {
	// FetchOCSP posts a DER OCSPRequest to url and returns the DER response.
	FetchOCSP(ctx context.Context, url string, req []byte) ([]byte, error)
	// FetchCRL retrieves the DER CRL at url.
	FetchCRL(ctx context.Context, url string) ([]byte, error)
}

// ValidationOptions configures signature verification. The zero value checks
// no revocation, so existing calls behave exactly as before. Mirrors the
// intent of Aspose.PDF for .NET's ValidationOptions / OcspSettings.
type ValidationOptions struct {
	Revocation RevocationCheck
	Fetcher    RevocationFetcher // nil uses the built-in HTTP client
	Timeout    time.Duration     // per request; zero means 10 seconds
}

// httpRevocationFetcher is the built-in transport.
type httpRevocationFetcher struct{ client *http.Client }

func (f httpRevocationFetcher) FetchOCSP(ctx context.Context, url string, req []byte) ([]byte, error) {
	r, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(req))
	if err != nil {
		return nil, err
	}
	r.Header.Set("Content-Type", "application/ocsp-request")
	return f.do(r)
}

func (f httpRevocationFetcher) FetchCRL(ctx context.Context, url string) ([]byte, error) {
	r, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	return f.do(r)
}

func (f httpRevocationFetcher) do(r *http.Request) ([]byte, error) {
	resp, err := f.client.Do(r)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: HTTP %d", r.URL, resp.StatusCode)
	}
	// A revocation answer is small; a hostile or broken endpoint must not be
	// able to hand us a gigabyte.
	return io.ReadAll(io.LimitReader(resp.Body, 10<<20))
}

// fetcherFor returns the fetcher to use and the timeout to bound it with.
func (o ValidationOptions) fetcherFor() (RevocationFetcher, time.Duration) {
	timeout := o.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	if o.Fetcher != nil {
		return o.Fetcher, timeout
	}
	return httpRevocationFetcher{client: &http.Client{Timeout: timeout}}, timeout
}

// checkRevocation answers for one certificate. embedded holds revocation
// material already in the document (DER OCSP responses and CRLs); it is tried
// first, and it is all that is tried when the mode is RevocationOffline.
func checkRevocation(cert, issuer *x509.Certificate, embedded [][]byte, o ValidationOptions) *RevocationStatus {
	if o.Revocation == RevocationNone {
		return nil
	}
	if cert == nil || issuer == nil {
		return &RevocationStatus{Err: fmt.Errorf("revocation: no issuer certificate for %s", certName(cert))}
	}

	// Material the document carries, whichever kind it is.
	for _, der := range embedded {
		if st := statusFromOCSPDER(der, cert, issuer); st != nil {
			st.Source = RevocationFromDocument
			return st
		}
		if state, revokedAt, reason, thisUpdate, err := checkCRL(der, cert, issuer); err == nil {
			return &RevocationStatus{
				Status: state, Source: RevocationFromDocument,
				RevokedAt: revokedAt, Reason: reason, ProducedAt: thisUpdate,
			}
		}
	}

	if o.Revocation != RevocationOnline {
		return &RevocationStatus{Err: fmt.Errorf("revocation: the document carries no usable material for %s", certName(cert))}
	}

	fetcher, timeout := o.fetcherFor()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	var lastErr error
	for _, url := range cert.OCSPServer {
		req, err := buildOCSPRequest(cert, issuer)
		if err != nil {
			lastErr = err
			break
		}
		der, err := fetcher.FetchOCSP(ctx, url, req)
		if err != nil {
			lastErr = fmt.Errorf("revocation: OCSP %s: %w", url, err)
			continue
		}
		if st := statusFromOCSPDER(der, cert, issuer); st != nil {
			st.Source = RevocationFromOCSP
			return st
		}
		lastErr = fmt.Errorf("revocation: OCSP %s returned no usable answer", url)
	}

	for _, url := range cert.CRLDistributionPoints {
		der, err := fetcher.FetchCRL(ctx, url)
		if err != nil {
			lastErr = fmt.Errorf("revocation: CRL %s: %w", url, err)
			continue
		}
		state, revokedAt, reason, thisUpdate, err := checkCRL(der, cert, issuer)
		if err != nil {
			lastErr = fmt.Errorf("revocation: CRL %s: %w", url, err)
			continue
		}
		return &RevocationStatus{
			Status: state, Source: RevocationFromCRL,
			RevokedAt: revokedAt, Reason: reason, ProducedAt: thisUpdate,
		}
	}

	if lastErr == nil {
		lastErr = fmt.Errorf("revocation: %s names no OCSP responder or CRL distribution point", certName(cert))
	}
	return &RevocationStatus{Err: lastErr}
}

// statusFromOCSPDER interprets der as an OCSP response about cert, returning
// nil when it is not one or does not answer about this certificate. A
// response that parses but fails verification yields an unknown status
// carrying the reason — never silence.
func statusFromOCSPDER(der []byte, cert, issuer *x509.Certificate) *RevocationStatus {
	basic, data, err := parseOCSPResponse(der)
	if err != nil {
		return nil
	}
	if err := verifyOCSPResponse(basic, data, issuer); err != nil {
		return &RevocationStatus{Err: err}
	}
	id, err := buildOCSPCertID(cert, issuer)
	if err != nil {
		return &RevocationStatus{Err: err}
	}
	single, err := ocspSingleFor(data, id)
	if err != nil {
		return nil
	}
	state, revokedAt, reason, err := ocspStatusOf(single)
	return &RevocationStatus{
		Status: state, RevokedAt: revokedAt, Reason: reason,
		ProducedAt: data.ProducedAt, Err: err,
	}
}

// certName is a readable name for error messages.
func certName(cert *x509.Certificate) string {
	if cert == nil {
		return "the certificate"
	}
	if cert.Subject.CommonName != "" {
		return cert.Subject.CommonName
	}
	return cert.SerialNumber.String()
}
```

- [ ] **Step 4: Wire the options into verification**

In `sign_verify.go`, change the two entry points to take options and pass them down, and add the two result fields. Replace the `VerifySignatures` and `VerifySignature` signatures with:

```go
func (d *Document) VerifySignatures(opts ...ValidationOptions) ([]SignatureVerification, error) {
	if len(d.source) == 0 {
		return nil, fmt.Errorf("VerifySignatures: no source bytes (open the document from a file or stream)")
	}
	o := lastValidationOption(opts)
	var out []SignatureVerification
	for _, sf := range d.collectSignatureFields() {
		out = append(out, d.verifyOneSignatureWithOptions(sf, o))
	}
	return out, nil
}

func (d *Document) VerifySignature(fieldName string, opts ...ValidationOptions) (SignatureVerification, error) {
	if len(d.source) == 0 {
		return SignatureVerification{}, fmt.Errorf("VerifySignature: no source bytes (open the document from a file or stream)")
	}
	o := lastValidationOption(opts)
	for _, sf := range d.collectSignatureFields() {
		if sf.name == fieldName {
			return d.verifyOneSignatureWithOptions(sf, o), nil
		}
	}
	return SignatureVerification{}, fmt.Errorf("VerifySignature: no signature field named %q", fieldName)
}

// lastValidationOption returns the effective options, last one winning.
func lastValidationOption(opts []ValidationOptions) ValidationOptions {
	if len(opts) == 0 {
		return ValidationOptions{}
	}
	return opts[len(opts)-1]
}

// verifyOneSignatureWithOptions runs the cryptographic verification and, when
// asked, the revocation check on top of it. Revocation never changes Valid:
// what to do about a revoked certificate is the caller's policy.
func (d *Document) verifyOneSignatureWithOptions(sf sigFieldRef, o ValidationOptions) SignatureVerification {
	res := verifyOneSignature(d.source, sf.name, sf.sig)
	if o.Revocation == RevocationNone || res.Certificate == nil {
		return res
	}
	issuer := issuerOf(res.Certificate, res.Chain)
	res.Revocation = checkRevocation(res.Certificate, issuer, d.embeddedRevocationMaterial(), o)
	return res
}

// issuerOf finds the certificate that signed cert among the ones travelling
// with the signature. A self-signed certificate is its own issuer.
func issuerOf(cert *x509.Certificate, chain []*x509.Certificate) *x509.Certificate {
	for _, c := range chain {
		if cert.CheckSignatureFrom(c) == nil {
			return c
		}
	}
	if cert.CheckSignatureFrom(cert) == nil {
		return cert
	}
	return nil
}
```

Add the two fields to `SignatureVerification`, next to `Chain`:

```go
	// Revocation is what the issuer says about the signing certificate. It is
	// nil unless ValidationOptions asked for a check. A revoked certificate
	// does not make Valid false: Valid stays cryptographic, and the policy is
	// the caller's.
	Revocation *RevocationStatus
	// LTVEnabled reports that the document carries everything needed to
	// verify this signature offline. Mirrors Aspose.PDF for .NET's
	// IsLtvEnabled.
	LTVEnabled bool
```

Add the document-material accessor to `revocation.go` — for now it reports nothing, and Task 8 fills it in from the `/DSS`:

```go
// embeddedRevocationMaterial returns the DER OCSP responses and CRLs the
// document carries. Task 8 feeds this from the /DSS; until then a document
// carries none.
func (d *Document) embeddedRevocationMaterial() [][]byte {
	return d.dssMaterial()
}
```

And in `dss.go`, which Task 7 creates, the stub belongs with the rest of the DSS code. Until Task 7 exists, put this temporary definition at the bottom of `revocation.go` and move it in Task 7:

```go
// dssMaterial returns the revocation material stored in the document's /DSS.
func (d *Document) dssMaterial() [][]byte { return nil }
```

- [ ] **Step 5: Run the tests**

Run: `go test -run "TestValidationOptions|TestRevocationOffline|TestSign|TestVerify" .`
Expected: PASS — including the existing signature tests, which must be unaffected.

- [ ] **Step 6: Commit**

```bash
git add revocation.go revocation_test.go sign_verify.go
git commit -m "$(cat <<'EOF'
feat: revocation checking wired into verification (pdf-go-x2s6)

ValidationOptions selects how far verification goes — nothing (the zero
value, so every existing call is unchanged), material already in the
document, or fetching what it lacks from the responder and distribution
points the certificate names. Fetching goes through RevocationFetcher, which
is how tests stay offline and how a caller substitutes a proxy or a cache.

Revocation never changes Valid. It stays cryptographic — the signature is
intact and covers the bytes — and what to do about a revoked certificate is
the caller's policy, not this library's.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 6: The built-in HTTP fetcher, tested against a local server

**Files:**
- Test: `revocation_test.go` (append)

**Interfaces:**
- Consumes: everything from Task 5. Produces no new API — this task proves the default transport works without ever leaving the machine.

- [ ] **Step 1: Write the failing test**

Append to `revocation_test.go` (add `"net/http"`, `"net/http/httptest"`):

```go
// The built-in fetcher must speak the wire protocol correctly: POST with the
// OCSP content type for a responder, GET for a CRL.
func TestBuiltinFetcherSpeaksHTTP(t *testing.T) {
	var gotMethod, gotContentType string
	var gotBody []byte
	ocspServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotContentType = r.Header.Get("Content-Type")
		gotBody, _ = io.ReadAll(r.Body)
		w.Write([]byte{0x30, 0x03, 0x0A, 0x01, 0x06}) // a minimal "unauthorized" response
	}))
	defer ocspServer.Close()

	crlServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("CRL fetched with %s, want GET", r.Method)
		}
		w.Write([]byte("not a crl"))
	}))
	defer crlServer.Close()

	f := pdf.DefaultRevocationFetcher(2 * time.Second)
	if _, err := f.FetchOCSP(context.Background(), ocspServer.URL, []byte{1, 2, 3}); err != nil {
		t.Fatalf("FetchOCSP: %v", err)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("OCSP fetched with %s, want POST", gotMethod)
	}
	if gotContentType != "application/ocsp-request" {
		t.Errorf("Content-Type = %q, want application/ocsp-request", gotContentType)
	}
	if string(gotBody) != string([]byte{1, 2, 3}) {
		t.Errorf("the request body did not arrive intact: %v", gotBody)
	}
	if _, err := f.FetchCRL(context.Background(), crlServer.URL); err != nil {
		t.Fatalf("FetchCRL: %v", err)
	}
}

// A responder that answers with an HTTP error is a failed fetch, not an
// empty answer that might read as "good".
func TestBuiltinFetcherRejectsHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusInternalServerError)
	}))
	defer server.Close()

	f := pdf.DefaultRevocationFetcher(2 * time.Second)
	if _, err := f.FetchCRL(context.Background(), server.URL); err == nil {
		t.Fatal("an HTTP 500 was accepted as a CRL")
	}
}
```

Add `"io"` and `"time"` to the imports.

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test -run TestBuiltinFetcher .`
Expected: FAIL — `undefined: pdf.DefaultRevocationFetcher`.

- [ ] **Step 3: Export the constructor**

Append to `revocation.go`:

```go
// DefaultRevocationFetcher returns the built-in HTTP fetcher, the one used
// when ValidationOptions.Fetcher is nil. Exported so a caller can wrap it —
// to add a cache, or to log what is fetched — rather than reimplement it.
// timeout bounds each request; zero means 10 seconds.
func DefaultRevocationFetcher(timeout time.Duration) RevocationFetcher {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return httpRevocationFetcher{client: &http.Client{Timeout: timeout}}
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test -run TestBuiltinFetcher .`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add revocation.go revocation_test.go
git commit -m "$(cat <<'EOF'
feat: expose the built-in revocation fetcher (pdf-go-x2s6)

DefaultRevocationFetcher is the transport used when none is supplied, now
exported so a caller can wrap it — adding a cache or logging what goes out —
instead of reimplementing the wire details. Tested against a local server:
POST with the OCSP content type, GET for a CRL, and an HTTP error treated as
a failed fetch rather than an empty answer that might read as good.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 7: Appending a revision, and the /DSS objects

**Files:**
- Create: `dss.go`
- Modify: `sign_incremental.go` (extract the revision writer)
- Test: `dss_test.go` (package `asposepdf_test`)

**Interfaces:**
- Consumes: `lastStartxref`, `writeIncrementalXref`, `xrefRow`, `(*Document).incrementalID`, `writeValue`, `deepCopyValue` (existing); `contentsBytes` (`sign_verify.go`).
- Produces:
  - `func (d *Document) appendRevision(baseNextID int, modified map[int]pdfValue, encState *encryptState) ([]byte, error)` in `sign_incremental.go`
  - `func (d *Document) buildDSS(material map[string]dssEntry) error` in `dss.go`, where `type dssEntry struct { Certs, OCSPs, CRLs [][]byte }` keyed by the `/VRI` key
  - `func vriKey(contents []byte) string`
  - `func (d *Document) dssMaterial() [][]byte` (moved here from `revocation.go`, now reading the `/DSS`)

- [ ] **Step 1: Extract the revision writer**

In `sign_incremental.go`, move the serialization tail of `buildIncrementalSignedPDF` — everything from `// --- Collect everything to emit` down to the `startxref` line — into a method, and call it from where that code was:

```go
// appendRevision writes d.source followed by one incremental revision
// carrying every object numbered at or above baseNextID in d.objects plus the
// modified values of existing objects, then a cross-reference table and a
// trailer pointing at the previous one. No earlier byte moves, which is what
// keeps existing signatures valid.
func (d *Document) appendRevision(baseNextID int, modified map[int]pdfValue, encState *encryptState) ([]byte, error) {
	prevXref, err := lastStartxref(d.source)
	if err != nil {
		return nil, err
	}

	type emitObj struct {
		num, gen int
		val      pdfValue
	}
	var emit []emitObj
	for n := baseNextID; n < d.nextID; n++ {
		if obj := d.objects[n]; obj != nil {
			emit = append(emit, emitObj{num: n, gen: 0, val: obj.Value})
		}
	}
	for num, val := range modified {
		gen := 0
		if obj := d.objects[num]; obj != nil {
			gen = obj.Gen
		}
		emit = append(emit, emitObj{num: num, gen: gen, val: val})
	}
	sort.Slice(emit, func(i, j int) bool { return emit[i].num < emit[j].num })

	var buf bytes.Buffer
	buf.Write(d.source)
	if d.source[len(d.source)-1] != '\n' {
		buf.WriteByte('\n')
	}

	identity := func(n int) int { return n }
	offsets := make(map[int]int64, len(emit))
	for _, e := range emit {
		offsets[e.num] = int64(buf.Len())
		var encFn func([]byte) ([]byte, error)
		if encState != nil {
			num, gen := e.num, e.gen
			encFn = func(b []byte) ([]byte, error) { return encState.encryptBytes(num, gen, b) }
		}
		fmt.Fprintf(&buf, "%d %d obj\n", e.num, e.gen)
		if err := writeValue(&buf, e.val, identity, encFn); err != nil {
			return nil, err
		}
		buf.WriteString("\nendobj\n")
	}

	size := d.origSize
	for _, e := range emit {
		if e.num+1 > size {
			size = e.num + 1
		}
	}
	rows := make([]xrefRow, len(emit))
	for i, e := range emit {
		rows[i] = xrefRow{num: e.num, gen: e.gen, off: offsets[e.num]}
	}
	xrefOff := int64(buf.Len())
	writeIncrementalXref(&buf, rows)

	id0, id1 := d.incrementalID()
	buf.WriteString("trailer\n<<")
	fmt.Fprintf(&buf, " /Size %d", size)
	fmt.Fprintf(&buf, " /Root %d 0 R", d.catalogNum)
	fmt.Fprintf(&buf, " /Prev %d", prevXref)
	if encState != nil && d.encryptObjNum > 0 {
		fmt.Fprintf(&buf, " /Encrypt %d 0 R", d.encryptObjNum)
	}
	buf.WriteString(" /ID [")
	writeHexBytes(&buf, id0)
	writeHexBytes(&buf, id1)
	buf.WriteString("] >>\n")
	fmt.Fprintf(&buf, "startxref\n%d\n%%%%EOF\n", xrefOff)
	return buf.Bytes(), nil
}
```

`buildIncrementalSignedPDF` then ends with:

```go
	out, err := d.appendRevision(baseNextID, modified, encState)
	if err != nil {
		return nil, err
	}
	return d.applySignature(out)
```

and its own `prevXref` computation moves into `appendRevision` — delete the now-duplicated lines from the signing path.

- [ ] **Step 2: Run the existing signature tests to prove the extraction changed nothing**

Run: `go test -run "TestSign|TestVerify" .`
Expected: PASS, exactly as before. If anything fails, the extraction is wrong — fix it before going further; this refactor must be behaviour-free.

- [ ] **Step 3: Write the failing DSS test**

Create `dss_test.go`:

```go
// SPDX-License-Identifier: MIT

package asposepdf_test

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"regexp"
	"strings"
	"testing"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

// The /DSS must appear in the catalog, carry the signer certificate, and be
// announced through the Adobe extension level Acrobat looks for.
func TestAddValidationInfoWritesDSS(t *testing.T) {
	doc, cert := signedTestDocument(t)
	if err := doc.AddValidationInfo(); err != nil {
		t.Fatalf("AddValidationInfo: %v", err)
	}
	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	out := buf.Bytes()

	for _, want := range []string{"/DSS", "/Certs", "/VRI", "/ADBE", "/ExtensionLevel 5"} {
		if !bytes.Contains(out, []byte(want)) {
			t.Errorf("the output does not carry %q", want)
		}
	}
	if !bytes.Contains(out, cert.Raw) {
		t.Error("the signer certificate was not embedded")
	}

	// The /VRI key is the uppercase SHA-1 of the signature /Contents octets.
	m := regexp.MustCompile(`/VRI\s*<<\s*/([0-9A-F]{40})`).FindSubmatch(out)
	if m == nil {
		t.Fatalf("no /VRI entry with a 40-hex-digit key in the output")
	}
	if _, err := hex.DecodeString(string(m[1])); err != nil {
		t.Errorf("the /VRI key is not hex: %v", err)
	}
	_ = sha1.Size
}

// Adding validation info must not disturb the signature it documents.
func TestAddValidationInfoKeepsSignatureValid(t *testing.T) {
	doc, _ := signedTestDocument(t)
	before, err := doc.VerifySignatures()
	if err != nil || len(before) != 1 || !before[0].Valid {
		t.Fatalf("the fixture is not a valid signed document: %v / %+v", err, before)
	}
	if err := doc.AddValidationInfo(); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	reopened, err := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	after, err := reopened.VerifySignatures()
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != 1 {
		t.Fatalf("got %d signatures after adding validation info, want 1", len(after))
	}
	if !after[0].Valid {
		t.Errorf("the signature broke: %v", after[0].Err)
	}
	if !after[0].IntegrityOK {
		t.Error("IntegrityOK = false after appending a revision")
	}
	// The signature covered the file as it was, so it no longer covers the
	// whole of the extended file — that is correct and expected.
	if strings.TrimSpace(after[0].FieldName) == "" {
		t.Error("the field name was lost")
	}
}
```

- [ ] **Step 4: Run the test to verify it fails**

Run: `go test -run TestAddValidationInfo .`
Expected: FAIL — `doc.AddValidationInfo undefined`.

- [ ] **Step 5: Write the implementation**

Create `dss.go`:

```go
// SPDX-License-Identifier: MIT

package asposepdf

import (
	"crypto/sha1"
	"fmt"
	"strings"
	"time"
)

// Long-term validation (ISO 32000-2 §12.8.4.3, ETSI EN 319 142 / PAdES B-LT).
// The certificates and the revocation material a verifier needs are written
// into the document itself, so the signature can still be validated after the
// certificate expires or the responder goes away. They are appended as a new
// revision, leaving every existing byte — and therefore every existing
// signature — untouched.

// dssEntry is the material belonging to one signature.
type dssEntry struct {
	Certs [][]byte // certificate DER
	OCSPs [][]byte // BasicOCSPResponse DER
	CRLs  [][]byte // CRL DER
}

// vriKey is the /VRI dictionary key for a signature: the uppercase base-16
// SHA-1 of its /Contents octets, exactly as stored.
func vriKey(contents []byte) string {
	sum := sha1.Sum(contents)
	return strings.ToUpper(fmt.Sprintf("%x", sum))
}

// buildDSS creates the /DSS object graph for the given per-signature material
// and attaches it to the catalog, together with the Adobe extension
// declaration Acrobat requires to recognise it in a PDF 1.7 file. The objects
// land in d.objects; the caller appends the revision.
func (d *Document) buildDSS(material map[string]dssEntry) error {
	if d.catalog == nil {
		return fmt.Errorf("AddValidationInfo: document has no catalog")
	}

	// One stream per distinct DER, shared between the global arrays and the
	// per-signature /VRI entries.
	refs := map[string]pdfRef{}
	streamRef := func(der []byte) pdfRef {
		k := string(der)
		if ref, ok := refs[k]; ok {
			return ref
		}
		num := d.nextID
		d.nextID++
		d.objects[num] = &pdfObject{Num: num, Value: &pdfStream{
			Dict: pdfDict{"/Length": len(der)},
			Data: append([]byte(nil), der...),
		}}
		ref := pdfRef{Num: num}
		refs[k] = ref
		return ref
	}

	var allCerts, allOCSPs, allCRLs pdfArray
	vri := pdfDict{}
	for key, e := range material {
		entry := pdfDict{"/TU": pdfDateString(time.Now())}
		var certs, ocsps, crls pdfArray
		for _, der := range e.Certs {
			r := streamRef(der)
			certs = append(certs, r)
			allCerts = append(allCerts, r)
		}
		for _, der := range e.OCSPs {
			r := streamRef(der)
			ocsps = append(ocsps, r)
			allOCSPs = append(allOCSPs, r)
		}
		for _, der := range e.CRLs {
			r := streamRef(der)
			crls = append(crls, r)
			allCRLs = append(allCRLs, r)
		}
		if len(certs) > 0 {
			entry["/Cert"] = certs
		}
		if len(ocsps) > 0 {
			entry["/OCSP"] = ocsps
		}
		if len(crls) > 0 {
			entry["/CRL"] = crls
		}
		vri[pdfName("/"+key)] = entry
	}

	dss := pdfDict{}
	if len(allCerts) > 0 {
		dss["/Certs"] = allCerts
	}
	if len(allOCSPs) > 0 {
		dss["/OCSPs"] = allOCSPs
	}
	if len(allCRLs) > 0 {
		dss["/CRLs"] = allCRLs
	}
	if len(vri) > 0 {
		dss["/VRI"] = vri
	}

	dssNum := d.nextID
	d.nextID++
	d.objects[dssNum] = &pdfObject{Num: dssNum, Value: dss}
	d.catalog["/DSS"] = pdfRef{Num: dssNum}

	// Acrobat only recognises a /DSS in a 1.7 file when the extension is
	// declared.
	d.catalog["/Extensions"] = pdfDict{
		"/ADBE": pdfDict{
			"/BaseVersion":   pdfName("/1.7"),
			"/ExtensionLevel": 5,
		},
	}
	return nil
}

// dssMaterial returns every OCSP response and CRL the document carries in its
// /DSS, as DER. Order is unspecified; callers try each in turn.
func (d *Document) dssMaterial() [][]byte {
	dss, ok := resolveRefToDict(d.objects, d.catalog["/DSS"])
	if !ok {
		return nil
	}
	var out [][]byte
	for _, key := range []pdfName{"/OCSPs", "/CRLs"} {
		arr := d.resolveArray(dss[key])
		for _, v := range arr {
			ref, ok := v.(pdfRef)
			if !ok {
				continue
			}
			obj := d.objects[ref.Num]
			if obj == nil {
				continue
			}
			st, ok := obj.Value.(*pdfStream)
			if !ok {
				continue
			}
			data := decodedStreamData(st)
			if len(data) == 0 {
				continue
			}
			out = append(out, data)
		}
	}
	return out
}
```

Delete the temporary `dssMaterial` stub from `revocation.go`.

- [ ] **Step 6: Add AddValidationInfo**

Append to `dss.go`:

```go
// AddValidationInfo collects the certificates and revocation material for
// every signature in the document and writes them into the catalog as /DSS,
// appended as a new revision so signatures already in the file stay valid.
// This is long-term validation: the evidence travels with the document, so it
// still verifies once the certificate expires or the responder is gone.
//
// The document must have been opened from a file or stream. Use
// ValidationOptions to allow the network — with the zero value nothing is
// fetched, and only material the document already carries is recorded.
// Mirrors the intent of Aspose.PDF for .NET's LTV support.
func (d *Document) AddValidationInfo(opts ...ValidationOptions) error {
	if len(d.source) == 0 {
		return fmt.Errorf("AddValidationInfo: requires a document opened from an existing PDF (Open/OpenStream)")
	}
	o := lastValidationOption(opts)
	fields := d.collectSignatureFields()
	if len(fields) == 0 {
		return fmt.Errorf("AddValidationInfo: the document carries no signatures")
	}

	material := map[string]dssEntry{}
	for _, sf := range fields {
		res := verifyOneSignature(d.source, sf.name, sf.sig)
		if res.Certificate == nil {
			continue
		}
		entry := dssEntry{Certs: [][]byte{res.Certificate.Raw}}
		for _, c := range res.Chain {
			entry.Certs = append(entry.Certs, c.Raw)
		}
		if o.Revocation == RevocationOnline {
			issuer := issuerOf(res.Certificate, res.Chain)
			if issuer != nil {
				if der, kind, err := fetchRevocationDER(res.Certificate, issuer, o); err == nil {
					switch kind {
					case RevocationFromOCSP:
						entry.OCSPs = append(entry.OCSPs, der)
					case RevocationFromCRL:
						entry.CRLs = append(entry.CRLs, der)
					}
				}
			}
		}
		material[vriKey(contentsBytes(sf.sig["/Contents"]))] = entry
	}
	if len(material) == 0 {
		return fmt.Errorf("AddValidationInfo: no signature carried a usable certificate")
	}

	if d.nextID < d.origSize {
		d.nextID = d.origSize
	}
	baseNextID := d.nextID
	if err := d.buildDSS(material); err != nil {
		return err
	}
	cat := deepCopyValue(pdfValue(d.catalog)).(pdfDict)
	cat["/Type"] = pdfName("/Catalog")
	modified := map[int]pdfValue{d.catalogNum: cat}

	var encState *encryptState
	if d.preserved != nil {
		encState = d.preserved
	}
	out, err := d.appendRevision(baseNextID, modified, encState)
	if err != nil {
		return err
	}
	d.source = out
	return nil
}
```

`AddValidationInfo` replaces `d.source`, so a subsequent `WriteTo`/`Save` must emit those bytes rather than rebuilding the document. Add that to `(*Document).WriteTo` in `document.go`, right at the top:

```go
	// A document whose source was replaced by an appended revision (LTV) is
	// written back verbatim: rebuilding it would drop the revision.
	if d.revisionAppended {
		n, err := w.Write(d.source)
		return int64(n), err
	}
```

with a new `revisionAppended bool` field on `Document`, set to true at the end of `AddValidationInfo`.

- [ ] **Step 7: Add the fetch helper**

Append to `revocation.go`:

```go
// fetchRevocationDER retrieves revocation material for cert and returns it
// verbatim, so it can be stored in a /DSS. Unlike checkRevocation, which
// interprets the answer, this keeps the bytes.
func fetchRevocationDER(cert, issuer *x509.Certificate, o ValidationOptions) ([]byte, RevocationSource, error) {
	fetcher, timeout := o.fetcherFor()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	for _, url := range cert.OCSPServer {
		req, err := buildOCSPRequest(cert, issuer)
		if err != nil {
			break
		}
		der, err := fetcher.FetchOCSP(ctx, url, req)
		if err != nil {
			continue
		}
		basic, data, err := parseOCSPResponse(der)
		if err != nil {
			continue
		}
		if err := verifyOCSPResponse(basic, data, issuer); err != nil {
			continue
		}
		// Store the BasicOCSPResponse, which is what /OCSPs holds.
		return basic.raw(), RevocationFromOCSP, nil
	}
	for _, url := range cert.CRLDistributionPoints {
		der, err := fetcher.FetchCRL(ctx, url)
		if err != nil {
			continue
		}
		if _, _, _, _, err := checkCRL(der, cert, issuer); err != nil {
			continue
		}
		return der, RevocationFromCRL, nil
	}
	return nil, RevocationFromDocument, fmt.Errorf("revocation: nothing to store for %s", certName(cert))
}
```

and give the basic response a way to hand back its own DER, in `ocsp.go`:

```go
// raw re-encodes the BasicOCSPResponse, which is the form /DSS /OCSPs holds.
func (b *ocspBasicResponse) raw() []byte {
	der, err := asn1.Marshal(*b)
	if err != nil {
		return nil
	}
	return der
}
```

- [ ] **Step 8: Run the tests**

Run: `go test -run "TestAddValidationInfo|TestSign|TestVerify" .`
Expected: PASS.

- [ ] **Step 9: Commit**

```bash
git add dss.go dss_test.go revocation.go ocsp.go sign_incremental.go document.go
git commit -m "$(cat <<'EOF'
feat: write long-term validation material into the document (pdf-go-x2s6)

AddValidationInfo collects the certificates — and, when the network is
allowed, the revocation material — for every signature and writes them into
the catalog as /DSS with per-signature /VRI entries, appended as a new
revision so nothing already in the file moves and every existing signature
stays valid. The Adobe extension level is declared alongside, without which
Acrobat does not recognise a /DSS in a 1.7 file.

The revision-appending tail of the incremental signer is now a method both
paths share; the extraction is behaviour-free and the signing tests prove it.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 8: LTV end to end — offline verification, the SignOptions flag, documentation

**Files:**
- Modify: `sign.go`, `sign_verify.go`, `dss.go`
- Test: `dss_test.go` (append)
- Modify: `CLAUDE.md`, `README.md`, `CHANGELOG.md`

**Interfaces:**
- Consumes: everything above.
- Produces: `SignOptions.LTV bool`; `SignatureVerification.LTVEnabled` populated; no other new API.

- [ ] **Step 1: Write the failing test**

Append to `dss_test.go` (add `"context"`, `"crypto/x509"`, `"errors"`, `"time"`, and `"net/http"`/`"net/http/httptest"` if not already imported):

```go
// A document carrying its own revocation material verifies offline: the
// fetcher must never be called, and the status must come from the document.
func TestOfflineVerificationUsesEmbeddedMaterial(t *testing.T) {
	doc, _ := signedTestDocumentWithCA(t)
	if err := doc.AddValidationInfo(pdf.ValidationOptions{
		Revocation: pdf.RevocationOnline,
		Fetcher:    fakeOCSPFetcher(t),
	}); err != nil {
		t.Fatalf("AddValidationInfo: %v", err)
	}
	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	reopened, err := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}

	sigs, err := reopened.VerifySignatures(pdf.ValidationOptions{
		Revocation: pdf.RevocationOffline,
		Fetcher:    refusingFetcher{t}, // fails the test if called
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(sigs) != 1 {
		t.Fatalf("got %d signatures, want 1", len(sigs))
	}
	st := sigs[0].Revocation
	if st == nil {
		t.Fatal("no revocation status")
	}
	if st.Status != pdf.RevocationGood {
		t.Errorf("status = %v (%v), want good from the embedded response", st.Status, st.Err)
	}
	if st.Source != pdf.RevocationFromDocument {
		t.Errorf("source = %v, want FromDocument", st.Source)
	}
	if !sigs[0].LTVEnabled {
		t.Error("LTVEnabled = false for a document carrying its own material")
	}
}
```

This needs a document signed by a real CA (so there is an issuer to check against) and a fetcher that answers OCSP from that CA. Add both helpers to `dss_test.go`; they mirror the internal test helpers but live in the external test package, so they build their own certificates:

```go
// signedTestDocumentWithCA signs a document with a certificate issued by a CA
// that names an OCSP responder, and returns the document plus the CA.
func signedTestDocumentWithCA(t *testing.T) (*pdf.Document, *x509.Certificate) {
	t.Helper()
	caKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	caTmpl := x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "LTV Test CA"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().AddDate(1, 0, 0),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, &caTmpl, &caTmpl, caKey.Public(), caKey)
	if err != nil {
		t.Fatal(err)
	}
	ca, err := x509.ParseCertificate(caDER)
	if err != nil {
		t.Fatal(err)
	}

	leafKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	leafTmpl := x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "LTV Test Signer"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().AddDate(1, 0, 0),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		OCSPServer:   []string{"http://ocsp.example.invalid"},
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, &leafTmpl, ca, leafKey.Public(), caKey)
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := x509.ParseCertificate(leafDER)
	if err != nil {
		t.Fatal(err)
	}

	doc := pdf.NewDocument(400, 200)
	page, _ := doc.Page(1)
	if err := page.AddText("LTV", pdf.TextStyle{Font: pdf.FontHelvetica, Size: 14},
		pdf.Rectangle{LLX: 20, LLY: 100, URX: 380, URY: 140}); err != nil {
		t.Fatal(err)
	}
	if err := doc.Sign(pdf.SignOptions{
		Certificate: leaf, PrivateKey: leafKey,
		Chain: []*x509.Certificate{ca}, Name: "LTV Signer",
	}); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	signed, err := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	// Keep the CA key reachable for the fake responder.
	ltvCAKey = caKey
	ltvCA = ca
	return signed, ca
}

var (
	ltvCA    *x509.Certificate
	ltvCAKey *rsa.PrivateKey
)
```

The fake responder itself cannot reuse the internal helpers (different package), so it builds the response through the public path of an `httptest` server that a small OCSP encoder in the test drives. Rather than duplicate the encoder, **run this test in the internal package instead**: move `TestOfflineVerificationUsesEmbeddedMaterial`, `signedTestDocumentWithCA` and the fake responder into a new file `dss_internal_test.go` (package `asposepdf`), where `ocspTestResponse` from Task 2 is already available, and have the fetcher return `ocspTestResponse(t, ca, caKey, leaf, "good", …)`.

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test -run TestOfflineVerificationUsesEmbeddedMaterial .`
Expected: FAIL — `LTVEnabled` is false and the status is unknown, because nothing populates them yet.

- [ ] **Step 3: Populate LTVEnabled**

In `sign_verify.go`, inside `verifyOneSignatureWithOptions`, after the revocation check:

```go
	res.LTVEnabled = d.hasLTVMaterial(sf, res)
```

and add to `dss.go`:

```go
// hasLTVMaterial reports whether the document carries what a verifier needs
// to check this signature without a network: the signer certificate and a
// revocation answer covering it.
func (d *Document) hasLTVMaterial(sf sigFieldRef, res SignatureVerification) bool {
	if res.Certificate == nil {
		return false
	}
	dss, ok := resolveRefToDict(d.objects, d.catalog["/DSS"])
	if !ok {
		return false
	}
	if len(d.resolveArray(dss["/Certs"])) == 0 {
		return false
	}
	issuer := issuerOf(res.Certificate, res.Chain)
	if issuer == nil {
		return false
	}
	for _, der := range d.dssMaterial() {
		if st := statusFromOCSPDER(der, res.Certificate, issuer); st != nil && st.Err == nil {
			return true
		}
		if _, _, _, _, err := checkCRL(der, res.Certificate, issuer); err == nil {
			return true
		}
	}
	return false
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test -run TestOfflineVerificationUsesEmbeddedMaterial .`
Expected: PASS.

- [ ] **Step 5: Add the SignOptions.LTV convenience**

In `sign.go`, add the field next to `TimestampURL`:

```go
	// LTV appends a second revision carrying the long-term validation
	// material (/DSS) after the signature is written, so the signature can be
	// verified once the certificate expires. Requires ValidationOptions-level
	// network access through AddValidationInfo, which it calls with
	// RevocationOnline; without a reachable responder only the certificates
	// are embedded.
	LTV bool
```

carry it into `signConfig` as `ltv bool`, and in the save path — `document.go`, where the signed bytes are produced — follow the signature with:

```go
	if d.sign != nil && d.sign.ltv {
		reopened, err := OpenStream(bytes.NewReader(out))
		if err != nil {
			return nil, fmt.Errorf("sign: reopen for LTV: %w", err)
		}
		if err := reopened.AddValidationInfo(ValidationOptions{Revocation: RevocationOnline}); err != nil {
			return nil, fmt.Errorf("sign: LTV: %w", err)
		}
		out = reopened.source
	}
```

Add a test to `dss_internal_test.go` asserting that a document signed with `LTV: true` comes back carrying a `/DSS` and still verifies.

- [ ] **Step 6: Run the whole suite, vet and lint**

Run: `gofmt -l . ; go vet ./... && go test ./... && golangci-lint run`
Expected: `gofmt` lists none of the files this plan touched, vet silent, tests pass, lint reports `0 issues`.

- [ ] **Step 7: Cross-check the OCSP encoding with OpenSSL**

Our OCSP request must be readable by something that is not us. Write the request produced by `buildOCSPRequest` to `result_files/ocsp/req.der` from a temporary test, then:

```bash
openssl ocsp -reqin result_files/ocsp/req.der -req_text -noverify | head -20
```

Expected: OpenSSL prints the request — `Version: 1 (0x0)`, a `Certificate ID` with the SHA-1 hashes and the serial. If it cannot parse it, the DER is wrong regardless of what our own round-trip test says. Record the output in the commit message and delete the temporary test.

- [ ] **Step 8: Documentation**

`CLAUDE.md`, in the signatures section, after the `VerifySignatures` bullet:

```markdown
- **Revocation and LTV** (`ocsp.go` / `crl.go` / `revocation.go` / `dss.go`, epic `pdf-go-x2s6`; design `docs/superpowers/specs/2026-09-16-signature-revocation-ltv-design.md`): `VerifySignatures`/`VerifySignature` take `ValidationOptions{Revocation, Fetcher, Timeout}`. `RevocationNone` (zero value) checks nothing, `RevocationOffline` uses only material the document carries, `RevocationOnline` also fetches from the certificate's OCSP responder and CRL distribution points. The result gains `Revocation *RevocationStatus{Status, Source, RevokedAt, Reason, ProducedAt, Err}` and `LTVEnabled bool` (mirrors Aspose's `IsLtvEnabled`). **`Valid` keeps its cryptographic meaning** — a revoked certificate never flips it, because the policy is the caller's. OCSP is hand-rolled on `encoding/asn1` beside the CMS code (`buildOCSPRequest`, `parseOCSPResponse`, `verifyOCSPResponse`): a response counts only when signed by the issuer or by a delegated responder the issuer signed carrying `id-kp-OCSPSigning`, and an expired or future-dated answer is `Unknown` with the reason, never a comfortable "good". CRLs go through `x509.ParseRevocationList` plus the same issuer-signature and freshness checks. Fetching is behind `RevocationFetcher` (the built-in transport is `DefaultRevocationFetcher`), which is how tests stay offline and how a caller inserts a proxy or cache. `(*Document).AddValidationInfo(opts...)` writes the certificates and revocation material into the catalog as `/DSS` with per-signature `/VRI` (key: uppercase SHA-1 of the signature's `/Contents` octets) plus the `/ADBE` extension-level-5 declaration Acrobat needs, appended as **a new revision** so existing signatures stay valid; `SignOptions.LTV` runs it straight after signing. Out of scope: building a path to a trust anchor, document timestamps (`/DocTimeStamp`, PAdES B-LTA), OCSP nonces, delta CRLs
```

`README.md`: one bullet in the corporate format, beside the signatures bullet — check the surrounding indentation and width with `grep -n "digital signature" README.md` and match it:

```markdown
- **Revocation checking and LTV** — `VerifySignatures(ValidationOptions{Revocation: RevocationOnline})` asks the
  issuer whether a signing certificate was revoked, over OCSP or a CRL, and reports the answer with its
  source and date; `Document.AddValidationInfo` writes that evidence into the document as a `/DSS`, so the
  signature still verifies after the certificate expires. Network access is opt-in and routed through an
  interface you can replace. Mirrors Aspose.PDF for .NET's `ValidationOptions` and its LTV support.
```

`CHANGELOG.md`, first entry under `## [Unreleased]` → `### Added`:

```markdown
- **Signature revocation checking and LTV** — `ValidationOptions` on `VerifySignatures`/`VerifySignature` asks the issuer whether the signing certificate was revoked, over **OCSP** (hand-rolled on `encoding/asn1`, like the rest of the CMS stack — no dependency) or a **CRL**, and reports status, source, revocation time and reason. A response counts only when signed by the issuer or a delegated responder carrying the OCSPSigning usage, and a stale answer is `Unknown` with the reason rather than a comfortable "good". Network access is opt-in and goes through `RevocationFetcher`, so the default is offline and a caller can insert a proxy or cache. `Document.AddValidationInfo` writes the certificates and revocation material into the catalog as a `/DSS` with per-signature `/VRI`, appended as a new revision so existing signatures stay valid — long-term validation, the document carrying its own evidence — and `SignOptions.LTV` does it straight after signing. `Valid` keeps its cryptographic meaning throughout: revocation is reported, never folded in. (`pdf-go-x2s6`)
```

- [ ] **Step 9: Commit**

```bash
git add sign.go sign_verify.go dss.go dss_internal_test.go dss_test.go CLAUDE.md README.md CHANGELOG.md
git commit -m "$(cat <<'EOF'
feat: long-term validation end to end (pdf-go-x2s6)

A document carrying its own /DSS now verifies offline: the embedded OCSP
response is found, checked against the issuer and reported as the source,
and LTVEnabled says the evidence travels with the file. SignOptions.LTV runs
AddValidationInfo straight after signing for callers who want it in one step.

The OCSP request was cross-checked with OpenSSL, which parses and prints it
with the expected CertID and serial — the same independent arbitration the
CMS work gets.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
EOF
)"
```

- [ ] **Step 10: Close the epic**

```bash
bd update pdf-go-x2s6 --status closed
```

---

## Self-Review

**Spec coverage.** Opt-in network — Tasks 5 and 6. Trust stays the caller's (responder signature verified, no path building) — Task 3. `RevocationFetcher` with a built-in default — Tasks 5 and 6. `Valid` unchanged — Task 5, asserted in `TestRevocationOfflineDoesNotFetch`. The public API of the spec's "Verification" section — Task 5. `RevocationStatus` fields — Task 5. LTV API (`AddValidationInfo`, `SignOptions.LTV`) — Tasks 7 and 8. `/DSS`, `/VRI`, `/Extensions` shapes — Task 7. `LTVEnabled` — Task 8. Validation list: OCSP good/revoked/unknown/expired/wrong-signer/delegated-with-and-without-EKU — Tasks 2 and 3; CRL absent/listed/wrong-issuer/expired — Task 4; offline with a refusing fetcher — Tasks 5 and 8; LTV structure and signatures surviving the appended revision — Task 7; round trip — Task 8. The spec's honest limit (no external LTV validator on this machine) is met in Task 8 Step 7 by cross-checking the OCSP request with OpenSSL, which is the one piece an outside tool can arbitrate.

**Placeholder scan.** No TBD or "handle errors appropriately" remains. Two steps deliberately instruct judgement rather than fixed code, and both name the exact decision: Task 2 Step 3's note about the revoked-info re-tagging (keep the one branch that works, delete the dead one) and Task 8 Step 1's instruction to move the offline test into the internal package so it can reuse `ocspTestResponse` instead of duplicating an encoder.

**Type consistency.** `RevocationState` and its constants are defined once (Task 2) and used in Tasks 4, 5, 7, 8. `ocspCertID`, `ocspBasicResponse`, `ocspResponseData`, `ocspSingleResponse` are defined in Tasks 1-2 and consumed unchanged afterwards. `checkCRL`'s five results are used with the same meanings in Tasks 5, 7 and 8. `dssEntry{Certs, OCSPs, CRLs}` and `vriKey` are defined in Task 7 and used in Tasks 7 and 8. `appendRevision(baseNextID, modified, encState)` is extracted in Task 7 Step 1 and called in Task 7 Steps 1 and 6. `lastValidationOption` is defined in Task 5 and reused in Task 7's `AddValidationInfo`. `issuerOf` and `statusFromOCSPDER` are defined in Task 5 and reused in Task 8's `hasLTVMaterial`.
