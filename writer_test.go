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
// than minting a fresh one every time — including after real content changes,
// which is the only way to tell this apart from the content-hash fallback
// coincidentally reproducing the same value: a no-op reopen+resave of
// unchanged content would still match even without the reuse branch at all,
// since buildDocumentPDF is itself deterministic (found by mutation-testing
// the fix during review — an earlier version of this test only did a no-op
// resave and stayed green with the reuse branch deleted).
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
	// A real content mutation, not a no-op resave: without this, the
	// content-hash fallback could reproduce the same ID by coincidence even
	// if the d.docID reuse branch were missing entirely.
	page, err := reopened.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	if err := page.AddText("a whole new paragraph that changes every byte downstream",
		pdf.TextStyle{Font: pdf.FontHelvetica, Size: 12}, pdf.Rectangle{LLX: 50, LLY: 600, URX: 545, URY: 650}); err != nil {
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

// allTrailerFileIDs extracts every /ID [<hex><hex>] occurrence in data —
// a linearized file carries two trailers (first-page and main) that must
// agree (ISO 32000-1 §14.4).
func allTrailerFileIDs(t *testing.T, data []byte) [][2]string {
	t.Helper()
	matches := trailerIDRe.FindAllSubmatch(data, -1)
	if len(matches) == 0 {
		t.Fatalf("no /ID [<...><...>] entry found in output:\n%s", data)
	}
	out := make([][2]string, len(matches))
	for i, m := range matches {
		out[i] = [2]string{string(m[1]), string(m[2])}
	}
	return out
}

// TestWriteToLinearizedDistinctDocumentsGetDistinctFileID pins pdf-go-hy6n
// (GitHub issue #16 point 3, shadowhunter1470-svg): WriteToLinearized used
// to derive /ID from only the object count and page count
// (md5("aspose-linearized-<size>-<pages>")), so two DIFFERENT documents
// with the same shape — same number of objects and pages, different
// content — collided on the same /ID, defeating ISO 32000-1 §14.4's "high
// probability of being unique" requirement entirely.
func TestWriteToLinearizedDistinctDocumentsGetDistinctFileID(t *testing.T) {
	a := textOnlyDoc(t, "Contract A")
	b := textOnlyDoc(t, "Payroll B")
	var bufA, bufB bytes.Buffer
	if _, err := a.WriteToLinearized(&bufA); err != nil {
		t.Fatal(err)
	}
	if _, err := b.WriteToLinearized(&bufB); err != nil {
		t.Fatal(err)
	}
	idA0, idA1 := trailerFileID(t, bufA.Bytes())
	idB0, idB1 := trailerFileID(t, bufB.Bytes())
	if idA0 == idB0 && idA1 == idB1 {
		t.Errorf("two different documents with the same object/page count got the same /ID: %q/%q", idA0, idA1)
	}
}

// TestWriteToLinearizedReusesOpenedDocumentFileID confirms the permanence
// half of ISO 32000-1 §14.4 now applies to the linearized writer too: a
// document opened from an existing file keeps that file's own /ID when
// saved via WriteToLinearized, exactly as WriteTo already does (both
// just go through documentFileID, which returns the original d.docID
// verbatim before ever considering a content hash). Before the fix, the
// linearized writer never consulted d.docID at all, so re-linearizing an
// opened file silently replaced its permanent identifier with one
// derived from nothing but object/page count.
func TestWriteToLinearizedReusesOpenedDocumentFileID(t *testing.T) {
	src := textOnlyDoc(t, "has an original /ID")
	var srcBuf bytes.Buffer
	if _, err := src.WriteTo(&srcBuf); err != nil {
		t.Fatal(err)
	}
	wantID0, wantID1 := trailerFileID(t, srcBuf.Bytes())

	opened, err := pdf.OpenStream(bytes.NewReader(srcBuf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	var linBuf bytes.Buffer
	if _, err := opened.WriteToLinearized(&linBuf); err != nil {
		t.Fatal(err)
	}
	gotID0, gotID1 := trailerFileID(t, linBuf.Bytes())
	if gotID0 != wantID0 || gotID1 != wantID1 {
		t.Errorf("WriteToLinearized /ID = %q/%q, want the opened file's own /ID %q/%q", gotID0, gotID1, wantID0, wantID1)
	}
}

// TestWriteToLinearizedStableAcrossRepeatedSaves confirms a fresh (never
// opened from a file) document's linearized /ID is stable across
// repeated saves, the same "same input, same output" guarantee
// documentFileID's own doc comment promises for WriteTo (a never-saved
// document's hash depends on which writer serialized it first, but is
// then stable under that writer — this is the linearized writer's own
// instance of that same documented behavior, not a new requirement that
// it also match WriteTo's unrelated hash input).
func TestWriteToLinearizedStableAcrossRepeatedSaves(t *testing.T) {
	doc := textOnlyDoc(t, "stable")
	var buf1, buf2 bytes.Buffer
	if _, err := doc.WriteToLinearized(&buf1); err != nil {
		t.Fatal(err)
	}
	if _, err := doc.WriteToLinearized(&buf2); err != nil {
		t.Fatal(err)
	}
	id1a, id1b := trailerFileID(t, buf1.Bytes())
	id2a, id2b := trailerFileID(t, buf2.Bytes())
	if id1a != id2a || id1b != id2b {
		t.Errorf("two WriteToLinearized calls on the same unchanged document got different /ID: %q/%q vs %q/%q", id1a, id1b, id2a, id2b)
	}
}

// TestWriteToLinearizedTrailersAgreeOnFileID confirms a linearized file's
// own first-page and main trailers carry the SAME /ID (ISO 32000-1 §14.4
// requires a file's trailers to agree) — both are built from the one
// value linParams.fileID computes, not independently re-derived.
func TestWriteToLinearizedTrailersAgreeOnFileID(t *testing.T) {
	doc := textOnlyDoc(t, "internally consistent")
	var buf bytes.Buffer
	if _, err := doc.WriteToLinearized(&buf); err != nil {
		t.Fatal(err)
	}
	ids := allTrailerFileIDs(t, buf.Bytes())
	if len(ids) < 2 {
		t.Fatalf("expected at least 2 /ID occurrences (first-page + main trailer), got %d", len(ids))
	}
	for i := 1; i < len(ids); i++ {
		if ids[i] != ids[0] {
			t.Errorf("trailer %d has /ID %v, want %v (same as the first)", i, ids[i], ids[0])
		}
	}
}
