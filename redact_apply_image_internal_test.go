// SPDX-License-Identifier: MIT

package asposepdf

import (
	"strings"
	"testing"
)

func TestRewriteImageNoRegions(t *testing.T) {
	in := []byte("q\n100 0 0 50 200 600 cm\n/Img1 Do\nQ\n")
	out, err := rewriteImageOperatorsInStream(in, nil)
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
	out, err := rewriteImageOperatorsInStream(in, regions)
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
	out, err := rewriteImageOperatorsInStream(in, regions)
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
	out, err := rewriteImageOperatorsInStream(in, regions)
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
	out, err := rewriteImageOperatorsInStream(in, regions)
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
	out, err := rewriteImageOperatorsInStream(in, regions)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "/Img1 Do") {
		t.Errorf("expected Do preserved (image far from region), got %q", out)
	}
}

func TestRewriteImagePassthroughNonImageOps(t *testing.T) {
	// Non-Do operators (text, path) should be passed through.
	in := []byte("BT\n/F1 12 Tf\n(Hello) Tj\nET\n100 100 m 200 200 l S\n")
	out, err := rewriteImageOperatorsInStream(in, []QuadPoint{
		{X1: 0, Y1: 1000, X2: 1000, Y2: 1000, X3: 0, Y3: 0, X4: 1000, Y4: 0},
	})
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
