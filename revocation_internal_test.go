// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"errors"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"
	"time"
)

// refusingFetcher fails the test if anything reaches for the network.
type refusingFetcher struct{ t *testing.T }

func (f refusingFetcher) FetchOCSP(_ context.Context, url string, _ []byte) ([]byte, error) {
	f.t.Errorf("an OCSP request went out to %s during an offline check", url)
	return nil, errors.New("refused")
}

func (f refusingFetcher) FetchCRL(_ context.Context, url string) ([]byte, error) {
	f.t.Errorf("a CRL request went out to %s during an offline check", url)
	return nil, errors.New("refused")
}

// fakeFetcher answers OCSP from an in-memory CA; it never touches the network.
type fakeFetcher struct {
	t      *testing.T
	ca     *x509.Certificate
	caKey  *rsa.PrivateKey
	leaf   *x509.Certificate
	status string
	calls  int
}

func (f *fakeFetcher) FetchOCSP(_ context.Context, _ string, _ []byte) ([]byte, error) {
	f.calls++
	return ocspTestResponse(f.t, f.ca, f.ca, f.caKey, f.leaf, f.status, time.Now().Add(-time.Hour), 1, time.Now().Add(time.Hour)), nil
}

func (f *fakeFetcher) FetchCRL(_ context.Context, url string) ([]byte, error) {
	return nil, errors.New("no CRL at " + url)
}

// signedByCA builds a one-page document signed by a certificate the test CA
// issued, and returns it reopened from its own bytes along with the CA, the
// CA key and the signer certificate.
func signedByCA(t *testing.T, ocspURL string) (*Document, *x509.Certificate, *rsa.PrivateKey, *x509.Certificate) {
	t.Helper()
	ca, caKey := ocspTestCA(t)

	leafKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := x509.Certificate{
		SerialNumber: big.NewInt(555),
		Subject:      pkix.Name{CommonName: "LTV Test Signer"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().AddDate(1, 0, 0),
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}
	if ocspURL != "" {
		tmpl.OCSPServer = []string{ocspURL}
	}
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, ca, leafKey.Public(), caKey)
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}

	doc := NewDocument(400, 200)
	page, err := doc.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	if err := page.AddText("Signed", TextStyle{Font: FontHelvetica, Size: 14},
		Rectangle{LLX: 20, LLY: 100, URX: 380, URY: 140}); err != nil {
		t.Fatal(err)
	}
	if err := doc.Sign(SignOptions{
		Certificate: leaf, PrivateKey: leafKey,
		Chain: []*x509.Certificate{ca}, Name: "LTV Signer",
	}); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	signed, err := OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	return signed, ca, caKey, leaf
}

// The zero ValidationOptions must check nothing, so every existing call keeps
// its behaviour and no caller is surprised by outbound traffic.
func TestValidationOptionsZeroValueChecksNothing(t *testing.T) {
	var o ValidationOptions
	if o.Revocation != RevocationNone {
		t.Errorf("zero Revocation = %v, want RevocationNone", o.Revocation)
	}
	doc, _, _, _ := signedByCA(t, "")
	sigs, err := doc.VerifySignatures()
	if err != nil {
		t.Fatal(err)
	}
	if len(sigs) != 1 {
		t.Fatalf("got %d signatures, want 1", len(sigs))
	}
	if sigs[0].Revocation != nil {
		t.Error("a status was reported although no check was asked for")
	}
}

