// SPDX-License-Identifier: MIT

package asposepdf_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

// skipIfNoPypdf skips the test if pypdf is not available.
func skipIfNoPypdf(t *testing.T) {
	t.Helper()
	cmd := exec.Command("python", "-c", "import pypdf")
	if err := cmd.Run(); err != nil {
		t.Skip("pypdf not available — skipping cross-tool test")
	}
}

// TestAES128_ReadableByPypdf verifies that our AES-128 encrypted output
// can be successfully read and decrypted by pypdf.
func TestAES128_ReadableByPypdf(t *testing.T) {
	skipIfNoPypdf(t)

	// Create a new document with some content.
	doc := pdf.NewDocument(595, 842)
	page, err := doc.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	err = page.AddText("Cross tool content", pdf.TextStyle{Font: pdf.FontHelvetica, Size: 12},
		pdf.Rectangle{LLX: 50, LLY: 700, URX: 545, URY: 720})
	if err != nil {
		t.Fatal(err)
	}

	// Encrypt with AES-128.
	doc.SetEncryption(pdf.EncryptionOptions{
		UserPassword: "x",
		Algorithm:    pdf.EncryptionAlgAES128,
	})

	// Save to a temporary file.
	tmp, err := os.CreateTemp("", "aes-readable-*.pdf")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmp.Name())
	if _, err := doc.WriteTo(tmp); err != nil {
		t.Fatal(err)
	}
	tmp.Close()

	// Use pypdf to decrypt and extract text from our encrypted PDF.
	script := `
import sys
from pypdf import PdfReader
r = PdfReader(r"` + filepath.ToSlash(tmp.Name()) + `")
if r.is_encrypted:
    r.decrypt("x")
print(r.pages[0].extract_text())
`
	out, err := exec.Command("python", "-c", script).Output()
	if err != nil {
		t.Fatalf("pypdf failed to read our AES output: %v", err)
	}
	if !strings.Contains(string(out), "Cross tool") {
		t.Errorf("pypdf extracted text missing expected content: %q", out)
	}
}

// TestAES128_ReadsPypdfOutput verifies that we can successfully decrypt
// and open PDFs encrypted by pypdf using AES-128.
func TestAES128_ReadsPypdfOutput(t *testing.T) {
	skipIfNoPypdf(t)

	// Create a temporary file to hold pypdf's output.
	tmp, err := os.CreateTemp("", "aes-from-pypdf-*.pdf")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmp.Name())
	tmp.Close()

	// Use pypdf to create an AES-128 encrypted PDF.
	script := `
from pypdf import PdfWriter
w = PdfWriter()
w.add_blank_page(width=595, height=842)
w.encrypt(user_password="x", owner_password="o", algorithm="AES-128")
with open(r"` + filepath.ToSlash(tmp.Name()) + `", "wb") as f:
    w.write(f)
`
	if err := exec.Command("python", "-c", script).Run(); err != nil {
		t.Fatalf("pypdf failed to build AES-128 PDF: %v", err)
	}

	// Verify pypdf's encrypted PDF exists and has content.
	raw, err := os.ReadFile(tmp.Name())
	if err != nil {
		t.Fatalf("read pypdf output: %v", err)
	}
	if len(raw) == 0 {
		t.Fatal("pypdf output is empty")
	}

	// Attempt to open and decrypt with our OpenStreamWithPassword.
	doc, err := pdf.OpenStreamWithPassword(bytes.NewReader(raw), "x")
	if err != nil {
		t.Errorf("our OpenStreamWithPassword on pypdf AES-128 output: %v", err)
	}
	if doc != nil && doc.PageCount() != 1 {
		t.Errorf("expected 1 page from pypdf blank page, got %d", doc.PageCount())
	}
}
