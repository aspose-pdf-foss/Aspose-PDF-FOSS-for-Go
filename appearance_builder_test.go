package asposepdf

import "testing"

func TestBuilderPushPopState(t *testing.T) {
	b := newAppearanceBuilder()
	b.PushState()
	b.PopState()
	if got := string(b.Bytes()); got != "q\nQ\n" {
		t.Errorf("got %q, want \"q\\nQ\\n\"", got)
	}
}

func TestBuilderConcatMatrix(t *testing.T) {
	b := newAppearanceBuilder()
	b.ConcatMatrix(1, 0, 0, 1, 10, 20)
	if got := string(b.Bytes()); got != "1 0 0 1 10 20 cm\n" {
		t.Errorf("got %q", got)
	}
}

func TestBuilderSetLineWidth(t *testing.T) {
	b := newAppearanceBuilder()
	b.SetLineWidth(2.5)
	if got := string(b.Bytes()); got != "2.5 w\n" {
		t.Errorf("got %q", got)
	}
}

func TestBuilderSetLineCap(t *testing.T) {
	b := newAppearanceBuilder()
	b.SetLineCap(LineCapRound)
	if got := string(b.Bytes()); got != "1 J\n" {
		t.Errorf("got %q", got)
	}
}

func TestBuilderSetLineJoin(t *testing.T) {
	b := newAppearanceBuilder()
	b.SetLineJoin(LineJoinBevel)
	if got := string(b.Bytes()); got != "2 j\n" {
		t.Errorf("got %q", got)
	}
}

func TestBuilderSetMiterLimit(t *testing.T) {
	b := newAppearanceBuilder()
	b.SetMiterLimit(10)
	if got := string(b.Bytes()); got != "10 M\n" {
		t.Errorf("got %q", got)
	}
}

func TestBuilderSetDashPattern(t *testing.T) {
	b := newAppearanceBuilder()
	b.SetDashPattern([]float64{3, 3}, 0)
	if got := string(b.Bytes()); got != "[3 3] 0 d\n" {
		t.Errorf("got %q", got)
	}
}

func TestBuilderSetDashPatternEmpty(t *testing.T) {
	b := newAppearanceBuilder()
	b.SetDashPattern(nil, 0)
	if got := string(b.Bytes()); got != "[] 0 d\n" {
		t.Errorf("got %q", got)
	}
}
