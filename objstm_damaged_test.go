// SPDX-License-Identifier: MIT

package asposepdf_test

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

// buildObjStmPDF builds a PDF whose catalog, page tree and page are members of
// one object stream (object 4) listed in a cross-reference stream. dict is
// the object stream's dictionary text after "/Type /ObjStm"; body is its data.
func buildObjStmPDF(dict, body string) []byte {
	var buf bytes.Buffer
	buf.WriteString("%PDF-1.5\n")
	off4 := buf.Len()
	fmt.Fprintf(&buf, "4 0 obj\n<< /Type /ObjStm %s /Length %d >>\nstream\n%s\nendstream\nendobj\n", dict, len(body), body)
	off5 := buf.Len()
	// W [1 2 1]: type, field2, field3. Objects 1-3 are compressed in stream 4.
	var x bytes.Buffer
	entry := func(a, b, c int) { x.Write([]byte{byte(a), byte(b >> 8), byte(b), byte(c)}) }
	entry(0, 0, 255)
	entry(2, 4, 0)
	entry(2, 4, 1)
	entry(2, 4, 2)
	entry(1, off4, 0)
	entry(1, off5, 0)
	fmt.Fprintf(&buf, "5 0 obj\n<< /Type /XRef /Size 6 /W [1 2 1] /Root 1 0 R /Length %d >>\nstream\n", x.Len())
	buf.Write(x.Bytes())
	fmt.Fprintf(&buf, "\nendstream\nendobj\nstartxref\n%d\n%%%%EOF\n", off5)
	return buf.Bytes()
}

// TestOpenDamagedObjectStream: an object stream whose /N, /First or header
// offsets are wrong must produce an error or a skipped object, never a panic.
func TestOpenDamagedObjectStream(t *testing.T) {
	members := "<< /Type /Catalog /Pages 2 0 R >> <</Type /Pages /Count 1 /Kids [3 0 R] /MediaBox [0 0 10 10]>> <</Type /Page /Parent 2 0 R>>"
	hdr := "1 0 2 28 3 80 "
	good := hdr + members
	cases := map[string][2]string{
		"valid":            {fmt.Sprintf("/N 3 /First %d", len(hdr)), good},
		"first past data":  {"/N 3 /First 99999", good},
		"negative first":   {"/N 3 /First -5", good},
		"huge N":           {fmt.Sprintf("/N 2000000000 /First %d", len(hdr)), good},
		"negative N":       {fmt.Sprintf("/N -1 /First %d", len(hdr)), good},
		"offset past data": {fmt.Sprintf("/N 3 /First %d", len(hdr)), "1 0 2 5000 3 80 " + members},
		"negative offset":  {fmt.Sprintf("/N 3 /First %d", len(hdr)), "1 0 2 -40 3 80 " + members},
		"huge offset":      {fmt.Sprintf("/N 3 /First %d", len(hdr)), "1 0 2 99999999999 3 80 " + members},
		"offset at end":    {fmt.Sprintf("/N 3 /First %d", len(hdr)), hdr + ""},
		"missing keys":     {"", good},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("panic: %v", r)
				}
			}()
			doc, err := pdf.OpenStream(bytes.NewReader(buildObjStmPDF(c[0], c[1])))
			if err == nil && doc == nil {
				t.Fatal("nil document without error")
			}
			if name == "valid" && err != nil {
				t.Fatalf("valid fixture rejected: %v", err)
			}
			path := filepath.Join(t.TempDir(), "d.pdf")
			if werr := os.WriteFile(path, buildObjStmPDF(c[0], c[1]), 0o600); werr != nil {
				t.Fatal(werr)
			}
			_, _ = pdf.Validate(path) // must not panic either
		})
	}
}
