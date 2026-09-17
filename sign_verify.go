// SPDX-License-Identifier: MIT

package asposepdf

import (
	"crypto/x509"
	"fmt"
	"strings"
	"time"
)

// SignatureVerification is the result of verifying one digital signature.
type SignatureVerification struct {
	FieldName           string            // the /T of the signature field
	SignerName          string            // /Name from the signature dictionary, if present
	Reason              string            // /Reason
	Location            string            // /Location
	SigningTime         time.Time         // parsed from /M (zero if absent/unparseable)
	Valid               bool              // signature cryptographically valid AND content intact
	IntegrityOK         bool              // the /ByteRange bytes match the signature digest
	CoversWholeDocument bool              // the signature covers the entire file
	Certificate         *x509.Certificate // signer certificate (nil on parse failure)
	// Chain holds the other certificates the signer embedded — the
	// intermediates a caller needs to build a path to a trust anchor. The
	// signer certificate itself is not repeated here.
	Chain []*x509.Certificate
	// Revocation is what the issuer says about the signing certificate. It is
	// nil unless ValidationOptions asked for a check. A revoked certificate
	// does not make Valid false: Valid stays cryptographic, and what to do
	// about revocation is the caller's policy.
	Revocation *RevocationStatus
	// LTVEnabled reports that the document carries everything needed to verify
	// this signature offline. Mirrors Aspose.PDF for .NET's IsLtvEnabled.
	LTVEnabled bool
	Err        error // non-nil reason when Valid is false
}

// VerifySignatures verifies every digital signature in the document and
// returns one result per signature field, in document order. The document
// must have been opened from a file or stream (the raw bytes are needed to
// resolve each /ByteRange). Trust-chain validation is left to the caller —
// Certificate is returned so it can be checked against a trust store.
//
// Mirrors the intent of Aspose.PDF for .NET's PdfFileSignature.VerifySignature.
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

// verifyOneSignatureWithOptions runs the cryptographic verification and, when
// asked, the revocation check on top of it.
func (d *Document) verifyOneSignatureWithOptions(sf sigFieldRef, o ValidationOptions) SignatureVerification {
	res := verifyOneSignature(d.source, sf.name, sf.sig)
	if res.Certificate == nil {
		return res
	}
	issuer := issuerOf(res.Certificate, res.Chain)
	if o.Revocation != RevocationNone {
		res.Revocation = checkRevocation(res.Certificate, issuer, d.dssMaterial(), o)
	}
	res.LTVEnabled = d.hasLTVMaterial(res.Certificate, issuer)
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

// SignatureNames returns the field name of every signature in the document,
// in document order. Mirrors Aspose.PDF for .NET's
// PdfFileSignature.GetSignatureNames.
func (d *Document) SignatureNames() []string {
	fields := d.collectSignatureFields()
	names := make([]string, 0, len(fields))
	for _, sf := range fields {
		names = append(names, sf.name)
	}
	return names
}

// VerifySignature verifies the one signature held by the named field. The
// error reports a missing field or missing source bytes; a signature that is
// present but invalid comes back in the result, with the reason in Err —
// there is nothing exceptional about a document failing to verify. Mirrors
// Aspose.PDF for .NET's PdfFileSignature.VerifySignature(sigName).
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

type sigFieldRef struct {
	name string
	sig  pdfDict
}

// collectSignatureFields walks /AcroForm/Fields for signature fields,
// pairing each field name with its /V signature dictionary.
func (d *Document) collectSignatureFields() []sigFieldRef {
	var res []sigFieldRef
	acro, ok := resolveRefToDict(d.objects, d.catalog["/AcroForm"])
	if !ok {
		return res
	}
	var walk func(arr pdfArray)
	walk = func(arr pdfArray) {
		for _, fv := range arr {
			fd, ok := resolveRefToDict(d.objects, fv)
			if !ok {
				continue
			}
			if n, _ := fd["/FT"].(pdfName); n == "/Sig" {
				if sig, ok := resolveRefToDict(d.objects, fd["/V"]); ok {
					res = append(res, sigFieldRef{name: decodeFormString(fd["/T"]), sig: sig})
				}
			}
			if kids := d.resolveArray(fd["/Kids"]); kids != nil {
				walk(kids)
			}
		}
	}
	walk(d.resolveArray(acro["/Fields"]))
	return res
}

// resolveArray resolves a value to a pdfArray, following one indirect
// reference; returns nil if it is not an array.
func (d *Document) resolveArray(v pdfValue) pdfArray {
	switch a := v.(type) {
	case pdfArray:
		return a
	case pdfRef:
		if obj, ok := d.objects[a.Num]; ok {
			if arr, ok := obj.Value.(pdfArray); ok {
				return arr
			}
		}
	}
	return nil
}

// verifyOneSignature verifies a single signature dictionary against the
// raw document bytes.
func verifyOneSignature(source []byte, fieldName string, sig pdfDict) SignatureVerification {
	res := SignatureVerification{
		FieldName:  fieldName,
		SignerName: decodeFormString(sig["/Name"]),
		Reason:     decodeFormString(sig["/Reason"]),
		Location:   decodeFormString(sig["/Location"]),
	}
	if t, ok := parsePDFDate(decodeFormString(sig["/M"])); ok {
		res.SigningTime = t
	}

	br := toIntArray(sig["/ByteRange"])
	if len(br) != 4 {
		res.Err = fmt.Errorf("missing or malformed /ByteRange")
		return res
	}
	start1, len1, start2, len2 := br[0], br[1], br[2], br[3]
	if start1 != 0 || len1 < 0 || start2 < 0 || len2 < 0 ||
		len1 > len(source) || start2+len2 > len(source) || start2 < len1 {
		res.Err = fmt.Errorf("/ByteRange out of bounds")
		return res
	}
	res.CoversWholeDocument = start1 == 0 && start2+len2 == len(source)

	content := make([]byte, 0, len1+len2)
	content = append(content, source[:len1]...)
	content = append(content, source[start2:start2+len2]...)

	cms := contentsBytes(sig["/Contents"])
	if len(cms) == 0 {
		res.Err = fmt.Errorf("missing /Contents")
		return res
	}
	signer, certs, err := verifyPKCS7Detached(cms, content)
	if err != nil {
		res.Err = err
		return res
	}
	res.Certificate = signer
	for _, c := range certs {
		if c.Equal(signer) {
			continue
		}
		res.Chain = append(res.Chain, c)
	}
	res.IntegrityOK = true
	res.Valid = true
	return res
}

// contentsBytes extracts the raw signature bytes from the /Contents value
// (a hex or literal string). Trailing zero padding is harmless — the
// PKCS#7 DER is self-delimiting.
func contentsBytes(v pdfValue) []byte {
	switch s := v.(type) {
	case pdfHexString:
		return []byte(s)
	case string:
		return []byte(s)
	}
	return nil
}

// toIntArray converts a pdfArray of numbers to []int.
func toIntArray(v pdfValue) []int {
	arr, ok := v.(pdfArray)
	if !ok {
		return nil
	}
	out := make([]int, 0, len(arr))
	for _, e := range arr {
		out = append(out, toInt(e))
	}
	return out
}

// parsePDFDate parses a PDF date string "D:YYYYMMDDHHmmSS..." (the time
// zone suffix is ignored). Returns false if it cannot be parsed.
func parsePDFDate(s string) (time.Time, bool) {
	s = strings.TrimPrefix(s, "D:")
	if len(s) < 14 {
		return time.Time{}, false
	}
	t, err := time.Parse("20060102150405", s[:14])
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}
