// SPDX-License-Identifier: MIT

// markdown_to_pdf converts a Markdown file (CommonMark + GFM tables,
// strikethrough, task lists, autolinks) to a paginated PDF, plus a PNG
// preview of page 1.
//
// Usage: go run ./_examples/markdown_to_pdf <input.md>
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: markdown_to_pdf <input.md>")
		os.Exit(2)
	}
	doc, err := pdf.MarkdownToDocument(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, "convert:", err)
		os.Exit(1)
	}
	stem := strings.TrimSuffix(filepath.Base(os.Args[1]), filepath.Ext(os.Args[1]))
	outDir := filepath.Join("result_files", "markdown")
	_ = os.MkdirAll(outDir, 0o755)
	out := filepath.Join(outDir, stem+".pdf")
	if err := doc.Save(out); err != nil {
		fmt.Fprintln(os.Stderr, "save:", err)
		os.Exit(1)
	}
	png, _ := os.Create(filepath.Join(outDir, stem+"_p1.png"))
	defer png.Close()
	page, _ := doc.Page(1)
	if err := page.RenderPNG(png, pdf.RenderOptions{DPI: 130}); err != nil {
		fmt.Fprintln(os.Stderr, "render:", err)
		os.Exit(1)
	}
	fmt.Printf("%s: %d page(s)\n", out, doc.PageCount())
}
