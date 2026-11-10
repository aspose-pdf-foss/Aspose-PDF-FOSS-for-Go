// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"strings"
)

// pdfVersion is a PDF version as major*10+minor (14 = PDF 1.4, 20 = PDF 2.0);
// 0 means unknown.
type pdfVersion int

const (
	pdfVersionDefault pdfVersion = 14 // what a built document is written as
	pdfVersion17      pdfVersion = 17
	pdfVersion20      pdfVersion = 20
)

func (v pdfVersion) header() string { return fmt.Sprintf("%%PDF-%d.%d\n", v/10, v%10) }

// headerVersion reads the version from a file's header (ISO 32000-1 §7.5.2),
// which a reader finds anywhere in the first 1024 bytes. Only versions that
// exist (1.0 to 1.7, 2.0) count, so a damaged or invented number is never
// carried into the output.
func headerVersion(data []byte) pdfVersion {
	head := data[:min(len(data), 1024)]
	i := bytes.Index(head, []byte("%PDF-"))
	if i < 0 || i+8 > len(head) {
		return 0
	}
	if i+8 < len(head) && head[i+8] >= '0' && head[i+8] <= '9' {
		return 0 // "1.10" and the like
	}
	return parseVersion(string(head[i+5 : i+8]))
}

// outputVersion is the version written in the header on save: never lower than
// the file the document was opened from (or the highest of the documents
// appended into it), or than 1.4 unless SetVersion chose a lower one, and at
// least what the features this writer emits need.
func (d *Document) outputVersion(encState *encryptState) pdfVersion {
	v := d.version
	if !d.versionSet {
		v = max(pdfVersionDefault, v)
	}
	if encState != nil {
		switch encState.algorithm {
		case EncryptionAlgAES128:
			v = max(v, 16) // AESV2 crypt filter, ISO 32000-1 §7.6.5
		case EncryptionAlgAES256:
			v = pdfVersion20 // revision 6 is defined by ISO 32000-2
		}
	}
	switch d.claimedPDFAPart() {
	case "4":
		v = pdfVersion20 // PDF/A-4 is defined on PDF 2.0
	case "1", "2", "3":
		v = min(v, pdfVersion17) // their header must read %PDF-1.n
	}
	return v
}

// parseVersion reads "1.0" to "1.7" or "2.0"; anything else is 0.
func parseVersion(s string) pdfVersion {
	if len(s) != 3 || s[1] != '.' || s[0] < '0' || s[0] > '9' || s[2] < '0' || s[2] > '9' {
		return 0
	}
	v := pdfVersion(s[0]-'0')*10 + pdfVersion(s[2]-'0')
	if (v >= 10 && v <= 17) || v == 20 {
		return v
	}
	return 0
}

func (v pdfVersion) String() string { return fmt.Sprintf("%d.%d", v/10, v%10) }

// Version returns the PDF version of the document, such as "1.7" or "2.0": the
// later of the version in the header of the file it was opened from and the
// catalog's /Version, which an incremental update uses to raise it (ISO
// 32000-1 §7.2.2). A document built in memory reports "1.4", what it is
// written as. Mirrors Aspose.PDF for .NET's Document.Version.
//
// Saving never writes a lower version, and may write a higher one when the
// output needs it: AES-128 encryption needs 1.6, AES-256 and PDF/A-4 need 2.0,
// and PDF/A-1, 2 and 3 are written as at most 1.7, as those parts require.
func (d *Document) Version() string {
	v := d.version
	if n, ok := d.catalog["/Version"].(pdfName); ok {
		v = max(v, parseVersion(strings.TrimPrefix(string(n), "/")))
	}
	if v == 0 {
		v = pdfVersionDefault
	}
	return v.String()
}

// SetVersion sets the PDF version the document is written as: "1.0" to "1.7"
// or "2.0". It replaces any catalog /Version, so Version reports exactly v
// afterwards. Saving may still write a higher version where the output needs
// it (see Version); setting a version below what the document's own content
// uses is the caller's decision. Mirrors creating an Aspose.PDF for .NET
// Document with a PdfVersion.
func (d *Document) SetVersion(v string) error {
	pv := parseVersion(v)
	if pv == 0 {
		return fmt.Errorf("SetVersion: %q is not a PDF version (1.0 to 1.7, or 2.0)", v)
	}
	d.version, d.versionSet = pv, true
	if d.catalog != nil {
		delete(d.catalog, "/Version")
	}
	return nil
}

// DocumentID is a file identifier (the trailer /ID, ISO 32000-1 §14.4) as
// upper-case hexadecimal. Original is fixed when the file is first written;
// Modified changes with each revision. Mirrors Aspose.PDF for .NET's
// Document.Id with its Original and Modified strings.
type DocumentID struct {
	Original string
	Modified string
}

// ID returns the file identifier of the file the document was opened from;
// ok is false for a document built in memory or a file without one. Saving
// keeps it (and gives a document without one an identifier derived from its
// content).
func (d *Document) ID() (id DocumentID, ok bool) {
	if len(d.docID) != 2 {
		return DocumentID{}, false
	}
	part := func(v pdfValue) (string, bool) {
		switch s := v.(type) {
		case pdfHexString:
			return strings.ToUpper(hex.EncodeToString(s)), true
		case string:
			return strings.ToUpper(hex.EncodeToString([]byte(s))), true
		}
		return "", false
	}
	o, ok1 := part(d.docID[0])
	m, ok2 := part(d.docID[1])
	if !ok1 || !ok2 {
		return DocumentID{}, false
	}
	return DocumentID{Original: o, Modified: m}, true
}
