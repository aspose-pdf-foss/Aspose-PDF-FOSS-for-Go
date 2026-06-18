// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"crypto"
	"crypto/x509"
	"encoding/hex"
	"fmt"
	"time"
)

// Digital signatures (ISO 32000-1 §12.8). A single invisible PKCS#7
// detached signature covering the whole file. The signature is configured
// with Sign() and applied at Save/WriteTo time, like SetEncryption.
//
// SECURITY / SCOPE NOTE (v1): one signature, "adbe.pkcs7.detached",
// SHA-256, RSA or ECDSA keys; invisible (zero-rect) widget; signs an
// unencrypted document. Out of scope: PAdES/CAdES, RFC 3161 timestamps,
// LTV, visible appearances, DocMDP certification, multiple/incremental
// signatures. Sign+Save is terminal — configure once and save once.

// signContentsHexLen is the fixed number of hex characters reserved for
// the /Contents placeholder (so the PKCS#7 blob can be spliced in without
// shifting any byte offsets). 8192 bytes is ample for a SHA-256 RSA/ECDSA
// signature plus a small certificate chain.
const signContentsHexLen = 16384

// byteRangePlaceholder and contentsPlaceholder are the exact byte
// sequences emitted into the signature dictionary; applySignature finds
// and patches them in the serialized file. Both are fixed width.
const byteRangePlaceholder = "[0 0000000000 0000000000 0000000000]"

func contentsPlaceholder() []byte {
	b := make([]byte, signContentsHexLen+2)
	b[0] = '<'
	for i := 1; i <= signContentsHexLen; i++ {
		b[i] = '0'
	}
	b[len(b)-1] = '>'
	return b
}

// SignOptions configures a digital signature. Certificate and PrivateKey
// are required; the key is any crypto.Signer (RSA or ECDSA), so callers
// load it however they like (PEM via the standard library, an HSM, a
// freshly generated test key, …) — no certificate file is needed in the
// repository.
type SignOptions struct {
	Certificate *x509.Certificate
	PrivateKey  crypto.Signer
	Chain       []*x509.Certificate // optional intermediates, embedded alongside the signer cert
	Reason      string              // optional /Reason
	Location    string              // optional /Location
	ContactInfo string              // optional /ContactInfo
	Name        string              // optional /Name (signer name shown by viewers)
	SigningTime time.Time           // zero → time of signing
}

type signConfig struct {
	cert                            *x509.Certificate
	key                             crypto.Signer
	chain                           []*x509.Certificate
	reason, location, contact, name string
	when                            time.Time
}

// Sign configures a digital signature applied on the next Save/WriteTo.
// Mirrors the intent of Aspose.PDF for .NET's PdfFileSignature.Sign,
// adapted to this library's options paradigm. Returns an error for a
// missing certificate or key.
func (d *Document) Sign(opts SignOptions) error {
	if opts.Certificate == nil || opts.PrivateKey == nil {
		return fmt.Errorf("Sign: Certificate and PrivateKey are required")
	}
	switch opts.Certificate.PublicKeyAlgorithm {
	case x509.RSA, x509.ECDSA:
	default:
		return fmt.Errorf("Sign: unsupported key algorithm %v (RSA or ECDSA)", opts.Certificate.PublicKeyAlgorithm)
	}
	d.sign = &signConfig{
		cert:     opts.Certificate,
		key:      opts.PrivateKey,
		chain:    opts.Chain,
		reason:   opts.Reason,
		location: opts.Location,
		contact:  opts.ContactInfo,
		name:     opts.Name,
		when:     opts.SigningTime,
	}
	return nil
}

// buildSignatureObjects adds the signature dictionary and an invisible Sig
// field/widget to the document, wires the /AcroForm and the page's
// /Annots, and sets /SigFlags. Called from buildDocumentPDF before objects
// are snapshotted. The /Contents and /ByteRange are fixed-width
// placeholders patched later by applySignature.
func (d *Document) buildSignatureObjects() {
	when := d.sign.when
	if when.IsZero() {
		when = time.Now()
	}

	sigID := d.nextID
	d.nextID++
	sigDict := pdfDict{
		"/Type":      pdfName("/Sig"),
		"/Filter":    pdfName("/Adobe.PPKLite"),
		"/SubFilter": pdfName("/adbe.pkcs7.detached"),
		"/ByteRange": pdfRaw([]byte(byteRangePlaceholder)),
		"/Contents":  pdfRaw(contentsPlaceholder()),
		"/M":         pdfDateString(when),
	}
	if d.sign.name != "" {
		sigDict["/Name"] = d.sign.name
	}
	if d.sign.reason != "" {
		sigDict["/Reason"] = d.sign.reason
	}
	if d.sign.location != "" {
		sigDict["/Location"] = d.sign.location
	}
	if d.sign.contact != "" {
		sigDict["/ContactInfo"] = d.sign.contact
	}
	d.objects[sigID] = &pdfObject{Num: sigID, Value: sigDict}

	// Combined signature field + invisible widget annotation on page 1.
	page1 := d.pages[0]
	fieldID := d.nextID
	d.nextID++
	fieldDict := pdfDict{
		"/Type":    pdfName("/Annot"),
		"/Subtype": pdfName("/Widget"),
		"/FT":      pdfName("/Sig"),
		"/T":       "Signature1",
		"/V":       pdfRef{Num: sigID},
		"/Rect":    pdfArray{0.0, 0.0, 0.0, 0.0},
		"/F":       132, // Print (4) + Locked (128)
		"/P":       pdfRef{Num: page1.Num},
	}
	d.objects[fieldID] = &pdfObject{Num: fieldID, Value: fieldDict}

	// /AcroForm: append the field, mark signatures present + append-only.
	acro := d.acroFormDict()
	appendSigField(d.objects, acro, pdfRef{Num: fieldID})
	acro["/SigFlags"] = 3

	// Page /Annots: the widget is its own annotation.
	appendAnnotToPage(d.objects, page1, pdfRef{Num: fieldID})
}

