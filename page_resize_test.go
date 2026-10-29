// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"fmt"
	"math"
	"testing"
)

// Tolerances in points. Horizontally SearchText positions are exact; its box
// height comes from the font size as scaled along the baseline, so under a
// non-uniform scale it differs from the geometrically scaled box by about a
// point. Pixel measurements round to whole pixels.
const (
	resizeTolX     = 0.6
	resizeTolY     = 1.5
	resizeTolPixel = 1.0
)

func resizeFixture(t *testing.T) (*Document, *Page) {
	t.Helper()
	doc := NewDocumentFromFormat(PageFormatA4)
	p, _ := doc.Page(1)
	// Blue, so the dark-pixel search finds only the black square.
	if err := p.AddText("Marker", TextStyle{Size: 20, Color: &Color{B: 1, A: 1}}, Rectangle{LLX: 100, LLY: 600, URX: 300, URY: 640}); err != nil {
		t.Fatal(err)
	}
	black := Color{A: 1}
	if err := p.DrawRectangle(Rectangle{LLX: 200, LLY: 200, URX: 300, URY: 300}, ShapeStyle{FillColor: &black}); err != nil {
		t.Fatal(err)
	}
	return doc, p
}

func markerRect(t *testing.T, p *Page) Rectangle {
	t.Helper()
	ms, err := p.SearchText("Marker")
	if err != nil || len(ms) != 1 {
		t.Fatalf("SearchText: %v, %d matches", err, len(ms))
	}
	return ms[0].Rect
}

func mapRect(r Rectangle, sx, sy, tx, ty float64) Rectangle {
	return Rectangle{LLX: r.LLX*sx + tx, LLY: r.LLY*sy + ty, URX: r.URX*sx + tx, URY: r.URY*sy + ty}
}

func assertRectNear(t *testing.T, what string, got, want Rectangle) {
	t.Helper()
	assertRectWithin(t, what, got, want, resizeTolX, resizeTolY)
}

func assertPixelRect(t *testing.T, what string, got, want Rectangle) {
	t.Helper()
	assertRectWithin(t, what, got, want, resizeTolPixel, resizeTolPixel)
}

func assertRectWithin(t *testing.T, what string, got, want Rectangle, tx, ty float64) {
	t.Helper()
	if math.Abs(got.LLX-want.LLX) > tx || math.Abs(got.URX-want.URX) > tx ||
		math.Abs(got.LLY-want.LLY) > ty || math.Abs(got.URY-want.URY) > ty {
		t.Errorf("%s = %+v, want %+v", what, got, want)
	}
}

// darkBBox is the bounding box, in points, of the dark pixels of a page
// rendered at 72 DPI (y measured up from the bottom, like PDF space).
func darkBBox(t *testing.T, p *Page) Rectangle {
	t.Helper()
	img, err := p.RenderImage(RenderOptions{DPI: 72})
	if err != nil {
		t.Fatal(err)
	}
	b := img.Bounds()
	minX, minY, maxX, maxY := b.Max.X, b.Max.Y, -1, -1
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if r, g, bl, _ := img.At(x, y).RGBA(); r < 0x1000 && g < 0x1000 && bl < 0x1000 {
				minX, minY, maxX, maxY = min(minX, x), min(minY, y), max(maxX, x), max(maxY, y)
			}
		}
	}
	h := float64(b.Dy())
	return Rectangle{LLX: float64(minX), LLY: h - float64(maxY+1), URX: float64(maxX + 1), URY: h - float64(minY)}
}

func TestResizePageScalesContent(t *testing.T) {
	doc, p := resizeFixture(t)
	before := markerRect(t, p)
	text, _ := p.ExtractText()

	if err := doc.ResizeContents(ResizePage(612, 792)); err != nil {
		t.Fatal(err)
	}
	mb, _ := p.MediaBox()
	if mb != (Rectangle{URX: 612, URY: 792}) {
		t.Fatalf("MediaBox = %+v, want letter", mb)
	}
	sx, sy := 612/PageFormatA4.Width, 792/PageFormatA4.Height
	assertRectNear(t, "Marker", markerRect(t, p), mapRect(before, sx, sy, 0, 0))
	// The filled square, found in the rendering.
	assertPixelRect(t, "square", darkBBox(t, p), mapRect(Rectangle{LLX: 200, LLY: 200, URX: 300, URY: 300}, sx, sy, 0, 0))
	if got, _ := p.ExtractText(); got != text {
		t.Errorf("text changed: %q → %q", text, got)
	}
}

