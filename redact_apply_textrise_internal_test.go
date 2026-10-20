// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"fmt"
	"testing"
)

// pdf-go-lx1f: rewriteTextOperatorsInStream tested a glyph's center at
// y=0 in text space regardless of any active text rise (Ts) — but
// render.go DOES honor Ts when actually drawing a glyph, so a redaction
// region drawn tightly around the VISIBLE (risen) position of a
// superscript/subscript glyph could miss the baseline position this
// pipeline actually tested, letting it survive. This only bites a
// caller-drawn region placed by eye against the rendered page — not the
// documented SearchText-based workflow, since SearchText's own Rect spans
// descender-to-ascender regardless of Ts and so always contains the
// baseline anyway (text.go has no Ts case either, deliberately left that
// way — see redact_apply_text.go's own doc comment).

// TestApplyRedactionsTextRiseAffectsGlyphCenter reproduces the bug
// directly: text drawn with a 20pt rise above its own Td baseline, and a
// region that tightly covers only the RISEN (visual) position, not the
// baseline. Before the fix, the glyph-center test point stays at the
// baseline (y=0 relative to tm) and the region misses it; after the fix
// the test point moves to the rise and the region catches it.
func TestApplyRedactionsTextRiseAffectsGlyphCenter(t *testing.T) {
	doc := NewDocumentFromFormat(PageFormatA4)
	page, err := doc.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	fontName, err := page.ensureStandardFontResource("/Helvetica")
	if err != nil {
		t.Fatal(err)
	}

	const secret = "RISE-SECRET"
	const baselineY = 300.0
	const rise = 20.0
	// ctm is identity, tm is a pure translation to (100, baselineY), so
	// the glyph's visible position is exactly baselineY+rise = 320 and
	// its baseline-only position is exactly baselineY = 300.
	content := fmt.Sprintf(
		"BT\n%s 12 Tf\n100 %s Td\n%s Ts\n(%s) Tj\nET\n",
		fontName, formatFloat(baselineY), formatFloat(rise), secret,
	)
	if err := page.appendToContentStream([]byte(content)); err != nil {
		t.Fatal(err)
	}

	// A region that tightly covers the RISEN position (y in [310,330])
	// but excludes the baseline (y=300) entirely — exactly the "hand-drawn
	// tightly over the rendered position" scenario from the ticket.
	region := Rectangle{LLX: 80, LLY: 310, URX: 250, URY: 330}
	ra := NewRedactAnnotation(page, region)
	if err := page.Annotations().Add(ra); err != nil {
		t.Fatal(err)
	}

	if err := doc.ApplyRedactions(); err != nil {
		t.Fatalf("ApplyRedactions: %v", err)
	}

	if found, n := documentContainsBytes(doc, secret); found {
		t.Errorf("%q still present in %d in-memory object(s) after ApplyRedactions (text rewriter ignored Ts)", secret, n)
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

// TestApplyRedactionsTextRiseDoesNotOverredact is the correctness
// counterpart: a region placed around the SAME risen band must leave
// NON-risen text sitting at the same (x, baselineY) position untouched —
// the fix must move the test point by the ACTUAL active rise, not just
// widen what counts as "inside" indiscriminately.
func TestApplyRedactionsTextRiseDoesNotOverredact(t *testing.T) {
	doc := NewDocumentFromFormat(PageFormatA4)
	page, err := doc.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	fontName, err := page.ensureStandardFontResource("/Helvetica")
	if err != nil {
		t.Fatal(err)
	}

	const survivor = "NO-RISE-SURVIVOR"
	const baselineY = 300.0
	// No Ts at all: drawn at the plain baseline.
	content := fmt.Sprintf(
		"BT\n%s 12 Tf\n100 %s Td\n(%s) Tj\nET\n",
		fontName, formatFloat(baselineY), survivor,
	)
	if err := page.appendToContentStream([]byte(content)); err != nil {
		t.Fatal(err)
	}

	region := Rectangle{LLX: 80, LLY: 310, URX: 250, URY: 330}
	ra := NewRedactAnnotation(page, region)
	if err := page.Annotations().Add(ra); err != nil {
		t.Fatal(err)
	}

	if err := doc.ApplyRedactions(); err != nil {
		t.Fatalf("ApplyRedactions: %v", err)
	}
	if found, _ := documentContainsBytes(doc, survivor); !found {
		t.Errorf("%q (at the plain baseline, no Ts active) was incorrectly removed by a region placed over a DIFFERENT, risen band", survivor)
	}
}

// TestApplyRedactionsTextRiseInTJArray is the TJ-array counterpart of
// TestApplyRedactionsTextRiseAffectsGlyphCenter — filterTJArray's inline
// single-byte glyph loop computes its own glyph-center point separately
// from filterTjStringSingleByte's, and pdf-go-njbg's own review found
// exactly this kind of TJ-specific gap once before (the cm-tracking fix
// initially covered only Tj).
func TestApplyRedactionsTextRiseInTJArray(t *testing.T) {
	doc := NewDocumentFromFormat(PageFormatA4)
	page, err := doc.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	fontName, err := page.ensureStandardFontResource("/Helvetica")
	if err != nil {
		t.Fatal(err)
	}

	const secret = "RISE-TJ-SECRET"
	const baselineY = 300.0
	const rise = 20.0
	content := fmt.Sprintf(
		"BT\n%s 12 Tf\n100 %s Td\n%s Ts\n[(%s)] TJ\nET\n",
		fontName, formatFloat(baselineY), formatFloat(rise), secret,
	)
	if err := page.appendToContentStream([]byte(content)); err != nil {
		t.Fatal(err)
	}

	region := Rectangle{LLX: 80, LLY: 310, URX: 250, URY: 330}
	ra := NewRedactAnnotation(page, region)
	if err := page.Annotations().Add(ra); err != nil {
		t.Fatal(err)
	}

	if err := doc.ApplyRedactions(); err != nil {
		t.Fatalf("ApplyRedactions: %v", err)
	}
	if found, n := documentContainsBytes(doc, secret); found {
		t.Errorf("%q still present in %d object(s) after ApplyRedactions (TJ-array path ignored Ts)", secret, n)
	}
}

// TestRewriteTextType0RiseAffectsGlyphCenter is the Type0/CID-font
// counterpart of TestApplyRedactionsTextRiseAffectsGlyphCenter —
// decodeCIDGlyphs computes its glyph-center point independently of the
// single-byte path. Uses the same direct rewriteTextOperatorsInStream +
// hand-built CID fontInfo technique as the pdf-go-eys1 tests
// (testRewriteCJKFontMap/buildCJKContent in
// redact_apply_cjk_cmap_internal_test.go) rather than routing through
// AddText+LoadFont: a real CJK string drawn with a Latin-only embedded
// font falls back to an /ActualText-wrapped notdef glyph (per CLAUDE.md's
// "Shaped text stays extractable" section), which would make a
// substring-in-content-bytes forensic check meaningless here (the
// secret's UTF-8 bytes never literally appear — only a UTF-16BE/hex
// /ActualText encoding of it does), not something this test is about.
func TestRewriteTextType0RiseAffectsGlyphCenter(t *testing.T) {
	// From testRewriteCJKFontMap: code 0x41 (single byte) -> CID 65,
	// width 600/1000 em. At 10pt and tm=Td(0,0)/ctm=identity, the
	// glyph's local-space center is x=3.0 (half of 6.0 units advance),
	// y=0 at the baseline or y=20 once risen.
	raw := []byte{0x41}
	content := []byte("BT\n/F1 10 Tf\n0 0 Td\n20 Ts\n(" + string(raw) + ") Tj\nET\n")

	// Covers the RISEN position (y in [15,25]) but not the baseline
	// (y=0).
	regions := []QuadPoint{{X1: 0, Y1: 15, X2: 10, Y2: 15, X3: 0, Y3: 25, X4: 10, Y4: 25}}

	out, err := rewriteTextOperatorsInStream(content, regions, testRewriteCJKFontMap(), nil, nil, nil)
	if err != nil {
		t.Fatalf("rewriteTextOperatorsInStream: %v", err)
	}
	if bytes.Contains(out, raw) {
		t.Errorf("the risen glyph's raw byte survived (Ts ignored by the Type0 path):\n%s", out)
	}
}

// TestRewriteTextType0RiseDoesNotOverredact is the correctness
// counterpart: the SAME region, with no Ts active (plain baseline), must
// leave the glyph untouched.
func TestRewriteTextType0RiseDoesNotOverredact(t *testing.T) {
	raw := []byte{0x41}
	content := []byte("BT\n/F1 10 Tf\n0 0 Td\n(" + string(raw) + ") Tj\nET\n")
	regions := []QuadPoint{{X1: 0, Y1: 15, X2: 10, Y2: 15, X3: 0, Y3: 25, X4: 10, Y4: 25}}

	out, err := rewriteTextOperatorsInStream(content, regions, testRewriteCJKFontMap(), nil, nil, nil)
	if err != nil {
		t.Fatalf("rewriteTextOperatorsInStream: %v", err)
	}
	if !bytes.Contains(out, raw) {
		t.Errorf("the glyph at the plain baseline (no Ts active) was incorrectly dropped by a region placed over a different, risen band:\n%s", out)
	}
}

// TestApplyRedactionsTextRiseRestoredAfterQ confirms rise is scoped by
// q/Q like the other text graphics-state parameters (ISO 32000-1 Table
// 52): text drawn AFTER a matching Q must be tested at whatever rise was
// active before the q, not whatever a nested Ts set.
func TestApplyRedactionsTextRiseRestoredAfterQ(t *testing.T) {
	doc := NewDocumentFromFormat(PageFormatA4)
	page, err := doc.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	fontName, err := page.ensureStandardFontResource("/Helvetica")
	if err != nil {
		t.Fatal(err)
	}

	const survivor = "POST-Q-RISE-SURVIVOR"
	content := fmt.Sprintf(
		"q\nBT\n%s 12 Tf\n100 142 Td\n30 Ts\n(irrelevant filler) Tj\nET\nQ\n"+
			"BT\n%s 12 Tf\n100 400 Td\n(%s) Tj\nET\n",
		fontName, fontName, survivor,
	)
	if err := page.appendToContentStream([]byte(content)); err != nil {
		t.Fatal(err)
	}

	// A region placed around what y=400+30 (the stale, nested rise) would
	// wrongly test against, but NOT around the correct, un-risen y=400.
	region := Rectangle{LLX: 80, LLY: 425, URX: 300, URY: 445}
	ra := NewRedactAnnotation(page, region)
	if err := page.Annotations().Add(ra); err != nil {
		t.Fatal(err)
	}

	if err := doc.ApplyRedactions(); err != nil {
		t.Fatalf("ApplyRedactions: %v", err)
	}
	if found, _ := documentContainsBytes(doc, survivor); !found {
		t.Errorf("%q (drawn after the matching Q, rise restored to 0) was incorrectly removed", survivor)
	}
}

// TestApplyRedactionsFormXObjectInheritsPageRise confirms a Form XObject
// invoked while the page has an active text rise, and whose own content
// draws text with no Tf/Ts of its own (relying entirely on the inherited
// state, legal per ISO 32000-1 §8.10.1), is redacted using that inherited
// rise — the Form-XObject counterpart of
// TestApplyRedactionsTextRiseAffectsGlyphCenter, mirroring how pdf-go-w7ye
// threaded inherited font state through rewriteXObjectDoOperatorsInStream.
func TestApplyRedactionsFormXObjectInheritsPageRise(t *testing.T) {
	d := NewDocumentFromFormat(PageFormatA4)
	p, err := d.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	fontName, err := p.ensureStandardFontResource("/Helvetica")
	if err != nil {
		t.Fatal(err)
	}

	const secret = "INHERITED-RISE-SECRET"
	formID := d.nextID
	d.nextID++
	d.objects[formID] = &pdfObject{Num: formID, Value: &pdfStream{
		Dict: pdfDict{
			"/Type":    pdfName("/XObject"),
			"/Subtype": pdfName("/Form"),
			"/BBox":    pdfArray{0.0, 0.0, 200.0, 50.0},
		},
		// No Tf, no Ts of its own: relies entirely on the page's state
		// (font AND rise) in effect at the Do that invokes it.
		Data:    []byte(fmt.Sprintf("BT 10 5 Td (%s) Tj ET\n", secret)),
		Decoded: true,
	}}

	resources := p.pageResources()
	if resources == nil {
		resources = pdfDict{}
	}
	xobj, _ := resources["/XObject"].(pdfDict)
	if xobj == nil {
		xobj = pdfDict{}
		resources["/XObject"] = xobj
	}
	xobj["/Fm0"] = pdfRef{Num: formID}
	p.pageDict()["/Resources"] = resources

	const rise = 20.0
	// Place the form at (100, 300): its own content draws the secret at
	// local (10,5), with no Tf/Ts, so it inherits the page's active font
	// and rise — visible page-space position ≈ (110, 305+rise).
	content := fmt.Sprintf(
		"BT\n%s 12 Tf\n%s Ts\nET\nq\n1 0 0 1 100 300 cm\n/Fm0 Do\nQ\n",
		fontName, formatFloat(rise),
	)
	if err := p.appendToContentStream([]byte(content)); err != nil {
		t.Fatal(err)
	}

	region := Rectangle{LLX: 95, LLY: 300 + rise - 5, URX: 250, URY: 300 + rise + 15}
	ra := NewRedactAnnotation(p, region)
	if err := p.Annotations().Add(ra); err != nil {
		t.Fatal(err)
	}

	if err := d.ApplyRedactions(); err != nil {
		t.Fatalf("ApplyRedactions: %v", err)
	}
	if found, n := documentContainsBytes(d, secret); found {
		t.Errorf("%q still present in %d object(s) after ApplyRedactions (Form XObject did not inherit the page's active rise)", secret, n)
	}
}

// TestApplyRedactionsFormXObjectDoesNotMemoizeDifferentInheritedRise
// covers formCloneKey's rise field (pdf-go-wuv7's memoization cache,
// extended by pdf-go-lx1f): the SAME Form XObject, Do-invoked twice at
// the IDENTICAL composed CTM, but under two DIFFERENT active page rises
// at each Do, must redact into two INDEPENDENT clone objects, not share
// one — otherwise whichever placement's clone gets memoized first would
// be wrongly reused for the other, even though the two inherit different
// rise and so can legitimately need different redacted content (this
// test's own region happens to fully redact the secret under EITHER
// rise, so a shared/wrongly-reused clone would still look correct by a
// mere secret-presence check — formObjectCount is the oracle that
// actually distinguishes "two independent clones" from "one clone
// referenced twice").
func TestApplyRedactionsFormXObjectDoesNotMemoizeDifferentInheritedRise(t *testing.T) {
	d := NewDocumentFromFormat(PageFormatA4)
	p, err := d.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	fontName, err := p.ensureStandardFontResource("/Helvetica")
	if err != nil {
		t.Fatal(err)
	}

	const secret = "SHARED-FORM-RISE-SECRET"
	formID := d.nextID
	d.nextID++
	d.objects[formID] = &pdfObject{Num: formID, Value: &pdfStream{
		Dict: pdfDict{
			"/Type":    pdfName("/XObject"),
			"/Subtype": pdfName("/Form"),
			"/BBox":    pdfArray{0.0, 0.0, 200.0, 50.0},
		},
		Data:    []byte(fmt.Sprintf("BT 10 5 Td (%s) Tj ET\n", secret)),
		Decoded: true,
	}}
	resources := p.pageResources()
	if resources == nil {
		resources = pdfDict{}
	}
	xobj, _ := resources["/XObject"].(pdfDict)
	if xobj == nil {
		xobj = pdfDict{}
		resources["/XObject"] = xobj
	}
	xobj["/Fm0"] = pdfRef{Num: formID}
	p.pageDict()["/Resources"] = resources

	const rise = 20.0
	// Both placements use the IDENTICAL "1 0 0 1 100 300 cm" — the same
	// composed CTM — so the clone cache's ctm field alone cannot tell
	// them apart; only the inherited rise differs (0 vs rise).
	content := fmt.Sprintf(
		"q\nBT\n%s 12 Tf\nET\n1 0 0 1 100 300 cm\n/Fm0 Do\nQ\n"+ // rise=0 (never set)
			"q\nBT\n%s 12 Tf\n%s Ts\nET\n1 0 0 1 100 300 cm\n/Fm0 Do\nQ\n", // rise=20
		fontName, fontName, formatFloat(rise),
	)
	if err := p.appendToContentStream([]byte(content)); err != nil {
		t.Fatal(err)
	}

	// Wide enough to cover BOTH placements' visible position (y=305 for
	// rise=0, y=325 for rise=20) — so both calls genuinely redact
	// something and both would call storeClone, the precondition for a
	// cache-key collision to actually matter.
	region := Rectangle{LLX: 95, LLY: 295, URX: 250, URY: 335}
	ra := NewRedactAnnotation(p, region)
	if err := p.Annotations().Add(ra); err != nil {
		t.Fatal(err)
	}

	if err := d.ApplyRedactions(); err != nil {
		t.Fatalf("ApplyRedactions: %v", err)
	}

	// Two INDEPENDENT clones, one per inherited rise — the original form
	// object is left referenced by neither Do, so ApplyRedactions' own
	// trailing RemoveUnusedObjects call (run once at the end whenever at
	// least one page was redacted) reclaims it, leaving exactly the two
	// clones. A wrongly-shared clone (the bug this test targets) would
	// instead leave exactly ONE surviving form object after that same
	// sweep, since only one clone would ever have been created.
	if got := formObjectCount(d); got != 2 {
		t.Errorf("formObjectCount = %d, want 2 (two independent clones, one per inherited rise — the rise=0 and rise=%g placements must not share one memoized clone)", got, rise)
	}
	if found, n := documentContainsBytes(d, secret); found {
		t.Errorf("%q still present in %d object(s) after ApplyRedactions", secret, n)
	}
}
