// SPDX-License-Identifier: MIT

package asposepdf

import (
	"strings"
	"testing"
)

func TestRewritePathNoRegions(t *testing.T) {
	in := []byte("100 100 m\n200 100 l\n200 200 l\n100 200 l\nh\nS\n")
	out, err := rewritePathOperatorsInStream(in, nil)
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != string(in) {
		t.Errorf("expected unchanged, got %q", out)
	}
}

func TestRewritePathFullyInside(t *testing.T) {
	// Path is a rect 100..200, 100..200.
	in := []byte("100 100 m\n200 100 l\n200 200 l\n100 200 l\nh\nS\n")
	// Region covers entire path: 0..400, 0..400.
	regions := []QuadPoint{
		{X1: 0, Y1: 400, X2: 400, Y2: 400, X3: 0, Y3: 0, X4: 400, Y4: 0},
	}
	out, err := rewritePathOperatorsInStream(in, regions)
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	// Path should be fully dropped — no S, no m, no l should remain.
	if strings.Contains(s, " m") || strings.Contains(s, " l") || strings.Contains(s, "S\n") {
		t.Errorf("expected path fully dropped, got %q", s)
	}
}

func TestRewritePathFullyOutside(t *testing.T) {
	in := []byte("100 100 m\n200 100 l\n200 200 l\n100 200 l\nh\nS\n")
	// Region far away: 500..600, 500..600.
	regions := []QuadPoint{
		{X1: 500, Y1: 600, X2: 600, Y2: 600, X3: 500, Y3: 500, X4: 600, Y4: 500},
	}
	out, err := rewritePathOperatorsInStream(in, regions)
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if !strings.Contains(s, " m\n") {
		t.Errorf("expected m kept, got %q", s)
	}
	if !strings.Contains(s, "S\n") {
		t.Errorf("expected S kept, got %q", s)
	}
}

func TestRewritePathPartialOverlap(t *testing.T) {
	// Path: rect 100..300, 100..300.
	in := []byte("100 100 m\n300 100 l\n300 300 l\n100 300 l\nh\nf\n")
	// Region partial: 200..400, 200..400 covers upper-right quadrant.
	regions := []QuadPoint{
		{X1: 200, Y1: 400, X2: 400, Y2: 400, X3: 200, Y3: 200, X4: 400, Y4: 200},
	}
	out, err := rewritePathOperatorsInStream(in, regions)
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	// Path should still be present (filled), but wrapped in q/Q with W* n clip.
	if !strings.Contains(s, "f\n") {
		t.Errorf("expected fill op kept, got %q", s)
	}
	if !strings.Contains(s, "W*") {
		t.Errorf("expected W* clip, got %q", s)
	}
	if !strings.Contains(s, "q\n") {
		t.Errorf("expected q wrapper, got %q", s)
	}
	if !strings.Contains(s, "Q\n") {
		t.Errorf("expected Q wrapper, got %q", s)
	}
}

func TestRewritePathRectangleConstruction(t *testing.T) {
	// Use re operator instead of m/l.
	in := []byte("100 100 200 200 re\nS\n")
	// Region fully covers it.
	regions := []QuadPoint{
		{X1: 0, Y1: 400, X2: 400, Y2: 400, X3: 0, Y3: 0, X4: 400, Y4: 0},
	}
	out, err := rewritePathOperatorsInStream(in, regions)
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if strings.Contains(s, " re\n") || strings.Contains(s, "S\n") {
		t.Errorf("expected re path dropped, got %q", s)
	}
}

func TestRewritePathMultiplePaths(t *testing.T) {
	// Two paths: first inside redact region, second outside.
	in := []byte("100 100 m\n150 150 l\nS\n300 300 m\n350 350 l\nS\n")
	regions := []QuadPoint{
		{X1: 0, Y1: 200, X2: 200, Y2: 200, X3: 0, Y3: 0, X4: 200, Y4: 0},
	}
	out, err := rewritePathOperatorsInStream(in, regions)
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	// First path (100,100→150,150) is inside region → dropped.
	// Second path (300,300→350,350) is outside → kept.
	// So: should NOT contain "100 100 m" or "150 150 l" (first path).
	if strings.Contains(s, "150 150") {
		t.Errorf("expected first path dropped, got %q", s)
	}
	if !strings.Contains(s, "300 300") {
		t.Errorf("expected second path kept, got %q", s)
	}
}

func TestRewritePathCTMAware(t *testing.T) {
	// Path uses CTM scaling. Local path is at (10, 10) but CTM scales by 10x.
	in := []byte("q\n10 0 0 10 0 0 cm\n10 10 m\n20 20 l\nS\nQ\n")
	// Local path (10,10) → (20,20), under 10x scale becomes user-space (100,100) → (200,200).
	// Region 50..150, 50..150 — partial overlap with user-space bbox 100..200, 100..200.
	regions := []QuadPoint{
		{X1: 50, Y1: 150, X2: 150, Y2: 150, X3: 50, Y3: 50, X4: 150, Y4: 50},
	}
	out, err := rewritePathOperatorsInStream(in, regions)
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	// Partial overlap should add W* clip.
	if !strings.Contains(s, "W*") {
		t.Errorf("expected W* clip from CTM-aware partial, got %q", s)
	}
}

func TestRewritePathPassthroughText(t *testing.T) {
	// Non-path ops should pass through.
	in := []byte("BT\n/F1 12 Tf\n(Hello) Tj\nET\n")
	out, err := rewritePathOperatorsInStream(in, []QuadPoint{
		{X1: 0, Y1: 1000, X2: 1000, Y2: 1000, X3: 0, Y3: 0, X4: 1000, Y4: 0},
	})
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if !strings.Contains(s, "(Hello)") {
		t.Errorf("expected text preserved, got %q", s)
	}
}
