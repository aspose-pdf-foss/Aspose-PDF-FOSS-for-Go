// SPDX-License-Identifier: MIT

package asposepdf_test

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"regexp"
	"strings"
	"testing"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

// buildSmallPDF returns a valid two-page document as bytes.
func buildSmallPDF(t *testing.T) []byte {
	t.Helper()
	doc := pdf.NewDocument(300, 200)
	if err := doc.AddBlankPage(300, 200); err != nil {
		t.Fatal(err)
	}
	page, err := doc.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	if err := page.AddText("Repair me", pdf.TextStyle{Font: pdf.FontHelvetica, Size: 14},
		pdf.Rectangle{LLX: 20, LLY: 100, URX: 280, URY: 140}); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

var startxrefRE = regexp.MustCompile(`startxref\s+(\d+)`)

// An intact file needs no repair, and says so.
func TestRepairReportCleanDocument(t *testing.T) {
	doc, err := pdf.OpenStream(bytes.NewReader(buildSmallPDF(t)))
	if err != nil {
		t.Fatal(err)
	}
	if doc.NeedsRepair() {
		t.Errorf("NeedsRepair = true for an intact document: %+v", doc.RepairReport())
	}
	if r := doc.RepairReport(); r.XRefReconstructed || r.ObjectsRecovered != 0 || r.TrailerRecovered {
		t.Errorf("RepairReport = %+v, want the zero value", r)
	}
}

// A file whose startxref points nowhere still opens — the reader rebuilds the
// cross-reference table by scanning — and the report says that happened.
func TestRepairReportRebuiltXref(t *testing.T) {
	data := buildSmallPDF(t)
	broken := startxrefRE.ReplaceAll(data, []byte("startxref\n999999"))
	if bytes.Equal(data, broken) {
		t.Fatal("test fixture: no startxref found to corrupt")
	}

	doc, err := pdf.OpenStream(bytes.NewReader(broken))
	if err != nil {
		t.Fatalf("OpenStream on a file with a broken startxref: %v", err)
	}
	if doc.PageCount() != 2 {
		t.Errorf("PageCount = %d, want 2", doc.PageCount())
	}
	if !doc.NeedsRepair() {
		t.Error("NeedsRepair = false after the xref had to be rebuilt")
	}
	r := doc.RepairReport()
	if !r.XRefReconstructed {
		t.Error("XRefReconstructed = false")
	}
	if r.ObjectsRecovered == 0 {
		t.Error("ObjectsRecovered = 0, want the objects the scan found")
	}

	// The recovered document must still be usable and saveable.
	page, err := doc.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	text, err := page.ExtractText()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "Repair me") {
		t.Errorf("extracted %q, want it to contain the page text", text)
	}
	var out bytes.Buffer
	if _, err := doc.WriteTo(&out); err != nil {
		t.Fatalf("WriteTo after repair: %v", err)
	}
	reopened, err := pdf.OpenStream(bytes.NewReader(out.Bytes()))
	if err != nil {
		t.Fatalf("reopen the repaired output: %v", err)
	}
	if reopened.NeedsRepair() {
		t.Error("the saved document still needs repair — writing should produce a clean file")
	}
}

// Losing the trailer as well as the xref is survivable: /Root is recovered by
// finding the catalog object.
func TestRepairReportRecoveredTrailer(t *testing.T) {
	data := buildSmallPDF(t)
	broken := startxrefRE.ReplaceAll(data, []byte("startxref\n999999"))
	idx := bytes.LastIndex(broken, []byte("trailer"))
	if idx < 0 {
		t.Skip("writer emitted no trailer keyword")
	}
	broken = append(append([]byte{}, broken[:idx]...), bytes.Replace(broken[idx:], []byte("trailer"), []byte("trailerX"), 1)...)

	doc, err := pdf.OpenStream(bytes.NewReader(broken))
	if err != nil {
		t.Fatalf("OpenStream with no usable trailer: %v", err)
	}
	r := doc.RepairReport()
	if !r.XRefReconstructed {
		t.Error("XRefReconstructed = false")
	}
	if !r.TrailerRecovered {
		t.Error("TrailerRecovered = false, but the trailer keyword was destroyed")
	}
	if doc.PageCount() != 2 {
		t.Errorf("PageCount = %d, want 2", doc.PageCount())
	}
}

// A damaged classic-xref file whose startxref points nowhere opens by
// reconstructing the xref table (NeedsRepair() == true), so there is no real
// "last section" for an incremental signature to match. isXRefStream (a
// negative test — true for anything that isn't the literal "xref" keyword)
// used to treat the garbage offset as "must be a stream" and append an
// /XRef cross-reference stream; sectionIsXRefStream instead confirms the
// bytes are really a /Type /XRef object before taking that branch, so the
// repaired document correctly falls back to a classic table.
func TestSignIncrementalAfterRepairUsesClassicXref(t *testing.T) {
	data := buildSmallPDF(t)
	broken := startxrefRE.ReplaceAll(data, []byte("startxref\n999999"))
	if bytes.Equal(data, broken) {
		t.Fatal("test fixture: no startxref found to corrupt")
	}

	doc, err := pdf.OpenStream(bytes.NewReader(broken))
	if err != nil {
		t.Fatalf("OpenStream on a file with a broken startxref: %v", err)
	}
	if !doc.NeedsRepair() {
		t.Fatal("test fixture: expected NeedsRepair() == true")
	}

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	cert := newSelfSigned(t, key)
	if err := doc.Sign(pdf.SignOptions{Certificate: cert, PrivateKey: key, Incremental: true}); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if _, err := doc.WriteTo(&out); err != nil {
		t.Fatal(err)
	}

	if bytes.Contains(out.Bytes(), []byte("/XRef")) {
		t.Error("a repaired document's incremental signature appended a cross-reference stream, want a classic table")
	}
	if !bytes.Contains(out.Bytes(), []byte("\nxref\n")) {
		t.Error("a repaired document's incremental signature did not append a classic cross-reference table")
	}

	reopened, err := pdf.OpenStream(bytes.NewReader(out.Bytes()))
	if err != nil {
		t.Fatalf("reopen the signed output: %v", err)
	}
	sigs, err := reopened.VerifySignatures()
	if err != nil {
		t.Fatal(err)
	}
	if len(sigs) != 1 || !sigs[0].Valid {
		t.Fatalf("signature does not hold: %+v", sigs)
	}
}
