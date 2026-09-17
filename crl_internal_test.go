// SPDX-License-Identifier: MIT

package asposepdf

import (
	"crypto"
	"crypto/rand"
	"crypto/x509"
	"math/big"
	"testing"
	"time"
)

// testCRL issues a CRL from the CA listing the given entries as revoked.
func testCRL(t *testing.T, ca *x509.Certificate, caKey crypto.Signer, revoked []x509.RevocationListEntry, nextUpdate time.Time) []byte {
	t.Helper()
	// An already-expired list is still a well-formed one: keep ThisUpdate
	// before NextUpdate, which is what CreateRevocationList insists on.
	thisUpdate := time.Now().Add(-time.Minute)
	if !nextUpdate.IsZero() && !thisUpdate.Before(nextUpdate) {
		thisUpdate = nextUpdate.Add(-time.Hour)
	}
	der, err := x509.CreateRevocationList(rand.Reader, &x509.RevocationList{
		Number:                    big.NewInt(1),
		ThisUpdate:                thisUpdate,
		NextUpdate:                nextUpdate,
		RevokedCertificateEntries: revoked,
	}, ca, caKey)
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
	when := time.Now().Add(-72 * time.Hour).Truncate(time.Second).UTC()
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
	if !revokedAt.Equal(when) {
		t.Errorf("revokedAt = %v, want %v", revokedAt, when)
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
