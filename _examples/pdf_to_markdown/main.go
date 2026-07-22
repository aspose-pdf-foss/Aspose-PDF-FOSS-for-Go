// SPDX-License-Identifier: MIT

// pdf_to_markdown converts a PDF to GFM Markdown. Images are written next to
// the output (SHA-256 deduped) unless -embed or -no-images is given.
//
// Usage: go run ./_examples/pdf_to_markdown [-embed|-no-images] <input.pdf>
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

func main() {
	embed := flag.Bool("embed", false, "embed images as data: URLs")
	noImages := flag.Bool("no-images", false, "skip images")
	flag.Parse()
	if flag.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: pdf_to_markdown [-embed|-no-images] <input.pdf>")
		os.Exit(2)
	}
	doc, err := pdf.Open(flag.Arg(0))
	if err != nil {
		fmt.Fprintln(os.Stderr, "open:", err)
		os.Exit(1)
	}
	stem := strings.TrimSuffix(filepath.Base(flag.Arg(0)), filepath.Ext(flag.Arg(0)))
	outDir := filepath.Join("result_files", "markdown")
	_ = os.MkdirAll(outDir, 0o755)
	out := filepath.Join(outDir, stem+".md")
	if err := doc.SaveMarkdown(out, pdf.MarkdownSaveOptions{EmbedImages: *embed, NoImages: *noImages}); err != nil {
		fmt.Fprintln(os.Stderr, "convert:", err)
		os.Exit(1)
	}
	fmt.Printf("%s: %d page(s)\n", out, doc.PageCount())
}
