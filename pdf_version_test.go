// SPDX-License-Identifier: MIT

package asposepdf_test

import (
	"bytes"
	"testing"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

func savedBytes(t *testing.T, d *pdf.Document) []byte {
	t.Helper()
	var buf bytes.Buffer
	if _, err := d.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// openAsVersion builds a one-page file and rewrites its header to the given
// version (same length, so every offset stays valid), as another producer
// would have written it.
func openAsVersion(t *testing.T, header string) *pdf.Document {
	t.Helper()
	data := savedBytes(t, pdf.NewDocument(200, 200))
	if !bytes.HasPrefix(data, []byte("%PDF-1.4")) {
		t.Fatalf("built file starts %q", data[:8])
	}
	copy(data, header)
	d, err := pdf.OpenStream(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func headerOf(data []byte) string { return string(data[:8]) }

func TestSaveKeepsSourceVersion(t *testing.T) {
	for _, v := range []string{"%PDF-1.5", "%PDF-1.7", "%PDF-2.0"} {
		d := openAsVersion(t, v)
		p, _ := d.Page(1)
		if err := p.AddTextAt("x", pdf.TextStyle{}, 10, 10); err != nil {
			t.Fatal(err)
		}
		if got := headerOf(savedBytes(t, d)); got != v {
			t.Errorf("source %s saved as %s", v, got)
		}
	}
}

func TestSaveNeverRaisesWithoutReason(t *testing.T) {
	if got := headerOf(savedBytes(t, pdf.NewDocument(200, 200))); got != "%PDF-1.4" {
		t.Errorf("new document saved as %s, want %%PDF-1.4", got)
	}
	// An older source is written as 1.4, the oldest this writer produces.
	if got := headerOf(savedBytes(t, openAsVersion(t, "%PDF-1.2"))); got != "%PDF-1.4" {
		t.Errorf("1.2 source saved as %s, want %%PDF-1.4", got)
	}
}

func TestLinearizedKeepsSourceVersion(t *testing.T) {
	var buf bytes.Buffer
	if _, err := openAsVersion(t, "%PDF-2.0").WriteToLinearized(&buf); err != nil {
		t.Fatal(err)
	}
	if got := headerOf(buf.Bytes()); got != "%PDF-2.0" {
		t.Errorf("linearized 2.0 source saved as %s", got)
	}
}

func TestAES128NeedsPDF16(t *testing.T) {
	d := pdf.NewDocument(200, 200)
	d.SetEncryption(pdf.EncryptionOptions{UserPassword: "u", Algorithm: pdf.EncryptionAlgAES128})
	if got := headerOf(savedBytes(t, d)); got != "%PDF-1.6" {
		t.Errorf("AES-128 document saved as %s, want %%PDF-1.6", got)
	}
	d = openAsVersion(t, "%PDF-1.7")
	d.SetEncryption(pdf.EncryptionOptions{UserPassword: "u", Algorithm: pdf.EncryptionAlgRC4_128})
	if got := headerOf(savedBytes(t, d)); got != "%PDF-1.7" {
		t.Errorf("RC4 on a 1.7 source saved as %s, want %%PDF-1.7", got)
	}
}

func TestPDFAKeepsAHeaderItsPartAllows(t *testing.T) {
	d := openAsVersion(t, "%PDF-2.0")
	if _, err := d.ConvertToPDFA(pdf.PDFA2B); err != nil {
		t.Fatal(err)
	}
	if got := headerOf(savedBytes(t, d)); got != "%PDF-1.7" {
		t.Errorf("2.0 source converted to PDF/A-2b saved as %s, want %%PDF-1.7", got)
	}
}

func TestDerivedDocumentsKeepVersion(t *testing.T) {
	src := openAsVersion(t, "%PDF-2.0")
	parts, err := src.Split()
	if err != nil {
		t.Fatal(err)
	}
	ext, err := src.Extract(pdf.PageRange{From: 1, To: 1})
	if err != nil {
		t.Fatal(err)
	}
	nup, err := src.NUp(pdf.NUpOptions{Rows: 1, Cols: 2})
	if err != nil {
		t.Fatal(err)
	}
	merged := pdf.NewDocument(200, 200)
	merged.Append(src)
	for name, d := range map[string]*pdf.Document{"Split": parts[0], "Extract": ext, "NUp": nup, "Append": merged} {
		if got := headerOf(savedBytes(t, d)); got != "%PDF-2.0" {
			t.Errorf("%s of a 2.0 document saved as %s", name, got)
		}
	}
}

func TestImportFormKeepsSourceVersion(t *testing.T) {
	src := openAsVersion(t, "%PDF-2.0")
	f := src.CreateForm(50, 50)
	if err := f.Canvas().DrawRectangle(pdf.Rectangle{URX: 10, URY: 10}, pdf.ShapeStyle{FillColor: &pdf.Color{A: 1}}); err != nil {
		t.Fatal(err)
	}
	dst := pdf.NewDocument(200, 200)
	if _, err := dst.ImportForm(f); err != nil {
		t.Fatal(err)
	}
	if got := headerOf(savedBytes(t, dst)); got != "%PDF-2.0" {
		t.Errorf("document with a form from a 2.0 file saved as %s", got)
	}
}

func TestRepairedFileKeepsVersion(t *testing.T) {
	data := savedBytes(t, pdf.NewDocument(200, 200))
	copy(data, "%PDF-1.7")
	// Break the cross-reference offset so the file opens through reconstruction.
	i := bytes.LastIndex(data, []byte("startxref"))
	broken := append(append([]byte{}, data[:i]...), []byte("startxref\n999999\n%%EOF\n")...)
	d, err := pdf.OpenStream(bytes.NewReader(broken))
	if err != nil {
		t.Fatal(err)
	}
	if !d.NeedsRepair() {
		t.Fatal("file opened without reconstruction")
	}
	if got := headerOf(savedBytes(t, d)); got != "%PDF-1.7" {
		t.Errorf("repaired 1.7 file saved as %s", got)
	}
}
