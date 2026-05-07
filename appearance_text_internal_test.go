package asposepdf

import (
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