func TestResizeMarginsShrinksIntoMargins(t *testing.T) {
	doc, p := resizeFixture(t)
	before := markerRect(t, p)
	if err := doc.ResizeContents(ResizeMargins(50, 30, 40, 20)); err != nil {
		t.Fatal(err)
	}
	mb, _ := p.MediaBox()
	if mb != (Rectangle{URX: PageFormatA4.Width, URY: PageFormatA4.Height}) {
		t.Fatalf("page size changed: %+v", mb)
	}
	sx := (PageFormatA4.Width - 80) / PageFormatA4.Width
	sy := (PageFormatA4.Height - 60) / PageFormatA4.Height
	assertRectNear(t, "Marker", markerRect(t, p), mapRect(before, sx, sy, 50, 20))
}

func TestResizeContentSizeCentredKeepingAspect(t *testing.T) {
	doc, p := resizeFixture(t)
	params := ResizeContentSize(300, 300)
	params.PreserveAspectRatio = true
	if err := doc.ResizeContents(params); err != nil {
		t.Fatal(err)
	}
	// 300 wide box, page aspect taller than wide: scale = 300/842 on both axes.
	s := 300 / PageFormatA4.Height
	tx := (PageFormatA4.Width-300)/2 + (300-PageFormatA4.Width*s)/2
	ty := (PageFormatA4.Height - 300) / 2
	assertPixelRect(t, "square", darkBBox(t, p), mapRect(Rectangle{LLX: 200, LLY: 200, URX: 300, URY: 300}, s, s, tx, ty))
}

// A cropped page: the visible region is what gets scaled onto the new page.
func TestResizeUsesCropBox(t *testing.T) {
	doc, p := resizeFixture(t)
	if err := p.SetCropBox(Rectangle{LLX: 150, LLY: 150, URX: 350, URY: 350}); err != nil {
		t.Fatal(err)
	}
	if err := doc.ResizeContents(ResizePage(400, 400)); err != nil {
		t.Fatal(err)
	}
	if _, has := p.pageDict()["/CropBox"]; has {
		t.Error("CropBox still set — the new page should be visible in full")
	}
	// The square (200..300) sits in the middle of the 200pt crop: ×2, offset −150.
	assertPixelRect(t, "square", darkBBox(t, p), Rectangle{LLX: 100, LLY: 100, URX: 300, URY: 300})
}

// Sizes are in the displayed orientation: a /Rotate 90 page resized to
// landscape letter keeps displaying landscape, and its content lands where
// the plain scale puts it.
func TestResizeRotatedPage(t *testing.T) {
	doc, p := resizeFixture(t)
	if err := doc.SetRotation(Rotate90); err != nil {
		t.Fatal(err)
	}
	before := markerRect(t, p)
	if err := doc.ResizeContents(ResizePage(792, 612)); err != nil {
		t.Fatal(err)
	}
	mb, _ := p.MediaBox()
	if mb != (Rectangle{URX: 612, URY: 792}) {
		t.Fatalf("MediaBox = %+v, want unrotated letter portrait", mb)
	}
	img, err := p.RenderImage(RenderOptions{DPI: 72})
	if err != nil {
		t.Fatal(err)
	}
	if b := img.Bounds(); b.Dx() != 792 || b.Dy() != 612 {
		t.Errorf("displayed size %dx%d, want 792x612", b.Dx(), b.Dy())
	}
	sx, sy := 612/PageFormatA4.Width, 792/PageFormatA4.Height
	assertRectNear(t, "Marker", markerRect(t, p), mapRect(before, sx, sy, 0, 0))
}

func TestResizeMovesAnnotations(t *testing.T) {
	doc, p := resizeFixture(t)
	r := markerRect(t, p)
	hl := NewHighlightAnnotation(p, r)
	q := QuadPoint{X1: r.LLX, Y1: r.URY, X2: r.URX, Y2: r.URY, X3: r.LLX, Y3: r.LLY, X4: r.URX, Y4: r.LLY}
	hl.SetQuadPoints([]QuadPoint{q})
	if err := p.Annotations().Add(hl); err != nil {
		t.Fatal(err)
	}
	// A uniform scale, so the SearchText box and the moved quad agree exactly.
	if err := doc.ResizeContents(ResizeMarginsPercent(10, 10, 10, 10)); err != nil {
		t.Fatal(err)
	}
	newMarker := markerRect(t, p)
	got := p.Annotations().At(0).(*HighlightAnnotation)
	assertRectNear(t, "highlight rect", got.Rect(), newMarker)
	qs := got.QuadPoints()
	if len(qs) != 1 {
		t.Fatalf("%d quads", len(qs))
	}
	assertRectNear(t, "quad", Rectangle{LLX: qs[0].X3, LLY: qs[0].Y3, URX: qs[0].X2, URY: qs[0].Y2}, newMarker)
}

