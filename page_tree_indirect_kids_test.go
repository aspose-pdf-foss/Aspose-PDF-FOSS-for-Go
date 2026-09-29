// SPDX-License-Identifier: MIT

package asposepdf_test

import (
	"bytes"
	"fmt"
	"testing"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

// buildPDFWithIndirectKids returns bytes of a minimal valid PDF whose root
// /Pages node stores /Kids and /Count as indirect references rather than a
// direct array and integer:
//
//	3 0 obj << /Type /Pages /Kids 4 0 R /Count 5 0 R >> endobj
//
// ISO 32000-1 §7.3.10 permits any object in a dictionary to be an indirect
// reference; §7.7.3.2 Table 29 does not exempt /Kids or /Count. Page trees in
// this shape occur in output from Oracle Reports (/Producer (Oracle PDF
// driver)) and are read correctly by poppler.
func buildPDFWithIndirectKids() []byte {
	var buf bytes.Buffer
	buf.WriteString("%PDF-1.4\n")
	buf.WriteString("%\xe2\xe3\xcf\xd3\n")

	offsets := map[int]int{}
	writeObj := func(id int, body string) {
		offsets[id] = buf.Len()
		fmt.Fprintf(&buf, "%d 0 obj\n%s\nendobj\n", id, body)
	}

	writeObj(1, "<< /Type /Catalog /Pages 3 0 R >>")
	writeObj(2, "<< /Length 44 >>\nstream\nBT /F0 12 Tf 72 720 Td (indirect kids) Tj ET\nendstream")
	writeObj(3, "<< /Type /Pages /Kids 4 0 R /Count 5 0 R >>") // both indirect
	writeObj(4, "[6 0 R 7 0 R]")                               // the /Kids array
	writeObj(5, "2")                                           // the /Count value
	writeObj(6, "<< /Type /Page /Parent 3 0 R /MediaBox [0 0 612 792] /Contents 2 0 R /Resources << >> >>")
	writeObj(7, "<< /Type /Page /Parent 3 0 R /MediaBox [0 0 612 792] /Contents 2 0 R /Resources << >> >>")

	xrefOff := buf.Len()
	fmt.Fprintf(&buf, "xref\n0 8\n0000000000 65535 f \n")
	for i := 1; i <= 7; i++ {
		fmt.Fprintf(&buf, "%010d 00000 n \n", offsets[i])
	}
	fmt.Fprintf(&buf, "trailer\n<< /Size 8 /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", xrefOff)
	return buf.Bytes()
}

// TestIndirectKidsPageTree verifies that a /Pages node whose /Kids is an
// indirect reference to an array is traversed. Regression: walkPageTreeRec
// type-asserted nodeDict["/Kids"].(pdfArray) directly, so an indirect /Kids
// failed the assertion, the node fell through to the leaf branch, and Open
// silently produced a zero-page document instead of an error.
func TestIndirectKidsPageTree(t *testing.T) {
	doc, err := pdf.OpenStream(bytes.NewReader(buildPDFWithIndirectKids()))
	if err != nil {
		t.Fatalf("OpenStream: %v", err)
	}
	if got := doc.PageCount(); got != 2 {
		t.Fatalf("PageCount() = %d, want 2", got)
	}
	for n := 1; n <= 2; n++ {
		if _, err := doc.Page(n); err != nil {
			t.Errorf("Page(%d): %v", n, err)
		}
	}
}
