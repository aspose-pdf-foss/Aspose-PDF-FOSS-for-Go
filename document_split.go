package asposepdf

import (
	"fmt"
	"os"
	"path/filepath"
)

// Split saves each page of the document as an individual PDF file in outputDir.
// nameFn receives the 1-based page number and total page count and must return a filename (not a path).
// Returns the paths of created files.
//
// Example:
//
//	doc, _ := asposepdf.Open("document.pdf")
//	paths, err := doc.Split("./pages", func(page, total int) string {
//	    return fmt.Sprintf("page_%d_of_%d.pdf", page, total)
//	})
func (d *Document) Split(outputDir string, nameFn func(page, total int) string) ([]string, error) {
	if len(d.pages) == 0 {
		return nil, fmt.Errorf("document has no pages")
	}
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return nil, fmt.Errorf("create output dir: %w", err)
	}
	total := len(d.pages)
	var paths []string
	for i := 0; i < total; i++ {
		outPath := filepath.Join(outputDir, nameFn(i+1, total))
		data, err := buildDocumentPDF(d.pages[i:i+1], d.patches, d.encryptConfig)
		if err != nil {
			return nil, fmt.Errorf("write page %d: %w", i+1, err)
		}
		if err := writeFile(outPath, data); err != nil {
			return nil, fmt.Errorf("write page %d: %w", i+1, err)
		}
		paths = append(paths, outPath)
	}
	return paths, nil
}

// Extract returns a new Document containing only the pages in the specified ranges.
// Ranges are 1-based and inclusive. Pages appear in the order the ranges are listed.
// The original document is not modified.
//
// Example:
//
//	doc, _ := asposepdf.Open("input.pdf")
//	extracted, err := doc.Extract(asposepdf.PageRange{1, 3}, asposepdf.PageRange{5, 5})
//	extracted.Save("output.pdf")
func (d *Document) Extract(ranges ...PageRange) (*Document, error) {
	if len(ranges) == 0 {
		return nil, fmt.Errorf("no page ranges specified")
	}
	var selected []mutablePage
	for _, r := range ranges {
		from, to, err := normalizeRange(r.From, r.To, len(d.pages))
		if err != nil {
			return nil, err
		}
		selected = append(selected, d.pages[from-1:to]...)
	}
	return &Document{
		pages:   selected,
		patches: d.patches,
	}, nil
}
