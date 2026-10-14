// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"strings"
	"testing"
)

// Form XObject redaction (pdf-go-w5hl): ApplyRedactions now recurses into
// a Form XObject's own content stream rather than refusing — the real
// fix for the v0.11.0 safety stopgap (unsafeFormXObjectName's old
// blanket refuse) that pdf-go-2qkq's review chain shipped instead, since
// recursing correctly needed working out the local-coordinate-space
// transform and the shared-placement clone-not-mutate requirement first.
// These tests live in the internal package (not redact_apply_test.go,
// which is asposepdf_test) because they need documentContainsBytes'
// forensic verification.

// TestApplyRedactionsFormXObjectFullyInRegion: a redaction region fully
// covering a Form XObject's placement must leave its text unrecoverable
// anywhere in the saved file, not just absent from ExtractText — exactly
// the distinction that hid pdf-go-2qkq in the first place.
func TestApplyRedactionsFormXObjectFullyInRegion(t *testing.T) {
	doc := NewDocumentFromFormat(PageFormatA4)
	form := doc.CreateForm(200, 100)
	const secret = "FORMXOBJECT-SECRET-998877"
	if err := form.Canvas().AddText(secret, TextStyle{Size: 12},
		Rectangle{LLX: 0, LLY: 0, URX: 200, URY: 100}); err != nil {
		t.Fatal(err)
	}
	page, err := doc.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	placement := Rectangle{LLX: 50, LLY: 600, URX: 250, URY: 700}
	if err := page.AddForm(form, placement); err != nil {
		t.Fatal(err)
	}
	ra := NewRedactAnnotation(page, placement) // fully covers the form
	if err := page.Annotations().Add(ra); err != nil {
		t.Fatal(err)
	}

	if err := doc.ValidateRedactions(); err != nil {
		t.Fatalf("ValidateRedactions: %v", err)
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
}

// TestApplyRedactionsFormXObjectSubRegion exercises the bbox-math path
// (classifyDoBbox's unit-square assumption is wrong for a Form XObject;
// formPlacementBbox exists because of exactly this) with a small region
// in a corner of the form's real footprint, far from its placement's CTM
// origin, and confirms text OUTSIDE that region (elsewhere in the same
// form) survives while text inside it does not.
func TestApplyRedactionsFormXObjectSubRegion(t *testing.T) {
	doc := NewDocumentFromFormat(PageFormatA4)
	form := doc.CreateForm(200, 100)
	// Short enough, at this size, to stay on ONE Tj (AddText word-wraps
	// a too-wide run into separate Tj calls, which would defeat the
	// literal-substring forensic check below regardless of whether
	// redaction itself works — a prior draft of this test had exactly
	// that bug: it "passed" under a mutation that skipped redaction
	// entirely, because the secret was never contiguous in the stream
	// to begin with).
	const secret = "SECRET1"
	const survivor = "FORMXOBJECT-SURVIVOR"
	if err := form.Canvas().AddText(secret, TextStyle{Size: 8},
		Rectangle{LLX: 150, LLY: 80, URX: 200, URY: 95}); err != nil {
		t.Fatal(err)
	}
	if err := form.Canvas().AddText(survivor, TextStyle{Size: 10},
		Rectangle{LLX: 0, LLY: 0, URX: 140, URY: 16}); err != nil {
		t.Fatal(err)
	}
	page, err := doc.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	// Scale 1:1 (placement rect == form's own 200x100 size), so the old,
	// unit-square-based bbox math would have placed its bogus ~1x1 "bbox"
	// right at (50, 600) — nowhere near this region.
	placement := Rectangle{LLX: 50, LLY: 600, URX: 250, URY: 700}
	if err := page.AddForm(form, placement); err != nil {
		t.Fatal(err)
	}
	// A small region in the far corner of the form's REAL footprint
	// (translated into page space by the placement's own origin: the
	// secret sits at local (150..200, 80..95), i.e. page (200..250,
	// 680..695)), well away from the placement rect's own origin corner.
	region := Rectangle{LLX: 195, LLY: 675, URX: 240, URY: 696}
	ra := NewRedactAnnotation(page, region)
	if err := page.Annotations().Add(ra); err != nil {
		t.Fatal(err)
	}

	if err := doc.ValidateRedactions(); err != nil {
		t.Fatalf("ValidateRedactions: %v", err)
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
	p1, err := reopened.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	txt, err := p1.ExtractText()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.ReplaceAll(txt, "\n", ""), survivor) {
		t.Errorf("text elsewhere in the same form was lost: got %q, want it to still contain %q", txt, survivor)
	}
}

// TestApplyRedactionsFormXObjectSharedPlacementNotTouched is THE key
// correctness test for the clone-not-mutate requirement (redactFormDo's
// doc comment): the same Form XObject object placed on two different
// pages, where a redaction region overlaps only ONE of them. The
// untouched page's placement must render/extract exactly as before —
// proving the original form's content stream was never rewritten in
// place, only this one Do operand redirected to a private, cloned copy.
func TestApplyRedactionsFormXObjectSharedPlacementNotTouched(t *testing.T) {
	doc := NewDocumentFromFormat(PageFormatA4)
	form := doc.CreateForm(200, 100)
	const secret = "SHARED-FORM-SECRET"
	if err := form.Canvas().AddText(secret, TextStyle{Size: 12},
		Rectangle{LLX: 0, LLY: 0, URX: 200, URY: 100}); err != nil {
		t.Fatal(err)
	}

	page1, err := doc.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	placement := Rectangle{LLX: 50, LLY: 600, URX: 250, URY: 700}
	if err := page1.AddForm(form, placement); err != nil {
		t.Fatal(err)
	}

	if err := doc.AddBlankPage(595, 842); err != nil {
		t.Fatal(err)
	}
	page2, err := doc.Page(2)
	if err != nil {
		t.Fatal(err)
	}
	// Same form object, placed again — a shared logo stamped on a second
	// page. This page gets NO redact annotation: its placement must stay
	// fully intact.
	if err := page2.AddForm(form, placement); err != nil {
		t.Fatal(err)
	}

	ra := NewRedactAnnotation(page1, placement) // only page 1 is redacted
	if err := page1.Annotations().Add(ra); err != nil {
		t.Fatal(err)
	}

	if err := doc.ApplyRedactions(); err != nil {
		t.Fatalf("ApplyRedactions: %v", err)
	}

	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}

	p1, err := reopened.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	txt1, err := p1.ExtractText()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(txt1, secret) {
		t.Errorf("page 1 (redacted) still contains %q: %q", secret, txt1)
	}

	p2, err := reopened.Page(2)
	if err != nil {
		t.Fatal(err)
	}
	txt2, err := p2.ExtractText()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(txt2, secret) {
		t.Errorf("page 2 (NOT redacted, shares the same Form XObject) lost its text: got %q, want it to still contain %q", txt2, secret)
	}
}