// An offline check must never call the fetcher, and must say why it cannot
// answer rather than reporting a comfortable "good".
func TestRevocationOfflineDoesNotFetch(t *testing.T) {
	doc, _, _, _ := signedByCA(t, "http://ocsp.example.invalid")
	sigs, err := doc.VerifySignatures(ValidationOptions{
		Revocation: RevocationOffline,
		Fetcher:    refusingFetcher{t},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(sigs) != 1 {
		t.Fatalf("got %d signatures, want 1", len(sigs))
	}
	st := sigs[0].Revocation
	if st == nil {
		t.Fatal("no revocation status reported")
	}
	if st.Status != RevocationUnknown {
		t.Errorf("status = %v, want unknown for a document carrying no material", st.Status)
	}
	if st.Err == nil {
		t.Error("an unknown status came with no explanation")
	}
	if !sigs[0].Valid {
		t.Errorf("revocation checking changed Valid: %v", sigs[0].Err)
	}
	if sigs[0].LTVEnabled {
		t.Error("LTVEnabled = true for a document with no /DSS")
	}
}

// Online checking reports what the responder says, and where it came from.
func TestRevocationOnlineReportsResponder(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status string
		want   RevocationState
	}{
		{"good", "good", RevocationGood},
		{"revoked", "revoked", RevocationRevoked},
	} {
		t.Run(tc.name, func(t *testing.T) {
			doc, ca, caKey, leaf := signedByCA(t, "http://ocsp.example.invalid")
			f := &fakeFetcher{t: t, ca: ca, caKey: caKey, leaf: leaf, status: tc.status}
			sigs, err := doc.VerifySignatures(ValidationOptions{Revocation: RevocationOnline, Fetcher: f})
			if err != nil {
				t.Fatal(err)
			}
			st := sigs[0].Revocation
			if st == nil {
				t.Fatal("no revocation status reported")
			}
			if st.Status != tc.want {
				t.Errorf("status = %v (%v), want %v", st.Status, st.Err, tc.want)
			}
			if st.Source != RevocationFromOCSP {
				t.Errorf("source = %v, want OCSP", st.Source)
			}
			if f.calls == 0 {
				t.Error("the responder was never asked")
			}
			// A revoked certificate must not flip the cryptographic verdict.
			if !sigs[0].Valid {
				t.Errorf("Valid became false: %v", sigs[0].Err)
			}
		})
	}
}

// The built-in fetcher must speak the wire protocol correctly: POST with the
// OCSP content type for a responder, GET for a CRL.
func TestBuiltinFetcherSpeaksHTTP(t *testing.T) {
	var gotMethod, gotContentType string
	var gotBody []byte
	ocspServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotContentType = r.Header.Get("Content-Type")
		gotBody, _ = io.ReadAll(r.Body)
		_, _ = w.Write([]byte{0x30, 0x03, 0x0A, 0x01, 0x06}) // a minimal "unauthorized" response
	}))
	defer ocspServer.Close()

	crlServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("CRL fetched with %s, want GET", r.Method)
		}
		_, _ = w.Write([]byte("not a crl"))
	}))
	defer crlServer.Close()

	f := DefaultRevocationFetcher(2 * time.Second)
	if _, err := f.FetchOCSP(context.Background(), ocspServer.URL, []byte{1, 2, 3}); err != nil {
		t.Fatalf("FetchOCSP: %v", err)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("OCSP fetched with %s, want POST", gotMethod)
	}
	if gotContentType != "application/ocsp-request" {
		t.Errorf("Content-Type = %q, want application/ocsp-request", gotContentType)
	}
	if !bytes.Equal(gotBody, []byte{1, 2, 3}) {
		t.Errorf("the request body did not arrive intact: %v", gotBody)
	}
	if _, err := f.FetchCRL(context.Background(), crlServer.URL); err != nil {
		t.Fatalf("FetchCRL: %v", err)
	}
}

// A responder that answers with an HTTP error is a failed fetch, not an empty
// answer that might read as "good".
func TestBuiltinFetcherRejectsHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "nope", http.StatusInternalServerError)
	}))
	defer server.Close()

	f := DefaultRevocationFetcher(2 * time.Second)
	if _, err := f.FetchCRL(context.Background(), server.URL); err == nil {
		t.Fatal("an HTTP 500 was accepted as a CRL")
	}
}

