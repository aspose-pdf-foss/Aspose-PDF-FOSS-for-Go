// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"fmt"
	"testing"
)

// pdf-go-w7ye: a Form XObject's own content may draw text with Tj/TJ but
// no Tf of its own, relying entirely on whatever font was active on the
// PAGE at the point of the Do that invokes it — legal per ISO 32000-1
// §8.10.1 (a form inherits the full graphics state, text state included,
// except the CTM). rewriteXObjectDoOperatorsInStream (redact_apply_image.go)
// used to track only ctm across q/Q/cm while walking the page's own
// content, so redactFormXObject's nested call into such a form's content
// always started rewriteTextOperatorsInStream at a ZEROED text state —
// font.known stayed false for the form's own Tj, which (since pdf-go-oq9i)
// made ApplyRedactions REFUSE outright (an error) rather than leaving the
// text silently unredacted. Still a real gap: a document using this
// legal, if unusual, construct could not be redacted at all. These tests
// confirm the fix resolves the font from the state in effect at the Do
// and actually redacts the form's inherited-font text.

// TestApplyRedactionsFormXObjectInheritsPageFont is the baseline
// reproduction: one Form XObject, invoked right after the page sets an
// active font+size with Tf, draws its secret with a bare Tj and no Tf of
// its own.
func TestApplyRedactionsFormXObjectInheritsPageFont(t *testing.T) {
	d := NewDocumentFromFormat(PageFormatA4)
	p, err := d.Page(1)
	if err != nil {
		t.Fatal(err)
	}

	fontName, err := p.ensureStandardFontResource("/Helvetica")
	if err != nil {
		t.Fatal(err)
	}

	const secret = "INHERITED-FONT-SECRET"
	formID := d.nextID
	d.nextID++
	d.objects[formID] = &pdfObject{Num: formID, Value: &pdfStream{
		Dict: pdfDict{
			"/Type":    pdfName("/XObject"),
			"/Subtype": pdfName("/Form"),
			"/BBox":    pdfArray{0.0, 0.0, 200.0, 100.0},
			// No /Resources at all: this form's own content never calls
			// Tf, so it has nothing of its own to resolve a font through
			// — it relies entirely on the page's inherited state.
		},
		Data:    []byte(fmt.Sprintf("BT 20 50 Td (%s) Tj ET\n", secret)),
		Decoded: true,
	}}

	resources := p.pageResources()
	if resources == nil {
		resources = pdfDict{}
		p.pageDict()["/Resources"] = resources
	}
	resources["/XObject"] = pdfDict{"/Fm0": pdfRef{Num: formID}}
	content := fmt.Sprintf("%s 12 Tf\nq 1 0 0 1 50 600 cm /Fm0 Do Q\n", fontName)
	if err := p.appendToContentStream([]byte(content)); err != nil {
		t.Fatal(err)
	}

	// The secret's real page-space position: form-local (20, 50) placed
	// via the "cm" translation (50, 600) => around page (70, 650). Give
	// the region generous margin since Helvetica-12's glyph widths are
	// what the fix must now correctly resolve to place glyph centers.
	ra := NewRedactAnnotation(p, Rectangle{LLX: 50, LLY: 630, URX: 260, URY: 670})
	if err := p.Annotations().Add(ra); err != nil {
		t.Fatal(err)
	}

	if err := d.ValidateRedactions(); err != nil {
		t.Fatalf("ValidateRedactions: %v", err)
	}
	if err := d.ApplyRedactions(); err != nil {
		t.Fatalf("ApplyRedactions: %v (a Form XObject relying on an inherited font should now be redactable, not refused)", err)
	}

	if found, n := documentContainsBytes(d, secret); found {
		t.Errorf("%q still present in %d in-memory object(s) after ApplyRedactions", secret, n)
	}
	var buf bytes.Buffer
	if _, err := d.WriteTo(&buf); err != nil {
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

// TestApplyRedactionsFormXObjectInheritsDifferentFontsPerPlacement places
// TWO Form XObjects, each relying on an inherited font, but with a
// DIFFERENT font size active at each one's own Do — Helvetica 20pt for
// the first, Helvetica 6pt for the second — and a redaction region sized
// to only work if the glyph-center math used the font size genuinely
// active AT THAT Do, not (say) whichever Tf happened to run last while
// walking the page's content, or a stale value left over from a
// mishandled q/Q restore. A region built from the 20pt glyph's real
// (wider) footprint would miss the 6pt secret entirely if the inherited
// state were wrong in either direction, and vice versa.
func TestApplyRedactionsFormXObjectInheritsDifferentFontsPerPlacement(t *testing.T) {
	d := NewDocumentFromFormat(PageFormatA4)
	p, err := d.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	fontName, err := p.ensureStandardFontResource("/Helvetica")
	if err != nil {
		t.Fatal(err)
	}

	const secretBig = "BIGSECRET"
	const secretSmall = "SMALLSECRET"

	makeForm := func(secret string) int {
		id := d.nextID
		d.nextID++
		d.objects[id] = &pdfObject{Num: id, Value: &pdfStream{
			Dict: pdfDict{
				"/Type":    pdfName("/XObject"),
				"/Subtype": pdfName("/Form"),
				"/BBox":    pdfArray{0.0, 0.0, 300.0, 50.0},
			},
			Data:    []byte(fmt.Sprintf("BT 5 15 Td (%s) Tj ET\n", secret)),
			Decoded: true,
		}}
		return id
	}
	formBigID := makeForm(secretBig)
	formSmallID := makeForm(secretSmall)

	resources := p.pageResources()
	if resources == nil {
		resources = pdfDict{}
		p.pageDict()["/Resources"] = resources
	}
	resources["/XObject"] = pdfDict{
		"/FmBig":   pdfRef{Num: formBigID},
		"/FmSmall": pdfRef{Num: formSmallID},
	}

	// Big-font placement at (50, 700); small-font placement far away at
	// (50, 100), so the two regions below can't accidentally both cover
	// both placements.
	content := fmt.Sprintf(
		"%[1]s 40 Tf\nq 1 0 0 1 50 700 cm /FmBig Do Q\n"+
			"%[1]s 5 Tf\nq 1 0 0 1 50 100 cm /FmSmall Do Q\n",
		fontName)
	if err := p.appendToContentStream([]byte(content)); err != nil {
		t.Fatal(err)
	}

	// A region that only has room to catch the SMALL (5pt) glyphs' real
	// footprint — form-local (5, 15) placed at (50, 100) => around page
	// (55, 115). At 40pt this text would be roughly 8x wider/taller and
	// escape a region this tight.
	raSmall := NewRedactAnnotation(p, Rectangle{LLX: 50, LLY: 110, URX: 110, URY: 128})
	if err := p.Annotations().Add(raSmall); err != nil {
		t.Fatal(err)
	}
	// A region sized for the BIG (40pt) glyphs' real footprint at (50, 700).
	raBig := NewRedactAnnotation(p, Rectangle{LLX: 50, LLY: 705, URX: 280, URY: 750})
	if err := p.Annotations().Add(raBig); err != nil {
		t.Fatal(err)
	}

	if err := d.ApplyRedactions(); err != nil {
		t.Fatalf("ApplyRedactions: %v", err)
	}

	// A plain "is the whole literal secret string gone" check is not
	// enough here: if the inherited font SIZE is resolved wrong for one
	// placement (e.g. always the other placement's size), only the
	// glyphs that happen to land inside the local region by coincidence
	// get dropped — the rest survive as a shorter, no-longer-contiguous
	// fragment, which also makes the full literal substring "disappear"
	// even though most of the secret is still sitting right there in the
	// file. redact_apply_form_internal_test.go's own
	// TestApplyRedactionsFormXObjectSubRegion comment documents exactly
	// this trap from an earlier draft. assertFullyGone instead checks
	// every overlapping 4-character window of the secret survives
	// nowhere, which a merely-PARTIAL redaction (the wrong-fontSize
	// failure mode) would almost certainly still leave several of.
	assertFullyGone(t, d, secretBig)
	assertFullyGone(t, d, secretSmall)
}

// TestApplyRedactionsFormXObjectInheritedFontRestoredByQ confirms that
// rewriteXObjectDoOperatorsInStream's "Q" case restores the FULL text
// state it saves on "q" (font, fontName, fontSize, charSpace, wordSpace,
// horizScale, leading) — not just ctm — mirroring the analogous bug this
// project already found and fixed once for redact_apply_text.go's own
// q/Q handling (pdf-go-njbg's review chain: "q/Q only saved/restored ctm,
// not the rest of the text GRAPHICS state"). A Tf set INSIDE a q...Q
// scope must not leak out past the matching Q: a form invoked AFTER that
// Q must inherit whatever font was active BEFORE the q, not whatever was
// left active inside it.
func TestApplyRedactionsFormXObjectInheritedFontRestoredByQ(t *testing.T) {
	d := NewDocumentFromFormat(PageFormatA4)
	p, err := d.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	fontName, err := p.ensureStandardFontResource("/Helvetica")
	if err != nil {
		t.Fatal(err)
	}

	// Deliberately NOT sharing any substring with each other (unlike an
	// earlier draft's "OUTERSECRET"/"INNERSECRET", which both happen to
	// contain "ERSE" — a shared window made assertFullyGone's per-window
	// check against ONE secret trip on bytes that were actually part of
	// the OTHER secret's legitimately-surviving head).
	const secretOuter = "FLAMINGOXJ"
	const secretInner = "WHISKERZQP"

	makeForm := func(secret string) int {
		id := d.nextID
		d.nextID++
		d.objects[id] = &pdfObject{Num: id, Value: &pdfStream{
			Dict: pdfDict{
				"/Type":    pdfName("/XObject"),
				"/Subtype": pdfName("/Form"),
				"/BBox":    pdfArray{0.0, 0.0, 300.0, 50.0},
			},
			Data:    []byte(fmt.Sprintf("BT 5 15 Td (%s) Tj ET\n", secret)),
			Decoded: true,
		}}
		return id
	}
	formInnerID := makeForm(secretInner)
	formOuterID := makeForm(secretOuter)

	resources := p.pageResources()
	if resources == nil {
		resources = pdfDict{}
		p.pageDict()["/Resources"] = resources
	}
	resources["/XObject"] = pdfDict{
		"/FmInner": pdfRef{Num: formInnerID},
		"/FmOuter": pdfRef{Num: formOuterID},
	}

	// Active font before the q: 30pt. Inside q...Q, switched to 6pt for
	// FmInner. After Q, the OUTER form is invoked — it must inherit the
	// 30pt active BEFORE the q, not the 6pt left over from inside it.
	content := fmt.Sprintf(
		"%[1]s 30 Tf\n"+
			"q\n%[1]s 6 Tf\n1 0 0 1 50 100 cm /FmInner Do\nQ\n"+
			"1 0 0 1 50 700 cm /FmOuter Do\n",
		fontName)
	if err := p.appendToContentStream([]byte(content)); err != nil {
		t.Fatal(err)
	}

	// Region covering the INNER form's ENTIRE placement (its whole
	// 300x50 BBox) — generous on purpose; this form's own check only
	// cares that it starts from the font set just before its own Do
	// (6pt), regardless of Q, not about region tightness.
	raInner := NewRedactAnnotation(p, Rectangle{LLX: 50, LLY: 100, URX: 350, URY: 150})
	if err := p.Annotations().Add(raInner); err != nil {
		t.Fatal(err)
	}
	// A TAIL-only window over where the LAST few characters of
	// OUTERSECRET land at its real 30pt size (starting at local x=5,
	// "OUTERSECRET" at 30pt spans roughly to local x≈220) — deliberately
	// NOT covering the run's start. A smaller, wrongly-inherited size
	// (e.g. the 6pt leaked from inside the q, if Q failed to restore
	// fontSize) would compute the WHOLE run as only ~40 local units wide,
	// never reaching this window at all — so NONE of it would be
	// classified as inside any region, and the entire literal string
	// would survive untouched (droppedCount == 0 keeps the original Tj
	// verbatim), not merely partially — an easy, unambiguous signal a
	// tail-only window is specifically designed to produce.
	raOuter := NewRedactAnnotation(p, Rectangle{LLX: 200, LLY: 710, URX: 275, URY: 728})
	if err := p.Annotations().Add(raOuter); err != nil {
		t.Fatal(err)
	}

	if err := d.ApplyRedactions(); err != nil {
		t.Fatalf("ApplyRedactions: %v", err)
	}

	// The INNER form's region covers its whole real (6pt) footprint —
	// expect it fully gone, same oracle as the other tests in this file.
	assertFullyGone(t, d, secretInner)

	// The OUTER region deliberately covers only the TAIL of where
	// OUTERSECRET lands at its real (correctly-restored) 30pt size, so
	// the two possible outcomes are unambiguous and different, not just
	// "more vs less complete":
	//   - fontSize correctly restored to 30 by Q: the run is wide enough
	//     to reach the tail region, so the TAIL gets dropped while the
	//     HEAD (outside any region) survives — a genuine partial match.
	//   - fontSize wrongly left at 6 (leaked from inside the q): the
	//     whole run is computed as far too narrow to ever reach the
	//     tail region at all, so NOTHING is classified as inside any
	//     region and the ENTIRE original string survives, tail included.
	// Checking the tail is gone therefore distinguishes the two outcomes
	// directly, where checking the whole string's absence (as
	// assertFullyGone does) cannot: a correct partial match legitimately
	// still contains the head, which assertFullyGone would misreport as
	// a failure.
	if found, n := documentContainsBytes(d, "GOXJ"); found {
		t.Errorf(`"GOXJ" (tail of secret %q) still present in %d object(s) — the font size active before the "q" did not survive past the matching "Q"`, secretOuter, n)
	}
	if found, _ := documentContainsBytes(d, "FLAM"); !found {
		t.Error(`"FLAM" (head of secret, outside any region) unexpectedly gone — expected a genuine partial match, not a full one`)
	}
}

// assertFullyGone fails the test if ANY overlapping 4-character window of
// secret is still found anywhere in doc's objects — a stronger check than
// documentContainsBytes(doc, secret) alone, which a merely partial
// redaction (breaking the string's contiguity without actually dropping
// most of its glyphs) can satisfy by accident.
func assertFullyGone(t *testing.T, doc *Document, secret string) {
	t.Helper()
	const windowLen = 4
	if len(secret) < windowLen {
		if found, n := documentContainsBytes(doc, secret); found {
			t.Errorf("%q still present in %d object(s)", secret, n)
		}
		return
	}
	for i := 0; i+windowLen <= len(secret); i++ {
		window := secret[i : i+windowLen]
		if found, n := documentContainsBytes(doc, window); found {
			t.Errorf("%q (window %q of secret %q) still present in %d object(s) — redaction may have only partially removed this run", window, window, secret, n)
		}
	}
}
