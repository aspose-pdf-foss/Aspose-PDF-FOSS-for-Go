// SPDX-License-Identifier: MIT

package asposepdf_test

import (
	"os"
	"path/filepath"
	"testing"

	asposepdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

func TestRemoveImageRoundTrip(t *testing.T) {
	doc, err := asposepdf.Open("testdata/PdfWithImages.pdf")
	if err != nil {
		t.Fatalf("open: %v", err)
	}

	page, _ := doc.Page(1)
	infos, err := page.ImageInfos()
	if err != nil {
		t.Fatalf("ImageInfos: %v", err)
	}
	if len(infos) == 0 {
		t.Fatal("expected at least 1 image")
	}
	origCount := len(infos)
	t.Logf("original image count: %d", origCount)

	// Remove first image.
	err = infos[0].Remove()
	if err != nil {
		t.Fatalf("Remove: %v", err)
	}

	outDir := filepath.Join("result_files", "TestRemoveImageRoundTrip")
	os.MkdirAll(outDir, 0o755)
	outPath := filepath.Join(outDir, "output.pdf")
	if err := doc.Save(outPath); err != nil {
		t.Fatalf("save: %v", err)
	}

	// Reopen and verify image count decreased.
	reopened, err := asposepdf.Open(outPath)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	p, _ := reopened.Page(1)
	newInfos, err := p.ImageInfos()
	if err != nil {
		t.Fatalf("ImageInfos after reopen: %v", err)
	}

	if len(newInfos) >= origCount {
		t.Errorf("expected fewer images after removal: got %d, original %d", len(newInfos), origCount)
	}
	t.Logf("image count after removal: %d", len(newInfos))
}

func TestRemoveAllImagesRoundTrip(t *testing.T) {
	doc, err := asposepdf.Open("testdata/PdfWithImages.pdf")
	if err != nil {
		t.Fatalf("open: %v", err)
	}

	page, _ := doc.Page(1)
	infos, err := page.ImageInfos()
	if err != nil {
		t.Fatalf("ImageInfos: %v", err)
	}
	t.Logf("removing %d images", len(infos))

	for _, info := range infos {
		if err := info.Remove(); err != nil {
			t.Fatalf("Remove: %v", err)
		}
	}

	outDir := filepath.Join("result_files", "TestRemoveAllImagesRoundTrip")
	os.MkdirAll(outDir, 0o755)
	outPath := filepath.Join(outDir, "output.pdf")
	if err := doc.Save(outPath); err != nil {
		t.Fatalf("save: %v", err)
	}

	// Reopen and verify no images remain.
	reopened, err := asposepdf.Open(outPath)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	p, _ := reopened.Page(1)
	newInfos, _ := p.ImageInfos()
	if len(newInfos) > 0 {
		t.Errorf("expected 0 images after removing all, got %d", len(newInfos))
	}
}
