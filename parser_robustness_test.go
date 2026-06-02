// SPDX-License-Identifier: MIT

package asposepdf_test

import (
	"bytes"
	"strings"
	"testing"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

// buildPDF assembles a minimal one-page PDF from object bodies, computing
// a traditional xref table with the given line-ending style ("\n", "\r",
// or "\r\n") so tests can exercise the parser's tolerance.
func buildPDF(t *testing.T, eol string, objects []string) []byte {
	t.Helper()
	var buf bytes.Buffer
	buf.WriteString("%PDF-1.7\n")
	offsets := make([]int, len(objects)+1)
	for i, body := range objects {
		offsets[i+1] = buf.Len()
		buf.WriteString(body)
		if !strings.HasSuffix(body, "\n") {
			buf.WriteByte('\n')
		}
	}
	xrefStart := buf.Len()
	// Cross-reference table with the requested EOL between entries.
	buf.WriteString("xref" + eol)
	buf.WriteString("0 " + itoa(len(objects)+1) + eol)
	buf.WriteString("0000000000 65535 f " + eol)
	for i := 1; i <= len(objects); i++ {
		buf.WriteString(pad10(offsets[i]) + " 00000 n " + eol)
	}
	buf.WriteString("trailer\n")
	buf.WriteString("<< /Size " + itoa(len(objects)+1) + " /Root 1 0 R >>\n")
	buf.WriteString("startxref\n")
	buf.WriteString(itoa(xrefStart) + "\n")
	buf.WriteString("%%EOF")
	return buf.Bytes()
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

func pad10(n int) string {
	s := itoa(n)
	for len(s) < 10 {
		s = "0" + s
	}
	return s
}

// TestOpenIndirectStreamLength opens a PDF whose content stream uses an
// indirect /Length (/Length N 0 R) — valid per ISO 32000-1 §7.3.8.2 and
// emitted by several real-world producers. The parser must resolve it (or
// fall back to scanning for endstream) rather than failing.
func TestOpenIndirectStreamLength(t *testing.T) {
	content := "BT /F1 12 Tf 72 720 Td (Hi) Tj ET"
	objects := []string{
		"1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj",
		"2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj",
		"3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] /Contents 4 0 R " +
			"/Resources << /Font << /F1 << /Type /Font /Subtype /Type1 /BaseFont /Helvetica >> >> >> >>\nendobj",
		// Stream length is an indirect reference to object 5.
		"4 0 obj\n<< /Length 5 0 R >>\nstream\n" + content + "\nendstream\nendobj",
		"5 0 obj\n" + itoa(len(content)) + "\nendobj",
	}
	data := buildPDF(t, "\n", objects)

	doc, err := pdf.OpenStream(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("OpenStream with indirect /Length: %v", err)
	}
	if doc.PageCount() != 1 {
		t.Errorf("PageCount = %d, want 1", doc.PageCount())
	}
	page, _ := doc.Page(1)
	got, err := page.ExtractText()
	if err != nil {
		t.Fatalf("ExtractText: %v", err)
	}
	if !strings.Contains(got, "Hi") {
		t.Errorf("extracted text = %q, want it to contain %q", got, "Hi")
	}
}

// TestOpenReconstructsBrokenXref opens a PDF whose xref subsection header
// is off by one ("1 N" while the first row is the object-0 free entry, as
// seen in real producer output). The normal parse resolves the catalog to
// the wrong offset; the library must fall back to reconstructing the xref
// by scanning for object headers.
func TestOpenReconstructsBrokenXref(t *testing.T) {
	objects := []string{
		"1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj",
		"2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj",
		"3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] >>\nendobj",
	}
	var buf bytes.Buffer
	buf.WriteString("%PDF-1.7\n")
	offsets := make([]int, len(objects)+1)
	for i, body := range objects {
		offsets[i+1] = buf.Len()
		buf.WriteString(body + "\n")
	}
	xrefStart := buf.Len()
	// Deliberately wrong subsection start: "1" instead of "0", so the
	// object-0 free row shifts every real object's offset by one.
	buf.WriteString("xref\n")
	buf.WriteString("1 " + itoa(len(objects)+1) + "\n")
	buf.WriteString("0000000000 65535 f \n")
	for i := 1; i <= len(objects); i++ {
		buf.WriteString(pad10(offsets[i]) + " 00000 n \n")
	}
	buf.WriteString("trailer\n<< /Size " + itoa(len(objects)+1) + " /Root 1 0 R >>\n")
	buf.WriteString("startxref\n" + itoa(xrefStart) + "\n%%EOF")

	doc, err := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("OpenStream with off-by-one xref: %v", err)
	}
	if doc.PageCount() != 1 {
		t.Errorf("PageCount = %d, want 1 (reconstruction should recover the page)", doc.PageCount())
	}
}

// TestOpenCROnlyXref opens a PDF whose cross-reference table uses
// classic-Mac CR-only (\r) line endings. The xref entry reader must treat
// CR as a line terminator, not just LF.
func TestOpenCROnlyXref(t *testing.T) {
	objects := []string{
		"1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj",
		"2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj",
		"3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] >>\nendobj",
	}
	data := buildPDF(t, "\r", objects)

	doc, err := pdf.OpenStream(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("OpenStream with CR-only xref: %v", err)
	}
	if doc.PageCount() != 1 {
		t.Errorf("PageCount = %d, want 1", doc.PageCount())
	}
}
