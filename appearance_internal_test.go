package asposepdf

import (
	"strings"
	"testing"
)

func TestBeveledColorPair(t *testing.T) {
	base := Color{R: 0.5, G: 0.5, B: 0.5, A: 1}
	light, dark := beveledColorPair(base, false)
	// Light = 50% blend with white → all channels 0.75
	if light.R != 0.75 || light.G != 0.75 || light.B != 0.75 {
		t.Errorf("light = %+v, want {0.75 0.75 0.75 1}", light)
	}
	// Dark = base * 0.5 → all channels 0.25
	if dark.R != 0.25 || dark.G != 0.25 || dark.B != 0.25 {
		t.Errorf("dark = %+v, want {0.25 0.25 0.25 1}", dark)
	}
}

func TestBeveledColorPairInverted(t *testing.T) {
	// Inverted = Inset style — light/dark swapped.
	base := Color{R: 0.5, G: 0.5, B: 0.5, A: 1}
	light, dark := beveledColorPair(base, true)
	if light.R != 0.25 {
		t.Errorf("inverted light.R = %v, want 0.25 (Inset swaps)", light.R)
	}
	if dark.R != 0.75 {
		t.Errorf("inverted dark.R = %v, want 0.75 (Inset swaps)", dark.R)
	}
}

// drawLineEnding emits content-stream operators. Each style produces a
// distinguishable shape; verify presence of expected operators.

func TestDrawLineEndingNone(t *testing.T) {
	b := newAppearanceBuilder()
	drawLineEnding(b, LineEndingNone, 0, 0, 0, 1, nil)
	if got := string(b.Bytes()); got != "" {
		t.Errorf("None should emit nothing, got %q", got)
	}
}

func TestDrawLineEndingShapesEmitGeometry(t *testing.T) {
	cases := []struct {
		style LineEndingStyle
		// Each style must emit at least one path-construction op (m / l / c / re).
		minPathOps int
	}{
		{LineEndingSquare, 1},       // 1 re op (rectangle operator)
		{LineEndingCircle, 4},       // 4 c ops (Ellipse)
		{LineEndingDiamond, 3},      // 3 l ops (m + 3 l + h)
		{LineEndingOpenArrow, 2},    // 2 l ops
		{LineEndingClosedArrow, 2},  // 2 l ops (m + 2 l + h)
		{LineEndingButt, 1},         // 1 l op
		{LineEndingROpenArrow, 2},   // 2 l ops
		{LineEndingRClosedArrow, 2}, // 2 l ops (m + 2 l + h)
		{LineEndingSlash, 1},        // 1 l op
	}
	for _, tc := range cases {
		t.Run(string(rune('A'+int(tc.style))), func(t *testing.T) {
			b := newAppearanceBuilder()
			drawLineEnding(b, tc.style, 50, 50, 0, 1, nil)
			out := string(b.Bytes())
			pathOps := strings.Count(out, " l\n") + strings.Count(out, " c\n") + strings.Count(out, " re\n")
			if pathOps < tc.minPathOps {
				t.Errorf("style %v: %d path ops, want >= %d. Output: %q", tc.style, pathOps, tc.minPathOps, out)
			}
		})
	}
}

func TestDrawLineEndingClosedArrowFills(t *testing.T) {
	b := newAppearanceBuilder()
	drawLineEnding(b, LineEndingClosedArrow, 50, 50, 0, 1, &Color{R: 1, G: 0, B: 0, A: 1})
	out := string(b.Bytes())
	if !strings.Contains(out, "B\n") && !strings.Contains(out, "b\n") {
		t.Errorf("ClosedArrow should fill+stroke (B or b), got %q", out)
	}
}
