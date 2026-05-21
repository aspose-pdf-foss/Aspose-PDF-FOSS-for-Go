package asposepdf_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	asposepdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

func TestImageToDocumentFiles(t *testing.T) {
	groups := testGroups(t)
	for _, group := range groups {
		path := group[0]
		name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		t.Run(name, func(t *testing.T) {
			doc, err := asposepdf.ImageToDocument(path)
			if err != nil {
				t.Fatalf("ImageToDocument: %v", err)
			}
			if doc.PageCount() != 1 {
				t.Fatalf("pages = %d, want 1", doc.PageCount())
			}

			page, _ := doc.Page(1)
			size, _ := page.Size()
			t.Logf("%s: page size %.1f x %.1f pt", name, size.Width, size.Height)

			outDir := filepath.Join("result_files", "TestImageToDocumentFiles", name)
			os.MkdirAll(outDir, 0o755)
			outPath := filepath.Join(outDir, name+".pdf")
			if err := doc.Save(outPath); err != nil {
				t.Fatalf("save: %v", err)
			}

			// Reopen and extract image back.
			reopened, err := asposepdf.Open(outPath)
			if err != nil {
				t.Fatalf("reopen: %v", err)
			}
			p, _ := reopened.Page(1)
			images, err := p.ExtractImages()
			if err != nil {
				t.Fatalf("ExtractImages: %v", err)
			}
			if len(images) != 1 {
				t.Errorf("expected 1 image, got %d", len(images))
			}
		})
	}
}

func TestImageToDocumentWithMargins(t *testing.T) {
	doc, err := asposepdf.ImageToDocument("testdata/aspose-logo.png", asposepdf.ImageToDocumentOptions{
		PageWidth:    595,
		PageHeight:   842,
		MarginLeft:   72,
		MarginRight:  72,
		MarginTop:    72,
		MarginBottom: 72,
	})
	if err != nil {
		t.Fatalf("ImageToDocument: %v", err)
	}

	page, _ := doc.Page(1)
	size, _ := page.Size()
	if size.Width < 594 || size.Width > 596 {
		t.Errorf("width = %.1f, want ~595", size.Width)
	}

	outDir := filepath.Join("result_files", "TestImageToDocumentWithMargins")
	os.MkdirAll(outDir, 0o755)
	if err := doc.Save(filepath.Join(outDir, "logo_a4.pdf")); err != nil {
		t.Fatalf("save: %v", err)
	}
	t.Logf("page size: %.1f x %.1f pt", size.Width, size.Height)
}
