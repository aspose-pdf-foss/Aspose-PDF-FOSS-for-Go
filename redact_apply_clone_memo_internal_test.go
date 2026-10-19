// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"testing"
)

// pdf-go-wuv7: every region-intersecting Form XObject placement used to
// get its own full clone object, even when several placements were
// byte-identical (same form object, same composed matrix, same inherited
// text state) — nothing memoized a clone per (form, ctm, inherited)
// within one ApplyRedactions call, so a document placing one large,
// compressible Form XObject many times at the same spot (or several
// annotations sharing one appearance stream) inflated the saved file
// with many full-size duplicate clones.

// formObjectCount counts how many *pdfStream objects in d.objects are
// Form XObjects (/Subtype /Form) — the clone-count oracle this file's
// tests use.
func formObjectCount(d *Document) int {
	n := 0
	for _, obj := range d.objects {
		if st, ok := obj.Value.(*pdfStream); ok && dictGetName(st.Dict, "/Subtype") == "/Form" {
			n++
		}
	}
	return n
}

// TestApplyRedactionsFormXObjectMemoizesIdenticalPlacements places the
// SAME Form XObject twice at the exact same spot on one page (two
// separate Do operators, same form, same cm) — a producer stamping a
// logo or watermark more than once at a fixed position is a realistic
// shape for this, though the mechanism works for any count. Both
// placements must redact correctly, but only ONE clone object should
// exist afterward — the second placement must reuse the first's clone
// rather than duplicating it.
func TestApplyRedactionsFormXObjectMemoizesIdenticalPlacements(t *testing.T) {
	doc := NewDocumentFromFormat(PageFormatA4)
	form := doc.CreateForm(100, 50)
	const secret = "MEMOIZED-PLACEMENT-SECRET"
	if err := form.Canvas().AddText(secret, TextStyle{Size: 10},
		Rectangle{LLX: 0, LLY: 0, URX: 100, URY: 50}); err != nil {
		t.Fatal(err)
	}
	page, err := doc.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	placement := Rectangle{LLX: 50, LLY: 600, URX: 150, URY: 650}
	if err := page.AddForm(form, placement); err != nil {
		t.Fatal(err)
	}
	if err := page.AddForm(form, placement); err != nil { // same form, same spot, again
		t.Fatal(err)
	}

	ra := NewRedactAnnotation(page, placement)
	if err := page.Annotations().Add(ra); err != nil {
		t.Fatal(err)
	}

	if err := doc.ApplyRedactions(); err != nil {
		t.Fatalf("ApplyRedactions: %v", err)
	}

	if found, n := documentContainsBytes(doc, secret); found {
		t.Errorf("%q still present in %d object(s) after ApplyRedactions", secret, n)
	}

	if got := formObjectCount(doc); got != 1 {
		t.Errorf("formObjectCount = %d, want 1 (the two identical placements should share one memoized clone, not produce two)", got)
	}
}

// TestApplyRedactionsFormXObjectDoesNotMemoizeDifferentPlacements is the
// correctness counterpart: two placements of the SAME form at DIFFERENT
// spots (different composed CTM) must NOT be treated as identical —
// each gets its own clone, since their redacted content can genuinely
// differ (the region maps to different local coordinates in each).
func TestApplyRedactionsFormXObjectDoesNotMemoizeDifferentPlacements(t *testing.T) {
	doc := NewDocumentFromFormat(PageFormatA4)
	form := doc.CreateForm(100, 50)
	const secret = "DISTINCT-PLACEMENT-SECRET"
	if err := form.Canvas().AddText(secret, TextStyle{Size: 10},
		Rectangle{LLX: 0, LLY: 0, URX: 100, URY: 50}); err != nil {
		t.Fatal(err)
	}
	page, err := doc.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	p1 := Rectangle{LLX: 50, LLY: 600, URX: 150, URY: 650}
	p2 := Rectangle{LLX: 50, LLY: 200, URX: 150, URY: 250} // different spot
	if err := page.AddForm(form, p1); err != nil {
		t.Fatal(err)
	}
	if err := page.AddForm(form, p2); err != nil {
		t.Fatal(err)
	}

	// A region covering only the SECOND placement's position.
	ra := NewRedactAnnotation(page, p2)
	if err := page.Annotations().Add(ra); err != nil {
		t.Fatal(err)
	}

	if err := doc.ApplyRedactions(); err != nil {
		t.Fatalf("ApplyRedactions: %v", err)
	}

	if found, n := documentContainsBytes(doc, secret); found {
		t.Errorf("%q still present in %d object(s) — the untouched-looking first placement must not have leaked its own copy of the secret via a wrongly-shared clone", secret, n)
	}
}

