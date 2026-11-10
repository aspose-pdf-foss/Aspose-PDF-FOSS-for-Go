// SPDX-License-Identifier: MIT

package asposepdf

import (
	"errors"
	"fmt"
)

// ErrDeprecatedFeature is returned by ConvertToPDF20 when the document relies
// on something PDF 2.0 deprecates and the conversion cannot drop on its own:
// encryption other than AES-256 revision 6. Mirrors Aspose.PDF for .NET's
// DeprecatedFeatureException.
var ErrDeprecatedFeature = errors.New("feature deprecated in PDF 2.0")

// pdf20DeprecatedSubFilters are the signature formats ISO 32000-2 §12.8.3
// deprecates: both hash with SHA-1.
var pdf20DeprecatedSubFilters = map[pdfName]bool{"/adbe.pkcs7.sha1": true, "/adbe.x509.rsa_sha1": true}

// ConvertToPDF20 makes the document a PDF 2.0 document (ISO 32000-2): it is
// written with a %PDF-2.0 header, and what PDF 2.0 deprecates is removed —
//
//   - the document information dictionary, but for /CreationDate and /ModDate:
//     the rest moves into the XMP metadata (custom entries under the pdfx
//     namespace), where PDF 2.0 keeps it;
//   - /CharSet and /CIDSet in font descriptors, and /ProcSet in resources;
//   - /Suspects in /MarkInfo;
//   - usage-rights signatures (/UR, /UR3 in the catalog /Perms);
//   - signatures in the SHA-1 formats adbe.pkcs7.sha1 and adbe.x509.rsa_sha1:
//     the signature field stays, unsigned.
//
// Encryption other than AES-256 is deprecated too, and is not changed
// silently: ConvertToPDF20 returns an error wrapping ErrDeprecatedFeature and
// changes nothing; call SetEncryption with EncryptionAlgAES256 (or
// RemoveEncryption) first. A document identifying itself as PDF/A-1, 2 or 3 is
// refused as well, as those parts require a PDF 1.x file; convert it with
// ConvertToPDFA(PDFA4) instead.
//
// Mirrors Aspose.PDF for .NET's Document.Convert with PdfFormat.v_2_0.
func (d *Document) ConvertToPDF20() error {
	if alg, ok := d.encryptionInEffect(); ok && alg != EncryptionAlgAES256 {
		return fmt.Errorf("ConvertToPDF20: the document is encrypted with %s: %w; use EncryptionAlgAES256", algorithmName(alg), ErrDeprecatedFeature)
	}
	if d.preserved != nil && d.preserved.revision == 5 {
		return fmt.Errorf("ConvertToPDF20: AES-256 revision 5 encryption: %w; set the encryption again", ErrDeprecatedFeature)
	}
	switch part := d.claimedPDFAPart(); part {
	case "1", "2", "3":
		return fmt.Errorf("ConvertToPDF20: the document is PDF/A-%s, which requires PDF 1.x; use ConvertToPDFA(PDFA4)", part)
	}

	if err := d.moveInfoForPDF20(); err != nil {
		return fmt.Errorf("ConvertToPDF20: %w", err)
	}
	d.removePDF20Deprecated()
	d.version, d.versionSet = pdfVersion20, true
	delete(d.catalog, "/Version")
	return nil
}

// encryptionInEffect is the algorithm the next save encrypts with, if any.
func (d *Document) encryptionInEffect() (EncryptionAlgorithm, bool) {
	switch {
	case d.encrypt != nil:
		return d.encrypt.algorithm, true
	case d.preserved != nil:
		return d.preserved.algorithm, true
	}
	return 0, false
}

func algorithmName(a EncryptionAlgorithm) string {
	switch a {
	case EncryptionAlgAES128:
		return "AES-128"
	case EncryptionAlgRC4_128:
		return "RC4-128"
	case EncryptionAlgRC4_40:
		return "RC4-40"
	}
	return "AES-256"
}

// moveInfoForPDF20 puts the /Info entries into the XMP packet and leaves only
// the dates in /Info, which is what PDF 2.0 still allows there (ISO 32000-2
// §14.3.3). XMP extension schemas are carried over whole.
func (d *Document) moveInfoForPDF20() error {
	raw, _ := d.XMPRaw()
	var keep pdfDict
	for _, k := range []string{"/CreationDate", "/ModDate"} {
		if v, ok := d.info[k]; ok {
			if keep == nil {
				keep = pdfDict{}
			}
			keep[k] = v
		}
	}
	if len(raw) == 0 && len(d.info) == len(keep) {
		return nil // nothing to move
	}
	var extensions []string
	if len(raw) > 0 {
		extensions, _ = xmpExtensionBlocks(string(raw))
	}
	meta, _ := d.XMP()
	d.reconcileInfoAndXMP(&meta)
	d.moveCustomInfoToXMP(&meta)
	if err := d.SetXMP(meta); err != nil {
		return err
	}
	d.info = keep
	if len(extensions) == 0 {
		return nil
	}
	out, err := d.XMPRaw()
	if err != nil {
		return err
	}
	return d.SetXMPRaw([]byte(insertXMPDescriptions(string(out), extensions)))
}

// removePDF20Deprecated drops the dictionary entries PDF 2.0 deprecates.
func (d *Document) removePDF20Deprecated() {
	if mark, ok := resolveRefToDict(d.objects, d.catalog["/MarkInfo"]); ok {
		delete(mark, "/Suspects")
	}
	removed := map[int]bool{} // signature dictionaries dropped, by object number
	d.pdfaWalkDicts(func(dict pdfDict) {
		delete(dict, "/ProcSet")
		if dict["/Type"] == pdfName("/FontDescriptor") {
			delete(dict, "/CharSet")
			delete(dict, "/CIDSet")
		}
		if dict["/FT"] == pdfName("/Sig") {
			if sig, ok := resolveRefToDict(d.objects, dict["/V"]); ok && pdf20DeprecatedSubFilters[dictName(sig, "/SubFilter")] {
				if r, ok := dict["/V"].(pdfRef); ok {
					removed[r.Num] = true
				}
				delete(dict, "/V")
			}
		}
	})
	if perms, ok := resolveRefToDict(d.objects, d.catalog["/Perms"]); ok {
		delete(perms, "/UR")
		delete(perms, "/UR3")
		if r, ok := perms["/DocMDP"].(pdfRef); ok && removed[r.Num] {
			delete(perms, "/DocMDP") // the certification it named is gone
		}
		if len(perms) == 0 {
			delete(d.catalog, "/Perms")
		}
	}
}

func dictName(d pdfDict, key string) pdfName {
	n, _ := d[key].(pdfName)
	return n
}
