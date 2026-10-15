// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"testing"
)

// TestApplyRedactionsErrorPathSweepsEarlierPages pins a bug found by an
// independent review of pdf-go-oq9i (the unknown-font refusal): before
// this fix, ApplyRedactions returned immediately on the first page that
// errored, WITHOUT calling RemoveUnusedObjects -- so any EARLIER page in
// the same call that had already redacted successfully still had its
// pre-redaction content stream sitting dead, but unswept, in d.objects.
// Since the writer has no reachability pass of its own, that original,
// un-redacted stream (holding the secret in full) would still end up in
// the saved file despite ApplyRedactions reporting failure overall --
// exactly pdf-go-2qkq's class of leak, reached through the error path.
// This was a theoretical edge case before pdf-go-oq9i (the rewriters
// used to silently pass through rather than ever erroring mid-document);
// the new unknown-font refusal makes a later page's failure a real,
// reachable scenario.
func TestApplyRedactionsErrorPathSweepsEarlierPages(t *testing.T) {
	doc := NewDocumentFromFormat(PageFormatA4)

	// Page 1: an ordinary redaction that succeeds.
	page1, err := doc.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	const secret1 = "ERRORPATH-PAGE1-SECRET"
	if err := page1.AddText(secret1, TextStyle{Font: FontHelvetica, Size: 12},
		Rectangle{LLX: 50, LLY: 700, URX: 500, URY: 720}); err != nil {
		t.Fatal(err)
	}
	matches1, err := page1.SearchText(secret1)
	if err != nil {
		t.Fatal(err)
	}
	if len(matches1) != 1 {
		t.Fatalf("SearchText(%q) on page 1: got %d matches, want 1", secret1, len(matches1))
	}
	ra1 := NewRedactAnnotation(page1, matches1[0].Rect)
	if err := page1.Annotations().Add(ra1); err != nil {
		t.Fatal(err)
	}

	// Page 2: a redaction that must REFUSE -- an unknown-font Tj inside
	// the redact region (same construction as
	// TestApplyRedactionsUnknownFontRefuses).
	if err := doc.AddBlankPage(595, 842); err != nil {
		t.Fatal(err)
	}
	page2, err := doc.Page(2)
	if err != nil {
		t.Fatal(err)
	}
	fontID := doc.nextID
	doc.nextID++
	doc.objects[fontID] = &pdfObject{Num: fontID, Value: pdfDict{
		"/Type":     pdfName("/Font"),
		"/Subtype":  pdfName("/Type1"),
		"/BaseFont": pdfName("/UnknownCustomFontXYZ"),
	}}
	page2.pageDict()["/Resources"] = pdfDict{"/Font": pdfDict{"/F0": pdfRef{Num: fontID}}}
	if err := page2.appendToContentStream([]byte(
		"BT\n/F0 12 Tf\n100 700 Td\n(ERRORPATH-PAGE2-UNKNOWN) Tj\nET\n",
	)); err != nil {
		t.Fatal(err)
	}
	ra2 := NewRedactAnnotation(page2, Rectangle{LLX: 95, LLY: 695, URX: 400, URY: 715})
	if err := page2.Annotations().Add(ra2); err != nil {
		t.Fatal(err)
	}

	if err := doc.ApplyRedactions(); err == nil {
		t.Fatal("ApplyRedactions: want an error from page 2's unknown font, got nil")
	}

	// Page 1 redacted successfully before page 2 failed -- its
	// pre-redaction stream must already be swept, in memory ...
	if found, n := documentContainsBytes(doc, secret1); found {
		t.Errorf("%q (page 1, redacted before page 2's error) still present in %d in-memory object(s)", secret1, n)
	}
	// ... and after a save+reopen, since that is what actually matters
	// to a caller who (mistakenly, or for diagnostics) saves the
	// document despite the reported error.
	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if found, n := documentContainsBytes(reopened, secret1); found {
		t.Errorf("%q recoverable from %d object(s) in the saved+reopened file despite ApplyRedactions reporting an error", secret1, n)
	}
}
