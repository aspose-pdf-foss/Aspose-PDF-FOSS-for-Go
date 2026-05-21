// SPDX-License-Identifier: MIT

package asposepdf

import (
	"regexp"
	"strconv"
	"strings"
	"testing"
)

func TestDrawRoundedRect(t *testing.T) {
	b := newAppearanceBuilder()
	drawRoundedRect(b, 0, 0, 100, 50, 5)
	out := string(b.Bytes())
	// Should contain: 1 m + 4 c (corner arcs) + 4 l (sides) + 1 h.
	if strings.Count(out, " m\n") != 1 {
		t.Errorf("expected 1 m op, got %d in %q", strings.Count(out, " m\n"), out)
	}
	if strings.Count(out, " c\n") != 4 {
		t.Errorf("expected 4 c ops, got %d in %q", strings.Count(out, " c\n"), out)
	}
	if strings.Count(out, " l\n") != 4 {
		t.Errorf("expected 4 l ops, got %d in %q", strings.Count(out, " l\n"), out)
	}
	if !strings.HasSuffix(out, "h\n") {
		t.Errorf("expected h close, got %q", out)
	}
}

func TestDrawRoundedRectClampsRadius(t *testing.T) {
	// Radius larger than half-dimension should clamp.
	b := newAppearanceBuilder()
	drawRoundedRect(b, 0, 0, 10, 10, 100)
	out := string(b.Bytes())
	if strings.Count(out, " c\n") != 4 {
		t.Errorf("expected 4 c ops even with clamped radius, got %d", strings.Count(out, " c\n"))
	}
}

func TestStampVisualParamsAllNames(t *testing.T) {
	cases := []struct {
		name  StampName
		label string
	}{
		{StampNameApproved, "APPROVED"},
		{StampNameAsIs, "AS IS"},
		{StampNameConfidential, "CONFIDENTIAL"},
		{StampNameDepartmental, "DEPARTMENTAL"},
		{StampNameDraft, "DRAFT"},
		{StampNameExperimental, "EXPERIMENTAL"},
		{StampNameExpired, "EXPIRED"},
		{StampNameFinal, "FINAL"},
		{StampNameForComment, "FOR COMMENT"},
		{StampNameForPublicRelease, "FOR PUBLIC RELEASE"},
		{StampNameNotApproved, "NOT APPROVED"},
		{StampNameNotForPublicRelease, "NOT FOR PUBLIC RELEASE"},
		{StampNameSold, "SOLD"},
		{StampNameTopSecret, "TOP SECRET"},
	}
	for _, tc := range cases {
		primary, fill, label := stampVisualParams(tc.name)
		if label != tc.label {
			t.Errorf("name=%v: label=%q, want %q", tc.name, label, tc.label)
		}
		// Sanity-check colors are non-zero (some channel must be > 0).
		if primary.R == 0 && primary.G == 0 && primary.B == 0 {
			t.Errorf("name=%v: primary all zero", tc.name)
		}
		if fill.R == 0 && fill.G == 0 && fill.B == 0 {
			t.Errorf("name=%v: fill all zero", tc.name)
		}
	}
}

func TestStampVisualParamsUnknownDefaults(t *testing.T) {
	primary, fill, label := stampVisualParams(StampNameUnknown)
	if label != "" {
		t.Errorf("Unknown label = %q, want empty", label)
	}
	// Default = Draft (orange).
	if primary.R == 0 && primary.G == 0 && primary.B == 0 {
		t.Errorf("Unknown primary all zero")
	}
	_ = fill
}

func TestDrawCalloutLine2Point(t *testing.T) {
	b := newAppearanceBuilder()
	start := Point{X: 50, Y: 50}
	pts := []Point{
		{X: 100, Y: 60}, // knee
		{X: 200, Y: 80}, // endpoint
	}
	drawCalloutLine(b, start, pts, 1.0, &Color{R: 0, G: 0, B: 0, A: 1}, LineEndingNone)
	out := string(b.Bytes())
	// Expect: m + 2 l + S (start → knee → endpoint).
	if strings.Count(out, " m\n") < 1 {
		t.Errorf("expected 1+ m ops, got %d in %q", strings.Count(out, " m\n"), out)
	}
	if strings.Count(out, " l\n") < 2 {
		t.Errorf("expected 2+ l ops, got %d in %q", strings.Count(out, " l\n"), out)
	}
	if strings.Count(out, "S\n") < 1 {
		t.Errorf("expected 1+ S op (stroke), got %d in %q", strings.Count(out, "S\n"), out)
	}
}

