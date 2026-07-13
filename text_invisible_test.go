// SPDX-License-Identifier: MIT

package asposepdf

import (
	"image"
	"strings"
	"testing"
)

// countDarkPixels returns the number of pixels darker than mid-gray.
func countDarkPixels(img image.Image) int {
	b := img.Bounds()
	n := 0
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bl, _ := img.At(x, y).RGBA()
			if r < 0x8000 && g < 0x8000 && bl < 0x8000 {
				n++
			}
		}
	}
	return n
}

// TestTextStyleInvisible: Invisible=true must paint nothing (text rendering
// mode 3) while the text stays extractable and searchable.
func TestTextStyleInvisible(t *testing.T) {
	mustPage := func(d *Document) *Page {
		p, err := d.Page(1)
		if err != nil {
			t.Fatal(err)
		}
		return p
	}
	rect := Rectangle{LLX: 50, LLY: 700, URX: 545, URY: 780}

	visible := NewDocumentFromFormat(PageFormatA4)
	if err := mustPage(visible).AddText("Hidden layer probe", TextStyle{Size: 24}, rect); err != nil {
		t.Fatal(err)
	}
	invisible := NewDocumentFromFormat(PageFormatA4)
	if err := mustPage(invisible).AddText("Hidden layer probe", TextStyle{Size: 24, Invisible: true, Underline: true, Strikethrough: true}, rect); err != nil {
		t.Fatal(err)
	}

	visImg, err := visible.RenderImage(1, RenderOptions{DPI: 96})
	if err != nil {
		t.Fatal(err)
	}
	invImg, err := invisible.RenderImage(1, RenderOptions{DPI: 96})
	if err != nil {
		t.Fatal(err)
	}

	if n := countDarkPixels(visImg); n == 0 {
		t.Error("visible text painted no pixels (test harness broken)")
	}
	if n := countDarkPixels(invImg); n != 0 {
		t.Errorf("invisible text painted %d dark pixels; want 0", n)
	}

	// Extraction and search must still see the hidden text.
	text, err := mustPage(invisible).ExtractText()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "Hidden layer probe") {
		t.Errorf("invisible text not extractable; got %q", text)
	}
	matches, err := mustPage(invisible).SearchText("layer probe")
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 {
		t.Fatalf("SearchText found %d matches; want 1", len(matches))
	}
	// The match box is baseline-derived (extends up to baseline+size), so
	// assert overlap with the AddText rect rather than strict containment.
	if m := matches[0]; m.Rect.URX < rect.LLX || m.Rect.LLX > rect.URX ||
		m.Rect.URY < rect.LLY || m.Rect.LLY > rect.URY {
		t.Errorf("match rect %+v does not intersect the AddText rect %+v", m.Rect, rect)
	}

	// Round-trip: invisibility and extractability survive Save+Open.
	var sb strings.Builder
	if _, err := invisible.WriteTo(&sb); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenStream(strings.NewReader(sb.String()))
	if err != nil {
		t.Fatal(err)
	}
	rtImg, err := reopened.RenderImage(1, RenderOptions{DPI: 96})
	if err != nil {
		t.Fatal(err)
	}
	if n := countDarkPixels(rtImg); n != 0 {
		t.Errorf("after round-trip invisible text painted %d dark pixels; want 0", n)
	}
	rtText, err := mustPage(reopened).ExtractText()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rtText, "Hidden layer probe") {
		t.Errorf("after round-trip invisible text not extractable; got %q", rtText)
	}
}
