// SPDX-License-Identifier: MIT

package asposepdf_test

import (
	"os"
	"path/filepath"
	"testing"

	asposepdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

func TestAddImage(t *testing.T) {
	doc, err := asposepdf.Open("testdata/4pages.pdf")
	if err != nil {
		t.Fatalf("open: %v", err)
	}

	page, _ := doc.Page(1)
	size, _ := page.Size()

	// Add JPEG.
	rect := asposepdf.Rectangle{LLX: 10, LLY: size.Height - 110, URX: 110, URY: size.Height - 10}
	if err := page.AddImage("testdata/Penguins.jpg", rect); err != nil {
		t.Fatalf("AddImage JPEG: %v", err)
	}

	// Add PNG.
	rect2 := asposepdf.Rectangle{LLX: 120, LLY: size.Height - 110, URX: 220, URY: size.Height - 10}
	if err := page.AddImage("testdata/aspose-logo.png", rect2); err != nil {
		t.Fatalf("AddImage PNG: %v", err)
	}

	outDir := filepath.Join("result_files", "TestAddImage")
	os.MkdirAll(outDir, 0o755)
	outPath := filepath.Join(outDir, "output.pdf")
	if err := doc.Save(outPath); err != nil {
		t.Fatalf("save: %v", err)
	}

	// Reopen and verify images are extractable.
	reopened, err := asposepdf.Open(outPath)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}

	page1, _ := reopened.Page(1)
	images, err := page1.ExtractImages()
	if err != nil {
		t.Fatalf("ExtractImages: %v", err)
	}

	// Should have at least the 2 images we added.
	if len(images) < 2 {
		t.Errorf("expected at least 2 images, got %d", len(images))
	}

	t.Logf("added 2 images to page 1, extracted %d images after save/reopen", len(images))
}

func TestAddImageFromStream(t *testing.T) {
	doc, err := asposepdf.Open("testdata/4pages.pdf")
	if err != nil {
		t.Fatalf("open: %v", err)
	}

	f, err := os.Open("testdata/Koala.jpg")
	if err != nil {
		t.Fatalf("open image: %v", err)
	}
	defer f.Close()

	page, _ := doc.Page(1)
	rect := asposepdf.Rectangle{LLX: 50, LLY: 50, URX: 200, URY: 200}
	if err := page.AddImageFromStream(f, rect); err != nil {
		t.Fatalf("AddImageFromStream: %v", err)
	}

	outDir := filepath.Join("result_files", "TestAddImageFromStream")
	os.MkdirAll(outDir, 0o755)
	outPath := filepath.Join(outDir, "output.pdf")
	if err := doc.Save(outPath); err != nil {
		t.Fatalf("save: %v", err)
	}
	t.Logf("added image from stream, saved to %s", outPath)
}
