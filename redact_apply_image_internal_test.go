// SPDX-License-Identifier: MIT

package asposepdf

import (
	"strings"
	"testing"
)

func TestRewriteImageNoRegions(t *testing.T) {
	in := []byte("q\n100 0 0 50 200 600 cm\n/Img1 Do\nQ\n")
	out, err := rewriteImageOperatorsInStream(in, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != string(in) {
		t.Errorf("expected unchanged output, got %q", out)
	}
}

func TestRewriteImageFullyOutside(t *testing.T) {
	// Image at (200..300, 600..650). Region at (0..100, 0..100). Disjoint.
	in := []byte("q\n100 0 0 50 200 600 cm\n/Img1 Do\nQ\n")
	regions := []QuadPoint{
		{X1: 0, Y1: 100, X2: 100, Y2: 100, X3: 0, Y3: 0, X4: 100, Y4: 0},
	}
	out, err := rewriteImageOperatorsInStream(in, regions, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "/Img1 Do") {
		t.Errorf("expected Do preserved, got %q", out)
	}
}

func TestRewriteImageFullyInside(t *testing.T) {
	// Image at (200..300, 600..650). Region covers (0..600, 500..700) — fully contains the image.
	in := []byte("q\n100 0 0 50 200 600 cm\n/Img1 Do\nQ\n")
	regions := []QuadPoint{
		{X1: 0, Y1: 700, X2: 600, Y2: 700, X3: 0, Y3: 500, X4: 600, Y4: 500},
	}
	out, err := rewriteImageOperatorsInStream(in, regions, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), "/Img1 Do") {
		t.Errorf("expected Do dropped, got %q", out)
	}
}