// acroFormDict returns the catalog's /AcroForm as a live, mutable dict,
// creating it (with an empty /Fields) when absent. A referenced AcroForm
// is mutated in place.
func (d *Document) acroFormDict() pdfDict {
	if d.catalog == nil {
		d.catalog = pdfDict{}
	}
	switch v := d.catalog["/AcroForm"].(type) {
	case pdfDict:
		return v
	case pdfRef:
		if obj, ok := d.objects[v.Num]; ok {
			if dict, ok := obj.Value.(pdfDict); ok {
				return dict
			}
		}
	}
	nd := pdfDict{"/Fields": pdfArray{}}
	d.catalog["/AcroForm"] = nd
	return nd
}

// appendSigField appends fieldRef to /AcroForm/Fields, handling both the
// inline-array and indirect-array storage forms.
func appendSigField(objects map[int]*pdfObject, acro pdfDict, fieldRef pdfRef) {
	switch v := acro["/Fields"].(type) {
	case pdfRef:
		if obj, ok := objects[v.Num]; ok {
			if arr, ok := obj.Value.(pdfArray); ok {
				obj.Value = append(arr, fieldRef)
				return
			}
		}
	case pdfArray:
		acro["/Fields"] = append(v, fieldRef)
		return
	}
	acro["/Fields"] = pdfArray{fieldRef}
}

// applySignature finds the /ByteRange and /Contents placeholders in the
// serialized PDF, fills the real byte range, computes the PKCS#7 over the
// signed bytes, and splices the hex-encoded signature into /Contents.
func (d *Document) applySignature(raw []byte) ([]byte, error) {
	out := make([]byte, len(raw))
	copy(out, raw)

	brIdx := bytes.Index(out, []byte(byteRangePlaceholder))
	if brIdx < 0 {
		return nil, fmt.Errorf("sign: /ByteRange placeholder not found")
	}
	cph := contentsPlaceholder()
	cIdx := bytes.Index(out, cph)
	if cIdx < 0 {
		return nil, fmt.Errorf("sign: /Contents placeholder not found")
	}
	offOpen := cIdx                 // position of '<'
	offClose := cIdx + len(cph) - 1 // position of '>'

	// /ByteRange = [0 len1 start2 len2]. The excluded gap is the entire
	// /Contents value INCLUDING the angle brackets — i.e. range 1 ends just
	// before '<' and range 2 begins just after '>'. (Adobe convention, as
	// checked by validators: gap length == 2·sigBytes + 2 brackets.)
	len1 := offOpen
	start2 := offClose + 1
	len2 := len(out) - start2
	br := fmt.Sprintf("[0 %010d %010d %010d]", len1, start2, len2)
	if len(br) != len(byteRangePlaceholder) {
		return nil, fmt.Errorf("sign: /ByteRange width overflow (file too large)")
	}
	copy(out[brIdx:brIdx+len(byteRangePlaceholder)], br)

	// Hash the signed bytes (excluding the /Contents hex gap) — note the
	// patched /ByteRange above is part of range 1.
	content := make([]byte, 0, len1+len2)
	content = append(content, out[:len1]...)
	content = append(content, out[start2:]...)

	when := d.sign.when
	if when.IsZero() {
		when = time.Now()
	}
	pkcs7, err := buildPKCS7Detached(content, d.sign.cert, d.sign.key, d.sign.chain, when)
	if err != nil {
		return nil, err
	}
	hexSig := make([]byte, hex.EncodedLen(len(pkcs7)))
	hex.Encode(hexSig, pkcs7)
	if len(hexSig) > signContentsHexLen {
		return nil, fmt.Errorf("sign: signature too large (%d > %d hex chars); raise signContentsHexLen", len(hexSig), signContentsHexLen)
	}
	gap := out[offOpen+1 : offClose] // exactly signContentsHexLen bytes
	for i := range gap {
		gap[i] = '0'
	}
	copy(gap, hexSig)
	return out, nil
}

// pdfDateString formats t as a PDF date string (ISO 32000-1 §7.9.4),
// e.g. D:20260618130500+03'00'.
func pdfDateString(t time.Time) string {
	_, offSec := t.Zone()
	sign := '+'
	if offSec < 0 {
		sign = '-'
		offSec = -offSec
	}
	oh := offSec / 3600
	om := (offSec % 3600) / 60
	return fmt.Sprintf("D:%04d%02d%02d%02d%02d%02d%c%02d'%02d'",
		t.Year(), int(t.Month()), t.Day(), t.Hour(), t.Minute(), t.Second(), sign, oh, om)
}
