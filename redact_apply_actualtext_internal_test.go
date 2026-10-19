// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"strconv"
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

// TestApplyRedactionsScrubsActualTextFoundViaSearchText pins a gap an
// independent review found in the /ActualText scrub itself: the
// extractor emits an /ActualText span's replacement text AT EMC,
// positioned wherever the text matrix has moved TO BY THEN — advanceGlyph
// still runs for every one of the span's own (suppressed) glyphs, but
// the position-recording half of emitRune is exactly what /ActualText
// suppresses, so the replacement's reported position does not overlap
// where those glyphs are actually drawn. The documented workflow —
// find the secret with SearchText, build a RedactAnnotation from THAT
// match's own Rect — therefore builds a region around the EMC-time
// position, not around the span's filler glyphs. Before this fix, the
// scrub only fired when a glyph was actually dropped INSIDE the span,
// which this region (correctly built from SearchText's own answer)
// never does, since it sits past the filler glyphs entirely — so
// ApplyRedactions reported success while leaving the secret fully
// present, for the exact caller workflow this whole feature exists to
// support.
func TestApplyRedactionsScrubsActualTextFoundViaSearchText(t *testing.T) {
	doc := NewDocumentFromFormat(PageFormatA4)
	page, err := doc.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	fontName, err := page.ensureStandardFontResource("/Helvetica")
	if err != nil {
		t.Fatal(err)
	}

	const secret = "VIA-SEARCHTEXT-SECRET"
	const filler = "XXXXXX"
	content := "BT\n" + fontName + " 12 Tf\n100 700 Td\n" +
		"/Span << /ActualText (" + secret + ") >> BDC\n" +
		"(" + filler + ") Tj\n" +
		"EMC\nET\n"
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

// TestApplyRedactionsActualTextScrubPreservesRefValuedProperty pins a
// bug found by an independent review: the scrub rebuilds a BDC's
// properties dict as a copy of the original minus /ActualText, which
// can carry forward any OTHER property the original had — including one
// whose VALUE is itself an indirect reference (e.g. "/Other 7 0 R"),
// legal PDF syntax for a BDC properties dict entry. serializeOperand
// (image_remove.go, shared by every content-stream rewriter in this
// package) had no case for a bare pdfRef value, so re-serializing it
// fell through to Go's own "%v" struct formatting ("{7 0}") instead of
// valid PDF syntax ("7 0 R") -- silently corrupting the content stream.
// This re-parses the rewritten content and confirms it is still valid
// AND that the reference round-trips correctly, not just that parsing
// doesn't error.
func TestApplyRedactionsActualTextScrubPreservesRefValuedProperty(t *testing.T) {
	doc := NewDocumentFromFormat(PageFormatA4)
	page, err := doc.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	fontName, err := page.ensureStandardFontResource("/Helvetica")
	if err != nil {
		t.Fatal(err)
	}

	// An arbitrary other object this BDC's properties dict refers to by
	// indirect reference, alongside /ActualText.
	otherID := doc.nextID
	doc.nextID++
	doc.objects[otherID] = &pdfObject{Num: otherID, Value: pdfDict{"/Marker": pdfName("/X")}}

	const secret = "REF-VALUED-PROPS-SECRET"
	content := "BT\n" + fontName + " 12 Tf\n100 700 Td\n" +
		"/Span << /ActualText (" + secret + ") /Other " + strconv.Itoa(otherID) + " 0 R >> BDC\n" +
		"(VISIBLE) Tj\n" +
		"EMC\nET\n"
	if err := page.appendToContentStream([]byte(content)); err != nil {
		t.Fatal(err)
	}

	ra := NewRedactAnnotation(page, Rectangle{LLX: 95, LLY: 695, URX: 200, URY: 715})
	if err := page.Annotations().Add(ra); err != nil {
		t.Fatal(err)
	}
	if err := doc.ApplyRedactions(); err != nil {
		t.Fatalf("ApplyRedactions: %v", err)
	}

	data, err := page.contentStreams()
	if err != nil {
		t.Fatal(err)
	}
	ops, err := parseContentStream(data)
	if err != nil {
		t.Fatalf("the rewritten content stream no longer parses: %v (content: %s)", err, data)
	}
	found := false
	for _, op := range ops {
		if op.Operator != "BDC" || len(op.Operands) < 2 {
			continue
		}
		props, ok := op.Operands[1].(pdfDict)
		if !ok {
			continue
		}
		if _, hasAT := props["/ActualText"]; hasAT {
			t.Errorf("BDC properties still carry /ActualText after scrubbing: %#v", props)
		}
		ref, ok := props["/Other"].(pdfRef)
		if !ok {
			t.Fatalf("/Other did not round-trip as a pdfRef, got %#v (type %T)", props["/Other"], props["/Other"])
		}
		if ref.Num != otherID {
			t.Errorf("/Other round-tripped to object %d, want %d", ref.Num, otherID)
		}
		found = true
	}
	if !found {
		t.Fatal("no BDC op with a properties dict found in the rewritten content")
	}
}

// TestApplyRedactionsActualTextScrubPropagatesToParentSpan isolates the
// explicit "propagate dropped to the parent span on EMC" step
// (redact_apply_text.go) from markDropped's OWN broader effect (which
// already marks every span on the stack, including open ancestors, the
// moment a glyph is actually dropped INSIDE them — making that simpler
// case pass even without the explicit propagation). To exercise
// propagation specifically, the INNER span's scrub here is triggered
// by the EMC-time ActualText-position check (pdf-go-1ha8's SearchText
// fix), which markDropped never touches at all, and the OUTER span's
// own EMC is deliberately moved (via an extra Td) to a DIFFERENT
// position that the region does NOT cover — so if OUTER's /ActualText
// is scrubbed too, it can only be because INNER's dropped flag
// propagated to it, not because OUTER's own position check also fired.
func TestApplyRedactionsActualTextScrubPropagatesToParentSpan(t *testing.T) {
	doc := NewDocumentFromFormat(PageFormatA4)
	page, err := doc.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	fontName, err := page.ensureStandardFontResource("/Helvetica")
	if err != nil {
		t.Fatal(err)
	}

	const outerSecret = "NESTED-OUTER-SECRET"
	const innerSecret = "NESTED-INNER-SECRET"
	// /Inner has NO glyphs of its own at all — the ONLY way its scrub
	// can fire is the EMC-position check (markDropped is never called,
	// since there is no Tj/TJ inside it to call it from), isolating
	// that path from the glyph-drop one.
	content := "BT\n" + fontName + " 12 Tf\n100 700 Td\n" +
		"/Outer << /ActualText (" + outerSecret + ") >> BDC\n" +
		"/Inner << /ActualText (" + innerSecret + ") >> BDC\n" +
		"EMC\n" + // closes /Inner, right at the Td anchor (100, 700)
		"200 0 Td\n" + // move well clear before closing /Outer
		"EMC\n" + // closes /Outer, now at (300, 700) -- outside the region
		"ET\n"
	if err := page.appendToContentStream([]byte(content)); err != nil {
		t.Fatal(err)
	}

	// A tight region around exactly (100, 700) -- /Inner's EMC point,
	// nowhere near /Outer's (300, 700).
	ra := NewRedactAnnotation(page, Rectangle{LLX: 90, LLY: 690, URX: 110, URY: 710})
	if err := page.Annotations().Add(ra); err != nil {
		t.Fatal(err)
	}
	if err := doc.ApplyRedactions(); err != nil {
		t.Fatalf("ApplyRedactions: %v", err)
	}

	if found, n := documentContainsBytes(doc, innerSecret); found {
		t.Errorf("inner /ActualText %q still present in %d object(s)", innerSecret, n)
	}
	if found, n := documentContainsBytes(doc, outerSecret); found {
		t.Errorf("outer /ActualText %q still present in %d object(s) — dropped-flag propagation to the parent span (not triggered by its own, differently-positioned EMC) did not scrub it", outerSecret, n)
	}
}

// TestApplyRedactionsActualTextScrubsAcrossFormXObjectInsideSpan pins a
// gap an independent review found: the TEXT rewriter and the Form-
// XObject/image rewriter are two SEPARATE, sequential passes over the
// same page content, each with its own idea of what "dropped" means --
// a Form XObject placed via Do inside a BDC/ActualText span is redacted
// by the LATER pass, which the text pass (where BDC/EMC/ActualText
// tracking lives) has no visibility into at all. Without a dedicated
// check, a redaction region that fully covers a Form XObject's own
// secret text -- drawn inside an ActualText span whose OWN direct
// glyphs are unrelated filler -- left that span's /ActualText
// completely unscrubbed, even though the Form's text was correctly
// redacted by the later pass.
func TestApplyRedactionsActualTextScrubsAcrossFormXObjectInsideSpan(t *testing.T) {
	doc := NewDocumentFromFormat(PageFormatA4)
	page, err := doc.Page(1)
	if err != nil {
		t.Fatal(err)
	}

	form := doc.CreateForm(100, 50)
	const formSecret = "FORM-INSIDE-SPAN-SECRET"
	if err := form.Canvas().AddText(formSecret, TextStyle{Size: 12},
		Rectangle{LLX: 0, LLY: 0, URX: 100, URY: 50}); err != nil {
		t.Fatal(err)
	}

	const actualTextSecret = "ACTUALTEXT-OVER-FORM-SECRET"
	// Filler glyphs OUTSIDE the redaction region -- only the Form
	// XObject (placed via Do) falls inside it.
	fontName, err := page.ensureStandardFontResource("/Helvetica")
	if err != nil {
		t.Fatal(err)
	}
	if err := page.appendToContentStream([]byte(
		"BT\n" + fontName + " 12 Tf\n10 10 Td\n" +
			"/Span << /ActualText (" + actualTextSecret + ") >> BDC\n" +
			"(filler) Tj\n" +
			"ET\n",
	)); err != nil {
		t.Fatal(err)
	}
	// The form is placed at (200,600)-(300,650), far from "filler" at
	// (10,10), and the closing EMC comes after the Do.
	placement := Rectangle{LLX: 200, LLY: 600, URX: 300, URY: 650}
	if err := page.AddForm(form, placement); err != nil {
		t.Fatal(err)
	}
	if err := page.appendToContentStream([]byte("EMC\n")); err != nil {
		t.Fatal(err)
	}

	ra := NewRedactAnnotation(page, placement) // covers only the Form
	if err := page.Annotations().Add(ra); err != nil {
		t.Fatal(err)
	}

	if err := doc.ApplyRedactions(); err != nil {
		t.Fatalf("ApplyRedactions: %v", err)
	}

	if found, n := documentContainsBytes(doc, formSecret); found {
		t.Errorf("%q (inside the redacted Form XObject) still present in %d object(s)", formSecret, n)
	}
	if found, n := documentContainsBytes(doc, actualTextSecret); found {
		t.Errorf("%q (the enclosing /ActualText, never itself touched by a glyph drop) still present in %d object(s) — the Form-XObject-inside-span case was not scrubbed", actualTextSecret, n)
	}
}

// TestApplyRedactionsUnmatchedEMCDoesNotPanic confirms a malformed
// content stream (an EMC with no matching BDC/BMC — e.g. a truncated or
// hand-edited file) is handled defensively: the mcStack guard must see
// it as empty and skip, not panic.
func TestApplyRedactionsUnmatchedEMCDoesNotPanic(t *testing.T) {
	fonts := testRewriteFontMap()
	in := []byte("BT\n/F1 12 Tf\n100 700 Td\n(Hello) Tj\nEMC\nET\n")
	regions := []QuadPoint{
		{X1: 0, Y1: 720, X2: 1000, Y2: 720, X3: 0, Y3: 0, X4: 1000, Y4: 0},
	}
	if _, err := rewriteTextOperatorsInStream(in, regions, fonts, nil, nil, nil); err != nil {
		t.Fatalf("unexpected error on an unmatched EMC: %v", err)
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
