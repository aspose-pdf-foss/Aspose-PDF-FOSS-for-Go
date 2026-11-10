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

func TestVersion(t *testing.T) {
	if v := pdf.NewDocument(200, 200).Version(); v != "1.4" {
		t.Errorf("new document Version = %s, want 1.4", v)
	}
	for _, h := range []string{"%PDF-1.2", "%PDF-1.7", "%PDF-2.0"} {
		if v := openAsVersion(t, h).Version(); v != h[5:] {
			t.Errorf("%s source Version = %s", h, v)
		}
	}
}

func TestSetVersion(t *testing.T) {
	for _, c := range []struct {
		v, header string
		alg       *pdf.EncryptionAlgorithm
	}{
		{"2.0", "%PDF-2.0", nil},
		{"1.1", "%PDF-1.1", nil}, // a chosen version is written as is, even below 1.4
		{"1.7", "%PDF-1.7", nil},
	} {
		d := openAsVersion(t, "%PDF-1.5")
		if err := d.SetVersion(c.v); err != nil {
			t.Fatal(err)
		}
		if d.Version() != c.v {
			t.Errorf("Version after SetVersion(%s) = %s", c.v, d.Version())
		}
		data := savedBytes(t, d)
		if got := headerOf(data); got != c.header {
			t.Errorf("SetVersion(%s) saved as %s", c.v, got)
		}
		re, err := pdf.OpenStream(bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}
		if re.Version() != c.v {
			t.Errorf("reopened Version = %s, want %s", re.Version(), c.v)
		}
	}
	// The output still gets what its features need.
	d := pdf.NewDocument(200, 200)
	_ = d.SetVersion("1.1")
	d.SetEncryption(pdf.EncryptionOptions{UserPassword: "u", Algorithm: pdf.EncryptionAlgAES128})
	if got := headerOf(savedBytes(t, d)); got != "%PDF-1.6" {
		t.Errorf("AES-128 at version 1.1 saved as %s, want %%PDF-1.6", got)
	}
	// A version set before splitting carries over.
	d = pdf.NewDocument(200, 200)
	_ = d.SetVersion("1.2")
	parts, _ := d.Split()
	if got := headerOf(savedBytes(t, parts[0])); got != "%PDF-1.2" {
		t.Errorf("split of a 1.2 document saved as %s", got)
	}
}

func TestSetVersionRejects(t *testing.T) {
	d := openAsVersion(t, "%PDF-1.7")
	for _, v := range []string{"", "1.8", "3.0", "1.10", "2", "v1.7", "1,7"} {
		if err := d.SetVersion(v); err == nil {
			t.Errorf("SetVersion(%q) accepted", v)
		}
	}
	if d.Version() != "1.7" {
		t.Errorf("a rejected SetVersion changed Version to %s", d.Version())
	}
}

func TestDocumentID(t *testing.T) {
	d := pdf.NewDocument(200, 200)
	if _, ok := d.ID(); ok {
		t.Error("a document built in memory reports an ID")
	}
	re, err := pdf.OpenStream(bytes.NewReader(savedBytes(t, d)))
	if err != nil {
		t.Fatal(err)
	}
	id, ok := re.ID()
	if !ok || len(id.Original) != 32 || len(id.Modified) != 32 {
		t.Fatalf("ID of a saved file = %+v, %v; want two 16-byte identifiers", id, ok)
	}
	for _, r := range id.Original + id.Modified {
		if !(r >= '0' && r <= '9' || r >= 'A' && r <= 'F') {
			t.Fatalf("ID %+v is not upper-case hex", id)
		}
	}
	again, err := pdf.OpenStream(bytes.NewReader(savedBytes(t, re)))
	if err != nil {
		t.Fatal(err)
	}
	if id2, _ := again.ID(); id2 != id {
		t.Errorf("ID changed on re-save: %+v → %+v", id, id2)
	}
}