func TestDrawCalloutLine3Point(t *testing.T) {
	b := newAppearanceBuilder()
	start := Point{X: 50, Y: 50}
	pts := []Point{
		{X: 100, Y: 60},
		{X: 150, Y: 80},
		{X: 200, Y: 90},
	}
	drawCalloutLine(b, start, pts, 1.0, nil, LineEndingNone)
	out := string(b.Bytes())
	// Expect: m + 3 l (start → knee1 → knee2 → endpoint).
	if strings.Count(out, " l\n") < 3 {
		t.Errorf("expected 3+ l ops, got %d in %q", strings.Count(out, " l\n"), out)
	}
}

func TestDrawCalloutLineWithEnding(t *testing.T) {
	b := newAppearanceBuilder()
	start := Point{X: 0, Y: 0}
	pts := []Point{
		{X: 50, Y: 0},
		{X: 100, Y: 0},
	}
	// Provide a stroke color so that paintShape inside drawLineEnding
	// receives a non-nil fill and emits B (fill+stroke) for ClosedArrow.
	drawCalloutLine(b, start, pts, 1.0, &Color{R: 0, G: 0, B: 0, A: 1}, LineEndingClosedArrow)
	out := string(b.Bytes())
	// ClosedArrow drawn via paintShape → fills with B (or b).
	if !strings.Contains(out, "B\n") && !strings.Contains(out, "b\n") {
		t.Errorf("ClosedArrow should fill+stroke (B or b) at endpoint; output: %q", out)
	}
}

func TestDrawCalloutLineSkipsEmpty(t *testing.T) {
	b := newAppearanceBuilder()
	drawCalloutLine(b, Point{}, nil, 1.0, nil, LineEndingNone)
	if len(b.Bytes()) != 0 {
		t.Errorf("empty pts should emit nothing, got %q", string(b.Bytes()))
	}
	drawCalloutLine(b, Point{}, []Point{{X: 1, Y: 1}}, 1.0, nil, LineEndingNone)
	if len(b.Bytes()) != 0 {
		t.Errorf("single-point pts should emit nothing, got %q", string(b.Bytes()))
	}
}

func TestDrawCloudyRectBorderProducesCurves(t *testing.T) {
	b := newAppearanceBuilder()
	drawCloudyRectBorder(b, 100, 50, 1.0, &Color{R: 0, G: 0, B: 0, A: 1}, 1.0)
	out := string(b.Bytes())
	cCount := strings.Count(out, " c\n")
	if cCount < 8 {
		// Expect lots of cubics (4 sides × ~N bulges × 2 cubics each).
		// For 100×50 with intensity 1.0 and lineWidth 1.0:
		// bulge step ≈ 10pt, so ~10 bulges on long side (100pt), ~5 on short (50pt).
		// Total ≈ (10+5+10+5)×2 = 60 cubics. Use a conservative lower bound.
		t.Errorf("expected lots of c ops for cloudy border, got %d in %q", cCount, out)
	}
	// Should also have a stroke at the end.
	if !strings.Contains(out, "S\n") {
		t.Errorf("expected stroke (S) op; got %q", out)
	}
}

func TestDrawCloudyRectBorderHigherIntensity(t *testing.T) {
	// Higher intensity = larger bulges = fewer segments per side = fewer curves.
	b1 := newAppearanceBuilder()
	drawCloudyRectBorder(b1, 100, 50, 1.0, nil, 1.0)
	c1 := strings.Count(string(b1.Bytes()), " c\n")

	b2 := newAppearanceBuilder()
	drawCloudyRectBorder(b2, 100, 50, 1.0, nil, 2.0)
	c2 := strings.Count(string(b2.Bytes()), " c\n")

	if c2 >= c1 {
		t.Errorf("intensity 2.0 should produce fewer cubics than intensity 1.0; got %d vs %d", c2, c1)
	}
}

