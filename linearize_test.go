// SPDX-License-Identifier: MIT

package asposepdf_test

import (
	"bytes"
	"path/filepath"
	"testing"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

func buildLinDoc(t *testing.T, pages int) *pdf.Document {
	t.Helper()
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	for i := 1; i < pages; i++ {
		if err := doc.AddBlankPageFromFormat(pdf.PageFormatA4); err != nil {
			t.Fatalf("AddBlankPage: %v", err)
		}
	}
	for i := 1; i <= pages; i++ {
		p, err := doc.Page(i)
		if err != nil {
			t.Fatal(err)
		}
		if err := p.DrawRectangle(pdf.Rectangle{LLX: 50, LLY: 50, URX: 250, URY: 250},
			pdf.ShapeStyle{LineStyle: pdf.LineStyle{Color: &pdf.Color{A: 1}, Width: 2}}); err != nil {
			t.Fatalf("DrawRectangle: %v", err)
		}
	}
	return doc
}

// TestLinearizeRoundTrip checks that a linearized document re-opens with the
// same page count and stays a valid, readable PDF.
func TestLinearizeRoundTrip(t *testing.T) {
	for _, n := range []int{1, 2, 3, 5} {
		doc := buildLinDoc(t, n)
		var buf bytes.Buffer
		written, err := doc.WriteToLinearized(&buf)
		if err != nil {
			t.Fatalf("WriteToLinearized(%d pages): %v", n, err)
		}
		if int(written) != buf.Len() {
			t.Errorf("WriteToLinearized returned %d, wrote %d", written, buf.Len())
		}
		data := buf.Bytes()
		if !bytes.HasPrefix(data, []byte("%PDF-")) {
			t.Errorf("output not a PDF (%d pages)", n)
		}
		// The linearization parameter dictionary must be reachable from the
		// front of the file (ISO 32000-1 Annex F: within the first 1024 bytes).
		head := data
		if len(head) > 1024 {
			head = head[:1024]
		}
		if !bytes.Contains(head, []byte("/Linearized")) {
			t.Errorf("/Linearized dict not within first 1024 bytes (%d pages)", n)
		}
		out, err := pdf.OpenStream(bytes.NewReader(data))
		if err != nil {
			t.Fatalf("reopen linearized (%d pages): %v", n, err)
		}
		if out.PageCount() != n {
			t.Errorf("reopened page count = %d, want %d", out.PageCount(), n)
		}
	}
}

// TestSaveLinearized writes to a file and re-opens it.
func TestSaveLinearized(t *testing.T) {
	doc := buildLinDoc(t, 3)
	path := filepath.Join(t.TempDir(), "lin.pdf")
	if err := doc.SaveLinearized(path); err != nil {
		t.Fatalf("SaveLinearized: %v", err)
	}
	out, err := pdf.Open(path)
	if err != nil {
		t.Fatalf("Open linearized: %v", err)
	}
	if out.PageCount() != 3 {
		t.Errorf("page count = %d, want 3", out.PageCount())
	}
}

// TestLinearizeRejectsEncryptionAndSigning checks the documented restrictions.
func TestLinearizeRejectsEncryptionAndSigning(t *testing.T) {
	doc := buildLinDoc(t, 2)
	doc.SetEncryption(pdf.EncryptionOptions{UserPassword: "u", OwnerPassword: "o"})
	if err := doc.SaveLinearized(filepath.Join(t.TempDir(), "e.pdf")); err == nil {
		t.Error("expected error linearizing an encrypted document")
	}

	empty := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	// Remove the only page to exercise the no-pages guard.
	if err := empty.DeletePage(1); err == nil {
		var buf bytes.Buffer
		if _, err := empty.WriteToLinearized(&buf); err == nil {
			t.Error("expected error linearizing a document with no pages")
		}
	}
}
