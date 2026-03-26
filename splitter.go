// Package asposepdf provides PDF manipulation utilities without external dependencies.
package asposepdf

import (
	"fmt"
	"os"
	"path/filepath"
)

// SplitDocument splits all pages of doc into individual files saved to outputDir.
// Returns the paths of created files (one per page).
//
// Example:
//
//	doc, _ := asposepdf.Open("document.pdf")
//	paths, err := asposepdf.SplitDocument(doc, "./pages")
func SplitDocument(doc *Document, outputDir string) ([]string, error) {
	return SplitDocumentFunc(doc, outputDir, 1, 0, func(page, _ int) string {
		return fmt.Sprintf("page%03d.pdf", page)
	})
}

// SplitDocumentRange splits only the pages in the range [from, to] (1-based, inclusive).
// A to value of 0 means "last page".
//
// Example:
//
//	doc, _ := asposepdf.Open("document.pdf")
//	paths, err := asposepdf.SplitDocumentRange(doc, "./pages", 2, 4)
func SplitDocumentRange(doc *Document, outputDir string, from, to int) ([]string, error) {
	return SplitDocumentFunc(doc, outputDir, from, to, func(page, _ int) string {
		return fmt.Sprintf("page%03d.pdf", page)
	})
}

// SplitDocumentFunc splits pages in the range [from, to] (1-based, inclusive; to=0 means last page),
// using nameFn to produce the filename for each page. nameFn receives the 1-based page number
// and the total page count and must return a filename (not a path).
//
// Example — name pages by their number out of total:
//
//	doc, _ := asposepdf.Open("document.pdf")
//	paths, err := asposepdf.SplitDocumentFunc(doc, "./pages", 1, 0,
//	    func(page, total int) string {
//	        return fmt.Sprintf("page_%d_of_%d.pdf", page, total)
//	    },
//	)
func SplitDocumentFunc(doc *Document, outputDir string, from, to int, nameFn func(page, total int) string) ([]string, error) {
	total := doc.PageCount()
	var err error
	from, to, err = normalizeRange(from, to, total)
	if err != nil {
		return nil, err
	}

	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return nil, fmt.Errorf("create output dir: %w", err)
	}

	var paths []string
	for i := from - 1; i < to; i++ {
		outPath := filepath.Join(outputDir, nameFn(i+1, total))
		data, err := buildDocumentPDF(doc.pages[i:i+1], doc.patches, doc.encryptConfig)
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

// ExtractDocument creates a new PDF at outputPath containing only the pages in the specified ranges.
// Ranges are 1-based and inclusive. Pages appear in the order the ranges are listed.
//
// Example:
//
//	doc, _ := asposepdf.Open("input.pdf")
//	err := asposepdf.ExtractDocument(doc, "output.pdf",
//	    asposepdf.PageRange{1, 3},
//	    asposepdf.PageRange{5, 5},
//	)
func ExtractDocument(doc *Document, outputPath string, ranges ...PageRange) error {
	if len(ranges) == 0 {
		return fmt.Errorf("no page ranges specified")
	}
	var selected []mutablePage
	for _, r := range ranges {
		from, to, err := normalizeRange(r.From, r.To, doc.PageCount())
		if err != nil {
			return err
		}
		selected = append(selected, doc.pages[from-1:to]...)
	}
	data, err := buildDocumentPDF(selected, doc.patches, doc.encryptConfig)
	if err != nil {
		return err
	}
	return writeFile(outputPath, data)
}

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
	base := filepath.Base(inputPath)
	ext := filepath.Ext(base)
	name := base[:len(base)-len(ext)]
	return SplitDocumentFunc(doc, outputDir, 1, 0, func(page, _ int) string {
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
	base := filepath.Base(inputPath)
	ext := filepath.Ext(base)
	name := base[:len(base)-len(ext)]
	return SplitDocumentFunc(doc, outputDir, from, to, func(page, _ int) string {
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
	return SplitDocumentFunc(doc, outputDir, from, to, nameFn)
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
	return ExtractDocument(doc, outputPath, ranges...)
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