// The /DSS must appear in the catalog, carry the signer certificate and the
// responder's answer, and be announced through the Adobe extension level
// Acrobat looks for.
func TestAddValidationInfoWritesDSS(t *testing.T) {
	doc, ca, caKey, leaf := signedByCA(t, "http://ocsp.example.invalid")
	f := &fakeFetcher{t: t, ca: ca, caKey: caKey, leaf: leaf, status: "good"}
	if err := doc.AddValidationInfo(ValidationOptions{Revocation: RevocationOnline, Fetcher: f}); err != nil {
		t.Fatalf("AddValidationInfo: %v", err)
	}
	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	out := buf.Bytes()

	for _, want := range []string{"/DSS", "/Certs", "/OCSPs", "/VRI", "/ADBE", "/ExtensionLevel 5"} {
		if !bytes.Contains(out, []byte(want)) {
			t.Errorf("the output does not carry %q", want)
		}
	}
	if !bytes.Contains(out, leaf.Raw) {
		t.Error("the signer certificate was not embedded")
	}
	if !bytes.Contains(out, ca.Raw) {
		t.Error("the issuer certificate was not embedded")
	}
	m := regexp.MustCompile(`/VRI\s*<<\s*/([0-9A-F]{40})`).FindSubmatch(out)
	if m == nil {
		t.Fatal("no /VRI entry with a 40-hex-digit key in the output")
	}
	if _, err := hex.DecodeString(string(m[1])); err != nil {
		t.Errorf("the /VRI key is not hex: %v", err)
	}
}

// Adding validation info appends a revision, so the signature it documents
// must survive untouched.
func TestAddValidationInfoKeepsSignatureValid(t *testing.T) {
	doc, ca, caKey, leaf := signedByCA(t, "http://ocsp.example.invalid")
	before, err := doc.VerifySignatures()
	if err != nil || len(before) != 1 || !before[0].Valid {
		t.Fatalf("the fixture is not a valid signed document: %v / %+v", err, before)
	}
	f := &fakeFetcher{t: t, ca: ca, caKey: caKey, leaf: leaf, status: "good"}
	if err := doc.AddValidationInfo(ValidationOptions{Revocation: RevocationOnline, Fetcher: f}); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenStream(bytes.NewReader(buf.Bytes()))
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
	if after[0].FieldName != before[0].FieldName {
		t.Errorf("field name changed: %q → %q", before[0].FieldName, after[0].FieldName)
	}
}

// A document carrying its own revocation material verifies offline: the
// fetcher must never be called, and the answer must come from the document.
func TestOfflineVerificationUsesEmbeddedMaterial(t *testing.T) {
	doc, ca, caKey, leaf := signedByCA(t, "http://ocsp.example.invalid")
	f := &fakeFetcher{t: t, ca: ca, caKey: caKey, leaf: leaf, status: "good"}
	if err := doc.AddValidationInfo(ValidationOptions{Revocation: RevocationOnline, Fetcher: f}); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}

	sigs, err := reopened.VerifySignatures(ValidationOptions{
		Revocation: RevocationOffline,
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
	if st.Status != RevocationGood {
		t.Errorf("status = %v (%v), want good from the embedded response", st.Status, st.Err)
	}
	if st.Source != RevocationFromDocument {
		t.Errorf("source = %v, want document", st.Source)
	}
	if !sigs[0].LTVEnabled {
		t.Error("LTVEnabled = false for a document carrying its own material")
	}
}

// SignOptions.LTV appends the validation material straight after signing. The
// certificate here names no responder, so nothing is fetched and only the
// certificates are embedded — which is the documented behaviour when no
// responder is reachable.
func TestSignWithLTVAppendsDSS(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := x509.Certificate{
		SerialNumber: big.NewInt(777),
		Subject:      pkix.Name{CommonName: "LTV Self Signer"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().AddDate(1, 0, 0),
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, key.Public(), key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}

	doc := NewDocument(400, 200)
	page, err := doc.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	if err := page.AddText("LTV", TextStyle{Font: FontHelvetica, Size: 14},
		Rectangle{LLX: 20, LLY: 100, URX: 380, URY: 140}); err != nil {
		t.Fatal(err)
	}
	if err := doc.Sign(SignOptions{Certificate: cert, PrivateKey: key, Name: "Signer", LTV: true}); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	out := buf.Bytes()
	if !bytes.Contains(out, []byte("/DSS")) {
		t.Error("signing with LTV wrote no /DSS")
	}
	if !bytes.Contains(out, cert.Raw) {
		t.Error("the signer certificate was not embedded")
	}

	reopened, err := OpenStream(bytes.NewReader(out))
	if err != nil {
		t.Fatal(err)
	}
	sigs, err := reopened.VerifySignatures()
	if err != nil {
		t.Fatal(err)
	}
	if len(sigs) != 1 || !sigs[0].Valid {
		t.Fatalf("the LTV-signed document does not verify: %+v", sigs)
	}
}
