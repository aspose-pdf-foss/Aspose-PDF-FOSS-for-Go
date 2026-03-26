// Package asposepdf provides PDF manipulation utilities without external dependencies.
package asposepdf

import (
	"fmt"
	"path/filepath"
)

// Split splits a PDF file into individual page files saved to outputDir.
// Returns the paths of created files (one per page).
//
// Example:
//
//	paths, err := asposepdf.Split("document.pdf", "./pages")
func Split(inputPath, outputDir string) ([]string, error) {
	doc, err := Open(inputPath)
	if err != nil {
		return nil, err
	}
	name := stemName(inputPath)
	return doc.Split(outputDir, func(page, _ int) string {
		return fmt.Sprintf("%s_page%03d.pdf", name, page)
	})
}

// SplitRange splits only the pages in the range [from, to] (1-based, inclusive).
// A to value of 0 means "last page".
//
// Example:
//
//	paths, err := asposepdf.SplitRange("document.pdf", "./pages", 2, 4)
func SplitRange(inputPath, outputDir string, from, to int) ([]string, error) {
	doc, err := Open(inputPath)
	if err != nil {
		return nil, err
	}
	if err := doc.ExtractPages(PageRange{from, to}); err != nil {
		return nil, err
	}
	name := stemName(inputPath)
	return doc.Split(outputDir, func(page, _ int) string {
		return fmt.Sprintf("%s_page%03d.pdf", name, page)
	})
}

// SplitFunc splits pages in the range [from, to] (1-based, inclusive; to=0 means last page),
// using nameFn to produce the filename for each page. nameFn receives the 1-based page number
// and the total page count and must return a filename (not a path).
//
// Example — name pages by their number out of total:
//
//	paths, err := asposepdf.SplitFunc("document.pdf", "./pages", 1, 0,
//	    func(page, total int) string {
//	        return fmt.Sprintf("page_%d_of_%d.pdf", page, total)
//	    },
//	)
func SplitFunc(inputPath, outputDir string, from, to int, nameFn func(page, total int) string) ([]string, error) {
	doc, err := Open(inputPath)
	if err != nil {
		return nil, err
	}
	if err := doc.ExtractPages(PageRange{from, to}); err != nil {
		return nil, err
	}
	return doc.Split(outputDir, nameFn)
}

// Extract creates a new PDF at outputPath containing only the pages in the specified ranges.
// Ranges are 1-based and inclusive. Pages appear in the order the ranges are listed.
// Use From == To to include a single page.
//
// Example — keep pages 1–3 and page 5 from a 6-page PDF:
//
//	err := asposepdf.Extract("input.pdf", "output.pdf",
//	    asposepdf.PageRange{1, 3},
//	    asposepdf.PageRange{5, 5},
//	)
func Extract(inputPath, outputPath string, ranges ...PageRange) error {
	doc, err := Open(inputPath)
	if err != nil {
		return err
	}
	return doc.Extract(outputPath, ranges...)
}

// stemName returns the filename stem (base without extension) of a path.
func stemName(inputPath string) string {
	base := filepath.Base(inputPath)
	ext := filepath.Ext(base)
	return base[:len(base)-len(ext)]
}

// normalizeRange clamps from/to to valid bounds [1, total] and validates ordering.
func normalizeRange(from, to, total int) (int, int, error) {
	if from < 1 {
		from = 1
	}
	if to < 1 || to > total {
		to = total
	}
	if from > to {
		return 0, 0, fmt.Errorf("invalid range: from=%d > to=%d", from, to)
	}
	return from, to, nil
}
