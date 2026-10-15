// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"strings"
	"testing"
)

// TestApplyRedactionsScrubsInlineActualTextWhenGlyphsDropped pins
// pdf-go-1ha8: a BDC /ActualText property (routine in accessibility-
// tagged output from Word, InDesign, etc.) is a SEPARATE textual
// representation of whatever is drawn between that BDC and its
// matching EMC — the drawn glyphs here are the literal word "VISIBLE",
// never the secret itself, which lives ONLY in /ActualText. Before this
// fix, redacting the (fully in-region) "VISIBLE" glyphs left
// /ActualText completely untouched, so ExtractText/SearchText on the
// "redacted" file could still return the secret verbatim via that
// replacement mechanism. The forensic check below (documentContainsBytes,
// which decodes every object and searches for the literal string,
// exactly like pdf-go-2qkq's regression tests) is the only way to catch
// this — the secret is never in the GLYPH stream at all, so a glyph-only
// check would trivially "pass" even with the bug fully present.
func TestApplyRedactionsScrubsInlineActualTextWhenGlyphsDropped(t *testing.T) {
	doc := NewDocumentFromFormat(PageFormatA4)
	page, err := doc.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	fontName, err := page.ensureStandardFontResource("/Helvetica")
	if err != nil {
		t.Fatal(err)
	}

	const secret = "ACTUALTEXT-SECRET-998877"
	content := "BT\n" + fontName + " 12 Tf\n100 700 Td\n" +
		"/Span << /ActualText (" + secret + ") >> BDC\n" +
		"(VISIBLE) Tj\n" +
		"EMC\nET\n"
	if err := page.appendToContentStream([]byte(content)); err != nil {
		t.Fatal(err)
	}

	// Region fully covers "VISIBLE" at (100, 700).
	ra := NewRedactAnnotation(page, Rectangle{LLX: 95, LLY: 695, URX: 200, URY: 715})
	if err := page.Annotations().Add(ra); err != nil {
		t.Fatal(err)
	}

	if err := doc.ApplyRedactions(); err != nil {
		t.Fatalf("ApplyRedactions: %v", err)
	}

	if found, n := documentContainsBytes(doc, secret); found {
		t.Errorf("%q (never in the glyph stream, only in /ActualText) still present in %d in-memory object(s) after ApplyRedactions", secret, n)
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
	// Also confirm it doesn't leak through the ordinary read API, which
	// is the whole point of this bug (not just forensic bytes).
	rp, err := reopened.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	txt, err := rp.ExtractText()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(txt, secret) {
		t.Errorf("ExtractText on the redacted page still returned %q via the (un-scrubbed) /ActualText replacement: %q", secret, txt)
	}
}

// TestApplyRedactionsActualTextScrubDoesNotMutateSharedProperties pins
// the sharing hazard this fix must avoid, the same class pdf-go-w5hl's
// Form-XObject /Resources sharing bugs were: a BDC's properties can be
// referenced BY NAME from the page's /Resources/Properties dict, and
// that SAME name can be reused by several, independent marked-content
// spans. Scrubbing /ActualText for the span that got redacted must
// NEVER mutate the shared /Properties entry in place — doing so would
// silently break every OTHER, untouched span using the same name. Here,
// two spans both reference /MC0; only the first is redacted. The fix
// must rewrite ONLY the first span's own BDC op (converting it to a
// fresh inline dict with /ActualText removed) while leaving the
// second span's "/MC0 BDC" — and the shared /Properties resource object
// itself — completely untouched.
func TestApplyRedactionsActualTextScrubDoesNotMutateSharedProperties(t *testing.T) {
	d := NewDocumentFromFormat(PageFormatA4)
	p, err := d.Page(1)
	if err != nil {
		t.Fatal(err)
	}

	fontID := d.nextID
	d.nextID++
	d.objects[fontID] = &pdfObject{Num: fontID, Value: pdfDict{
		"/Type":     pdfName("/Font"),
		"/Subtype":  pdfName("/Type1"),
		"/BaseFont": pdfName("/Helvetica"),
		"/Encoding": pdfName("/WinAnsiEncoding"),
	}}

	const sharedSecret = "SHARED-PROPERTIES-SECRET"
	propsID := d.nextID
	d.nextID++
	d.objects[propsID] = &pdfObject{Num: propsID, Value: pdfDict{
		"/ActualText": sharedSecret,
	}}

	resources := pdfDict{
		"/Font":       pdfDict{"/F0": pdfRef{Num: fontID}},
		"/Properties": pdfDict{"/MC0": pdfRef{Num: propsID}},
	}
	p.pageDict()["/Resources"] = resources

	// Span A (redacted): "AAA" at (100, 700). Span B (untouched): "BBB"
	// at (100, 400) — far from the redaction region.
	content := "BT\n/F0 12 Tf\n" +
		"100 700 Td\n/Span /MC0 BDC\n(AAA) Tj\nEMC\n" +
		"100 -300 Td\n/Span /MC0 BDC\n(BBB) Tj\nEMC\n" +
		"ET\n"
	if err := p.appendToContentStream([]byte(content)); err != nil {
		t.Fatal(err)
	}

	ra := NewRedactAnnotation(p, Rectangle{LLX: 95, LLY: 695, URX: 200, URY: 715})
	if err := p.Annotations().Add(ra); err != nil {
		t.Fatal(err)
	}

	if err := d.ApplyRedactions(); err != nil {
		t.Fatalf("ApplyRedactions: %v", err)
	}

	// The shared /Properties resource object itself must be untouched —
	// confirming the scrub never mutated it in place.
	propsObj, ok := d.objects[propsID]
	if !ok {
		t.Fatalf("shared /Properties object %d was removed entirely", propsID)
	}
	propsDict, ok := propsObj.Value.(pdfDict)
	if !ok {
		t.Fatalf("shared /Properties object %d is no longer a dict: %#v", propsID, propsObj.Value)
	}
	if v, ok := propsDict["/ActualText"]; !ok || v != sharedSecret {
		t.Errorf("shared /Properties object %d was mutated: /ActualText = %#v, want %q", propsID, v, sharedSecret)
	}

	// The rewritten page content must reference /MC0 exactly ONCE — the
	// untouched span B's. Span A's occurrence must have been converted
	// to a fresh inline dict (no longer naming /MC0 at all).
	data, err := p.contentStreams()
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(data), "/MC0"); n != 1 {
		t.Errorf("rewritten content references /MC0 %d time(s), want exactly 1 (span A's redacted occurrence should have been converted to an inline dict): %s", n, data)
	}
}
