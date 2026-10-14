// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"fmt"
	"testing"
)

// TestApplyRedactionsTextUnderNonIdentityCTM pins pdf-go-njbg end-to-end,
// reproducing the exact workflow the bug report used: draw text after a
// non-identity "cm" (the common top-left-origin Y-flip many producers,
// including Chrome/Skia, emit once at the start of a page's content),
// find it with SearchText (which IS CTM-aware and reports the real,
// post-cm page-space position), build a RedactAnnotation from that
// match's own Rect — the correct, intended workflow — then
// ApplyRedactions, and forensically confirm the secret is gone from the
// saved file (not just absent from ExtractText, which is exactly the
// distinction that hid pdf-go-2qkq originally).
//
// Before the fix, rewriteTextOperatorsInStream tracked the text matrix
// (tm) but never the graphics-state CTM a "cm" operator sets, so a
// glyph's computed position disagreed with where SearchText (and every
// other CTM-aware consumer) say it actually is — the region built from
// SearchText's own answer never matched, and ApplyRedactions reported
// success while leaving the secret fully recoverable.
func TestApplyRedactionsTextUnderNonIdentityCTM(t *testing.T) {
	doc := NewDocumentFromFormat(PageFormatA4)
	page, err := doc.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	size, err := page.Size()
	if err != nil {
		t.Fatal(err)
	}

	fontName, err := page.ensureStandardFontResource("/Helvetica")
	if err != nil {
		t.Fatal(err)
	}

	const secret = "CTM-ESCAPE-SECRET"
	// Flip Y around the page height, then draw at local (100, 142) --
	// which that flip maps to real page-space (100, size.Height-142).
	content := fmt.Sprintf(
		"q\n1 0 0 -1 0 %s cm\nBT\n%s 12 Tf\n100 142 Td\n(%s) Tj\nET\nQ\n",
		formatFloat(size.Height), fontName, secret,
	)
	if err := page.appendToContentStream([]byte(content)); err != nil {
		t.Fatal(err)
	}

	// The real, intended workflow: find the secret, trust what SearchText
	// reports as its page-space bounding box.
	matches, err := page.SearchText(secret)
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 {
		t.Fatalf("SearchText(%q): got %d matches, want 1 (is the CTM-aware extraction pipeline itself broken?)", secret, len(matches))
	}

	ra := NewRedactAnnotation(page, matches[0].Rect)
	if err := page.Annotations().Add(ra); err != nil {
		t.Fatal(err)
	}

	if err := doc.ApplyRedactions(); err != nil {
		t.Fatalf("ApplyRedactions: %v", err)
	}

	if found, n := documentContainsBytes(doc, secret); found {
		t.Errorf("%q still present in %d in-memory object(s) after ApplyRedactions (text rewriter ignored the page's \"cm\")", secret, n)
	}
	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if found, n := documentContainsBytes(reopened, secret); found {
		t.Errorf("%q recoverable from %d object(s) in the saved+reopened file", secret, n)
	}
}

// TestApplyRedactionsTextUnderNonIdentityCTMKeepsTextOutsideRegion
// confirms the fix is a position correction, not a blunt "redact
// everything under a cm" overcorrection: text drawn under the SAME
// non-identity cm, but outside the redaction region once correctly
// mapped to page space, must survive.
func TestApplyRedactionsTextUnderNonIdentityCTMKeepsTextOutsideRegion(t *testing.T) {
	doc := NewDocumentFromFormat(PageFormatA4)
	page, err := doc.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	size, err := page.Size()
	if err != nil {
		t.Fatal(err)
	}
	fontName, err := page.ensureStandardFontResource("/Helvetica")
	if err != nil {
		t.Fatal(err)
	}

	const secret = "CTM-REDACT-ME"
	const survivor = "CTM-SURVIVOR"
	content := fmt.Sprintf(
		"q\n1 0 0 -1 0 %s cm\nBT\n%s 12 Tf\n100 142 Td\n(%s) Tj\nET\nQ\n"+
			"q\n1 0 0 -1 0 %s cm\nBT\n%s 12 Tf\n100 500 Td\n(%s) Tj\nET\nQ\n",
		formatFloat(size.Height), fontName, secret,
		formatFloat(size.Height), fontName, survivor,
	)
	if err := page.appendToContentStream([]byte(content)); err != nil {
		t.Fatal(err)
	}

	matches, err := page.SearchText(secret)
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 {
		t.Fatalf("SearchText(%q): got %d matches, want 1", secret, len(matches))
	}
	ra := NewRedactAnnotation(page, matches[0].Rect)
	if err := page.Annotations().Add(ra); err != nil {
		t.Fatal(err)
	}

	if err := doc.ApplyRedactions(); err != nil {
		t.Fatalf("ApplyRedactions: %v", err)
	}

	if found, n := documentContainsBytes(doc, secret); found {
		t.Errorf("%q still present in %d in-memory object(s) after ApplyRedactions", secret, n)
	}
	if found, _ := documentContainsBytes(doc, survivor); !found {
		t.Errorf("%q (outside the redaction region, same cm) was incorrectly removed", survivor)
	}
}

// TestApplyRedactionsTextCTMRestoredAfterQ confirms the q/Q bracketing
// around a "cm" is scoped correctly: text drawn INSIDE a q...cm...Q
// block is positioned under the flip, but text drawn AFTER the matching
// Q must be positioned as if the flip never happened (ctm restored to
// whatever was active before the q). A mutation that forgot to pop the
// ctm stack on Q would leave the second block's glyphs mapped through
// the stale flipped CTM too, so a page-space region placed at the
// second block's true (identity-CTM) position would miss it.
func TestApplyRedactionsTextCTMRestoredAfterQ(t *testing.T) {
	doc := NewDocumentFromFormat(PageFormatA4)
	page, err := doc.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	size, err := page.Size()
	if err != nil {
		t.Fatal(err)
	}
	fontName, err := page.ensureStandardFontResource("/Helvetica")
	if err != nil {
		t.Fatal(err)
	}

	const secret = "POST-Q-SECRET"
	content := fmt.Sprintf(
		"q\n1 0 0 -1 0 %s cm\nBT\n%s 12 Tf\n100 142 Td\n(flipped, irrelevant) Tj\nET\nQ\n"+
			"BT\n%s 12 Tf\n100 400 Td\n(%s) Tj\nET\n",
		formatFloat(size.Height), fontName,
		fontName, secret,
	)
	if err := page.appendToContentStream([]byte(content)); err != nil {
		t.Fatal(err)
	}

	matches, err := page.SearchText(secret)
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 {
		t.Fatalf("SearchText(%q): got %d matches, want 1", secret, len(matches))
	}
	ra := NewRedactAnnotation(page, matches[0].Rect)
	if err := page.Annotations().Add(ra); err != nil {
		t.Fatal(err)
	}

	if err := doc.ApplyRedactions(); err != nil {
		t.Fatalf("ApplyRedactions: %v", err)
	}
	if found, n := documentContainsBytes(doc, secret); found {
		t.Errorf("%q (drawn after the matching Q, under identity CTM) still present in %d object(s) after ApplyRedactions", secret, n)
	}
}