// TestApplyRedactionsFormXObjectSharedNestedFormResourcesNotCorrupted
// pins a severe bug an independent review found (and this reproduced
// directly, before fixing it): a Form XObject's OWN /Resources dict is
// shared by EVERY placement of that same form object, not just the one
// a redaction happens to intersect. redactFormXObject used to resolve
// and mutate that shared dict directly when redirecting a NESTED Do to
// a clone (or pruning the stale name afterward) — so redacting content
// reached through ONE placement of an outer form silently corrupted a
// completely different, never-redacted placement of the SAME outer form
// elsewhere, breaking its nested content (the stale/missing /XObject
// entry made its own Do operand dangle). Here: one outer form (with its
// own /Resources, placing an inner form) is placed on two pages; only
// page 1 is redacted. Page 2's placement — sharing the outer form's
// object, and therefore its /Resources, with page 1 — must render
// exactly as before: the inner form's text must still be there.
func TestApplyRedactionsFormXObjectSharedNestedFormResourcesNotCorrupted(t *testing.T) {
	doc := NewDocumentFromFormat(PageFormatA4)
	inner := doc.CreateForm(100, 50)
	const secret = "SHARED-NESTED-SECRET"
	if err := inner.Canvas().AddText(secret, TextStyle{Size: 10},
		Rectangle{LLX: 0, LLY: 0, URX: 100, URY: 50}); err != nil {
		t.Fatal(err)
	}

	outer := doc.CreateForm(200, 100)
	if err := outer.Canvas().AddForm(inner, Rectangle{LLX: 50, LLY: 25, URX: 150, URY: 75}); err != nil {
		t.Fatal(err)
	}

	page1, err := doc.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	placement := Rectangle{LLX: 50, LLY: 600, URX: 250, URY: 700}
	if err := page1.AddForm(outer, placement); err != nil {
		t.Fatal(err)
	}

	if err := doc.AddBlankPage(595, 842); err != nil {
		t.Fatal(err)
	}
	page2, err := doc.Page(2)
	if err != nil {
		t.Fatal(err)
	}
	// Same OUTER form object (hence the same /Resources, which names the
	// INNER form) placed again — page 2 gets NO redact annotation at all.
	if err := page2.AddForm(outer, placement); err != nil {
		t.Fatal(err)
	}

	ra := NewRedactAnnotation(page1, placement) // covers the outer form entirely -> recurses into inner
	if err := page1.Annotations().Add(ra); err != nil {
		t.Fatal(err)
	}

	if err := doc.ApplyRedactions(); err != nil {
		t.Fatalf("ApplyRedactions: %v", err)
	}

	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}

	p1, err := reopened.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	txt1, err := p1.ExtractText()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(txt1, secret) {
		t.Errorf("page 1 (redacted) still contains %q: %q", secret, txt1)
	}

	p2, err := reopened.Page(2)
	if err != nil {
		t.Fatal(err)
	}
	txt2, err := p2.ExtractText()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.ReplaceAll(txt2, "\n", ""), secret) {
		t.Errorf("page 2 (NOT redacted, shares the outer form's /Resources with page 1) lost the inner form's text: got %q, want it to still contain %q", txt2, secret)
	}
}

