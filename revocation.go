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

// Revocation checking (epic pdf-go-x2s6). Whether a certificate was revoked is
// answered by its issuer, over OCSP (RFC 6960) or through a CRL (RFC 5280).
// Both are reached through RevocationFetcher, which is how tests avoid the
// network and how a caller with a proxy or a cache substitutes its own
// transport.
//
// Network access is opt-in: the zero ValidationOptions checks nothing, and
// RevocationOffline uses only material the document already carries. A library
// that quietly contacted a CA would also leak which documents are being
// checked, and when.

// RevocationCheck selects how far verification goes in establishing whether a
// signing certificate was revoked.
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

// String names the source, for messages and test failures.
func (s RevocationSource) String() string {
	switch s {
	case RevocationFromOCSP:
		return "OCSP"
	case RevocationFromCRL:
		return "CRL"
	}
	return "document"
}

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
	// Revocation selects how far to go in establishing revocation status.
	Revocation RevocationCheck
	// Fetcher supplies revocation material; nil uses the built-in HTTP client.
	Fetcher RevocationFetcher
	// Timeout bounds each network request; zero means 10 seconds.
	Timeout time.Duration
}

// lastValidationOption returns the effective options, the last one winning.
func lastValidationOption(opts []ValidationOptions) ValidationOptions {
	if len(opts) == 0 {
		return ValidationOptions{}
	}
	return opts[len(opts)-1]
}

// httpRevocationFetcher is the built-in transport.
type httpRevocationFetcher struct{ client *http.Client }

// DefaultRevocationFetcher returns the built-in HTTP fetcher, the one used
// when ValidationOptions.Fetcher is nil. Exported so a caller can wrap it — to
// add a cache, or to log what is fetched — rather than reimplement it. timeout
// bounds each request; zero means 10 seconds.
func DefaultRevocationFetcher(timeout time.Duration) RevocationFetcher {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return httpRevocationFetcher{client: &http.Client{Timeout: timeout}}
}

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
	defer func() { _ = resp.Body.Close() }()
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
		return &RevocationStatus{
			Err: fmt.Errorf("revocation: the document carries no usable material for %s", certName(cert)),
		}
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
// nil when it is not one or does not answer about this certificate. A response
// that parses but fails verification yields an unknown status carrying the
// reason — never silence.
func statusFromOCSPDER(der []byte, cert, issuer *x509.Certificate) *RevocationStatus {
	basic, data, err := parseOCSPAny(der)
	if err != nil {
		return nil
	}
	if err := verifyOCSPResponse(basic, issuer); err != nil {
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
		basic, _, err := parseOCSPResponse(der)
		if err != nil {
			continue
		}
		if err := verifyOCSPResponse(basic, issuer); err != nil {
			continue
		}
		// Store the BasicOCSPResponse, which is what /OCSPs holds.
		if raw := basic.raw(); len(raw) > 0 {
			return raw, RevocationFromOCSP, nil
		}
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
