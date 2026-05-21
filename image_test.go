package asposepdf_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	asposepdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

func TestExtractImages(t *testing.T) {
	groups := testGroups(t)
	for _, group := range groups {
		path := group[0]
		name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		t.Run(name, func(t *testing.T) {
			doc, err := asposepdf.Open(path)
			if err != nil {
				t.Fatalf("open: %v", err)
			}
			allImages, err := doc.ExtractImages()
			if err != nil {
				t.Fatalf("ExtractImages: %v", err)
			}

			outDir := filepath.Join(resultDir, "TestExtractImages", name)
			os.MkdirAll(outDir, 0o755)

			total := 0
			for pageIdx, images := range allImages {
				for imgIdx, img := range images {
					ext := ".png"
					if img.Format == asposepdf.ImageFormatJPEG {
						ext = ".jpg"
					}
					outPath := filepath.Join(outDir, fmt.Sprintf("page%d_img%d%s", pageIdx+1, imgIdx+1, ext))
					if err := img.Save(outPath); err != nil {
						t.Errorf("save %s: %v", outPath, err)
					}
					if img.Width <= 0 || img.Height <= 0 {
						t.Errorf("page %d img %d: invalid dimensions %dx%d", pageIdx+1, imgIdx+1, img.Width, img.Height)
					}
					if len(img.Data) == 0 {
						t.Errorf("page %d img %d: empty data", pageIdx+1, imgIdx+1)
					}
					total++
				}
			}
			t.Logf("%s: extracted %d images to %s", name, total, outDir)
		})
	}
}

func TestImageInfos(t *testing.T) {
	groups := testGroups(t)
	for _, group := range groups {
		path := group[0]
		name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		t.Run(name, func(t *testing.T) {
			doc, err := asposepdf.Open(path)
			if err != nil {
				t.Fatalf("open: %v", err)
			}

			// Get infos (lightweight).
			allInfos, err := doc.ImageInfos()
			if err != nil {
				t.Fatalf("ImageInfos: %v", err)
			}

			// Get images (full extraction) for comparison.
			allImages, err := doc.ExtractImages()
			if err != nil {
				t.Fatalf("ExtractImages: %v", err)
			}

			if len(allInfos) != len(allImages) {
				t.Fatalf("page count: infos=%d, images=%d", len(allInfos), len(allImages))
			}

			for pageIdx := range allInfos {
				if len(allInfos[pageIdx]) != len(allImages[pageIdx]) {
					t.Errorf("page %d: infos=%d, images=%d",
						pageIdx+1, len(allInfos[pageIdx]), len(allImages[pageIdx]))
					continue
				}
				for imgIdx, info := range allInfos[pageIdx] {
					img := allImages[pageIdx][imgIdx]
					if info.Width != img.Width || info.Height != img.Height {
						t.Errorf("page %d img %d: info=%dx%d, image=%dx%d",
							pageIdx+1, imgIdx+1, info.Width, info.Height, img.Width, img.Height)
					}
					if info.Format != img.Format {
						t.Errorf("page %d img %d: info format=%d, image format=%d",
							pageIdx+1, imgIdx+1, info.Format, img.Format)
					}
				}
			}

			// Verify Extract() works on each info.
			totalExtracted := 0
			for _, infos := range allInfos {
				for i := range infos {
					extracted, err := infos[i].Extract()
					if err != nil {
						t.Errorf("Extract: %v", err)
						continue
					}
					if len(extracted.Data) == 0 {
						t.Error("extracted image has empty data")
					}
					totalExtracted++
				}
			}
			t.Logf("%s: %d infos, all extracted successfully", name, totalExtracted)
		})
	}
}