// Drawing after a resize uses the new page's coordinates, unscaled.
func TestResizeThenDraw(t *testing.T) {
	doc, p := resizeFixture(t)
	if err := doc.ResizeContents(ResizePagePct(50, 50)); err != nil {
		t.Fatal(err)
	}
	if err := p.AddText("After", TextStyle{Size: 10}, Rectangle{LLX: 20, LLY: 20, URX: 200, URY: 40}); err != nil {
		t.Fatal(err)
	}
	ms, err := p.SearchText("After")
	if err != nil || len(ms) != 1 {
		t.Fatalf("SearchText: %v %d", err, len(ms))
	}
	if r := ms[0].Rect; math.Abs(r.LLX-20) > 1 || r.URY > 41 || r.LLY < 19 {
		t.Errorf("text drawn after resize at %+v, want inside (20,20)-(200,40)", r)
	}
	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	re, err := OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	rp, _ := re.Page(1)
	if size, _ := rp.Size(); math.Abs(size.Width-PageFormatA4.Width/2) > 0.01 {
		t.Errorf("reopened width %v", size.Width)
	}
}

func TestResizeRejectsBadParameters(t *testing.T) {
	cases := map[string]ContentsResizeParameters{
		"margins wider than the page": ResizeMargins(400, 400, 0, 0),
		"negative margin":             ResizeMargins(-1, 0, 0, 0),
		"zero page":                   ResizePage(0, 100),
		"content larger than page":    ResizeContentSize(1000, 100),
	}
	for name, params := range cases {
		doc, p := resizeFixture(t)
		if err := doc.ResizeContents(params); err == nil {
			t.Errorf("%s: no error", name)
		}
		if mb, _ := p.MediaBox(); mb.URX != PageFormatA4.Width {
			t.Errorf("%s: page changed despite the error", name)
		}
	}
	doc, _ := resizeFixture(t)
	if err := doc.ResizeContents(ResizePage(100, 100), 2); err == nil {
		t.Error("page 2 of 1: no error")
	}
}

// Every page is checked before any is changed.
func TestResizeIsAtomic(t *testing.T) {
	doc, p1 := resizeFixture(t)
	if err := doc.AddBlankPage(100, 100); err != nil {
		t.Fatal(err)
	}
	contentsBefore := fmt.Sprint(p1.pageDict()["/Contents"])
	// 150pt margins fit A4 but not the 100pt second page.
	if err := doc.ResizeContents(ResizeMargins(150, 150, 0, 0)); err == nil {
		t.Fatal("no error")
	}
	if got := fmt.Sprint(p1.pageDict()["/Contents"]); got != contentsBefore {
		t.Error("page 1 was resized although page 2 failed")
	}
}

// Margins are in the displayed orientation on every rotation: the square,
// found in the rendered (displayed) page, moves exactly as the displayed
// page shrinks into the margins.
func TestResizeMarginsOnEveryRotation(t *testing.T) {
	for _, rot := range []RotationAngle{Rotate0, Rotate90, Rotate180, Rotate270} {
		doc, p := resizeFixture(t)
		if err := doc.SetRotation(rot); err != nil {
			t.Fatal(err)
		}
		before := darkBBox(t, p)
		img, _ := p.RenderImage(RenderOptions{DPI: 72})
		dw, dh := float64(img.Bounds().Dx()), float64(img.Bounds().Dy())

		const l, r, top, b = 60, 20, 10, 40
		if err := doc.ResizeContents(ResizeMargins(l, r, top, b)); err != nil {
			t.Fatal(err)
		}
		sx, sy := (dw-l-r)/dw, (dh-top-b)/dh
		assertPixelRect(t, fmt.Sprintf("rotate %d: square", rot), darkBBox(t, p), mapRect(before, sx, sy, l, b))
	}
}
