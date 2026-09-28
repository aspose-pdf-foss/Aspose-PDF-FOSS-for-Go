// SPDX-License-Identifier: MIT

package asposepdf

import "testing"

// addRawLZWStream injects a bare object using /Filter /LZWDecode — the
// library never writes this filter itself (LZWDecode is read-only support),
// so there is no public-API way to produce one; a document that merely
// passed through Open on a file using it would carry the same dict shape.
func addRawLZWStream(doc *Document) {
	data := []byte{0x80, 0x0B, 0x60, 0x50, 0x22, 0x0C, 0x0C, 0x85, 0x01}
	id := doc.nextID
	doc.nextID++
	doc.objects[id] = &pdfObject{Num: id, Value: &pdfStream{
		Dict: pdfDict{
			"/Filter": pdfName("/LZWDecode"),
			"/Length": len(data),
		},
		Data:    data,
		Decoded: false,
	}}
}

// TestValidatePDFA1FlagsLZWFilter covers the LZW_FILTER rule, which had zero
// test coverage before this — PDF/A-1 (both "a" and "b" conformance, ISO
// 19005-1) prohibits the LZWDecode filter; PDF/A-2/-3 permit it. The checker
// used to gate on == PDFA1B specifically (mirroring the same bug fixed for
// TRANSPARENCY), so PDFA1A silently skipped the check.
func TestValidatePDFA1FlagsLZWFilter(t *testing.T) {
	for _, format := range []PDFAFormat{PDFA1B, PDFA1A} {
		doc := NewDocumentFromFormat(PageFormatA4)
		addRawLZWStream(doc)
		rep := doc.ValidatePDFA(format)
		found := false
		for _, iss := range rep.Issues {
			if iss.Rule == "LZW_FILTER" {
				found = true
			}
		}
		if !found {
			t.Errorf("format %v: expected LZW_FILTER for a document with an LZWDecode stream", format)
		}
	}

	doc := NewDocumentFromFormat(PageFormatA4)
	addRawLZWStream(doc)
	rep := doc.ValidatePDFA(PDFA2B)
	for _, iss := range rep.Issues {
		if iss.Rule == "LZW_FILTER" {
			t.Error("PDF/A-2 permits LZWDecode; LZW_FILTER should not be reported")
		}
	}
}
