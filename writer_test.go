// SPDX-License-Identifier: MIT

package asposepdf_test

import (
	"bytes"
	"regexp"
	"testing"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

// trailerFileID extracts the first /ID entry's two hex strings from a
// serialized PDF, as either a classic trailer or a cross-reference stream
// dict writes it: /ID [<hex><hex>].
var trailerIDRe = regexp.MustCompile(`/ID\s*\[\s*<([0-9A-Fa-f]*)>\s*<([0-9A-Fa-f]*)>`)

func trailerFileID(t *testing.T, data []byte) (string, string) {
	t.Helper()
	m := trailerIDRe.FindSubmatch(data)
	if m == nil {
		t.Fatalf("no /ID [<...><...>] entry found in output:\n%s", data)
	}
	return string(m[1]), string(m[2])
}

func textOnlyDoc(t *testing.T, text string) *pdf.Document {
	t.Helper()
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	page, err := doc.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	if err := page.AddText(text, pdf.TextStyle{Font: pdf.FontHelvetica, Size: 12},
		pdf.Rectangle{LLX: 50, LLY: 750, URX: 545, URY: 800}); err != nil {
		t.Fatal(err)
	}
	return doc
}

// TestWriteToUnencryptedDocumentHasFileID is the direct regression for GitHub
// issue #11: the classic (non-object-stream) writer used to write /ID only
// inside the encrypted branch, so veraPDF rejected any PDF/A document saved
// with plain WriteTo (ISO 19005-2 §6.1.3 requires /ID unconditionally).
func TestWriteToUnencryptedDocumentHasFileID(t *testing.T) {
	doc := textOnlyDoc(t, "Hello PDF/A")
	if _, err := doc.ConvertToPDFA(pdf.PDFA2B); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	id0, id1 := trailerFileID(t, buf.Bytes())
	if id0 == "" || id1 == "" {
		t.Errorf("/ID elements are empty: %q, %q", id0, id1)
	}
}

// TestWriteToFileIDDependsOnContent proves the generated /ID is a real
// derivation from the written body, not a hard-coded placeholder: two
// documents with different content must not collide.
func TestWriteToFileIDDependsOnContent(t *testing.T) {
	var buf1, buf2 bytes.Buffer
	if _, err := textOnlyDoc(t, "alpha").WriteTo(&buf1); err != nil {
		t.Fatal(err)
	}
	if _, err := textOnlyDoc(t, "bravo").WriteTo(&buf2); err != nil {
		t.Fatal(err)
	}
	id1, _ := trailerFileID(t, buf1.Bytes())
	id2, _ := trailerFileID(t, buf2.Bytes())
	if id1 == id2 {
		t.Errorf("different documents produced the same /ID %q", id1)
	}
}

// TestWriteToFileIDStableAcrossResaves: saving the same in-memory document
// twice must yield the same /ID, matching the deterministic-ID contract
// (and incidentally making the two outputs byte-identical).
func TestWriteToFileIDStableAcrossResaves(t *testing.T) {
	doc := textOnlyDoc(t, "stable")
	var buf1, buf2 bytes.Buffer
	if _, err := doc.WriteTo(&buf1); err != nil {
		t.Fatal(err)
	}
	if _, err := doc.WriteTo(&buf2); err != nil {
		t.Fatal(err)
	}
	id1a, id1b := trailerFileID(t, buf1.Bytes())
	id2a, id2b := trailerFileID(t, buf2.Bytes())
	if id1a != id2a || id1b != id2b {
		t.Errorf("/ID changed across re-saves of the same document: (%q,%q) vs (%q,%q)", id1a, id1b, id2a, id2b)
	}
}

// TestWriteToPreservesFileIDAcrossReopen: ISO 32000-1 §14.4 treats the first
// /ID element as a permanent identifier for the file's lineage. Re-saving a
// document opened from an existing (unencrypted) file must keep it rather
// than minting a fresh one every time.
func TestWriteToPreservesFileIDAcrossReopen(t *testing.T) {
	var original bytes.Buffer
	if _, err := textOnlyDoc(t, "original").WriteTo(&original); err != nil {
		t.Fatal(err)
	}
	wantID0, wantID1 := trailerFileID(t, original.Bytes())

	reopened, err := pdf.OpenStream(bytes.NewReader(original.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	var resaved bytes.Buffer
	if _, err := reopened.WriteTo(&resaved); err != nil {
		t.Fatal(err)
	}
	gotID0, gotID1 := trailerFileID(t, resaved.Bytes())
	if gotID0 != wantID0 || gotID1 != wantID1 {
		t.Errorf("/ID changed on reopen+resave: want (%q,%q), got (%q,%q)", wantID0, wantID1, gotID0, gotID1)
	}
}

// TestObjectStreamWriteHasFileID is TestWriteToUnencryptedDocumentHasFileID's
// counterpart for the cross-reference-stream writer (Optimize's
// CompressObjects), which builds its trailer dict independently of the
// classic path and had the identical gap.
func TestObjectStreamWriteHasFileID(t *testing.T) {
	doc := textOnlyDoc(t, "compressed")
	if _, err := doc.Optimize(pdf.OptimizationOptions{CompressObjects: true}); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(buf.Bytes(), []byte("/Type/XRef")) && !bytes.Contains(buf.Bytes(), []byte("/Type /XRef")) {
		t.Fatal("setup: expected a cross-reference stream (/Type /XRef), CompressObjects may not have taken effect")
	}
	id0, id1 := trailerFileID(t, buf.Bytes())
	if id0 == "" || id1 == "" {
		t.Errorf("/ID elements are empty: %q, %q", id0, id1)
	}
}

// TestWriteToEncryptedDocumentStillHasFileID guards the branch this fix left
// untouched: an encrypted document's /ID must still be the random value
// encryption's key derivation depends on, not the new content-derived one.
func TestWriteToEncryptedDocumentStillHasFileID(t *testing.T) {
	doc := textOnlyDoc(t, "secret")
	doc.SetPassword("user", "owner")
	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	id0, id1 := trailerFileID(t, buf.Bytes())
	if id0 == "" || id1 == "" {
		t.Errorf("/ID elements are empty: %q, %q", id0, id1)
	}
	if _, err := pdf.OpenStreamWithPassword(bytes.NewReader(buf.Bytes()), "user"); err != nil {
		t.Errorf("document no longer opens with its password: %v", err)
	}
}
