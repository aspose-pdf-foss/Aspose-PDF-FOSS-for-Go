// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"fmt"
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
	maj, dot, mnr := head[i+5], head[i+6], head[i+7]
	if dot != '.' || maj < '0' || maj > '9' || mnr < '0' || mnr > '9' {
		return 0
	}
	if i+8 < len(head) && head[i+8] >= '0' && head[i+8] <= '9' {
		return 0 // "1.10" and the like
	}
	v := pdfVersion(maj-'0')*10 + pdfVersion(mnr-'0')
	if (v >= 10 && v <= 17) || v == 20 {
		return v
	}
	return 0
}

// outputVersion is the version written in the header on save: never lower than
// the file the document was opened from (or the highest of the documents
// appended into it), and at least what the features this writer emits need.
func (d *Document) outputVersion(encState *encryptState) pdfVersion {
	v := max(pdfVersionDefault, d.version)
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