// TestFreeTextAPVAlignDiffersByPosition verifies that the text baseline Y
// coordinate inside /AP/N differs correctly for VAlignTop, VAlignMiddle,
// and VAlignBottom on an identical FreeTextAnnotation rect.
//
// VAlign is a rendering hint (not round-tripped through the PDF dict), so
// this test can only verify in-memory /AP/N stream bytes — exactly what the
// renderer produces for each VAlign value.
func TestFreeTextAPVAlignDiffersByPosition(t *testing.T) {
	// extractFirstTdY parses the first absolute Td Y coordinate from raw AP/N bytes.
	// renderTextInBuilder emits "X Y Td\n" for the first line; subsequent lines
	// are relative. We want the first Td only.
	extractFirstTdY := func(data []byte) float64 {
		re := regexp.MustCompile(`(-?\d+(?:\.\d+)?)\s+(-?\d+(?:\.\d+)?)\s+Td`)
		m := re.FindStringSubmatch(string(data))
		if m == nil {
			return -1
		}
		y, err := strconv.ParseFloat(m[2], 64)
		if err != nil {
			return -1
		}
		return y
	}

	// renderAP builds a FreeTextAnnotation with the given VAlign and returns the
	// /AP/N stream bytes (already decoded — stored as-is in pdfStream.Data).
	renderAP := func(va VAlign) []byte {
		doc := NewDocument(595, 842)
		page, err := doc.Page(1)
		if err != nil {
			t.Fatalf("Page(1): %v", err)
		}
		rect := Rectangle{LLX: 0, LLY: 0, URX: 200, URY: 100}
		ft := NewFreeTextAnnotation(page, rect, "Hello", TextStyle{
			Font:   FontHelvetica,
			Size:   12,
			VAlign: va,
		})
		// Access /AP/N via the dict field (internal package access).
		apDict, _ := ft.dict["/AP"].(pdfDict)
		if apDict == nil {
			t.Fatalf("VAlign=%v: /AP absent", va)
		}
		ref, ok := apDict["/N"].(pdfRef)
		if !ok {
			t.Fatalf("VAlign=%v: /AP/N is not a pdfRef", va)
		}
		obj, exists := doc.objects[ref.Num]
		if !exists {
			t.Fatalf("VAlign=%v: /AP/N object %d not found in doc.objects", va, ref.Num)
		}
		stream, ok := obj.Value.(*pdfStream)
		if !ok {
			t.Fatalf("VAlign=%v: /AP/N object is %T, want *pdfStream", va, obj.Value)
		}
		return stream.Data
	}

	topData := renderAP(VAlignTop)
	midData := renderAP(VAlignMiddle)
	botData := renderAP(VAlignBottom)

	yTop := extractFirstTdY(topData)
	yMiddle := extractFirstTdY(midData)
	yBottom := extractFirstTdY(botData)

	if yTop < 0 || yMiddle < 0 || yBottom < 0 {
		t.Logf("top AP:\n%s", topData)
		t.Logf("middle AP:\n%s", midData)
		t.Logf("bottom AP:\n%s", botData)
		t.Fatalf("failed to extract Y coordinates: top=%v middle=%v bottom=%v", yTop, yMiddle, yBottom)
	}

	// The text baseline Y must decrease from Top to Middle to Bottom.
	// Top: startY = rect.URY (100) → y near 100-ascent.
	// Middle: startY = URY - (height-totalTextHeight)/2 → y in the middle.
	// Bottom: startY = LLY + totalTextHeight → y near ascent-adjusted bottom.
	if !(yTop > yMiddle && yMiddle > yBottom) {
		t.Errorf("expected yTop > yMiddle > yBottom, got top=%.4f middle=%.4f bottom=%.4f",
			yTop, yMiddle, yBottom)
	}

}
