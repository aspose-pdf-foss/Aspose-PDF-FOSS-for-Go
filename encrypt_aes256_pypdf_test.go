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

func TestAES256_ReadableByPypdf(t *testing.T) {
	skipIfNoPypdf(t)
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	page.AddText("AES-256 cross tool", pdf.TextStyle{Font: pdf.FontHelvetica, Size: 12},
		pdf.Rectangle{LLX: 50, LLY: 700, URX: 545, URY: 720})
	doc.SetEncryption(pdf.EncryptionOptions{UserPassword: "x", Algorithm: pdf.EncryptionAlgAES256})

	tmp, err := os.CreateTemp("", "aes256-readable-*.pdf")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmp.Name())
	if _, err := doc.WriteTo(tmp); err != nil {
		t.Fatal(err)
	}
	tmp.Close()

	script := `
from pypdf import PdfReader
r = PdfReader(r"` + filepath.ToSlash(tmp.Name()) + `")
if r.is_encrypted:
    r.decrypt("x")
print(r.pages[0].extract_text())
`
	cmd := exec.Command("python", "-c", script)
	out, err := cmd.Output()
	if err != nil {
		// Capture stderr for better error reporting
		if ee, ok := err.(*exec.ExitError); ok {
			t.Fatalf("pypdf failed to read our AES-256 output: %v\nStderr: %s", err, string(ee.Stderr))
		}
		t.Fatalf("pypdf failed to read our AES-256 output: %v", err)
	}
	if !strings.Contains(string(out), "AES-256") {
		t.Errorf("pypdf extracted text missing expected content: %q", out)
	}
}

func TestAES256_ReadsPypdfOutput(t *testing.T) {
	skipIfNoPypdf(t)
	tmp, err := os.CreateTemp("", "aes256-from-pypdf-*.pdf")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmp.Name())
	tmp.Close()

	script := `
from pypdf import PdfWriter
w = PdfWriter()
w.add_blank_page(width=595, height=842)
w.encrypt(user_password="x", owner_password="o", algorithm="AES-256")
with open(r"` + filepath.ToSlash(tmp.Name()) + `", "wb") as f:
    w.write(f)
`
	if err := exec.Command("python", "-c", script).Run(); err != nil {
		t.Fatalf("pypdf failed to build AES-256 PDF: %v", err)
	}
	raw, _ := os.ReadFile(tmp.Name())
	if _, err := pdf.OpenStreamWithPassword(bytes.NewReader(raw), "x"); err != nil {
		t.Errorf("our OpenStreamWithPassword on pypdf AES-256 output: %v", err)
	}
}
