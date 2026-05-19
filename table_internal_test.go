package asposepdf

import "testing"

func TestMeasureText_SingleLine(t *testing.T) {
	style := TextStyle{Font: FontHelvetica, Size: 12}
	lines, lineHeight, err := measureText("Hello", style, 1000) // wide enough for one line
	if err != nil {
		t.Fatal(err)
	}
	if lines != 1 {
		t.Errorf("lines = %d, want 1", lines)
	}
	if lineHeight <= 0 {
		t.Errorf("lineHeight = %g, want > 0", lineHeight)
	}
}

func TestMeasureText_Wrap(t *testing.T) {
	style := TextStyle{Font: FontHelvetica, Size: 12}
	// 40pt is too narrow for "Hello World" (~ 60pt at 12pt Helvetica) — should wrap.
	lines, _, err := measureText("Hello World", style, 40)
	if err != nil {
		t.Fatal(err)
	}
	if lines < 2 {
		t.Errorf("expected wrap, got lines = %d", lines)
	}
}

func TestMeasureText_Empty(t *testing.T) {
	lines, _, err := measureText("", TextStyle{}, 100)
	if err != nil {
		t.Fatal(err)
	}
	if lines != 0 {
		t.Errorf("lines = %d, want 0 for empty text", lines)
	}
}

func TestMeasureText_DefaultsApplied(t *testing.T) {
	// Font nil → Helvetica; Size 0 → 12; LineSpacing 0 → 1.2.
	lines, lh, err := measureText("Hello", TextStyle{}, 1000)
	if err != nil {
		t.Fatal(err)
	}
	if lines != 1 {
		t.Errorf("lines = %d, want 1", lines)
	}
	// With Size=12, LineSpacing=1.2 → lineHeight = 14.4. Compute via
	// runtime float64 multiplication to match measureText's rounding (the
	// constant expression 12.0*1.2 would be folded at arbitrary precision
	// at compile time and round differently).
	size, spacing := 12.0, 1.2
	want := size * spacing
	if lh != want {
		t.Errorf("lineHeight = %g, want %g", lh, want)
	}
}
