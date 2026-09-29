// SPDX-License-Identifier: MIT

package asposepdf_test

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

// pypdfEncryptR5 has pypdf encrypt a plain PDF of ours with AES-256
// revision 5 (Adobe Extension Level 3), user "x", owner "o".
func pypdfEncryptR5(t *testing.T, plain []byte) []byte {
	t.Helper()
	dir := t.TempDir()
	in, out := filepath.Join(dir, "in.pdf"), filepath.Join(dir, "out.pdf")
	if err := os.WriteFile(in, plain, 0o600); err != nil {
		t.Fatal(err)
	}
	script := `
from pypdf import PdfReader, PdfWriter
w = PdfWriter(clone_from=PdfReader(r"` + filepath.ToSlash(in) + `"))
w.encrypt(user_password="x", owner_password="o", algorithm="AES-256-R5")
with open(r"` + filepath.ToSlash(out) + `", "wb") as f:
    w.write(f)
`
	if msg, err := exec.Command("python", "-c", script).CombinedOutput(); err != nil {
		t.Fatalf("pypdf failed to build AES-256-R5 PDF: %v\n%s", err, msg)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(data, []byte("/R 5")) {
		t.Fatal("pypdf output is not revision 5")
	}
	return data
}

// TestAES256R5_ReadsPypdfOutput opens a revision-5 PDF written by pypdf with
// the user and owner passwords, re-saves it, and checks that the re-save
// keeps revision 5 and still opens in both this library and pypdf with the
// original passwords, while ChangePassword upgrades the document to R=6.
func TestAES256R5_ReadsPypdfOutput(t *testing.T) {
	skipIfNoPypdf(t)
	src := pdf.NewDocument(595, 842)
	page, _ := src.Page(1)
	page.AddText("AES-256 revision 5", pdf.TextStyle{Font: pdf.FontHelvetica, Size: 12},
		pdf.Rectangle{LLX: 50, LLY: 700, URX: 545, URY: 720})
	var plain bytes.Buffer
	if _, err := src.WriteTo(&plain); err != nil {
		t.Fatal(err)
	}
	data := pypdfEncryptR5(t, plain.Bytes())

	if _, err := pdf.OpenStreamWithPassword(bytes.NewReader(data), "wrong"); !errors.Is(err, pdf.ErrInvalidPassword) {
		t.Errorf("wrong password: err = %v, want ErrInvalidPassword", err)
	}
	var doc *pdf.Document
	for _, pw := range []string{"x", "o"} {
		d, err := pdf.OpenStreamWithPassword(bytes.NewReader(data), pw)
		if err != nil {
			t.Fatalf("password %q: %v", pw, err)
		}
		if text, _ := d.ExtractText(); len(text) != 1 || !strings.Contains(text[0], "AES-256 revision 5") {
			t.Errorf("password %q: text = %q", pw, text)
		}
		doc = d
	}

	out := filepath.Join(t.TempDir(), "resaved.pdf")
	if err := doc.Save(out); err != nil {
		t.Fatal(err)
	}
	resaved, _ := os.ReadFile(out)
	if !bytes.Contains(resaved, []byte("/R 5")) {
		t.Error("re-save did not keep /R 5")
	}
	for _, pw := range []string{"x", "o"} {
		if _, err := pdf.OpenStreamWithPassword(bytes.NewReader(resaved), pw); err != nil {
			t.Errorf("re-save, password %q: %v", pw, err)
		}
	}
	script := `
from pypdf import PdfReader
r = PdfReader(r"` + filepath.ToSlash(out) + `")
r.decrypt("x")
print(r.pages[0].extract_text())
`
	got, err := exec.Command("python", "-c", script).CombinedOutput()
	if err != nil || !strings.Contains(string(got), "AES-256 revision 5") {
		t.Errorf("pypdf on re-save: %v\n%s", err, got)
	}

	// Changing the password drops the preserved state and upgrades to R=6.
	if err := doc.ChangePassword("new", ""); err != nil {
		t.Fatal(err)
	}
	var upgraded bytes.Buffer
	if _, err := doc.WriteTo(&upgraded); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(upgraded.Bytes(), []byte("/R 6")) {
		t.Error("ChangePassword did not upgrade to /R 6")
	}
	if _, err := pdf.OpenStreamWithPassword(bytes.NewReader(upgraded.Bytes()), "new"); err != nil {
		t.Errorf("after ChangePassword: %v", err)
	}
}

// TestAES256R5_XRefStreamWrongPasswordRejected is the revision-5 case of
// TestXRefStreamWrongPasswordRejected: pypdf's R=5 output rewritten with only
// a cross-reference stream must still reject a wrong password.
func TestAES256R5_XRefStreamWrongPasswordRejected(t *testing.T) {
	skipIfNoPypdf(t)
	var plain bytes.Buffer
	if _, err := pdf.NewDocument(595, 842).WriteTo(&plain); err != nil {
		t.Fatal(err)
	}
	data := toXRefStreamOnly(t, pypdfEncryptR5(t, plain.Bytes()))
	if _, err := pdf.OpenStreamWithPassword(bytes.NewReader(data), "wrong"); !errors.Is(err, pdf.ErrInvalidPassword) {
		t.Errorf("wrong password: err = %v, want ErrInvalidPassword", err)
	}
	if _, err := pdf.OpenStreamWithPassword(bytes.NewReader(data), "x"); err != nil {
		t.Errorf("user password: %v", err)
	}
}
