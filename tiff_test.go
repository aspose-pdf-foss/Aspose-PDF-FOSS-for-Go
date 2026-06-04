// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"image"
	"io"
	"testing"
)

// --- minimal TIFF reader for tests (our own output: LE, RGB8, one strip) ---

type tiffTestPage struct {
	w, h, comp int
	rgb        []byte // decompressed, row-major RGB top-to-bottom
}

func tiffReadPages(t *testing.T, data []byte) []tiffTestPage {
	t.Helper()
	if len(data) < 8 || string(data[0:2]) != "II" {
		t.Fatalf("bad TIFF header %q", data[:min(8, len(data))])
	}
	rd16 := func(o uint32) uint16 { return binary.LittleEndian.Uint16(data[o:]) }
	rd32 := func(o uint32) uint32 { return binary.LittleEndian.Uint32(data[o:]) }
	if rd16(2) != 42 {
		t.Fatalf("bad TIFF magic %d", rd16(2))
	}
	var pages []tiffTestPage
	for ifd := rd32(4); ifd != 0; {
		n := rd16(ifd)
		field := map[uint16]uint32{}
		for k := uint32(0); k < uint32(n); k++ {
			e := ifd + 2 + k*12
			field[rd16(e)] = rd32(e + 8)
		}
		comp := field[259]
		if comp == 0 {
			comp = 1
		}
		strip := data[field[273] : field[273]+field[279]]
		rgb := strip
		if comp == 8 {
			zr, err := zlib.NewReader(bytes.NewReader(strip))
			if err != nil {
				t.Fatalf("zlib: %v", err)
			}
			rgb, err = io.ReadAll(zr)
			if err != nil {
				t.Fatalf("zlib read: %v", err)
			}
		}
		pages = append(pages, tiffTestPage{w: int(field[256]), h: int(field[257]), comp: int(comp), rgb: rgb})
		ifd = rd32(ifd + 2 + uint32(n)*12)
	}
	return pages
}

func (p tiffTestPage) at(x, y int) (uint8, uint8, uint8) {
	o := (y*p.w + x) * 3
	return p.rgb[o], p.rgb[o+1], p.rgb[o+2]
}

// --- tests ---

func TestRenderTIFFSinglePage(t *testing.T) {
	doc := NewDocument(60, 40)
	p, _ := doc.Page(1)
	if err := p.DrawRectangle(Rectangle{LLX: 0, LLY: 0, URX: 60, URY: 40},
		ShapeStyle{FillColor: &Color{R: 1, A: 1}}); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := p.RenderTIFF(&buf, RenderOptions{DPI: 72}); err != nil {
		t.Fatalf("RenderTIFF: %v", err)
	}
	pages := tiffReadPages(t, buf.Bytes())
	if len(pages) != 1 {
		t.Fatalf("got %d pages, want 1", len(pages))
	}
	pg := pages[0]
	if pg.w != 60 || pg.h != 40 {
		t.Errorf("dims = %dx%d, want 60x40", pg.w, pg.h)
	}
	if pg.comp != 8 {
		t.Errorf("compression = %d, want 8 (Deflate)", pg.comp)
	}
	if r, g, b := pg.at(30, 20); r < 200 || g > 60 || b > 60 {
		t.Errorf("centre = (%d,%d,%d), want red", r, g, b)
	}
}

func TestRenderTIFFMultiPage(t *testing.T) {
	doc := NewDocument(60, 40) // page 1
	if err := doc.AddBlankPage(80, 50); err != nil {
		t.Fatal(err)
	}
	if err := doc.AddBlankPage(40, 40); err != nil {
		t.Fatal(err)
	}
	fills := []Color{{R: 1, A: 1}, {G: 1, A: 1}, {B: 1, A: 1}}
	for i, c := range fills {
		pg, _ := doc.Page(i + 1)
		sz, _ := pg.Size()
		col := c
		if err := pg.DrawRectangle(Rectangle{LLX: 0, LLY: 0, URX: sz.Width, URY: sz.Height},
			ShapeStyle{FillColor: &col}); err != nil {
			t.Fatal(err)
		}
	}

	var buf bytes.Buffer
	if err := doc.RenderTIFF(&buf, RenderOptions{DPI: 72}); err != nil {
		t.Fatalf("RenderTIFF: %v", err)
	}
	pages := tiffReadPages(t, buf.Bytes())
	if len(pages) != 3 {
		t.Fatalf("got %d pages, want 3", len(pages))
	}
	wantDims := [3][2]int{{60, 40}, {80, 50}, {40, 40}}
	for i, p := range pages {
		if p.w != wantDims[i][0] || p.h != wantDims[i][1] {
			t.Errorf("page %d dims = %dx%d, want %dx%d", i+1, p.w, p.h, wantDims[i][0], wantDims[i][1])
		}
	}
	// Each page must carry its own fill colour.
	if r, _, _ := pages[0].at(30, 20); r < 200 {
		t.Errorf("page 1 not red (R=%d)", r)
	}
	if _, g, _ := pages[1].at(40, 25); g < 200 {
		t.Errorf("page 2 not green (G=%d)", g)
	}
	if _, _, b := pages[2].at(20, 20); b < 200 {
		t.Errorf("page 3 not blue (B=%d)", b)
	}
}

// TestRenderTIFFPageSelection checks that an explicit page list is honoured in
// order, and out-of-range pages error.
func TestRenderTIFFPageSelection(t *testing.T) {
	doc := NewDocument(20, 20)
	_ = doc.AddBlankPage(20, 20)
	_ = doc.AddBlankPage(20, 20)

	var buf bytes.Buffer
	if err := doc.RenderTIFF(&buf, RenderOptions{DPI: 72}, 3, 1); err != nil {
		t.Fatalf("RenderTIFF: %v", err)
	}
	if pages := tiffReadPages(t, buf.Bytes()); len(pages) != 2 {
		t.Fatalf("got %d pages, want 2 (selected)", len(pages))
	}
	if err := doc.RenderTIFF(io.Discard, RenderOptions{DPI: 72}, 9); err == nil {
		t.Error("out-of-range page did not error")
	}
}

// TestEncodeTIFFUncompressed covers the Compression=1 path and exact pixels.
func TestEncodeTIFFUncompressed(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 3, 2))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = 10, 20, 30, 255
	}
	var buf bytes.Buffer
	if err := encodeTIFF(&buf, 1, 72, false, func(int) (image.Image, error) { return img, nil }); err != nil {
		t.Fatalf("encodeTIFF: %v", err)
	}
	pages := tiffReadPages(t, buf.Bytes())
	if len(pages) != 1 || pages[0].comp != 1 {
		t.Fatalf("pages=%d comp=%d, want 1 page comp 1", len(pages), pages[0].comp)
	}
	if len(pages[0].rgb) != 3*2*3 {
		t.Fatalf("rgb len = %d, want 18", len(pages[0].rgb))
	}
	if r, g, b := pages[0].at(1, 1); r != 10 || g != 20 || b != 30 {
		t.Errorf("pixel = (%d,%d,%d), want (10,20,30)", r, g, b)
	}
}