// TestApplyRedactionsAnnotationAppearanceMemoizesSharedStream confirms
// the same clone cache is shared with the annotation-appearance path
// (redactOneAnnotationAppearance): two different widgets whose /AP/N
// happens to point at the literal same stream object (a producer
// sharing one appearance across widgets with the same value/look is a
// realistic shape for this) must redact into a single shared clone, not
// two independent ones.
func TestApplyRedactionsAnnotationAppearanceMemoizesSharedStream(t *testing.T) {
	const secret = "SHARED-WIDGET-SECRET"
	d := NewDocumentFromFormat(PageFormatA4)
	rect := Rectangle{LLX: 50, LLY: 650, URX: 250, URY: 680}

	field1, err := d.Form().AddTextField(1, rect, "f1")
	if err != nil {
		t.Fatal(err)
	}
	if err := field1.SetValue(secret); err != nil {
		t.Fatal(err)
	}
	// Same /Rect as field1, so the two widgets' appearance placement
	// matrices (annotAppearanceMatrix) come out identical too.
	field2, err := d.Form().AddTextField(1, rect, "f2")
	if err != nil {
		t.Fatal(err)
	}
	if err := field2.SetValue(secret); err != nil {
		t.Fatal(err)
	}

	ap1, ok := resolveRefToDict(d.objects, field1.node.dict["/AP"])
	if !ok {
		t.Fatal("field1 has no /AP")
	}
	ap2, ok := resolveRefToDict(d.objects, field2.node.dict["/AP"])
	if !ok {
		t.Fatal("field2 has no /AP")
	}
	// Force field2 to reference the EXACT SAME appearance stream object
	// as field1 (not just a byte-identical copy): a producer sharing one
	// appearance across several widgets with the same value/look.
	ap2["/N"] = ap1["/N"]

	p, err := d.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	ra := NewRedactAnnotation(p, rect)
	if err := p.Annotations().Add(ra); err != nil {
		t.Fatal(err)
	}

	if err := d.ApplyRedactions(); err != nil {
		t.Fatalf("ApplyRedactions: %v", err)
	}

	// Checked at the appearance STREAM's own content, same as
	// TestApplyRedactionsRedactsWidgetAppearance — not a whole-document
	// scan, since each field's /V legitimately still carries the value
	// (the field's data model, deliberately out of this method's scope,
	// same as a FreeText annotation's own /Contents metadata) and would
	// make documentContainsBytes report a "leak" that isn't one.
	newAp1, ok := resolveRefToDict(d.objects, field1.node.dict["/AP"])
	if !ok {
		t.Fatal("field1 lost its /AP entirely")
	}
	newAp2, ok := resolveRefToDict(d.objects, field2.node.dict["/AP"])
	if !ok {
		t.Fatal("field2 lost its /AP entirely")
	}
	ref1, ok1 := newAp1["/N"].(pdfRef)
	ref2, ok2 := newAp2["/N"].(pdfRef)
	if !ok1 || !ok2 {
		t.Fatalf("one of the widgets' /AP/N is not a plain ref after redaction (field1: %T, field2: %T)", newAp1["/N"], newAp2["/N"])
	}
	if ref1.Num != ref2.Num {
		t.Errorf("expected both widgets to share the same memoized clone object, got distinct objects %d and %d", ref1.Num, ref2.Num)
	}

	stAfter, ok := resolveRef(d.objects, newAp1["/N"]).(*pdfStream)
	if !ok {
		t.Fatal("field1 /AP/N no longer resolves to a stream")
	}
	if bytes.Contains(decodedStreamData(stAfter), []byte(secret)) {
		t.Error("the shared appearance stream still draws the secret after ApplyRedactions")
	}
}

// TestNextFormXObjectResourceNameCursorStaysCorrect pins
// nextFormXObjectResourceName's second pdf-go-wuv7 complaint (clone
// naming quadratic in clone count): the budget's fmCursor lets each call
// start searching where the LAST one left off instead of always
// rescanning from "/Fm0". The cursor is purely a performance shortcut —
// this test's actual job is confirming it stays CORRECT under that
// shortcut: every name handed out is unique and not already present in
// xobjDict, across many calls sharing one budget, including the case
// where some low-numbered names already exist in the dict from some
// OTHER source (e.g. a third-party file's own pre-existing "/FmN"
// entries) that the cursor must still not collide with on its very
// first call.
func TestNextFormXObjectResourceNameCursorStaysCorrect(t *testing.T) {
	xobjDict := pdfDict{
		"/Fm0": pdfRef{Num: 1},
		"/Fm1": pdfRef{Num: 2},
		// "/Fm2" deliberately left free, so the first call must find it
		// rather than assuming indices 0.."whatever the cursor says" are
		// all free.
	}
	budget := &formRedactBudget{}

	const n = 500
	seen := map[string]bool{}
	for i := 0; i < n; i++ {
		name := nextFormXObjectResourceName(xobjDict, budget)
		if _, exists := xobjDict[name]; exists {
			t.Fatalf("call %d: nextFormXObjectResourceName returned %q, which already exists in xobjDict", i, name)
		}
		if seen[name] {
			t.Fatalf("call %d: nextFormXObjectResourceName returned %q twice", i, name)
		}
		seen[name] = true
		xobjDict[name] = pdfRef{Num: 1000 + i} // simulate the caller registering it
	}
	if len(seen) != n {
		t.Errorf("got %d unique names, want %d", len(seen), n)
	}
}
