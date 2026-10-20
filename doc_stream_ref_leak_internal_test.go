// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
)

// pdf-go-2apc / GHSA-696v-26w4-39vc: collectValueDepsDoc, markReachable
// and rewriteRefs (doc.go) treated every match of reRefDoc
// (`\b(\d+)\s+\d+\s+R\b`) inside STREAM DATA (page text, font programs,
// images, compressed bytes) as a real indirect object reference. Stream
// data is not PDF syntax — content streams reference resources by NAME,
// never by "N G R" — so a digit sequence shaped like a reference can
// appear in ordinary page text by coincidence (or be crafted
// deliberately) and get treated as if the page actually depends on that
// object, with three distinct consequences, each pinned by its own test
// below:
//
//  1. Extract/Split pull in an UNRELATED object (confidentiality: a
//     multi-party document split one page per recipient could leak
//     another recipient's page content into a file meant to hold only
//     one page).
//  2. Append rewrites the APPENDED document's own stream bytes
//     (integrity: visible text silently changes, nondeterministically,
//     whenever it happens to contain a reference-shaped substring to an
//     object that gets renumbered).
//  3. RemoveUnusedObjects keeps a genuinely dead object alive forever if
//     its number coincidentally appears in some OTHER stream's bytes.

// TestExtractDoesNotCopyUnrelatedPageContentViaStreamByteCoincidence
// reproduces the GHSA-696v-26w4-39vc Extract repro, but deterministically:
// rather than hoping a fixed range of digits happens to collide with some
// real object number, it reads page 2's ACTUAL content-stream object
// number and crafts page 1's own text to contain exactly that number in
// "N 0 R" shape — so the test is unambiguous regardless of how object
// numbers happen to be allocated.
func TestExtractDoesNotCopyUnrelatedPageContentViaStreamByteCoincidence(t *testing.T) {
	doc := NewDocument(595, 842)
	if err := doc.AddBlankPage(595, 842); err != nil {
		t.Fatal(err)
	}

	const secret = "CONFIDENTIAL-PAGE-TWO-ONLY"
	p2 := doc.Pages()[1]
	if err := p2.AddText(secret, TextStyle{Size: 10},
		Rectangle{LLX: 50, LLY: 700, URX: 545, URY: 792}); err != nil {
		t.Fatal(err)
	}
	// A blank page's /Contents already points at an (empty) stream
	// object before AddText ever runs, so the first AddText call takes
	// appendToContentStream's q/Q-wrap branch, not its "/Contents was
	// absent" branch — /Contents ends up an ARRAY, whose LAST entry is
	// the new stream actually holding the secret text.
	var p2ContentNum int
	switch c := p2.pageDict()["/Contents"].(type) {
	case pdfRef:
		p2ContentNum = c.Num
	case pdfArray:
		last, ok := c[len(c)-1].(pdfRef)
		if !ok {
			t.Fatalf("page 2's /Contents array's last entry is not a plain pdfRef (%T)", c[len(c)-1])
		}
		p2ContentNum = last.Num
	default:
		t.Fatalf("page 2's /Contents has unexpected type %T", c)
	}

	p1 := doc.Pages()[0]
	coincidence := fmt.Sprintf("Parts %d 0 R", p2ContentNum)
	if err := p1.AddText(coincidence, TextStyle{Size: 10},
		Rectangle{LLX: 50, LLY: 700, URX: 545, URY: 792}); err != nil {
		t.Fatal(err)
	}

	var b bytes.Buffer
	if _, err := doc.WriteTo(&b); err != nil {
		t.Fatal(err)
	}
	src, err := OpenStream(bytes.NewReader(b.Bytes()))
	if err != nil {
		t.Fatal(err)
	}

	one, err := src.Extract(PageRange{From: 1, To: 1})
	if err != nil {
		t.Fatal(err)
	}
	if one.PageCount() != 1 {
		t.Fatalf("Extract: got %d pages, want 1", one.PageCount())
	}

	if found, n := documentContainsBytes(one, secret); found {
		t.Errorf("page-2-only text %q leaked into the page-1-only extracted document (present in %d object(s)) — "+
			"a coincidental \"N 0 R\"-shaped digit sequence in page 1's own text pulled in page 2's unrelated content stream",
			secret, n)
	}
}

// TestAppendDoesNotRewriteAppendedStreamText reproduces the
// GHSA-696v-26w4-39vc Append repro: a document whose page text happens to
// contain substrings shaped like references to its OWN real objects (1
// and 3) must keep that text byte-for-byte after being appended into
// another document and renumbered — rewriteRefs must never touch stream
// DATA, only structural dict/array values.
func TestAppendDoesNotRewriteAppendedStreamText(t *testing.T) {
	a := NewDocument(595, 842)
	const text = "Item 1 0 R and 3 0 R"
	if err := a.Pages()[0].AddText(text, TextStyle{Size: 12},
		Rectangle{LLX: 50, LLY: 700, URX: 545, URY: 792}); err != nil {
		t.Fatal(err)
	}
	var abuf bytes.Buffer
	if _, err := a.WriteTo(&abuf); err != nil {
		t.Fatal(err)
	}
	aReopened, err := OpenStream(bytes.NewReader(abuf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}

	main := NewDocument(595, 842)
	var mbuf bytes.Buffer
	if _, err := main.WriteTo(&mbuf); err != nil {
		t.Fatal(err)
	}
	mainReopened, err := OpenStream(bytes.NewReader(mbuf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}

	mainReopened.Append(aReopened)

	var outBuf bytes.Buffer
	if _, err := mainReopened.WriteTo(&outBuf); err != nil {
		t.Fatal(err)
	}
	final, err := OpenStream(bytes.NewReader(outBuf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}

	texts, err := final.ExtractText()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, pt := range texts {
		if strings.Contains(pt, text) {
			found = true
		}
	}
	if !found {
		t.Errorf("appended page's text was rewritten; want the literal %q preserved verbatim, got pages: %q", text, texts)
	}
}

// TestRemoveUnusedObjectsDoesNotKeepDeadObjectViaStreamByteCoincidence
// reproduces the GHSA-696v-26w4-39vc RemoveUnusedObjects effect: a
// genuinely orphaned object (nothing in the document's real structure
// references it) must be removed even when some unrelated page's own
// text happens to contain a "N 0 R"-shaped digit sequence naming it.
func TestRemoveUnusedObjectsDoesNotKeepDeadObjectViaStreamByteCoincidence(t *testing.T) {
	doc := NewDocumentFromFormat(PageFormatA4)
	p, err := doc.Page(1)
	if err != nil {
		t.Fatal(err)
	}

	orphanID := doc.nextID
	doc.nextID++
	doc.objects[orphanID] = &pdfObject{Num: orphanID, Value: pdfDict{"/Type": pdfName("/Dummy")}}

	if err := p.AddText(fmt.Sprintf("See item %d 0 R for details", orphanID), TextStyle{Size: 12},
		Rectangle{LLX: 50, LLY: 700, URX: 545, URY: 792}); err != nil {
		t.Fatal(err)
	}

	doc.RemoveUnusedObjects()

	if _, stillThere := doc.objects[orphanID]; stillThere {
		t.Errorf("object %d was kept alive by a coincidental \"N 0 R\"-shaped digit sequence in an unrelated page's own text", orphanID)
	}
}
