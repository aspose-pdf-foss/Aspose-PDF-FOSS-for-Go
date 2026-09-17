// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"crypto/sha1"
	"crypto/x509"
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
// SHA-1 of its /Contents octets, exactly as stored (padding included).
func vriKey(contents []byte) string {
	sum := sha1.Sum(contents)
	return strings.ToUpper(fmt.Sprintf("%x", sum))
}

// AddValidationInfo collects the certificates and revocation material for
// every signature in the document and writes them into the catalog as /DSS,
// appended as a new revision so signatures already in the file stay valid.
// This is long-term validation: the evidence travels with the document, so it
// still verifies once the certificate expires or the responder is gone.
//
// The document must have been opened from a file or stream. Network access is
// opt-in through ValidationOptions — with the zero value nothing is fetched
// and only the certificates the signature carries are recorded. Mirrors the
// intent of Aspose.PDF for .NET's LTV support.
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
			if issuer := issuerOf(res.Certificate, res.Chain); issuer != nil {
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

	if d.catalog == nil {
		return fmt.Errorf("AddValidationInfo: document has no catalog")
	}
	if d.catalogNum == 0 {
		return fmt.Errorf("AddValidationInfo: cannot determine the catalog object number")
	}
	if d.nextID < d.origSize {
		d.nextID = d.origSize
	}
	baseNextID := d.nextID
	d.buildDSS(material)

	cat := deepCopyValue(pdfValue(d.catalog)).(pdfDict)
	cat["/Type"] = pdfName("/Catalog")
	modified := map[int]pdfValue{d.catalogNum: cat}

	out, err := d.appendRevision(baseNextID, modified, d.preserved)
	if err != nil {
		return err
	}
	d.source = out
	d.revisionAppended = true
	return nil
}

// buildDSS creates the /DSS object graph for the given per-signature material
// and attaches it to the catalog, together with the Adobe extension
// declaration Acrobat requires to recognise it in a PDF 1.7 file. The objects
// land in d.objects; the caller appends the revision.
func (d *Document) buildDSS(material map[string]dssEntry) {
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
		vri["/"+key] = entry
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
			"/BaseVersion":    pdfName("/1.7"),
			"/ExtensionLevel": 5,
		},
	}
}

// dssMaterial returns every OCSP response and CRL the document carries in its
// /DSS, as DER. Order is unspecified; callers try each in turn.
func (d *Document) dssMaterial() [][]byte {
	dss, ok := resolveRefToDict(d.objects, d.catalog["/DSS"])
	if !ok {
		return nil
	}
	var out [][]byte
	for _, key := range []string{"/OCSPs", "/CRLs"} {
		for _, v := range d.resolveArray(dss[key]) {
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
			if data := decodedStreamData(st); len(data) > 0 {
				out = append(out, data)
			}
		}
	}
	return out
}

// hasLTVMaterial reports whether the document carries what a verifier needs to
// check this signature without a network: the signer certificate and a
// revocation answer covering it.
func (d *Document) hasLTVMaterial(cert, issuer *x509.Certificate) bool {
	if cert == nil || issuer == nil {
		return false
	}
	dss, ok := resolveRefToDict(d.objects, d.catalog["/DSS"])
	if !ok || len(d.resolveArray(dss["/Certs"])) == 0 {
		return false
	}
	for _, der := range d.dssMaterial() {
		if st := statusFromOCSPDER(der, cert, issuer); st != nil && st.Err == nil {
			return true
		}
		if _, _, _, _, err := checkCRL(der, cert, issuer); err == nil {
			return true
		}
	}
	return false
}

// buildSignedWithLTV writes the signature, reopens the result and appends the
// validation material as a further revision. Two appended revisions is what
// Acrobat produces for an LTV-enabled signature, and the material can only be
// tied to a signature that already exists.
func buildSignedWithLTV(d *Document) ([]byte, error) {
	cfg := *d.sign
	cfg.ltv = false
	saved := d.sign
	d.sign = &cfg
	signed, err := buildDocumentPDF(d)
	d.sign = saved
	if err != nil {
		return nil, err
	}
	reopened, err := OpenStream(bytes.NewReader(signed))
	if err != nil {
		return nil, fmt.Errorf("sign: reopening the signed document for LTV failed: %w", err)
	}
	if err := reopened.AddValidationInfo(ValidationOptions{Revocation: RevocationOnline}); err != nil {
		return nil, fmt.Errorf("sign: LTV: %w", err)
	}
	return reopened.source, nil
}
