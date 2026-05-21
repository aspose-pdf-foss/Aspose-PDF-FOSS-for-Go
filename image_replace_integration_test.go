package asposepdf_test

import (
	"os"
	"path/filepath"
	"testing"

	asposepdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

func TestReplaceImageRoundTrip(t *testing.T) {
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

	origWidth := infos[0].Width
	t.Logf("original image: %dx%d %s", infos[0].Width, infos[0].Height, infos[0].Name)

	// Replace first image with a different one.
	err = infos[0].Replace("testdata/Koala.jpg")
	if err != nil {
		t.Fatalf("Replace: %v", err)
	}

	outDir := filepath.Join("result_files", "TestReplaceImageRoundTrip")
	os.MkdirAll(outDir, 0o755)
	outPath := filepath.Join(outDir, "output.pdf")
	if err := doc.Save(outPath); err != nil {
		t.Fatalf("save: %v", err)
	}

	// Reopen and verify.
	reopened, err := asposepdf.Open(outPath)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	p, _ := reopened.Page(1)
	newInfos, err := p.ImageInfos()
	if err != nil {
		t.Fatalf("ImageInfos after reopen: %v", err)
	}
	if len(newInfos) == 0 {
		t.Fatal("expected at least 1 image after reopen")
	}

	// Dimensions should differ from original (Koala.jpg has different size).
	if newInfos[0].Width == origWidth {
		t.Logf("warning: replacement image has same width as original (%d)", origWidth)
	}
	t.Logf("replaced image: %dx%d", newInfos[0].Width, newInfos[0].Height)
}

func TestReplaceImageFromStreamRoundTrip(t *testing.T) {
	doc, err := asposepdf.Open("testdata/PdfWithImages.pdf")
	if err != nil {
		t.Fatalf("open: %v", err)
	}

	page, _ := doc.Page(1)
	infos, _ := page.ImageInfos()
	if len(infos) == 0 {
		t.Fatal("expected at least 1 image")
	}

	f, err := os.Open("testdata/aspose-logo.png")
	if err != nil {
		t.Fatalf("open image: %v", err)
	}
	defer f.Close()

	err = infos[0].ReplaceFromStream(f)
	if err != nil {
		t.Fatalf("ReplaceFromStream: %v", err)
	}

	outDir := filepath.Join("result_files", "TestReplaceImageFromStreamRoundTrip")
	os.MkdirAll(outDir, 0o755)
	outPath := filepath.Join(outDir, "output.pdf")
	if err := doc.Save(outPath); err != nil {
		t.Fatalf("save: %v", err)
	}
	t.Log("replaced image from stream, saved to", outPath)
}
