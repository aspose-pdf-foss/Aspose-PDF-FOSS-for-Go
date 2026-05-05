package asposepdf

import (
	"testing"
)

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
	for _, tc := range []struct {
		cap  LineCap
		want string
	}{
		{LineCapButt, "0 J\n"},
		{LineCapRound, "1 J\n"},
		{LineCapSquare, "2 J\n"},
	} {
		b := newAppearanceBuilder()
		b.SetLineCap(tc.cap)
		if got := string(b.Bytes()); got != tc.want {
			t.Errorf("cap=%v: got %q, want %q", tc.cap, got, tc.want)
		}
	}
}

func TestBuilderSetLineJoin(t *testing.T) {
	for _, tc := range []struct {
		join LineJoin
		want string
	}{
		{LineJoinMiter, "0 j\n"},
		{LineJoinRound, "1 j\n"},
		{LineJoinBevel, "2 j\n"},
	} {
		b := newAppearanceBuilder()
		b.SetLineJoin(tc.join)
		if got := string(b.Bytes()); got != tc.want {
			t.Errorf("join=%v: got %q, want %q", tc.join, got, tc.want)
		}
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

func TestBuilderSetStrokeColorRGB(t *testing.T) {
	b := newAppearanceBuilder()
	b.SetStrokeColorRGB(Color{R: 1, G: 0.5, B: 0})
	if got := string(b.Bytes()); got != "1 0.5 0 RG\n" {
		t.Errorf("got %q", got)
	}
}

func TestBuilderSetFillColorRGB(t *testing.T) {
	b := newAppearanceBuilder()
	b.SetFillColorRGB(Color{R: 0, G: 1, B: 1})
	if got := string(b.Bytes()); got != "0 1 1 rg\n" {
		t.Errorf("got %q", got)
	}
}

func TestBuilderSetStrokeGray(t *testing.T) {
	b := newAppearanceBuilder()
	b.SetStrokeGray(0.25)
	if got := string(b.Bytes()); got != "0.25 G\n" {
		t.Errorf("got %q", got)
	}
}

func TestBuilderSetFillGray(t *testing.T) {
	b := newAppearanceBuilder()
	b.SetFillGray(0.75)
	if got := string(b.Bytes()); got != "0.75 g\n" {
		t.Errorf("got %q", got)
	}
}

func TestBuilderMoveTo(t *testing.T) {
	b := newAppearanceBuilder()
	b.MoveTo(10, 20)
	if got := string(b.Bytes()); got != "10 20 m\n" {
		t.Errorf("got %q", got)
	}
}

func TestBuilderLineTo(t *testing.T) {
	b := newAppearanceBuilder()
	b.LineTo(30, 40)
	if got := string(b.Bytes()); got != "30 40 l\n" {
		t.Errorf("got %q", got)
	}
}

func TestBuilderCurveTo(t *testing.T) {
	b := newAppearanceBuilder()
	b.CurveTo(1, 2, 3, 4, 5, 6)
	if got := string(b.Bytes()); got != "1 2 3 4 5 6 c\n" {
		t.Errorf("got %q", got)
	}
}

func TestBuilderRect(t *testing.T) {
	b := newAppearanceBuilder()
	b.Rect(0, 0, 100, 50)
	if got := string(b.Bytes()); got != "0 0 100 50 re\n" {
		t.Errorf("got %q", got)
	}
}

func TestBuilderClosePath(t *testing.T) {
	b := newAppearanceBuilder()
	b.ClosePath()
	if got := string(b.Bytes()); got != "h\n" {
		t.Errorf("got %q", got)
	}
}

func TestBuilderEllipse(t *testing.T) {
	// Exact golden bytes for a unit circle of radius 25 centered at (50, 50).
	// The control points use kappa ≈ 0.5522847498... → 25 × kappa ≈ 13.807119
	// (rounds to 6 decimals); the diagonally-opposite ordinate is 50 - 13.807119 = 36.192881.
	b := newAppearanceBuilder()
	b.Ellipse(50, 50, 25, 25)
	want := "75 50 m\n" +
		"75 63.807119 63.807119 75 50 75 c\n" +
		"36.192881 75 25 63.807119 25 50 c\n" +
		"25 36.192881 36.192881 25 50 25 c\n" +
		"63.807119 25 75 36.192881 75 50 c\n" +
		"h\n"
	if got := string(b.Bytes()); got != want {
		t.Errorf("Ellipse(50,50,25,25):\n got: %q\nwant: %q", got, want)
	}
}
