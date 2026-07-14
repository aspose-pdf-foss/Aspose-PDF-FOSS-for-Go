// SPDX-License-Identifier: MIT

// markdown_to_pdf converts a Markdown file (CommonMark + GFM tables,
// strikethrough, task lists, autolinks) to a paginated PDF, plus a PNG
// preview of page 1.
//
// Usage: go run ./_examples/markdown_to_pdf [-family "Arial"] <input.md>
//
// -family embeds an installed font family (full Unicode — Cyrillic, Greek…)
// instead of the Standard-14 Helvetica (Latin-1 only).
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
	family := flag.String("family", "", "font family to embed (e.g. Arial)")
	codeFamily := flag.String("code-family", "", "monospace family for code (e.g. Consolas)")
	flag.Parse()
	if flag.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: markdown_to_pdf [-family Arial] [-code-family Consolas] <input.md>")
		os.Exit(2)
	}
	doc, err := pdf.MarkdownToDocument(flag.Arg(0), pdf.MarkdownOptions{FontFamily: *family, CodeFontFamily: *codeFamily})
	if err != nil {
		fmt.Fprintln(os.Stderr, "convert:", err)
		os.Exit(1)
	}
	stem := strings.TrimSuffix(filepath.Base(flag.Arg(0)), filepath.Ext(flag.Arg(0)))
	outDir := filepath.Join("result_files", "markdown")
	_ = os.MkdirAll(outDir, 0o755)
	out := filepath.Join(outDir, stem+".pdf")
	if err := doc.Save(out); err != nil {
		fmt.Fprintln(os.Stderr, "save:", err)
		os.Exit(1)
	}
	for n := 1; n <= doc.PageCount(); n++ {
		png, err := os.Create(filepath.Join(outDir, fmt.Sprintf("%s_p%d.png", stem, n)))
		if err != nil {
			fmt.Fprintln(os.Stderr, "create:", err)
			os.Exit(1)
		}
		page, _ := doc.Page(n)
		err = page.RenderPNG(png, pdf.RenderOptions{DPI: 130})
		png.Close()
		if err != nil {
			fmt.Fprintln(os.Stderr, "render:", err)
			os.Exit(1)
		}
	}
	fmt.Printf("%s: %d page(s)\n", out, doc.PageCount())
}
