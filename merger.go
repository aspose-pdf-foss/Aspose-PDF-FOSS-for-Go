package asposepdf

import "fmt"

// Merge combines pages from multiple PDF files into a single output PDF.
// Pages are appended in the order the input files are listed.
//
// Example:
//
//	err := asposepdf.Merge("combined.pdf", "part1.pdf", "part2.pdf", "part3.pdf")
func Merge(outputPath string, inputPaths ...string) error {
	if len(inputPaths) == 0 {
		return fmt.Errorf("no input files specified")
	}

	var entries []pageRef
	for _, path := range inputPaths {
		doc, err := openDocument(path)
		if err != nil {
			return fmt.Errorf("open %q: %w", path, err)
		}
		pages, err := doc.pages()
		if err != nil {
			return fmt.Errorf("read pages from %q: %w", path, err)
		}
		for _, p := range pages {
			entries = append(entries, pageRef{src: doc, page: p})
		}
	}

	data, err := buildDocumentPDF(entries, nil, nil)
	if err != nil {
		return err
	}
	return writeFile(outputPath, data)
}