func TestRewriteImagePartialOverlap(t *testing.T) {
	// Image at (200..300, 600..650). Region partially covers it: (250..400, 620..700).
	in := []byte("q\n100 0 0 50 200 600 cm\n/Img1 Do\nQ\n")
	regions := []QuadPoint{
		{X1: 250, Y1: 700, X2: 400, Y2: 700, X3: 250, Y3: 620, X4: 400, Y4: 620},
	}
	out, err := rewriteImageOperatorsInStream(in, regions, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	// Should still contain Do (clipped, not removed).
	if !strings.Contains(s, "/Img1 Do") {
		t.Errorf("expected Do present with clip, got %q", s)
	}
	// Should contain W* clip operator.
	if !strings.Contains(s, "W*") {
		t.Errorf("expected even-odd clip W* operator, got %q", s)
	}
}

func TestRewriteImageMultipleDoOnPage(t *testing.T) {
	// Two images. First is dropped (fully inside redact), second is kept.
	in := []byte("q\n100 0 0 50 200 600 cm\n/Img1 Do\nQ\nq\n100 0 0 50 200 100 cm\n/Img2 Do\nQ\n")
	regions := []QuadPoint{
		{X1: 0, Y1: 700, X2: 600, Y2: 700, X3: 0, Y3: 500, X4: 600, Y4: 500},
	}
	out, err := rewriteImageOperatorsInStream(in, regions, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if strings.Contains(s, "/Img1 Do") {
		t.Errorf("expected Img1 dropped, got %q", s)
	}
	if !strings.Contains(s, "/Img2 Do") {
		t.Errorf("expected Img2 kept, got %q", s)
	}
}

func TestRewriteImageCTMNesting(t *testing.T) {
	// Two cm ops compose: outer 2x scale, inner 0.5x scale + translate.
	// Net CTM for Do: 2*0.5 = 1.0 scale, position depends on details.
	// Simpler test: ensure q/Q doesn't pollute outer CTM.
	in := []byte("q\n2 0 0 2 0 0 cm\nq\n1 0 0 1 100 200 cm\n/Img1 Do\nQ\nQ\n")
	// The image final CTM = 2*[1 0 0 1 100 200] = [2 0 0 2 200 400].
	// Image bbox = (200, 400) to (202, 402)? Wait: the unit square (0,0)-(1,1) under matrix [2 0 0 2 200 400] maps to:
	// (0,0) → (200, 400); (1,0) → (202, 400); (0,1) → (200, 402); (1,1) → (202, 402).
	// So bbox = (200..202, 400..402).
	// Region far away: keep.
	regions := []QuadPoint{
		{X1: 0, Y1: 100, X2: 50, Y2: 100, X3: 0, Y3: 0, X4: 50, Y4: 0},
	}
	out, err := rewriteImageOperatorsInStream(in, regions, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "/Img1 Do") {
		t.Errorf("expected Do preserved (image far from region), got %q", out)
	}
}

// TestUnsafeFormXObjectNameTreatsMissingBBoxAsUnsafe is the end-to-end
// counterpart of TestFormPlacementBboxMissingBBoxNotOK: a Form XObject
// with no /BBox must be flagged unsafe by unsafeFormXObjectName
// regardless of whether the redact region geometrically overlaps
// anything -- there is nothing reliable to compare it against.
func TestUnsafeFormXObjectNameTreatsMissingBBoxAsUnsafe(t *testing.T) {
	formNum := 1
	objects := map[int]*pdfObject{
		formNum: {Num: formNum, Value: &pdfStream{Dict: pdfDict{"/Subtype": pdfName("/Form")}}},
	}
	resources := pdfDict{"/XObject": pdfDict{"/Fm0": pdfRef{Num: formNum}}}
	ops := []contentOp{{Operator: "Do", Operands: []pdfValue{pdfName("/Fm0")}}}
	// A region nowhere near the page origin the Do is drawn at -- if the
	// (fixed) code still geometrically compared bboxes here, this region
	// would not intersect anything and the form would wrongly pass as safe.
	regions := []QuadPoint{{X1: 9000, Y1: 9100, X2: 9100, Y2: 9100, X3: 9000, Y3: 9000, X4: 9100, Y4: 9000}}

	name, found := unsafeFormXObjectName(ops, regions, objects, resources)
	if !found {
		t.Error("unsafeFormXObjectName: found = false for a Form XObject with no /BBox, want true")
	}
	if name != "/Fm0" {
		t.Errorf("unsafeFormXObjectName: name = %q, want /Fm0", name)
	}
}

func TestRewriteImagePassthroughNonImageOps(t *testing.T) {
	// Non-Do operators (text, path) should be passed through.
	in := []byte("BT\n/F1 12 Tf\n(Hello) Tj\nET\n100 100 m 200 200 l S\n")
	out, err := rewriteImageOperatorsInStream(in, []QuadPoint{
		{X1: 0, Y1: 1000, X2: 1000, Y2: 1000, X3: 0, Y3: 0, X4: 1000, Y4: 0},
	}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "Hello") {
		t.Errorf("expected text passed through, got %q", out)
	}
	if !strings.Contains(string(out), "S\n") {
		t.Errorf("expected Stroke passed through, got %q", out)
	}
}

// TestFormPlacementBboxNonIdentityMatrix pins a gap a review of
// formPlacementBbox's fix flagged: every existing test exercises a Form
// XObject with an absent (identity) /Matrix, relying only on AddForm's own
// placement cm for scaling -- never the /Matrix-aware branch itself. A
// 90-degree rotation /Matrix composed with a non-trivial CTM gives a bbox
// nothing resembling either operand's own footprint if the composition
// order or corner mapping were wrong, so this is a real check, not a
// tautology: expected values were independently cross-checked (by the
// reviewer) against two sequential matApplyPoint calls (apply /Matrix,
// then apply ctm, as two separate steps) rather than derived from this
// function's own code.
func TestFormPlacementBboxNonIdentityMatrix(t *testing.T) {
	st := &pdfStream{Dict: pdfDict{
		"/BBox":   pdfArray{0.0, 0.0, 100.0, 50.0},
		"/Matrix": pdfArray{0.0, 1.0, -1.0, 0.0, 0.0, 0.0}, // 90-degree rotation
	}}
	ctm := [6]float64{2, 0, 0, 1, 200, 300}
	minX, minY, maxX, maxY, ok := formPlacementBbox(nil, st, ctm)
	if !ok {
		t.Fatal("formPlacementBbox: ok = false, want true")
	}
	wantMinX, wantMinY, wantMaxX, wantMaxY := 100.0, 300.0, 200.0, 400.0
	if minX != wantMinX || minY != wantMinY || maxX != wantMaxX || maxY != wantMaxY {
		t.Errorf("bbox = (%v,%v)-(%v,%v), want (%v,%v)-(%v,%v)",
			minX, minY, maxX, maxY, wantMinX, wantMinY, wantMaxX, wantMaxY)
	}
}

// TestFormPlacementBboxMissingBBoxNotOK pins the fix for a medium-severity
// finding from an independent review: /BBox is required on every
// conformant Form XObject (ISO 32000-1 8.10.2), so its absence means
// non-conformant or adversarially-malformed input -- exactly the input a
// redaction-safety check should trust least. formPlacementBbox must report
// ok=false rather than silently guessing the unit square (the earlier,
// now-fixed false-negative risk), so the caller can treat it as unsafe
// outright.
func TestFormPlacementBboxMissingBBoxNotOK(t *testing.T) {
	st := &pdfStream{Dict: pdfDict{}}
	if _, _, _, _, ok := formPlacementBbox(nil, st, identityMatrix()); ok {
		t.Error("formPlacementBbox: ok = true for a Form XObject with no /BBox, want false")
	}
}
