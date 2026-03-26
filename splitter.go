// Package asposepdf provides PDF manipulation utilities without external dependencies.
package asposepdf

import (
	"fmt"
	"os"
	"path/filepath"
)

// Split splits a PDF file into individual page files saved to outputDir.
// Returns the paths of created files (one per page).
//
// Example:
//
//	paths, err := asposepdf.Split("document.pdf", "./pages")
func Split(inputPath, outputDir string) ([]string, error) {
	return SplitRange(inputPath, outputDir, 1, 0)
}

// SplitRange splits only the pages in the range [from, to] (1-based, inclusive).
// A to value of 0 means "last page".
//
// Example:
//
//	paths, err := asposepdf.SplitRange("document.pdf", "./pages", 2, 4)
func SplitRange(inputPath, outputDir string, from, to int) ([]string, error) {
	base := filepath.Base(inputPath)
	ext := filepath.Ext(base)
	name := base[:len(base)-len(ext)]

	nameFn := func(page, _ int) string {
		return fmt.Sprintf("%s_page%03d.pdf", name, page)
	}
	return SplitFunc(inputPath, outputDir, from, to, nameFn)
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
	doc, err := openDocument(inputPath)
	if err != nil {
		return nil, fmt.Errorf("open PDF: %w", err)
	}

	pages, err := doc.pages()
	if err != nil {
		return nil, fmt.Errorf("read pages: %w", err)
	}

	from, to, err = normalizeRange(from, to, len(pages))
	if err != nil {
		return nil, err
	}

	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return nil, fmt.Errorf("create output dir: %w", err)
	}

	total := len(pages)
	var paths []string
	for i := from - 1; i < to; i++ {
		outPath := filepath.Join(outputDir, nameFn(i+1, total))
		if err := writePage(doc, pages[i], outPath); err != nil {
			return nil, fmt.Errorf("write page %d: %w", i+1, err)
		}
		paths = append(paths, outPath)
	}

	return paths, nil
}

// PageRange specifies an inclusive range of pages (1-based).
type PageRange struct {
	From, To int
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
	if len(ranges) == 0 {
		return fmt.Errorf("no page ranges specified")
	}

	doc, err := openDocument(inputPath)
	if err != nil {
		return fmt.Errorf("open PDF: %w", err)
	}

	allPages, err := doc.pages()
	if err != nil {
		return fmt.Errorf("read pages: %w", err)
	}
	total := len(allPages)

	// Collect selected pages in order, validating each range.
	var selected []*pageInfo
	for _, r := range ranges {
		from, to, err := normalizeRange(r.From, r.To, total)
		if err != nil {
			return err
		}
		for i := from - 1; i < to; i++ {
			selected = append(selected, allPages[i])
		}
	}

	data, err := buildMultiPagePDF(doc, selected)
	if err != nil {
		return err
	}
	return writeFile(outputPath, data)
}

