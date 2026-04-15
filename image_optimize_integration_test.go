package asposepdf_test

import (
	"os"
	"path/filepath"
	"testing"

	asposepdf "github.com/aspose/pdf-for-go"
)

func TestOptimizeImagesRoundTrip(t *testing.T) {
	doc, err := asposepdf.Open("testdata/PdfWithImages.pdf")
	if err != nil {
		t.Fatalf("open: %v", err)
	}

	count, err := doc.OptimizeImages(asposepdf.OptimizeImageOptions{
		MaxDPI:           150,
		JPEGQuality:      75,
		ConvertPNGToJPEG: true,
	})
	if err != nil {
		t.Fatalf("OptimizeImages: %v", err)
	}
	t.Logf("optimized %d images", count)
	if count == 0 {
		t.Error("expected at least 1 image optimized")
	}

	outDir := filepath.Join("result_files", "TestOptimizeImagesRoundTrip")
	os.MkdirAll(outDir, 0o755)
	outPath := filepath.Join(outDir, "output.pdf")
	if err := doc.Save(outPath); err != nil {
		t.Fatalf("save: %v", err)
	}

	// Validate the output.
	report, err := asposepdf.Validate(outPath)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if !report.Valid {
		for _, issue := range report.Issues {
			t.Errorf("validation issue: [%s] %s", issue.Code, issue.Message)
		}
	}

	// Verify file size decreased.
	origInfo, _ := os.Stat("testdata/PdfWithImages.pdf")
	outInfo, _ := os.Stat(outPath)
	t.Logf("original: %d bytes, optimized: %d bytes", origInfo.Size(), outInfo.Size())
	if outInfo.Size() >= origInfo.Size() {
		t.Error("expected smaller file size after optimization")
	}
}
