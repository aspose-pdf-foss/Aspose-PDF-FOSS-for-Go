package asposepdf_test

import (
	"testing"

	pdf "github.com/aspose/pdf-for-go"
)

func TestPointConstruction(t *testing.T) {
	p := pdf.Point{X: 10, Y: 20}
	if p.X != 10 || p.Y != 20 {
		t.Errorf("Point = %+v, want {10 20}", p)
	}
}

func TestBorderStyleConstants(t *testing.T) {
	if pdf.BorderSolid != 0 {
		t.Errorf("BorderSolid = %d, want 0", pdf.BorderSolid)
	}
	// Verify the 5 constants are distinct and ordered.
	all := []pdf.BorderStyle{
		pdf.BorderSolid,
		pdf.BorderDashed,
		pdf.BorderBeveled,
		pdf.BorderInset,
		pdf.BorderUnderline,
	}
	for i, v := range all {
		if int(v) != i {
			t.Errorf("BorderStyle[%d] = %d, want %d", i, int(v), i)
		}
	}
}

func TestLineEndingStyleConstants(t *testing.T) {
	if pdf.LineEndingNone != 0 {
		t.Errorf("LineEndingNone = %d, want 0", pdf.LineEndingNone)
	}
	all := []pdf.LineEndingStyle{
		pdf.LineEndingNone,
		pdf.LineEndingSquare,
		pdf.LineEndingCircle,
		pdf.LineEndingDiamond,
		pdf.LineEndingOpenArrow,
		pdf.LineEndingClosedArrow,
		pdf.LineEndingButt,
		pdf.LineEndingROpenArrow,
		pdf.LineEndingRClosedArrow,
		pdf.LineEndingSlash,
	}
	for i, v := range all {
		if int(v) != i {
			t.Errorf("LineEndingStyle[%d] = %d, want %d", i, int(v), i)
		}
	}
}
