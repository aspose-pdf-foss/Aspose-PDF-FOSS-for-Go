# Signature Revocation and LTV — Design

Date: 2026-09-16 · Epic: pdf-go-x2s6 (beads) · Status: approved

## Goal

Answer two questions a signed PDF cannot answer today: *was the signing
certificate revoked?* and *will this signature still verify once the
certificate expires?* The first needs revocation checking (OCSP, RFC 6960, and
CRLs, RFC 5280); the second needs the revocation material embedded in the
document itself — long-term validation, the `/DSS` dictionary of ISO 32000-2
§12.8.4.3 and ETSI EN 319 142 (PAdES B-LT).

Mirrors the intent of Aspose.PDF for .NET's `ValidationOptions` / `OcspSettings`
(25.4) and `IsLtvEnabled`. Pure Go, standard library only — as with RFC 3161
timestamps, the ASN.1 and the HTTP are hand-rolled beside the existing CMS code.

Two phases, the second built on the first:

1. **Revocation checking at verification time** — an OCSP and CRL client, the
   status surfaced per signature.
2. **LTV** — the same client collects the material, which is written into the
   document as `/DSS` with `/VRI`, in a new revision.

## Decisions taken before design

- **Network is opt-in.** Verification stays offline unless asked, matching
  `SignOptions.TimestampURL` and the library's general refusal to fetch
  anything on its own (the Markdown renderer will not load remote images).
  A library that silently contacts a CA also leaks which documents are being
  checked and when.
- **Trust stays the caller's.** We verify the OCSP response's own signature
  and the CRL's issuer signature — without that a response means nothing —
  and report the revocation status. Building a path to a trusted root is
  policy, not PDF, and remains outside the library, as it is today.
- **A built-in client behind an interface.** `RevocationFetcher` is how the
  tests avoid the network and how a caller substitutes a proxy, a cache or an
  existing stack; the default implementation is ours.
- **`Valid` does not change meaning.** It stays cryptographic: the signature
  is intact and covers the bytes it claims. A revoked certificate shows up in
  `Revocation`, never by flipping `Valid` — otherwise existing code silently
  changes behaviour, and what to do about revocation is the caller's policy.

## Public API

### Verification

```go
func (d *Document) VerifySignatures(opts ...ValidationOptions) ([]SignatureVerification, error)
func (d *Document) VerifySignature(fieldName string, opts ...ValidationOptions) (SignatureVerification, error)

type ValidationOptions struct {
    // Revocation selects how far to go. The zero value checks nothing, so
    // existing calls behave exactly as before.
    Revocation RevocationCheck
    // Fetcher supplies revocation material; nil uses the built-in HTTP
    // client. Tests and callers with a proxy or cache substitute their own.
    Fetcher RevocationFetcher
    // Timeout bounds each network request; zero means 10 seconds.
    Timeout time.Duration
}

type RevocationCheck int

const (
    RevocationNone    RevocationCheck = iota // default: no checking
    RevocationOffline                        // only material already in the document
    RevocationOnline                         // fetch what the document lacks, per AIA/CDP
)
```

Result, added to `SignatureVerification`:

```go
    Revocation *RevocationStatus // nil unless checking was requested
    LTVEnabled bool              // the document carries everything needed to
                                 // verify this signature offline

type RevocationStatus struct {
    Status     RevocationState  // RevocationGood, RevocationRevoked, RevocationUnknown
    Source     RevocationSource // RevocationFromDocument, RevocationFromOCSP, RevocationFromCRL
    RevokedAt  time.Time        // set only when Status is RevocationRevoked
    Reason     int              // CRLReason code (RFC 5280 §5.3.1); 0 when unspecified
    ProducedAt time.Time        // when the responder produced the answer
    Err        error            // why the status is Unknown
}
```

`RevocationOffline` uses only what the file carries — the `/DSS`, and any
OCSP responses embedded in the CMS as unsigned attributes — which is exactly
what validating an LTV document without a network means.

### LTV

```go
func (d *Document) AddValidationInfo(opts ...ValidationOptions) error
```

Collects the chain and the revocation material for every signature in the
document and writes them into the catalog as `/DSS`, **as a new incremental
revision**, so signatures already in the file stay valid. This is the primary
shape because it is how LTV is done in practice: the material can only be tied
to a signature that already exists.

