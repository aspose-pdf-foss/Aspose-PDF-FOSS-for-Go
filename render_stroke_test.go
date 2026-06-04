// SPDX-License-Identifier: MIT

package asposepdf

import (
	"image"
	"testing"
)

func renderContent(t *testing.T, content string) image.Image {
	t.Helper()
	doc := NewDocument(100, 100)
	p, _ := doc.Page(1)
	if err := p.appendToContentStream([]byte(content)); err != nil {
		t.Fatal(err)
	}
	img, err := p.RenderImage(RenderOptions{DPI: 72}) // 1 px per point
	if err != nil {
		t.Fatal(err)
	}
	return img
}

func isBlack(img image.Image, x, y int) bool {
	r, g, b, _ := img.At(x, y).RGBA()
	return r>>8 < 80 && g>>8 < 80 && b>>8 < 80
}

// TestStrokeCaps checks butt / round / square line caps on a horizontal line
// ending at x=80 (half-width 5). Just past the end (x=84): butt leaves it white,
// round and square paint it; square reaches x=85 but not x=87.
func TestStrokeCaps(t *testing.T) {
	const body = "10 w 0 0 0 RG\n20 50 m 80 50 l S\n"
	butt := renderContent(t, "0 J "+body)
	round := renderContent(t, "1 J "+body)
	square := renderContent(t, "2 J "+body)

	for _, img := range []image.Image{butt, round, square} {
		if !isBlack(img, 50, 50) {
			t.Fatal("line body not painted")
		}
	}
	if isBlack(butt, 84, 50) {
		t.Error("butt cap extended past the endpoint")
	}
	if !isBlack(round, 84, 50) {
		t.Error("round cap did not paint past the endpoint")
	}
	if !isBlack(square, 84, 50) {
		t.Error("square cap did not paint up to half-width past the end")
	}
	if isBlack(square, 87, 50) {
		t.Error("square cap extended too far (should stop ~half-width past)")
	}
}

// TestStrokeDash checks a [10 10] dash on a horizontal line: on-spans paint,
// off-spans stay white.
func TestStrokeDash(t *testing.T) {
	img := renderContent(t, "4 w 0 0 0 RG\n[10 10] 0 d\n0 50 m 100 50 l S\n")
	if !isBlack(img, 5, 50) {
		t.Error("dash on-span (x=5) not painted")
	}
	if isBlack(img, 15, 50) {
		t.Error("dash off-span (x=15) painted")
	}
	if !isBlack(img, 25, 50) {
		t.Error("dash on-span (x=25) not painted")
	}
	if isBlack(img, 35, 50) {
		t.Error("dash off-span (x=35) painted")
	}
}

// blackCount counts black pixels in the device window [x0,x1)×[y0,y1).
func blackCount(img image.Image, x0, y0, x1, y1 int) int {
	n := 0
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			if isBlack(img, x, y) {
				n++
			}
		}
	}
	return n
}

// TestStrokeMiterVsBevel checks that a miter join fills more of the outer corner
// than a bevel join, which cuts the spike off. Path is an L meeting at user
// (40,70) → device corner (40,30); the outer apex extends up-left.
func TestStrokeMiterVsBevel(t *testing.T) {
	const path = "12 w 0 0 0 RG\n40 40 m 40 70 l 70 70 l S\n"
	miter := renderContent(t, "0 j "+path) // LineJoinMiter
	bevel := renderContent(t, "2 j "+path) // LineJoinBevel

	// Window around the outer corner (device ~ (40,30)).
	mc := blackCount(miter, 28, 18, 46, 36)
	bc := blackCount(bevel, 28, 18, 46, 36)
	if mc <= bc {
		t.Errorf("miter (%d) should fill more outer-corner pixels than bevel (%d)", mc, bc)
	}
}
