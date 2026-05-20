package asposepdf

import "testing"

func TestPath_NewIsEmpty(t *testing.T) {
	p := NewPath()
	if p == nil {
		t.Fatal("NewPath returned nil")
	}
	if len(p.ops) != 0 {
		t.Errorf("ops = %d, want 0", len(p.ops))
	}
}

func TestPath_MoveToLineToClose(t *testing.T) {
	p := NewPath().MoveTo(10, 20).LineTo(30, 40).Close()
	if len(p.ops) != 3 {
		t.Fatalf("ops = %d, want 3", len(p.ops))
	}
	if p.ops[0].kind != pathOpMoveTo || p.ops[0].x != 10 || p.ops[0].y != 20 {
		t.Errorf("op[0] = %+v", p.ops[0])
	}
	if p.ops[1].kind != pathOpLineTo || p.ops[1].x != 30 || p.ops[1].y != 40 {
		t.Errorf("op[1] = %+v", p.ops[1])
	}
	if p.ops[2].kind != pathOpClose {
		t.Errorf("op[2].kind = %v, want pathOpClose", p.ops[2].kind)
	}
}

func TestPath_Chaining(t *testing.T) {
	p := NewPath().MoveTo(0, 0).LineTo(1, 1).LineTo(2, 0).LineTo(1, -1).Close()
	if len(p.ops) != 5 {
		t.Errorf("len = %d, want 5", len(p.ops))
	}
}

func TestPath_CurveTo(t *testing.T) {
	p := NewPath().MoveTo(0, 0).CurveTo(10, 0, 20, 10, 30, 30)
	if len(p.ops) != 2 {
		t.Fatalf("ops = %d, want 2", len(p.ops))
	}
	op := p.ops[1]
	if op.kind != pathOpCurveTo {
		t.Errorf("kind = %v, want CurveTo", op.kind)
	}
	if op.c1x != 10 || op.c1y != 0 || op.c2x != 20 || op.c2y != 10 || op.x != 30 || op.y != 30 {
		t.Errorf("control points = %+v", op)
	}
}

func TestPath_QuadToConvertsToCubic(t *testing.T) {
	// Quadratic with current point (P0=0,0), control (Q=10,10), endpoint (P3=20,0).
	// Equivalent cubic control points:
	//   C1 = P0 + (2/3)(Q - P0) = (20/3, 20/3) ≈ (6.667, 6.667)
	//   C2 = P3 + (2/3)(Q - P3) = (40/3, 20/3) ≈ (13.333, 6.667)
	p := NewPath().MoveTo(0, 0).QuadTo(10, 10, 20, 0)
	if len(p.ops) != 2 {
		t.Fatalf("ops = %d, want 2", len(p.ops))
	}
	op := p.ops[1]
	if op.kind != pathOpCurveTo {
		t.Fatalf("kind = %v, want CurveTo (auto-converted)", op.kind)
	}
	const eps = 1e-9
	if absFloat(op.c1x-20.0/3) > eps || absFloat(op.c1y-20.0/3) > eps {
		t.Errorf("c1 = (%g, %g), want (20/3, 20/3)", op.c1x, op.c1y)
	}
	if absFloat(op.c2x-40.0/3) > eps || absFloat(op.c2y-20.0/3) > eps {
		t.Errorf("c2 = (%g, %g), want (40/3, 20/3)", op.c2x, op.c2y)
	}
	if op.x != 20 || op.y != 0 {
		t.Errorf("endpoint = (%g, %g), want (20, 0)", op.x, op.y)
	}
}

// absFloat is a tiny float64 absolute-value helper for tests.
// Named to avoid collision with builtin abs in future Go versions.
func absFloat(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
}

func TestPath_QuadToNoCurrentPoint_AssumesOrigin(t *testing.T) {
	// PDF spec says paths with no MoveTo start at (0,0). QuadTo should treat
	// the missing current point as (0,0) for control-point conversion.
	p := NewPath().QuadTo(10, 10, 20, 0)
	if len(p.ops) != 1 {
		t.Fatalf("ops = %d, want 1", len(p.ops))
	}
	op := p.ops[0]
	if op.kind != pathOpCurveTo {
		t.Errorf("kind = %v", op.kind)
	}
}