// TestApplyRedactionsFormXObjectNestedForm confirms the recursive case:
// a Form XObject that itself places another (nested) Form XObject, with
// redactable text inside the NESTED one, is still reached and redacted —
// not just the outer form's own direct content.
func TestApplyRedactionsFormXObjectNestedForm(t *testing.T) {
	doc := NewDocumentFromFormat(PageFormatA4)
	inner := doc.CreateForm(100, 50)
	const secret = "NESTED-FORM-SECRET"
	if err := inner.Canvas().AddText(secret, TextStyle{Size: 10},
		Rectangle{LLX: 0, LLY: 0, URX: 100, URY: 50}); err != nil {
		t.Fatal(err)
	}

	outer := doc.CreateForm(200, 100)
	// Place the inner form inside the outer form's own canvas.
	if err := outer.Canvas().AddForm(inner, Rectangle{LLX: 50, LLY: 25, URX: 150, URY: 75}); err != nil {
		t.Fatal(err)
	}

	page, err := doc.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	placement := Rectangle{LLX: 50, LLY: 600, URX: 250, URY: 700}
	if err := page.AddForm(outer, placement); err != nil {
		t.Fatal(err)
	}
	ra := NewRedactAnnotation(page, placement) // covers the whole outer form, hence the nested one too
	if err := page.Annotations().Add(ra); err != nil {
		t.Fatal(err)
	}

	if err := doc.ValidateRedactions(); err != nil {
		t.Fatalf("ValidateRedactions: %v", err)
	}
	if err := doc.ApplyRedactions(); err != nil {
		t.Fatalf("ApplyRedactions: %v", err)
	}

	if found, n := documentContainsBytes(doc, secret); found {
		t.Errorf("%q still present in %d in-memory object(s) after ApplyRedactions (nested Form XObject not reached)", secret, n)
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

// TestApplyRedactionsFormXObjectNonInvertibleMatrixRefuses: a Form
// XObject whose own /Matrix collapses to a non-invertible transform
// (det == 0) intersecting a region cannot have that region mapped into
// the form's local content space at all — ApplyRedactions/
// ValidateRedactions must refuse rather than silently produce wrong
// output, same "refuse rather than guess" principle as the missing-/BBox
// case this whole fix chain established.
func TestApplyRedactionsFormXObjectNonInvertibleMatrixRefuses(t *testing.T) {
	d := NewDocumentFromFormat(PageFormatA4)
	p, err := d.Page(1)
	if err != nil {
		t.Fatal(err)
	}

	formID := d.nextID
	d.nextID++
	d.objects[formID] = &pdfObject{Num: formID, Value: &pdfStream{
		Dict: pdfDict{
			"/Type":    pdfName("/XObject"),
			"/Subtype": pdfName("/Form"),
			"/BBox":    pdfArray{0.0, 0.0, 100.0, 100.0},
			// Degenerate (all-zero scale/shear) matrix: determinant 0,
			// not invertible.
			"/Matrix": pdfArray{0.0, 0.0, 0.0, 0.0, 0.0, 0.0},
		},
		Data:    []byte("1 0 0 rg 0 0 100 100 re f\n"),
		Decoded: true,
	}}

	resources := p.pageResources()
	if resources == nil {
		resources = pdfDict{}
		p.pageDict()["/Resources"] = resources
	}
	resources["/XObject"] = pdfDict{"/Fm0": pdfRef{Num: formID}}
	if err := p.appendToContentStream([]byte("q 1 0 0 1 50 600 cm /Fm0 Do Q\n")); err != nil {
		t.Fatal(err)
	}

	// The all-zero /Matrix composed with this cm's translation collapses
	// every point the form draws to the single point (50, 600) — so the
	// region must strictly CONTAIN that point, not just touch its edge
	// (classifyBbox's intersection test treats an edge-touching
	// zero-area bbox as disjoint), to reach the invertibility check at
	// all rather than being classified keepDo first.
	ra := NewRedactAnnotation(p, Rectangle{LLX: 40, LLY: 590, URX: 60, URY: 610})
	if err := p.Annotations().Add(ra); err != nil {
		t.Fatal(err)
	}

	if err := d.ValidateRedactions(); err == nil {
		t.Error("ValidateRedactions: want an error (non-invertible Form XObject matrix), got nil")
	}
	if err := d.ApplyRedactions(); err == nil {
		t.Error("ApplyRedactions: want an error (non-invertible Form XObject matrix), got nil")
	}
}

// TestApplyRedactionsFormXObjectCloneDoesNotLeakOriginal confirms the
// clone's housekeeping: once the only placement of a Form XObject is
// redacted (so the original object is no longer referenced by any Do
// operand — the redacted clone replaced it), RemoveUnusedObjects (which
// ApplyRedactions itself calls) reclaims the original, and it is gone
// from the saved file. This is the flip side of the shared-placement
// test: here there is only ONE placement, so clone-and-redirect must
// still let the old, now-unreferenced original go.
func TestApplyRedactionsFormXObjectCloneDoesNotLeakOriginal(t *testing.T) {
	doc := NewDocumentFromFormat(PageFormatA4)
	form := doc.CreateForm(200, 100)
	const secret = "CLONE-ORIGINAL-SECRET"
	if err := form.Canvas().AddText(secret, TextStyle{Size: 12},
		Rectangle{LLX: 0, LLY: 0, URX: 200, URY: 100}); err != nil {
		t.Fatal(err)
	}
	page, err := doc.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	placement := Rectangle{LLX: 50, LLY: 600, URX: 250, URY: 700}
	if err := page.AddForm(form, placement); err != nil {
		t.Fatal(err)
	}
	ra := NewRedactAnnotation(page, placement)
	if err := page.Annotations().Add(ra); err != nil {
		t.Fatal(err)
	}

	if err := doc.ApplyRedactions(); err != nil {
		t.Fatalf("ApplyRedactions: %v", err)
	}

	// The form's ORIGINAL object number must no longer be present at
	// all — ApplyRedactions' own RemoveUnusedObjects call reclaims it
	// once the page's only Do operand was redirected to the clone.
	if _, stillThere := doc.objects[form.formID]; stillThere {
		t.Errorf("original Form XObject object %d still present after ApplyRedactions reclaimed unused objects", form.formID)
	}
}

// TestFormRedactBudgetConsumeRefusesWhenExhausted is a focused unit test
// of the work-budget mechanism itself (maxFormRedactWork): consume()
// must start succeeding and, once the budget is spent, keep refusing —
// this is what bounds the TOTAL number of Form XObject placements
// ApplyRedactions will redact into across one call, independent of the
// depth cap (a shallow but wide fan-out of region-intersecting
// placements can exhaust this long before maxFormRedactDepth is ever
// reached — an independent review confirmed this is reachable: a form
// that places two region-intersecting copies of a child, each placing
// two more, doubles the work every level).
func TestFormRedactBudgetConsumeRefusesWhenExhausted(t *testing.T) {
	b := &formRedactBudget{}
	for i := 0; i < maxFormRedactWork; i++ {
		if err := b.consume(); err != nil {
			t.Fatalf("consume() #%d: unexpected error before the budget is exhausted: %v", i, err)
		}
	}
	if err := b.consume(); err == nil {
		t.Error("consume(): want an error once the work budget is exhausted, got nil")
	}

	// A nil budget (unit tests that call rewriteXObjectDoOperatorsInStream
	// directly with a hand-built op list, with no budget to pass) must
	// never panic and never refuse — it means "this call site doesn't
	// care to bound the work."
	var nilBudget *formRedactBudget
	if err := nilBudget.consume(); err != nil {
		t.Errorf("consume() on a nil budget: want nil, got %v", err)
	}
}

// TestApplyRedactionsFormXObjectExhaustedWorkBudgetRefuses is the
// integration-level counterpart: a form tree shaped to exceed
// maxFormRedactWork well before maxFormRedactDepth (fanout 3, depth 9 =
// 3^9 ≈ 19683 region-intersecting placements, each triggering its own
// redactFormXObject call since redactFormDo runs once per Do OCCURRENCE,
// not once per unique object -- so this needs only `depth` distinct form
// objects to build, reused via multiple Do operators per level, not an
// exponential number of objects). ApplyRedactions must refuse with an
// error rather than grinding through (and cloning!) tens of thousands of
// placements.
func TestApplyRedactionsFormXObjectExhaustedWorkBudgetRefuses(t *testing.T) {
	const fanout = 3
	const depth = 9 // fanout^depth ≈ 19683 > maxFormRedactWork (10000)

	doc := NewDocumentFromFormat(PageFormatA4)
	level := doc.CreateForm(20, 20)
	if err := level.Canvas().AddText("x", TextStyle{Size: 8}, Rectangle{LLX: 0, LLY: 0, URX: 20, URY: 20}); err != nil {
		t.Fatal(err)
	}
	for d := 0; d < depth; d++ {
		next := doc.CreateForm(60, 20)
		for i := 0; i < fanout; i++ {
			x := float64(i) * 20
			if err := next.Canvas().AddForm(level, Rectangle{LLX: x, LLY: 0, URX: x + 20, URY: 20}); err != nil {
				t.Fatal(err)
			}
		}
		level = next
	}

	page, err := doc.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	placement := Rectangle{LLX: 50, LLY: 600, URX: 50 + 60, URY: 620}
	if err := page.AddForm(level, placement); err != nil {
		t.Fatal(err)
	}
	ra := NewRedactAnnotation(page, placement) // covers the whole tree
	if err := page.Annotations().Add(ra); err != nil {
		t.Fatal(err)
	}

	if err := doc.ApplyRedactions(); err == nil {
		t.Error("ApplyRedactions: want an error (work budget exceeded by a wide fan-out of region-intersecting placements), got nil")
	}
}

// TestApplyRedactionsFormXObjectExceedsDepthRefuses: a chain of nested
// Form XObjects deeper than maxFormRedactDepth, each one fully covered
// by the redact region (so recursion is attempted all the way down),
// must refuse rather than silently leaving the deepest, unexamined
// levels unredacted — the "refuse rather than leave incomplete"
// principle this whole feature already applies to a non-invertible
// matrix or an unresolvable /BBox (code review of an earlier draft,
// which stopped silently at the cap instead of erroring).
func TestApplyRedactionsFormXObjectExceedsDepthRefuses(t *testing.T) {
	doc := NewDocumentFromFormat(PageFormatA4)
	level := doc.CreateForm(50, 50)
	if err := level.Canvas().AddText("leaf", TextStyle{Size: 8}, Rectangle{LLX: 0, LLY: 0, URX: 50, URY: 50}); err != nil {
		t.Fatal(err)
	}
	for d := 0; d < maxFormRedactDepth+2; d++ {
		next := doc.CreateForm(50, 50)
		if err := next.Canvas().AddForm(level, Rectangle{LLX: 0, LLY: 0, URX: 50, URY: 50}); err != nil {
			t.Fatal(err)
		}
		level = next
	}

	page, err := doc.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	placement := Rectangle{LLX: 50, LLY: 600, URX: 100, URY: 650}
	if err := page.AddForm(level, placement); err != nil {
		t.Fatal(err)
	}
	ra := NewRedactAnnotation(page, placement) // fully covers the whole chain
	if err := page.Annotations().Add(ra); err != nil {
		t.Fatal(err)
	}

	if err := doc.ValidateRedactions(); err == nil {
		t.Error("ValidateRedactions: want an error (nesting exceeds the redaction depth limit), got nil")
	}
	if err := doc.ApplyRedactions(); err == nil {
		t.Error("ApplyRedactions: want an error (nesting exceeds the redaction depth limit), got nil")
	}
}

// TestApplyRedactionsFormXObjectSharedPageResourcesNotCorrupted pins the
// page-level counterpart of the shared-/Resources bug: a PDF producer can
// have two pages share the exact same /Resources object via an indirect
// reference (a common size optimization), not just a Form XObject's own
// /Resources being shared across placements. Redacting page 1's Form
// XObject (covered by a region) must not corrupt page 2's otherwise
// identical /Fm0 Do -- page 2 has no redact annotation at all and must
// render/extract exactly as before.
func TestApplyRedactionsFormXObjectSharedPageResourcesNotCorrupted(t *testing.T) {
	d := NewDocumentFromFormat(PageFormatA4)
	if err := d.AddBlankPage(595, 842); err != nil {
		t.Fatal(err)
	}
	p1, err := d.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	p2, err := d.Page(2)
	if err != nil {
		t.Fatal(err)
	}

	const secret = "SHARED-PAGE-SECRET"
	formID := d.nextID
	d.nextID++
	d.objects[formID] = &pdfObject{Num: formID, Value: &pdfStream{
		Dict: pdfDict{
			"/Type":    pdfName("/XObject"),
			"/Subtype": pdfName("/Form"),
			"/BBox":    pdfArray{0.0, 0.0, 200.0, 100.0},
		},
		Data:    []byte("BT /F0 12 Tf 0 0 0 rg 10 50 Td (" + secret + ") Tj ET\n"),
		Decoded: true,
	}}

	fontID := d.nextID
	d.nextID++
	d.objects[fontID] = &pdfObject{Num: fontID, Value: pdfDict{
		"/Type":     pdfName("/Font"),
		"/Subtype":  pdfName("/Type1"),
		"/BaseFont": pdfName("/Helvetica"),
		"/Encoding": pdfName("/WinAnsiEncoding"),
	}}

	// ONE resources object, referenced indirectly by BOTH pages.
	resID := d.nextID
	d.nextID++
	d.objects[resID] = &pdfObject{Num: resID, Value: pdfDict{
		"/Font":    pdfDict{"/F0": pdfRef{Num: fontID}},
		"/XObject": pdfDict{"/Fm0": pdfRef{Num: formID}},
	}}

	p1.pageDict()["/Resources"] = pdfRef{Num: resID}
	p2.pageDict()["/Resources"] = pdfRef{Num: resID}
	if err := p1.appendToContentStream([]byte("q 1 0 0 1 50 600 cm /Fm0 Do Q\n")); err != nil {
		t.Fatal(err)
	}
	if err := p2.appendToContentStream([]byte("q 1 0 0 1 50 600 cm /Fm0 Do Q\n")); err != nil {
		t.Fatal(err)
	}

	// Redact page 1's placement only; page 2 gets no annotation at all.
	ra := NewRedactAnnotation(p1, Rectangle{LLX: 50, LLY: 600, URX: 250, URY: 700})
	if err := p1.Annotations().Add(ra); err != nil {
		t.Fatal(err)
	}

	if err := d.ApplyRedactions(); err != nil {
		t.Fatalf("ApplyRedactions: %v", err)
	}

	var buf bytes.Buffer
	if _, err := d.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}

	rp1, err := reopened.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	txt1, err := rp1.ExtractText()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(txt1, secret) {
		t.Errorf("page 1 (redacted) still contains %q: %q", secret, txt1)
	}

	rp2, err := reopened.Page(2)
	if err != nil {
		t.Fatal(err)
	}
	txt2, err := rp2.ExtractText()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(txt2, secret) {
		t.Errorf("page 2 (NOT redacted, shares /Resources with page 1 via indirect reference) lost its text: got %q, want it to still contain %q", txt2, secret)
	}
}
