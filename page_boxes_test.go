// SPDX-License-Identifier: MIT

package asposepdf_test

import (
	"path/filepath"
	"testing"

	asposepdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

func TestSetPageSize(t *testing.T) {
	doc := asposepdf.NewDocument(595, 842)
	p, _ := doc.Page(1)
	if err := p.SetPageSize(200, 300); err != nil {
		t.Fatalf("SetPageSize: %v", err)
	}
	sz, _ := p.Size()
	if sz.Width != 200 || sz.Height != 300 {
		t.Errorf("Size = %+v, want 200x300", sz)
	}
	mb, _ := p.MediaBox()
	if mb != (asposepdf.Rectangle{LLX: 0, LLY: 0, URX: 200, URY: 300}) {
		t.Errorf("MediaBox = %+v, want [0 0 200 300]", mb)
	}

	// Persists through save + reopen.
	out := filepath.Join(t.TempDir(), "size.pdf")
	if err := doc.Save(out); err != nil {
		t.Fatalf("Save: %v", err)
	}
	re, err := asposepdf.Open(out)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	rp, _ := re.Page(1)
	if rsz, _ := rp.Size(); rsz.Width != 200 || rsz.Height != 300 {
		t.Errorf("reloaded Size = %+v, want 200x300", rsz)
	}
}

func TestSetBoxesRoundTrip(t *testing.T) {
	doc := asposepdf.NewDocument(600, 800)
	p, _ := doc.Page(1)

	media := asposepdf.Rectangle{LLX: 0, LLY: 0, URX: 600, URY: 800}
	crop := asposepdf.Rectangle{LLX: 10, LLY: 20, URX: 590, URY: 780}
	trim := asposepdf.Rectangle{LLX: 15, LLY: 25, URX: 585, URY: 775}
	bleed := asposepdf.Rectangle{LLX: 5, LLY: 5, URX: 595, URY: 795}
	art := asposepdf.Rectangle{LLX: 20, LLY: 30, URX: 580, URY: 770}

	for name, err := range map[string]error{
		"SetMediaBox": p.SetMediaBox(media),
		"SetCropBox":  p.SetCropBox(crop),
		"SetTrimBox":  p.SetTrimBox(trim),
		"SetBleedBox": p.SetBleedBox(bleed),
		"SetArtBox":   p.SetArtBox(art),
	} {
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}

	out := filepath.Join(t.TempDir(), "boxes.pdf")
	if err := doc.Save(out); err != nil {
		t.Fatalf("Save: %v", err)
	}
	re, err := asposepdf.Open(out)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	rp, _ := re.Page(1)

	check := func(name string, got asposepdf.Rectangle, gErr error, want asposepdf.Rectangle) {
		t.Helper()
		if gErr != nil {
			t.Fatalf("%s: %v", name, gErr)
		}
		if got != want {
			t.Errorf("%s = %+v, want %+v", name, got, want)
		}
	}
	g, e := rp.MediaBox()
	check("MediaBox", g, e, media)
	g, e = rp.CropBox()
	check("CropBox", g, e, crop)
	g, e = rp.TrimBox()
	check("TrimBox", g, e, trim)
	g, e = rp.BleedBox()
	check("BleedBox", g, e, bleed)
	g, e = rp.ArtBox()
	check("ArtBox", g, e, art)
}

func TestBoxFallbackToMediaBox(t *testing.T) {
	doc := asposepdf.NewDocument(500, 700)
	p, _ := doc.Page(1)
	mb, _ := p.MediaBox()

	boxes := map[string]func() (asposepdf.Rectangle, error){
		"CropBox": p.CropBox, "TrimBox": p.TrimBox, "BleedBox": p.BleedBox, "ArtBox": p.ArtBox,
	}
	for name, fn := range boxes {
		r, err := fn()
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if r != mb {
			t.Errorf("%s = %+v, want fallback to MediaBox %+v", name, r, mb)
		}
	}
}

func TestSetCropBoxLeavesMediaBox(t *testing.T) {
	doc := asposepdf.NewDocument(600, 800)
	p, _ := doc.Page(1)

	crop := asposepdf.Rectangle{LLX: 50, LLY: 50, URX: 550, URY: 750}
	if err := p.SetCropBox(crop); err != nil {
		t.Fatalf("SetCropBox: %v", err)
	}
	if mb, _ := p.MediaBox(); mb != (asposepdf.Rectangle{LLX: 0, LLY: 0, URX: 600, URY: 800}) {
		t.Errorf("MediaBox changed by SetCropBox: %+v", mb)
	}
	if cb, _ := p.CropBox(); cb != crop {
		t.Errorf("CropBox = %+v, want %+v", cb, crop)
	}
}

func TestSetBoxValidation(t *testing.T) {
	doc := asposepdf.NewDocument(400, 400)
	p, _ := doc.Page(1)
	if err := p.SetMediaBox(asposepdf.Rectangle{LLX: 100, LLY: 100, URX: 100, URY: 200}); err == nil {
		t.Error("SetMediaBox with URX==LLX should error")
	}
	if err := p.SetPageSize(0, 0); err == nil {
		t.Error("SetPageSize(0,0) should error")
	}
	if err := p.SetPageSize(-10, 50); err == nil {
		t.Error("SetPageSize with negative width should error")
	}
}
