// SPDX-License-Identifier: MIT

package asposepdf

import (
	"crypto/x509"
	"fmt"
	"time"
)

// Certificate revocation lists (RFC 5280 §5). The parsing is the standard
// library's; what is added here is the part that decides whether a list is
// evidence about a particular certificate — it must be signed by that
// certificate's issuer, and it must not have expired.

// checkCRL reports what the list says about cert. The second result is the
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
		return RevocationUnknown, time.Time{}, 0, time.Time{},
			fmt.Errorf("crl: not issued by %s: %w", certName(issuer), err)
	}
	if !list.NextUpdate.IsZero() && time.Now().After(list.NextUpdate) {
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
