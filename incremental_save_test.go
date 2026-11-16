// SPDX-License-Identifier: MIT

package asposepdf_test

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"regexp"
	"strings"
	"testing"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

// incrDoc returns the bytes of a three-page document, each page saying its
// number.
func incrDoc(t *testing.T) []byte {
	t.Helper()
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	for i := 0; i < 2; i++ {
		if err := doc.AddBlankPageFromFormat(pdf.PageFormatA4); err != nil {
			t.Fatal(err)
		}
	}
	for i, p := range doc.Pages() {
		if err := p.AddTextAt("page "+string(rune('A'+i)), pdf.TextStyle{Font: pdf.FontHelvetica, Size: 12}, 72, 700); err != nil {
			t.Fatal(err)
		}
	}
	doc.SetInfo(pdf.DocumentInfo{Title: "Original"})
	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// writeIncremental saves d incrementally and reopens the result, checking
// that the opened file's bytes come first unchanged.
func writeIncremental(t *testing.T, d *pdf.Document, src []byte, password string) ([]byte, *pdf.Document) {
	t.Helper()
	var buf bytes.Buffer
	if _, err := d.WriteIncremental(&buf); err != nil {
		t.Fatal(err)
	}
	out := buf.Bytes()
	if !bytes.HasPrefix(out, src) {
		t.Fatal("the opened file's bytes were not kept")
	}
	var re *pdf.Document
	var err error
	if password != "" {
		re, err = pdf.OpenStreamWithPassword(bytes.NewReader(out), password)
	} else {
		re, err = pdf.OpenStream(bytes.NewReader(out))
	}
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if re.NeedsRepair() {
		t.Error("the saved file needed repair to open")
	}
	return out, re
}

func pageTexts(t *testing.T, d *pdf.Document) []string {
	t.Helper()
	texts, err := d.ExtractText()
	if err != nil {
		t.Fatal(err)
	}
	for i := range texts {
		texts[i] = strings.Join(strings.Fields(texts[i]), " ")
	}
	return texts
}

// TestWriteIncremental: an edit, new pages, reordering and new metadata are
// appended as one revision after the file's own bytes; with nothing changed
// the file is written as it was (pdf-go-d10c).
func TestWriteIncremental(t *testing.T) {
	src := incrDoc(t)

	d, _ := pdf.OpenStream(bytes.NewReader(src))
	var same bytes.Buffer
	if _, err := d.WriteIncremental(&same); err != nil || !bytes.Equal(same.Bytes(), src) {
		t.Errorf("unchanged document: %d bytes written for %d, %v", same.Len(), len(src), err)
	}

	d, _ = pdf.OpenStream(bytes.NewReader(src))
	p, _ := d.Page(2)
	if err := p.AddTextAt("added", pdf.TextStyle{Font: pdf.FontHelvetica, Size: 12}, 72, 600); err != nil {
		t.Fatal(err)
	}
	out, re := writeIncremental(t, d, src, "")
	if got := pageTexts(t, re); len(got) != 3 || got[0] != "page A" || got[1] != "page B added" || got[2] != "page C" {
		t.Errorf("after an edit: %q", got)
	}
	// Only the edited page, its new content and resources are appended: no
	// other page, no page tree, no catalog.
	rev := string(out[len(src):])
	if n := len(regexp.MustCompile(`/Type /Page[^s]`).FindAllString(rev, -1)); n != 1 {
		t.Errorf("%d page dictionaries in the revision, want 1", n)
	}
	if strings.Contains(rev, "/Type /Pages") || strings.Contains(rev, "/Type /Catalog") {
		t.Error("page tree or catalog rewritten for an edit on a page")
	}

	d, _ = pdf.OpenStream(bytes.NewReader(src))
	if err := d.AddBlankPageFromFormat(pdf.PageFormatA4); err != nil {
		t.Fatal(err)
	}
	np, _ := d.Page(4)
	if err := np.AddTextAt("page D", pdf.TextStyle{Font: pdf.FontHelvetica, Size: 12}, 72, 700); err != nil {
		t.Fatal(err)
	}
	if err := d.Reorder([]int{4, 3, 1}); err != nil {
		t.Fatal(err)
	}
	d.SetInfo(pdf.DocumentInfo{Title: "Changed"})
	out, re = writeIncremental(t, d, src, "")
	if rev := string(out[len(src):]); !strings.Contains(rev, "/Type /Pages") {
		t.Error("the page tree was not rewritten for new pages")
	}
	if got := pageTexts(t, re); strings.Join(got, "|") != "page D|page C|page A" {
		t.Errorf("after reordering: %q", got)
	}
	if info, _ := re.Info(); info.Title != "Changed" {
		t.Errorf("title %q", info.Title)
	}

	// Metadata removed: the newest trailer names none.
	d, _ = pdf.OpenStream(bytes.NewReader(src))
	d.ClearInfo()
	out, re = writeIncremental(t, d, src, "")
	if info, _ := re.Info(); info.Title != "" {
		t.Errorf("cleared title still %q", info.Title)
	}
	if tr := string(out[bytes.LastIndex(out, []byte("trailer")):]); strings.Contains(tr, "/Info") {
		t.Errorf("the newest trailer still names /Info: %s", tr)
	}

	// A catalog entry changed: the catalog is appended.
	d, _ = pdf.OpenStream(bytes.NewReader(src))
	d.SetPageMode(pdf.PageModeUseOutlines)
	_, re = writeIncremental(t, d, src, "")
	if re.PageMode() != pdf.PageModeUseOutlines {
		t.Errorf("page mode %v after an incremental save", re.PageMode())
	}
}

// TestWriteIncrementalKeepsSignature: a page edited after signing is saved
// as a revision the signature does not cover, and the signature stays valid.
func TestWriteIncrementalKeepsSignature(t *testing.T) {
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	src := signOneDoc(t, key, newSelfSigned(t, key))
	d, err := pdf.OpenStream(bytes.NewReader(src))
	if err != nil {
		t.Fatal(err)
	}
	p, _ := d.Page(1)
	if err := p.AddTextAt("Approved later", pdf.TextStyle{Font: pdf.FontHelvetica, Size: 10}, 30, 40); err != nil {
		t.Fatal(err)
	}
	_, re := writeIncremental(t, d, src, "")
	if got := pageTexts(t, re); !strings.Contains(got[0], "Approved later") {
		t.Errorf("edit lost: %q", got)
	}
	vs, err := re.VerifySignatures()
	if err != nil || len(vs) != 1 {
		t.Fatalf("signatures %d, %v", len(vs), err)
	}
	if !vs[0].Valid || !vs[0].IntegrityOK || vs[0].CoversWholeDocument {
		t.Errorf("signature after the edit: valid %v, integrity %v, covers whole %v (%v)", vs[0].Valid, vs[0].IntegrityOK, vs[0].CoversWholeDocument, vs[0].Err)
	}
}

// TestWriteIncrementalEncrypted: an encrypted document opened with its
// password is appended to with its own encryption.
func TestWriteIncrementalEncrypted(t *testing.T) {
	plain, _ := pdf.OpenStream(bytes.NewReader(incrDoc(t)))
	plain.SetEncryption(pdf.EncryptionOptions{UserPassword: "u", OwnerPassword: "o", Algorithm: pdf.EncryptionAlgAES256})
	var enc bytes.Buffer
	if _, err := plain.WriteTo(&enc); err != nil {
		t.Fatal(err)
	}
	src := enc.Bytes()
	d, err := pdf.OpenStreamWithPassword(bytes.NewReader(src), "u")
	if err != nil {
		t.Fatal(err)
	}
	// Unchanged, it is compared decrypted with the file: nothing to append.
	var same bytes.Buffer
	if _, err := d.WriteIncremental(&same); err != nil || !bytes.Equal(same.Bytes(), src) {
		t.Errorf("unchanged encrypted document: %d bytes written for %d, %v", same.Len(), len(src), err)
	}
	p, _ := d.Page(3)
	if err := p.AddTextAt("secret note", pdf.TextStyle{Font: pdf.FontHelvetica, Size: 12}, 72, 600); err != nil {
		t.Fatal(err)
	}
	out, re := writeIncremental(t, d, src, "u")
	if got := pageTexts(t, re); got[2] != "page C secret note" {
		t.Errorf("after an edit: %q", got)
	}
	if bytes.Contains(out[len(src):], []byte("secret note")) {
		t.Error("the revision holds the text unencrypted")
	}
	if _, err := pdf.OpenStream(bytes.NewReader(out)); err == nil {
		t.Error("the saved file opens without a password")
	}
}

// TestWriteIncrementalRefusals: a built document, one whose encryption was
// removed, changed or added, and one to be signed are refused.
func TestWriteIncrementalRefusals(t *testing.T) {
	var buf bytes.Buffer
	if _, err := pdf.NewDocumentFromFormat(pdf.PageFormatA4).WriteIncremental(&buf); err == nil {
		t.Error("a built document was saved incrementally")
	}
	plain, _ := pdf.OpenStream(bytes.NewReader(incrDoc(t)))
	plain.SetEncryption(pdf.EncryptionOptions{UserPassword: "u"})
	var enc bytes.Buffer
	if _, err := plain.WriteTo(&enc); err != nil {
		t.Fatal(err)
	}
	d, _ := pdf.OpenStreamWithPassword(bytes.NewReader(enc.Bytes()), "u")
	d.RemoveEncryption()
	if _, err := d.WriteIncremental(&buf); err == nil {
		t.Error("an encrypted document saved incrementally without its encryption")
	}
	d, _ = pdf.OpenStreamWithPassword(bytes.NewReader(enc.Bytes()), "u")
	if err := d.ChangePassword("new", "new"); err != nil {
		t.Fatal(err)
	}
	if _, err := d.WriteIncremental(&buf); err == nil {
		t.Error("an encrypted document saved incrementally with a new password")
	}
	d, _ = pdf.OpenStream(bytes.NewReader(incrDoc(t)))
	d.SetEncryption(pdf.EncryptionOptions{UserPassword: "u"})
	if _, err := d.WriteIncremental(&buf); err == nil {
		t.Error("encryption added in an incremental save")
	}
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	d, _ = pdf.OpenStream(bytes.NewReader(incrDoc(t)))
	if err := d.Sign(pdf.SignOptions{Certificate: newSelfSigned(t, key), PrivateKey: key}); err != nil {
		t.Fatal(err)
	}
	if _, err := d.WriteIncremental(&buf); err == nil {
		t.Error("a document to be signed saved by WriteIncremental")
	}
}