`SignOptions.LTV bool` is the convenience wrapper: after the signature is
written, a further revision carrying the `/DSS` is appended, using the same
serialize-reopen-append path that signing a freshly built encrypted document
already uses. The result is a file with two appended revisions, which is
normal and what Acrobat produces.

## Document structures

```
/Catalog
  /DSS <<
    /Certs [ N 0 R … ]   each a stream of certificate DER
    /OCSPs [ N 0 R … ]   each a stream of BasicOCSPResponse DER
    /CRLs  [ N 0 R … ]   each a stream of CRL DER
    /VRI << /<KEY> << /Cert [ … ] /OCSP [ … ] /CRL [ … ] /TU (D:…) >> >>
  >>
  /Extensions << /ADBE << /BaseVersion /1.7 /ExtensionLevel 5 >> >>
```

`<KEY>` is the uppercase base-16 SHA-1 of the signature's `/Contents` octets
exactly as stored — the hex-decoded value including its zero padding, which is
what pyHanko hashes. Implementations differ on whether the padding counts, and
the risk is bounded: both Acrobat and pyHanko accept a `/DSS` whose `/Certs`,
`/OCSPs` and `/CRLs` carry the material even when `/VRI` is absent or keyed
differently. `/VRI` speeds the lookup; it is not the only source.

The `/Extensions` declaration is required for Acrobat to recognise a `/DSS` in
a PDF 1.7 file.

`LTVEnabled` reports that the document carries a chain for the signature and
revocation material covering every certificate in it except a self-signed
root.

## Internals

| File | Responsibility |
|---|---|
| `ocsp.go` | OCSP request DER (`TBSRequest`, `CertID` with the issuer name and key hashes), response parsing (`OCSPResponse` → `BasicOCSPResponse` → `SingleResponse`), and verification of the response's own signature |
| `crl.go` | Fetching a CRL and parsing it with `x509.ParseRevocationList`; checking the issuer signature and looking up the serial |
| `revocation.go` | `RevocationFetcher`, the built-in HTTP implementation, the orchestration (document material first, then AIA/CDP), and a per-call cache so one certificate is asked about once |
| `dss.go` | Building the `/DSS`, `/VRI` and `/Extensions` objects and appending them as an incremental revision |

The OCSP response signature is verified against the issuer certificate, or
against a delegated responder certificate carried in the response when it is
issued by that issuer and bears the `id-kp-OCSPSigning` extended key usage.
A response that fails this check yields `RevocationUnknown` with the reason in
`Err` — never a silent "good".

`nextUpdate` in the past, or a `thisUpdate` in the future, likewise yields
`Unknown`: a stale answer is not an answer.

## Validation

No test touches the network. A throwaway CA is generated in memory, as the
signing tests already do, and:

- **OCSP**: a fake responder over `httptest` signs real `BasicOCSPResponse`
  structures. Cases: good; revoked with a date and a reason; unknown; a
  response signed by the wrong issuer (must be refused); an expired
  `nextUpdate` (must be Unknown, not Good); a delegated responder with the
  OCSPSigning EKU (accepted) and without it (refused).
- **CRL**: built with `x509.CreateRevocationList`. Cases: serial absent
  (good), serial listed (revoked, with the reason), wrong issuer signature
  (refused).
- **Offline**: a document carrying a `/DSS` verifies with
  `RevocationOffline` and a `Fetcher` that fails the test if it is called.
- **LTV**: after `AddValidationInfo`, assert the `/DSS` structure, the `/VRI`
  key, the `/Extensions` declaration, that earlier signatures still verify
  (the revision was appended, not rewritten), and that `LTVEnabled` is true.
- **Round trip**: sign → `AddValidationInfo` → reopen → verify offline →
  status Good, no network.

Honest limit: pyHanko is not installed here and pikepdf does not validate LTV,
so unlike the CMS work — which OpenSSL can arbitrate — there is no independent
validator for `/DSS` on this machine. The structures are checked against the
specification and by our own reader; Acrobat on the user's side is the
external confirmation if one is wanted.

## Out of scope

- Building a certification path to a trust anchor, and any "trusted / not
  trusted" verdict (unchanged from today).
- Document timestamps (`/DocTimeStamp`, PAdES B-LTA) and the archival
  refresh of validation material. The `/DSS` written here is B-LT.
- OCSP nonces and response caching across calls; the cache is per call.
- CRL delta updates, indirect CRLs, and issuing-distribution-point checks.
